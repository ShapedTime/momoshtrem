// Package race reads the Go race detector's reports (docs/streamlab/
// PLAN.md D8): the "WARNING: DATA RACE" blocks a -race build writes to
// GORACE's log_path, or to stderr in go test's output. Parse turns them
// into report.Race values, deduplicated by Signature: each access's kind,
// its top frame and its top momoshtrem frames. Two reports of one race in
// different repetitions, or with the accesses the other way round, share a
// signature.
package race

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Own is the prefix of momoshtrem's functions in a stack.
const Own = "github.com/shapedtime/momoshtrem/"

// lab marks the lab's own functions: they drive the code under test, so
// they don't tell races apart (a poller and a scenario reaching one racy
// access are one race).
var lab = []string{Own + "internal/streamlab/", Own + "cmd/streamlab"}

// ownFrames is how many momoshtrem frames of an access the signature
// keeps, below the access's own top frame: the racy function's callers.
const ownFrames = 2

// LogPrefix is the log_path check gives GORACE in each repetition's
// folder: the race detector appends ".<pid>".
const LogPrefix = "race"

// Env is the GORACE setting for a process whose races go to the folder
// dir. halt_on_error=0 keeps the process running after a race, so one
// run finds them all; exitcode=0 keeps a race from turning a finished
// repetition's exit into a crash: the logs say what raced.
func Env(dir string) string {
	return "GORACE=log_path=" + filepath.Join(dir, LogPrefix) + " halt_on_error=0 exitcode=0"
}

const (
	marker = "WARNING: DATA RACE"
	rule   = "=================="
)

// accessLine is an access's header: "Read at 0x… by goroutine 9:",
// "Previous write at 0x… by main goroutine:", "Atomic read at …".
var accessLine = regexp.MustCompile(`^(Previous )?((?:[Aa]tomic )?(?:[Rr]ead|[Ww]rite)) at 0x[0-9a-f]+ by (main goroutine|goroutine (\d+)):$`)

// createdLine is a goroutine's creation header: "Goroutine 9 (running)
// created at:".
var createdLine = regexp.MustCompile(`^Goroutine (\d+) \([a-z ]+\) created at:$`)

// Parse reads every race report in r, deduplicated by signature, in the
// order they first appear. where names the source in each Race.Where.
// Text that isn't a race report (a test's output, say) is skipped.
func Parse(r io.Reader, where string) ([]report.Race, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var (
		out   []report.Race
		block []string
		in    bool
	)
	flush := func() {
		if len(block) == 0 {
			return
		}
		if rc, ok := parseBlock(block); ok {
			if where != "" {
				rc.Where = []string{where}
			}
			out = report.MergeRaces(out, []report.Race{rc})
		}
		block, in = nil, false
	}
	for sc.Scan() {
		line := sc.Text()
		// go test indents nothing, but a test's log may prefix lines;
		// the detector's own lines start at column 0.
		switch {
		case strings.TrimSpace(line) == marker:
			flush()
			in = true
			block = []string{marker}
		case in && strings.TrimSpace(line) == rule:
			flush()
		case in:
			block = append(block, line)
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("race: reading %s: %w", orDash(where), err)
	}
	return out, nil
}

// ParseDir reads every race log check left in dir (LogPrefix.<pid>),
// each Race's Where the log's path relative to rel ("" for dir's own).
func ParseDir(dir, rel string) ([]report.Race, error) {
	logs, err := filepath.Glob(filepath.Join(dir, LogPrefix+".*"))
	if err != nil {
		return nil, err
	}
	sort.Strings(logs)
	var out []report.Race
	for _, path := range logs {
		f, err := os.Open(path)
		if err != nil {
			return out, err
		}
		where := filepath.Base(path)
		if rel != "" {
			where = filepath.Join(rel, where)
		}
		races, err := Parse(f, where)
		f.Close()
		if err != nil {
			return out, err
		}
		out = report.MergeRaces(out, races)
	}
	return out, nil
}

// section is one part of a report: an access or a goroutine's creation.
type section struct {
	header string
	frames []string // "func (file:line)"
	funcs  []string // the bare function names
}

// parseBlock parses one report, its lines between the marker and the
// closing rule.
func parseBlock(lines []string) (report.Race, bool) {
	var secs []*section
	var cur *section
	for _, raw := range lines[1:] {
		line := strings.TrimRight(raw, " \t")
		switch {
		case line == "":
			cur = nil
		case !strings.HasPrefix(line, " "):
			cur = &section{header: line}
			secs = append(secs, cur)
		case cur != nil && strings.HasPrefix(line, "      "):
			// The position under a function: "      /path/file.go:15 +0x30".
			if n := len(cur.frames); n > 0 {
				cur.frames[n-1] += " (" + position(strings.TrimSpace(line)) + ")"
			}
		case cur != nil:
			fn := strings.TrimSuffix(strings.TrimSpace(line), "()")
			cur.funcs = append(cur.funcs, fn)
			cur.frames = append(cur.frames, fn)
		}
	}
	rc := report.Race{Count: 1, Text: strings.Join(append(append([]string{rule}, lines...), rule), "\n")}
	created := map[string][]string{}
	for _, s := range secs {
		if m := createdLine.FindStringSubmatch(s.header); m != nil {
			created[m[1]] = s.frames
		}
	}
	var sigs []string
	for _, s := range secs {
		m := accessLine.FindStringSubmatch(s.header)
		if m == nil {
			continue
		}
		g := m[4]
		if g == "" {
			g = "main"
		}
		a := report.RaceAccess{Op: strings.ToLower(m[2]), Goroutine: g, Frames: s.frames, Created: created[g]}
		rc.Accesses = append(rc.Accesses, a)
		sigs = append(sigs, accessSignature(a.Op, s.funcs))
	}
	if len(rc.Accesses) == 0 {
		return report.Race{}, false
	}
	slices.Sort(sigs)
	rc.Signature = strings.Join(sigs, " | ")
	return rc, true
}

// accessSignature is an access's part of a signature: its kind, its top
// frame, then its top momoshtrem frames below that, the lab's left out
// (the top frames overall when it ran no momoshtrem code), momoshtrem's
// prefix dropped: "write anacrolix/torrent.(*reader).clearStorageReader <
// internal/streaming.(*PriorityReader).Close < internal/vfs.(*TorrentFile).Close".
// Function names only: a line that moves doesn't make a known race new.
func accessSignature(op string, funcs []string) string {
	if len(funcs) == 0 {
		return op + " ?"
	}
	keep := []string{funcs[0]}
	var own []string
	for _, f := range funcs[1:] {
		if strings.HasPrefix(f, Own) && !isLab(f) {
			own = append(own, f)
			if len(own) == ownFrames {
				break
			}
		}
	}
	if len(own) == 0 && !strings.HasPrefix(funcs[0], Own) {
		own = funcs[1:min(len(funcs), 1+ownFrames)]
	}
	keep = append(keep, own...)
	for i, f := range keep {
		keep[i] = short(f)
	}
	return op + " " + strings.Join(keep, " < ")
}

func isLab(fn string) bool {
	return slices.ContainsFunc(lab, func(p string) bool { return strings.HasPrefix(fn, p) })
}

// short drops momoshtrem's and GitHub's prefixes from a function name.
func short(fn string) string {
	if s, ok := strings.CutPrefix(fn, Own); ok {
		return s
	}
	return strings.TrimPrefix(fn, "github.com/")
}

// position is a frame's file and line, the last two path elements and no
// PC offset: "vfs/torrent_file.go:420".
func position(p string) string {
	p, _, _ = strings.Cut(p, " +0x")
	dir, file := filepath.Split(p)
	return filepath.Join(filepath.Base(filepath.Clean(dir)), file)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

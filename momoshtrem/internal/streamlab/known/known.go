// Package known is streamlab's known-issue registry (docs/streamlab/
// PLAN.md D6): the bugs the lab demonstrates and nobody has fixed yet. A
// check that fails because of one reports xfail and doesn't block; once
// it passes it reports xpass, and the entry should go.
//
// An entry explains only failures that carry its signature: the check
// that failed names the issue it matched (a #195 stall is a timed-out read
// at a wanted piece with the seeder connected, measured when the read
// broke; an F12 lag ends within the read timeout). It also explains them
// only at the rate it's known for. An intermittent issue (#195 hits about
// 1-4% of reads) that fires on every repetition, or on two or more and
// more than MaxRate of them, is a fail: a regression that breaks every
// read must not hide behind it. The rate is counted over a scenario's
// repetitions and over the whole run's (Tally). One firing among several
// repetitions can't tell a regression from the known rate, so the runner
// confirms it: a scenario with fewer than ConfirmBelow measured
// repetitions where one fired gets ConfirmReps more, and a regression
// fires again. Deterministic suspects (F2, F12, ...) reproduce on every
// repetition by nature; their entries say so (MaxRate Always) and rely on
// their signature to stay narrow.
package known

import (
	"fmt"
	"slices"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Sep joins several known issues in one Outcome's Known.
const Sep = "+"

// IDs splits an Outcome's Known into its issues.
func IDs(known string) []string {
	if known == "" {
		return nil
	}
	return strings.Split(known, Sep)
}

// DefaultMaxRate is the share of repetitions an intermittent issue may
// fire on and stay an expected failure.
const DefaultMaxRate = 0.25

// Confirmation repetitions: when an intermittent issue fires in a
// scenario with fewer than ConfirmBelow measured repetitions, the runner
// runs ConfirmReps more and judges over them all.
const (
	ConfirmBelow = 2
	ConfirmReps  = 2
)

// Always is the MaxRate of a deterministic issue: it fires on every
// repetition while it exists.
const Always = 1.0

// Env is what a verdict depends on beyond the repetitions.
type Env struct {
	// MIMEResolvesMKV is whether the MIME table resolves .mkv
	// (checks.MIME): production's image doesn't (F1).
	MIMEResolvesMKV bool
	// Guards are the scenario's guards: only a deterministic issue it
	// guards turns a pass into an xpass. A scenario that never reproduces
	// an issue can't say it's fixed.
	Guards []string
	// Run is each intermittent issue's tally over the whole run, every
	// scenario's measured repetitions (TallyRun). Without one, a
	// scenario's own repetitions are the whole run.
	Run map[string]Tally
}

// Tally is how often an issue fired: on Fired of Reps repetitions that ran
// a check it explains.
type Tally struct {
	Fired, Reps int
}

// over reports whether the tally is past rate: every repetition, or two
// or more and more than rate of them.
//
// A single firing is over only when it's the only repetition: among
// several, one stray firing is the known rate's to explain.
func (t Tally) over(rate float64) bool {
	return t.Reps > 0 && (t.Fired == t.Reps || (t.Fired >= 2 && float64(t.Fired)/float64(t.Reps) > rate))
}

// Fired reports whether an intermittent issue's signature is in the
// repetition's checks (by check ID), and which issue's.
func Fired(rep map[string]Outcome) (string, bool) {
	for _, is := range Issues {
		if !is.Intermittent() {
			continue
		}
		for _, c := range is.Checks {
			if o, ok := rep[c]; ok && o.Status == report.Fail && slices.Contains(IDs(o.Known), is.ID) {
				return is.ID, true
			}
		}
	}
	return "", false
}

// TallyRun counts each intermittent issue over a run: reps are every
// scenario's measured repetitions, each a map of its checks' outcomes by
// check ID.
func TallyRun(reps []map[string]Outcome) map[string]Tally {
	out := map[string]Tally{}
	for _, is := range Issues {
		if !is.Intermittent() {
			continue
		}
		var t Tally
		for _, rep := range reps {
			ran, fired := false, false
			for _, c := range is.Checks {
				o, ok := rep[c]
				if !ok || o.Status == report.NA {
					continue
				}
				ran = true
				fired = fired || (o.Status == report.Fail && slices.Contains(IDs(o.Known), is.ID))
			}
			if ran {
				t.Reps++
			}
			if fired {
				t.Fired++
			}
		}
		out[is.ID] = t
	}
	return out
}

// Issue is a known issue.
type Issue struct {
	ID    string
	Title string
	// Ref is the issue tracking it.
	Ref string
	// Checks are the checks whose failure it can explain.
	Checks []string
	// MaxRate is the share of repetitions it may fire on and stay an
	// xfail: DefaultMaxRate when 0, Always for a deterministic bug.
	MaxRate float64
	Doc     string
	// AppliesTo reports whether it can happen in env, and why not; nil:
	// everywhere.
	AppliesTo func(Env) (bool, string)
}

// Rate is the issue's MaxRate, the default filled in.
func (is Issue) Rate() float64 {
	if is.MaxRate <= 0 {
		return DefaultMaxRate
	}
	return is.MaxRate
}

// Intermittent reports whether the issue fires on some repetitions only.
func (is Issue) Intermittent() bool { return is.Rate() < Always }

// Applies reports whether the issue can happen in env.
func (is Issue) Applies(env Env) (bool, string) {
	if is.AppliesTo == nil {
		return true, ""
	}
	return is.AppliesTo(env)
}

// Issues is the registry. Each entry is removed by the PR that fixes it.
var Issues = []Issue{
	{
		ID:      "F1",
		Title:   "Without a MIME type for .mkv, every GET, HEAD and PROPFIND sniffs 512 bytes at offset 0",
		Ref:     "#171 (PLAN §3 F1)",
		Checks:  []string{"no-sniff-reads"},
		MaxRate: Always,
		Doc: "Production's alpine:3 image has no /etc/mime.types and Go's built-in table lacks .mkv, so http.ServeContent " +
			"and PROPFIND's getcontenttype read 512 bytes at offset 0 of each file: a reader, header priorities, a download. " +
			"Hosts with a MIME table (macOS, most Linux) can't reproduce it.",
		AppliesTo: func(env Env) (bool, string) {
			if env.MIMEResolvesMKV {
				return false, "this host's MIME table resolves .mkv, so nothing is sniffed here"
			}
			return true, ""
		},
	},
	{
		ID:      "F2",
		Title:   "The ETag changes with every request, so If-Range never matches",
		Ref:     "#73",
		Checks:  []string{"etag-stable"},
		MaxRate: Always,
		Doc: "ModTime is time.Now() in every Stat, and x/net's ETag is ModTime plus size: a new ETag per request, so a " +
			"client's If-Range never matches and the server answers 200 with the whole file.",
	},
	{
		ID:      "F3",
		Title:   "PROPFIND loads the torrent of every file it lists",
		Ref:     "#83",
		Checks:  []string{"propfind-no-load"},
		MaxRate: Always,
		Doc: "x/net's PROPFIND opens each listed file, and opening a torrent's file adds the torrent " +
			"(vfs.LibraryFS.openTorrentFile). The check is n/a when a torrent was loaded before the PROPFIND " +
			"(checks.F3Result.Conclusive).",
	},
	{
		ID:      "F6",
		Title:   "Closing a reader open 30 s or more resets the whole file's priorities, other readers' windows too",
		Ref:     "#216",
		Checks:  []string{"close-keeps-windows"},
		MaxRate: Always,
		Doc:     "PriorityReader.Close calls ResetPriorities over the file's every piece: another reader's Now and Readahead windows drop to None.",
	},
	{
		ID:      "F11",
		Title:   "Reading a piece the filecache LRU evicted breaks the stream",
		Ref:     "#215",
		Checks:  []string{"cache-holds-complete", "play-ok"},
		MaxRate: Always,
		Doc: "The piece cache (a filecache LRU) evicts pieces anacrolix still believes complete. Reading one fails: " +
			"anacrolix resyncs the piece's completion but returns the error, and the response ends early. Its play-ok " +
			"signature: a body that broke within 3 s of its last byte, at a piece anacrolix had complete and the cache no " +
			"longer holds. When the readahead window outgrows the cache, the LRU also evicts the dirty chunks of incomplete " +
			"pieces the responsive PriorityReader reads (forward play). Such a break carries the signature only with two proofs: " +
			"the giving-up read's last \"error reading\" names that piece, right before its \"read failed after completion " +
			"resync\" (reader.go:298, 339), within 3 s before the break; and the piece was partial before while its chunk at the " +
			"break is gone from the cache's incompleted/. Without both, it's a plain fail.",
	},
	{
		ID:      "F12",
		Title:   "A handler blocked in a read outlives its client's abort",
		Ref:     "#217",
		Checks:  []string{"abort-lag"},
		MaxRate: Always,
		Doc: "seekAndReadAtLeast and readWithTimeout read under context.Background() and read_timeout, not the request's " +
			"context: when a client aborts during a blocked read (an Infuse seek), the handler, its reader and its Now " +
			"window live on until the read returns. Its signature: the handler ended within the read timeout of the abort.",
	},
	{
		ID:      "F13",
		Title:   "A new reader's InitialPrioritize overwrites other open readers' windows",
		Ref:     "#209",
		Checks:  []string{"open-keeps-windows"},
		MaxRate: Always,
		Doc: "NewPriorityReader runs InitialPrioritize for every reader: the file's header and footer go to explicit High, " +
			"lowering another reader's Now and Readahead pieces there until that reader moves a piece.",
	},
	{
		ID:     "#195",
		Title:  "anacrolix loses a writer wakeup: a read waits up to the 1-minute keep-alive",
		Ref:    "#195",
		Checks: []string{"play-ok", "idle-resume", "watchdog"},
		Doc: "anacrolix v1.60.0's connection writer can miss a wakeup; with one useful peer a read then waits until the " +
			"keep-alive (60 s), past the lab's read timeout. Its signature: every broken body of the repetition was silent " +
			"for half the read timeout or more and broke at an incomplete piece with the seeder connected and downloads on " +
			"(the torrent neither idle nor paused, the seeder not paused by the lab), on at most two pieces. A watchdog trip " +
			"carries it when, at the trip, every stuck piece is wanted and incomplete with a seeder connected (not one the lab " +
			"paused) and downloads on, nothing progressed for half the read timeout or more, on at most two pieces: the same " +
			"signature, measured live. It hits about 1-4% of reads: one firing is confirmed with more " +
			"repetitions, and firing on every repetition, or on two or more and over 25% of them, is a fail.",
	},
	{
		ID:      "#231",
		Title:   "The responsive reader serves a chunk before its piece's hash check",
		Ref:     "#231",
		Checks:  []string{"integrity"},
		MaxRate: Always,
		Doc: "PriorityReader sets anacrolix's reader responsive, so a read returns a chunk as soon as it arrives, before " +
			"its piece is verified: a bad peer's bytes reach the player, and the hash failure only fixes later reads. Its " +
			"signature: every mismatched byte is one a lab seeder flipped on purpose (corrupt-piece's: the first byte of " +
			"each chunk of the corrupt piece, the media's byte inverted). Any other mismatch is a plain fail.",
	},
	{
		ID:      "#232",
		Title:   "A dropped HDRezka request's handler holds its streamer slot until its read gives up",
		Ref:     "#232",
		Checks:  []string{"abort-lag"},
		MaxRate: Always,
		Doc: "RemoteFile opens and reads under context.Background and its idle watchdog, not the request's context: a " +
			"handler blocked on a stalled CDN body outlives its client's drop, holding one of the streamer's 8 slots, so " +
			"a scrub during a CDN stall can take them all (F12's HDRezka counterpart). Its signature: every handler over " +
			"abort_lag_max served an HDRezka file and ended within the remote read timeout (its watchdog) plus a second. " +
			"In general a handler can linger up to 3× that (the reopen's open and read too), but the lab's stall ends " +
			"with the watchdog, so a longer lag is a regression.",
	},
	{
		ID:     "#225",
		Title:  "A timed-out read's goroutine outlives its closed TorrentFile",
		Ref:    "#225",
		Checks: []string{"quiescence"},
		Doc: "TorrentFile.readContext reads in a goroutine; after a read timeout the file is closed, but anacrolix's " +
			"reader.Close doesn't wake a waitAvailable blocked on the piece, so the goroutine lives until the piece " +
			"arrives. It needs a read to time out (a #195 stall, a slow swarm). Its signature: quiescence left nothing " +
			"but goroutines in anacrolix's waitAvailable under TorrentFile.readContext, after a read timed out in the " +
			"repetition.",
	},
}

// Lookup returns the issue with the ID.
func Lookup(id string) (Issue, bool) {
	for _, is := range Issues {
		if is.ID == id {
			return is, true
		}
	}
	return Issue{}, false
}

// ForCheck returns the issues that can explain check's failures.
func ForCheck(check string) []Issue {
	var out []Issue
	for _, is := range Issues {
		if slices.Contains(is.Checks, check) {
			out = append(out, is)
		}
	}
	return out
}

// Outcome is a check's result in one repetition: pass, fail or n/a, with
// the known issues whose signatures a failure carries: one ID, or several
// joined by Sep when the repetition's failures carry different ones, each
// matching its own (a #195 stall and an F11 break, say).
type Outcome struct {
	Status   report.Status
	Known    string
	Evidence string
}

// Verdict is a check's status over the repetitions.
type Verdict struct {
	Status report.Status
	Known  string
	// Note says why: which repetitions failed, which issue explains them,
	// or why it couldn't.
	Note string
}

// Judge gives check's status over the repetitions:
//   - n/a when no repetition could tell;
//   - fail when a failure carries no signature, or one of an issue that
//     doesn't explain this check or can't happen in env, or of a
//     deterministic issue the scenario doesn't guard;
//   - fail when an intermittent issue fired on every repetition, or on two
//     or more and more than its MaxRate of them: over the scenario's
//     repetitions, and over the run's (env.Run);
//   - xfail when known issues explain every failure;
//   - xpass when it passed every time though a deterministic issue the
//     scenario guards, and that can happen here, was expected to fail it;
//   - pass otherwise.
func Judge(check string, reps []Outcome, env Env) Verdict {
	n := 0
	fired := map[string]int{}
	var order []string
	for i, r := range reps {
		switch r.Status {
		case report.NA:
			continue
		case report.Pass:
			n++
			continue
		}
		n++
		if r.Known == "" {
			return Verdict{Status: report.Fail, Note: fmt.Sprintf("rep %d: %s", i+1, r.Evidence)}
		}
		for _, id := range IDs(r.Known) {
			is, ok := Lookup(id)
			if !ok || !slices.Contains(is.Checks, check) {
				return Verdict{Status: report.Fail, Note: fmt.Sprintf("rep %d: %s doesn't explain %s: %s", i+1, id, check, r.Evidence)}
			}
			if applies, why := is.Applies(env); !applies {
				return Verdict{Status: report.Fail, Note: fmt.Sprintf("rep %d: %s can't explain it here (%s): %s", i+1, id, why, r.Evidence)}
			}
			if !is.Intermittent() && !slices.Contains(env.Guards, is.ID) {
				return Verdict{Status: report.Fail, Note: fmt.Sprintf(
					"rep %d: %s explains failures only in the scenarios that guard it: %s", i+1, id, r.Evidence)}
			}
			if fired[id] == 0 {
				order = append(order, id)
			}
			fired[id]++
		}
	}
	if n == 0 {
		return Verdict{Status: report.NA, Note: "no repetition could tell"}
	}
	if len(order) == 0 {
		for _, is := range ForCheck(check) {
			if applies, _ := is.Applies(env); applies && !is.Intermittent() && slices.Contains(env.Guards, is.ID) {
				return Verdict{Status: report.XPass, Known: is.ID,
					Note: fmt.Sprintf("%s didn't reproduce on any of %d reps: fixed? Remove it from the registry", is.ID, n)}
			}
		}
		return Verdict{Status: report.Pass}
	}
	var notes []string
	for _, id := range order {
		is, _ := Lookup(id)
		k := fired[id]
		note := fmt.Sprintf("%s on %d of %d reps", id, k, n)
		if is.Intermittent() {
			here := Tally{Fired: k, Reps: n}
			run, ok := env.Run[id]
			if !ok {
				run = here
			}
			why := ""
			switch {
			case here.over(is.Rate()):
				why = fmt.Sprintf("its signature on %d of this scenario's %d reps", k, n)
			case run.over(is.Rate()):
				why = fmt.Sprintf("its signature on %d of the run's %d reps", run.Fired, run.Reps)
			}
			if why != "" {
				return Verdict{Status: report.Fail, Known: id, Note: fmt.Sprintf(
					"%s: %s, and an intermittent issue explains at most %.0f%% of reps and never all of them, so this is treated as a regression",
					id, why, is.Rate()*100)}
			}
			if ok {
				note += fmt.Sprintf(" (%d of the run's %d)", run.Fired, run.Reps)
			}
		}
		notes = append(notes, note)
	}
	return Verdict{Status: report.XFail, Known: order[0], Note: strings.Join(notes, "; ")}
}

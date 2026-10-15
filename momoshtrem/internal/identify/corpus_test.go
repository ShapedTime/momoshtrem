package identify

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// The corpus (testdata/corpus/*.yaml) is the classifier's spec; its schema
// is described at the top of anime.yaml.

type corpusCase struct {
	ID           string       `yaml:"id"`
	Show         string       `yaml:"show"`
	Torrent      string       `yaml:"torrent"`
	Golden       bool         `yaml:"golden"`
	Tags         []string     `yaml:"tags"`
	Now          string       `yaml:"now"`
	Hint         *corpusHint  `yaml:"hint"`
	DropSpecials bool         `yaml:"drop_specials"`
	Files        []corpusFile `yaml:"files"`

	source string // corpus file, for messages
}

type corpusHint struct {
	Season  *int `yaml:"season"`
	Episode *int `yaml:"episode"`
}

type corpusFile struct {
	Path      string       `yaml:"path"`
	Size      string       `yaml:"size"`
	Ambiguous bool         `yaml:"ambiguous"`
	Expect    corpusExpect `yaml:"expect"`
}

type corpusExpect struct {
	Kind       string    `yaml:"kind"`
	Eps        []string  `yaml:"eps"`
	Extra      string    `yaml:"extra"`
	MinConf    string    `yaml:"min_conf"`
	MaxConf    string    `yaml:"max_conf"`
	Acceptable []*string `yaml:"acceptable"`
}

// corpusNow is "today" for cases without their own.
var corpusNow = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func loadCorpus(t testing.TB) []corpusCase {
	t.Helper()
	paths, err := filepath.Glob("testdata/corpus/*.yaml")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no corpus files: %v", err)
	}
	var cases []corpusCase
	ids := map[string]bool{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var cs []corpusCase
		if err := yaml.Unmarshal(data, &cs); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		for i := range cs {
			if ids[cs[i].ID] {
				t.Fatalf("%s: duplicate case %q", p, cs[i].ID)
			}
			ids[cs[i].ID] = true
			cs[i].source = filepath.Base(p)
		}
		cases = append(cases, cs...)
	}
	return cases
}

// input builds the classifier's input for a case. Files of a multi-file
// case sit in a folder named after the torrent, as in a real torrent.
func (c corpusCase) input(t testing.TB) Input {
	t.Helper()
	cat := show(t, c.Show)
	if c.DropSpecials {
		cat = withoutSpecials(cat)
	}
	in := Input{TorrentName: c.Torrent, Show: cat, Now: corpusNow}
	if c.Now != "" {
		now, err := time.Parse("2006-01-02", c.Now)
		if err != nil {
			t.Fatalf("%s: now: %v", c.ID, err)
		}
		in.Now = now
	}
	if c.Hint != nil {
		in.Hint = &Hint{Season: c.Hint.Season, Episode: c.Hint.Episode}
	}
	if in.TorrentName == "" {
		if len(c.Files) != 1 {
			t.Fatalf("%s: a multi-file case needs a torrent name", c.ID)
		}
		in.TorrentName = c.Files[0].Path
	}
	for _, f := range c.Files {
		path := f.Path
		if len(c.Files) > 1 {
			path = c.Torrent + "/" + f.Path
		}
		size, err := parseSize(f.Size, ClassOf(f.Path))
		if err != nil {
			t.Fatalf("%s: %s: %v", c.ID, f.Path, err)
		}
		in.Files = append(in.Files, TorrentFile{Path: path, Size: size})
	}
	return in
}

// parseSize reads "1.2GB", "350MB", "40KB"; empty gives a typical size for
// the file's class.
func parseSize(s string, class FileClass) (int64, error) {
	if s == "" {
		switch class {
		case ClassVideo:
			return 1200 << 20, nil
		case ClassSubtitle:
			return 40 << 10, nil
		}
		return 200 << 10, nil
	}
	units := []struct {
		suffix string
		mult   float64
	}{{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 64)
			return int64(n * u.mult), err
		}
	}
	return strconv.ParseInt(s, 10, 64)
}

// expectedKind is the kind a file's expectation implies.
func (e corpusExpect) expectedKind() Kind {
	switch {
	case e.Kind != "":
		return Kind(e.Kind)
	case e.Extra == string(ExtraSample):
		return KindSample
	case e.Extra != "":
		return KindExtra
	case len(e.Eps) > 0:
		for _, ep := range e.Eps {
			if !strings.HasPrefix(ep, "S00") {
				return KindEpisode
			}
		}
		return KindSpecial
	}
	return KindUnknown
}

// targetLabel writes a proposal's targets like the corpus does:
// "S01E01+S01E02", or "null" for none.
func targetLabel(targets []Target) string {
	if len(targets) == 0 {
		return "null"
	}
	labels := make([]string, len(targets))
	for i, t := range targets {
		labels[i] = fmt.Sprintf("S%02dE%02d", t.Season, t.Episode)
	}
	sort.Strings(labels)
	return strings.Join(labels, "+")
}

func expectLabel(eps []string) string {
	if len(eps) == 0 {
		return "null"
	}
	labels := append([]string(nil), eps...)
	sort.Strings(labels)
	return strings.Join(labels, "+")
}

// corpusReport is the outcome of running the whole corpus.
type corpusReport struct {
	cases, files, golden, ambiguous  int
	unambiguous, exact               int
	byConf                           map[Confidence]int
	wrongHigh, maxConf, ambiguousBad []string
	misses, confMisses, longReasons  []string
}

func runCorpus(t *testing.T) corpusReport {
	r := corpusReport{byConf: map[Confidence]int{}}
	for _, c := range loadCorpus(t) {
		r.cases++
		if c.Golden {
			r.golden++
		}
		res := Classify(c.input(t))
		if len(res.Proposals) != len(c.Files) {
			t.Fatalf("%s: %d proposals for %d files", c.ID, len(res.Proposals), len(c.Files))
		}
		for i, f := range c.Files {
			p := res.Proposals[i]
			r.files++
			r.byConf[p.Confidence]++
			got := targetLabel(p.Targets)
			where := fmt.Sprintf("%s: %s", c.ID, f.Path)
			detail := fmt.Sprintf("%s\n      got %s %s [%s] %q", where, p.Kind, got, p.Confidence, p.Reason)

			if n := utf8.RuneCountInString(p.Reason); n > MaxReasonRunes {
				r.longReasons = append(r.longReasons, fmt.Sprintf("%s (%d runes)", where, n))
			}
			if f.Ambiguous {
				r.ambiguous++
				ok := false
				for _, a := range f.Expect.Acceptable {
					want := "null"
					if a != nil {
						want = *a
					}
					if want == got {
						ok = true
					}
				}
				if !ok {
					r.ambiguousBad = append(r.ambiguousBad, detail+"\n      acceptable: "+acceptableLabel(f.Expect.Acceptable))
					if p.Confidence == ConfidenceHigh {
						r.wrongHigh = append(r.wrongHigh, detail)
					}
				}
				if f.Expect.MaxConf != "" && p.Confidence.Rank() > Confidence(f.Expect.MaxConf).Rank() {
					r.maxConf = append(r.maxConf, detail+"\n      max_conf: "+f.Expect.MaxConf)
				}
				continue
			}

			r.unambiguous++
			wantKind := f.Expect.expectedKind()
			want := expectLabel(f.Expect.Eps)
			exact := p.Kind == wantKind && got == want && (f.Expect.Extra == "" || string(p.ExtraType) == f.Expect.Extra)
			if exact {
				r.exact++
			} else {
				wantDesc := fmt.Sprintf("%s %s", wantKind, want)
				if f.Expect.Extra != "" {
					wantDesc += " (" + f.Expect.Extra + ")"
				}
				r.misses = append(r.misses, detail+"\n      want "+wantDesc)
				if p.Confidence == ConfidenceHigh {
					r.wrongHigh = append(r.wrongHigh, detail+"\n      want "+wantDesc)
				}
			}
			if f.Expect.MaxConf != "" && p.Confidence.Rank() > Confidence(f.Expect.MaxConf).Rank() {
				r.maxConf = append(r.maxConf, detail+"\n      max_conf: "+f.Expect.MaxConf)
			}
			if exact && f.Expect.MinConf != "" && p.Confidence.Rank() < Confidence(f.Expect.MinConf).Rank() {
				r.confMisses = append(r.confMisses, detail+"\n      min_conf: "+f.Expect.MinConf)
			}
		}
	}
	return r
}

func acceptableLabel(as []*string) string {
	parts := make([]string, len(as))
	for i, a := range as {
		parts[i] = "null"
		if a != nil {
			parts[i] = *a
		}
	}
	return strings.Join(parts, ", ")
}

func (r corpusReport) accuracy() float64 {
	if r.unambiguous == 0 {
		return 0
	}
	return float64(r.exact) / float64(r.unambiguous)
}

func (r corpusReport) print(t *testing.T) {
	t.Logf("Corpus: %d cases (%d golden), %d files (%d ambiguous)", r.cases, r.golden, r.files, r.ambiguous)
	t.Logf("Exact on unambiguous files: %d/%d = %.1f%% (gate ≥ 95%%)", r.exact, r.unambiguous, 100*r.accuracy())
	t.Logf("Wrong high-confidence results: %d (gate 0)", len(r.wrongHigh))
	t.Logf("Confidence: high %d, medium %d, low %d, none %d",
		r.byConf[ConfidenceHigh], r.byConf[ConfidenceMedium], r.byConf[ConfidenceLow], r.byConf[ConfidenceNone])
	t.Logf("Ambiguous files outside their acceptable answers: %d; over max_conf: %d; reasons over %d runes: %d",
		len(r.ambiguousBad), len(r.maxConf), MaxReasonRunes, len(r.longReasons))
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		t.Logf("%s (%d):", title, len(items))
		for _, s := range items {
			t.Logf("  - %s", s)
		}
	}
	section("Misses", r.misses)
	section("Below min_conf (reported, not a gate)", r.confMisses)
	section("Wrong high", r.wrongHigh)
	section("Over max_conf", r.maxConf)
	section("Ambiguous outside acceptable", r.ambiguousBad)
	section("Reasons too long", r.longReasons)
}

// TestCorpus runs every corpus case and enforces the gates. Run it with -v
// for the report: go test ./internal/identify/ -run Corpus -v
func TestCorpus(t *testing.T) {
	r := runCorpus(t)
	r.print(t)

	if r.files < 150 {
		t.Errorf("corpus has %d files, want at least 150", r.files)
	}
	if r.golden < 20 {
		t.Errorf("corpus has %d golden cases, want at least 20", r.golden)
	}
	if r.accuracy() < 0.95 {
		t.Errorf("exact on unambiguous files = %.1f%%, want ≥ 95%%", 100*r.accuracy())
	}
	if len(r.wrongHigh) > 0 {
		t.Errorf("%d wrong high-confidence results, want 0", len(r.wrongHigh))
	}
	if len(r.maxConf) > 0 {
		t.Errorf("%d results over their max_conf", len(r.maxConf))
	}
	if len(r.ambiguousBad) > 0 {
		t.Errorf("%d ambiguous files outside their acceptable answers", len(r.ambiguousBad))
	}
	if len(r.longReasons) > 0 {
		t.Errorf("%d reasons over %d runes", len(r.longReasons), MaxReasonRunes)
	}
}

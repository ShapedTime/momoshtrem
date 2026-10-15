package identify

import (
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/shapedtime/momoshtrem/internal/identify/corpus"
)

// The corpus (testdata/corpus/*.yaml) is the classifier's spec; its schema
// is described at the top of anime.yaml. Package corpus loads it.

// corpusNow is "today" for cases without their own.
var corpusNow = corpus.DefaultNow

func loadCorpus(t testing.TB) []corpus.Case {
	t.Helper()
	cases, err := corpus.LoadCases("testdata")
	if err != nil {
		t.Fatal(err)
	}
	return cases
}

// caseInput builds the classifier's input for a case.
func caseInput(t testing.TB, c corpus.Case) Input { return caseInputWith(t, c, false) }

// caseInputWith builds the input, without the show's aliases when
// noAliases. cmd/classifier-eval builds it the same way.
func caseInputWith(t testing.TB, c corpus.Case, noAliases bool) Input {
	t.Helper()
	cat := show(t, c.Show)
	if c.DropSpecials {
		cat = withoutSpecials(cat)
	}
	if noAliases {
		cat = withoutAliases(cat)
	}
	now, err := c.NowTime()
	if err != nil {
		t.Fatal(err)
	}
	name, err := c.TorrentName()
	if err != nil {
		t.Fatal(err)
	}
	in := Input{TorrentName: name, Show: cat, Now: now}
	if c.Hint != nil {
		in.Hint = &Hint{Season: c.Hint.Season, Episode: c.Hint.Episode}
	}
	for i, f := range c.Files {
		size, err := corpus.ParseSize(f.Size)
		if err != nil {
			t.Fatalf("%s: %s: %v", c.ID, f.Path, err)
		}
		if size == 0 {
			size = typicalSize(ClassOf(f.Path))
		}
		in.Files = append(in.Files, TorrentFile{Path: c.FilePath(i), Size: size})
	}
	return in
}

// typicalSize is the size of a corpus file that gives none.
func typicalSize(class FileClass) int64 {
	switch class {
	case ClassVideo:
		return 1200 << 20
	case ClassSubtitle:
		return 40 << 10
	}
	return 200 << 10
}

// targetLabel writes a proposal's targets like the corpus does:
// "S01E01+S01E02", or "null" for none.
func targetLabel(targets []Target) string {
	labels := make([]string, len(targets))
	for i, t := range targets {
		labels[i] = corpus.Label(t.Season, t.Episode)
	}
	return corpus.JoinLabels(labels)
}

// corpusReport is the outcome of running the whole corpus.
type corpusReport struct {
	cases, files, golden, ambiguous  int
	unambiguous, exact               int
	byConf                           map[Confidence]int
	wrongHigh, maxConf, ambiguousBad []string
	misses, confMisses, longReasons  []string
}

func runCorpus(t *testing.T, noAliases bool) corpusReport {
	r := corpusReport{byConf: map[Confidence]int{}}
	for _, c := range loadCorpus(t) {
		r.cases++
		if c.Golden {
			r.golden++
		}
		res := Classify(caseInputWith(t, c, noAliases))
		if len(res.Proposals) != len(c.Files) {
			t.Fatalf("%s: %d proposals for %d files", c.ID, len(res.Proposals), len(c.Files))
		}
		for i, f := range c.Files {
			p := res.Proposals[i]
			r.files++
			r.byConf[p.Confidence]++
			got := corpus.Got{Kind: string(p.Kind), Extra: string(p.ExtraType), Targets: targetLabel(p.Targets)}
			where := fmt.Sprintf("%s: %s", c.ID, f.Path)
			detail := fmt.Sprintf("%s\n      got %s %s [%s] %q", where, p.Kind, got.Targets, p.Confidence, p.Reason)

			if n := utf8.RuneCountInString(p.Reason); n > MaxReasonRunes {
				r.longReasons = append(r.longReasons, fmt.Sprintf("%s (%d runes)", where, n))
			}
			if f.Ambiguous {
				r.ambiguous++
				if !f.Correct(got) {
					r.ambiguousBad = append(r.ambiguousBad, detail+"\n      acceptable: "+f.AcceptableLabel())
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
			exact := f.Correct(got)
			if exact {
				r.exact++
			} else {
				r.misses = append(r.misses, detail+"\n      want "+f.WantLabel())
				if p.Confidence == ConfidenceHigh {
					r.wrongHigh = append(r.wrongHigh, detail+"\n      want "+f.WantLabel())
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
	r := runCorpus(t, false)
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

// TestCorpusWithoutAliases runs the corpus without the shows' aliases, as
// the library has them until TMDB alternative titles are loaded. Fewer files
// are sure, but none may be wrong and high, over its max_conf, or outside
// its acceptable answers.
func TestCorpusWithoutAliases(t *testing.T) {
	r := runCorpus(t, true)
	t.Logf("Without aliases: exact %d/%d = %.1f%%, wrong high %d, over max_conf %d, ambiguous outside acceptable %d; high %d, medium %d, low %d, none %d",
		r.exact, r.unambiguous, 100*r.accuracy(), len(r.wrongHigh), len(r.maxConf), len(r.ambiguousBad),
		r.byConf[ConfidenceHigh], r.byConf[ConfidenceMedium], r.byConf[ConfidenceLow], r.byConf[ConfidenceNone])
	for _, s := range append(append(r.wrongHigh, r.maxConf...), r.ambiguousBad...) {
		t.Errorf("  - %s", s)
	}
}

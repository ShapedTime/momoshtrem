package main

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/identify/ai"
	"github.com/shapedtime/momoshtrem/internal/identify/corpus"
)

const (
	modeRules  = "rules"
	modeAuto   = "auto"
	modeAlways = "ai_always"
	// modeBlind is a diagnostic: ai_always without the rules' answers in
	// the prompt, so the AI's own answers show what it can do where the
	// rules have nothing. Not part of the gate.
	modeBlind = "ai_blind"
)

type config struct {
	testdata string
	modes    []string
	cases    []string
	parallel int
	verbose  bool
}

func (c config) needsAI() bool {
	return slices.ContainsFunc(c.modes, func(m string) bool { return m != modeRules })
}

// caseRun is a golden case ready to evaluate: its input and the rules'
// answer.
type caseRun struct {
	c     corpus.Case
	in    identify.Input
	rules []identify.Proposal
}

// loadGolden loads the golden cases and classifies them with the rules.
func loadGolden(testdata string, only []string) ([]caseRun, error) {
	cases, err := corpus.LoadCases(testdata)
	if err != nil {
		return nil, err
	}
	shows, err := corpus.LoadShows(testdata)
	if err != nil {
		return nil, err
	}
	var runs []caseRun
	for _, c := range cases {
		if !c.Golden || (len(only) > 0 && !slices.Contains(only, c.ID)) {
			continue
		}
		in, err := inputOf(c, shows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, caseRun{c: c, in: in, rules: identify.Classify(in).Proposals})
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("no golden cases in %s", testdata)
	}
	return runs, nil
}

// inputOf builds a case's classifier input, the way identify's corpus test
// does.
func inputOf(c corpus.Case, shows map[string]*corpus.Show) (identify.Input, error) {
	s, ok := shows[c.Show]
	if !ok {
		return identify.Input{}, fmt.Errorf("%s: no show fixture %q", c.ID, c.Show)
	}
	cat := catalogOf(s)
	if c.DropSpecials {
		eps := cat.Episodes[:0:0]
		for _, ep := range cat.Episodes {
			if ep.Season != 0 {
				eps = append(eps, ep)
			}
		}
		cat.Episodes = eps
	}
	now, err := c.NowTime()
	if err != nil {
		return identify.Input{}, err
	}
	name, err := c.TorrentName()
	if err != nil {
		return identify.Input{}, err
	}
	in := identify.Input{TorrentName: name, Show: cat, Now: now}
	if c.Hint != nil {
		in.Hint = &identify.Hint{Season: c.Hint.Season, Episode: c.Hint.Episode}
	}
	for i, f := range c.Files {
		size, err := corpus.ParseSize(f.Size)
		if err != nil {
			return identify.Input{}, fmt.Errorf("%s: %s: %w", c.ID, f.Path, err)
		}
		if size == 0 {
			size = typicalSize(identify.ClassOf(f.Path))
		}
		in.Files = append(in.Files, identify.TorrentFile{Path: c.FilePath(i), Size: size})
	}
	return in, nil
}

// catalogOf converts a corpus show (identify's fixtures_test.go has the
// same few lines).
func catalogOf(s *corpus.Show) *identify.ShowCatalog {
	cat := &identify.ShowCatalog{
		Title: s.Title, OriginalTitle: s.OriginalTitle, Aliases: s.Aliases, Year: s.Year,
		IsAnime: s.IsAnime, Ordering: identify.Ordering(s.Ordering), Countries: s.Countries,
	}
	for _, e := range s.Episodes {
		cat.Episodes = append(cat.Episodes, identify.Episode(e))
	}
	for _, a := range s.AltOrderings {
		ao := identify.AltOrdering{Name: a.Name}
		for _, g := range a.Groups {
			ao.Groups = append(ao.Groups, identify.AltGroup(g))
		}
		cat.AltOrderings = append(cat.AltOrderings, ao)
	}
	return cat
}

// typicalSize is the size of a corpus file that gives none.
func typicalSize(class identify.FileClass) int64 {
	switch class {
	case identify.ClassVideo:
		return 1200 << 20
	case identify.ClassSubtitle:
		return 40 << 10
	}
	return 200 << 10
}

// resolveInput is what the job service would hand the resolver. ai_blind
// hides the rules' answers from the model.
func resolveInput(mode string, r caseRun, scope []string) identify.ResolveInput {
	in := identify.ResolveInput{
		TorrentName: r.in.TorrentName, Files: r.in.Files, Show: r.in.Show,
		Rules: r.rules, Scope: scope, Hint: ai.HintText(r.in.Hint),
	}
	if mode == modeBlind {
		in.Rules = nil
	}
	return in
}

// scopeFor is the files a mode asks the AI about.
func scopeFor(mode string, rules []identify.Proposal) []string {
	switch mode {
	case modeAuto:
		return ai.UnsureFiles(rules)
	case modeAlways, modeBlind:
		return ai.Videos(rules)
	}
	return nil
}

// outcome is one torrent in one mode.
type outcome struct {
	caseID  string
	files   int
	correct int
	misses  []string
	changes []change
	called  bool
	usage   identify.Usage
	latency time.Duration
	err     error
	// alone scores the AI's own answers (ai_always).
	alone aloneScore
}

type change struct {
	where, before, after string
	beforeOK, afterOK    bool
}

// aloneScore is how the AI's answers fare without MergeAI's protection of
// the rules' high answers.
type aloneScore struct {
	videos, right, wrong, unanswered, wrongHigh int
	wrongs                                      []string
}

func evaluate(ctx context.Context, runs []caseRun, cfg config, resolver identify.Resolver) *report {
	rep := &report{torrents: len(runs)}
	for _, r := range runs {
		rep.files += len(r.c.Files)
		for _, p := range r.rules {
			if identify.ClassOf(p.Path) == identify.ClassVideo {
				rep.videos++
			}
		}
	}
	for _, mode := range cfg.modes {
		outs := make([]outcome, len(runs))
		do := func(i int) { outs[i] = evaluateOne(ctx, mode, runs[i], resolver) }
		if mode == modeRules || len(runs) == 1 {
			for i := range runs {
				do(i)
			}
		} else {
			// The first request writes the system prompt's cache entry;
			// the rest can read it once it has started.
			do(0)
			sem := make(chan struct{}, cfg.parallel)
			var wg sync.WaitGroup
			for i := 1; i < len(runs); i++ {
				wg.Add(1)
				sem <- struct{}{}
				go func(i int) {
					defer wg.Done()
					defer func() { <-sem }()
					do(i)
				}(i)
			}
			wg.Wait()
		}
		rep.modes = append(rep.modes, summarize(mode, outs))
	}
	return rep
}

func evaluateOne(ctx context.Context, mode string, r caseRun, resolver identify.Resolver) outcome {
	o := outcome{caseID: r.c.ID, files: len(r.c.Files)}
	final := r.rules
	scope := scopeFor(mode, r.rules)
	if len(scope) > 0 && resolver != nil {
		o.called = true
		start := time.Now()
		res, err := resolver.Resolve(ctx, resolveInput(mode, r, scope))
		o.latency, o.usage, o.err = time.Since(start), res.Usage, err
		if err == nil {
			final = identify.MergeAI(r.rules, res.Files, r.in.Show)
			if mode == modeAlways || mode == modeBlind {
				o.alone = scoreAlone(r, scope, res.Files)
			}
		}
	}
	for i, f := range r.c.Files {
		p, before := final[i], r.rules[i]
		ok := f.Correct(gotOf(p))
		if ok {
			o.correct++
		} else {
			o.misses = append(o.misses, fmt.Sprintf("%s: %s\n      got %s; want %s", r.c.ID, f.Path, describe(p), want(f)))
		}
		if describe(p) != describe(before) {
			o.changes = append(o.changes, change{
				where: r.c.ID + ": " + f.Path, before: describe(before), after: describe(p),
				beforeOK: f.Correct(gotOf(before)), afterOK: ok,
			})
		}
	}
	return o
}

// scoreAlone merges the AI's answers into the rules with nothing protected,
// and scores the videos in scope: an answer the AI didn't give (or that
// MergeAI rejected) is unanswered.
func scoreAlone(r caseRun, scope []string, files []identify.AIFile) aloneScore {
	open := make([]identify.Proposal, len(r.rules))
	for i, p := range r.rules {
		if p.Confidence == identify.ConfidenceHigh {
			p.Confidence = identify.ConfidenceMedium
		}
		open[i] = p
	}
	merged := identify.MergeAI(open, files, r.in.Show)
	inScope := map[string]bool{}
	for _, p := range scope {
		inScope[p] = true
	}
	var s aloneScore
	for i, f := range r.c.Files {
		p := merged[i]
		if !inScope[p.Path] {
			continue
		}
		s.videos++
		switch {
		case p.Source != identify.SourceAI:
			s.unanswered++
			s.wrongs = append(s.wrongs, fmt.Sprintf("%s: %s\n      no usable answer; want %s", r.c.ID, f.Path, want(f)))
		case f.Correct(gotOf(p)):
			s.right++
		default:
			s.wrong++
			if p.Confidence == identify.ConfidenceHigh {
				s.wrongHigh++
			}
			s.wrongs = append(s.wrongs, fmt.Sprintf("%s: %s\n      AI %s %q; want %s", r.c.ID, f.Path, describe(p), p.Reason, want(f)))
		}
	}
	return s
}

func gotOf(p identify.Proposal) corpus.Got {
	labels := make([]string, len(p.Targets))
	for i, t := range p.Targets {
		labels[i] = corpus.Label(t.Season, t.Episode)
	}
	return corpus.Got{Kind: string(p.Kind), Extra: string(p.ExtraType), Targets: corpus.JoinLabels(labels)}
}

// describe writes a proposal's outcome: "episode S01E05 [high]",
// "extra (opening)".
func describe(p identify.Proposal) string {
	g := gotOf(p)
	switch p.Kind {
	case identify.KindExtra, identify.KindSample:
		return fmt.Sprintf("%s (%s)", p.Kind, p.ExtraType)
	case identify.KindSubtitle:
		return fmt.Sprintf("subtitle %s", g.Targets)
	}
	if p.Confidence == identify.ConfidenceNone {
		return fmt.Sprintf("%s %s", p.Kind, g.Targets)
	}
	return fmt.Sprintf("%s %s [%s]", p.Kind, g.Targets, p.Confidence)
}

func want(f corpus.File) string {
	if f.Ambiguous {
		return "one of " + f.AcceptableLabel()
	}
	return f.WantLabel()
}

// modeSummary is a mode's totals.
type modeSummary struct {
	mode                    string
	files, correct          int
	calls                   int
	usage                   identify.Usage
	latencies               []time.Duration
	changes                 []change
	fixed, broke, unchanged int
	misses, errs            []string
	alone                   aloneScore
}

func summarize(mode string, outs []outcome) modeSummary {
	s := modeSummary{mode: mode}
	for _, o := range outs {
		s.files += o.files
		s.correct += o.correct
		s.misses = append(s.misses, o.misses...)
		if o.called {
			s.calls++
			s.usage = s.usage.Add(o.usage)
			s.latencies = append(s.latencies, o.latency)
		}
		if o.err != nil {
			s.errs = append(s.errs, fmt.Sprintf("%s: %v", o.caseID, o.err))
		}
		for _, c := range o.changes {
			s.changes = append(s.changes, c)
			switch {
			case !c.beforeOK && c.afterOK:
				s.fixed++
			case c.beforeOK && !c.afterOK:
				s.broke++
			default:
				s.unchanged++
			}
		}
		a := o.alone
		s.alone.videos += a.videos
		s.alone.right += a.right
		s.alone.wrong += a.wrong
		s.alone.unanswered += a.unanswered
		s.alone.wrongHigh += a.wrongHigh
		s.alone.wrongs = append(s.alone.wrongs, a.wrongs...)
	}
	return s
}

func (s modeSummary) accuracy() float64 { return ratio(s.correct, s.files) }

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// report is the whole run.
type report struct {
	model                   string
	torrents, files, videos int
	modes                   []modeSummary
}

func (r *report) print(w io.Writer, verbose bool) {
	fmt.Fprintf(w, "Golden set: %d torrents, %d files (%d videos). Model: %s\n\n", r.torrents, r.files, r.videos, r.model)
	fmt.Fprintf(w, "%-10s %-17s %-9s %-24s %-28s %-24s %s\n", "mode", "accuracy", "AI calls",
		"AI changes", "tokens in/write/read/out", "cost total (per torrent)", "latency p50/max")
	for _, m := range r.modes {
		acc := fmt.Sprintf("%d/%d %.1f%%", m.correct, m.files, 100*m.accuracy())
		if m.mode == modeRules {
			fmt.Fprintf(w, "%-10s %-17s %-9s\n", m.mode, acc, "-")
			continue
		}
		changes := fmt.Sprintf("%d: %d fixed, %d broke", len(m.changes), m.fixed, m.broke)
		tokens := fmt.Sprintf("%d/%d/%d/%d", m.usage.InputTokens, m.usage.CacheWriteTokens, m.usage.CacheReadTokens, m.usage.OutputTokens)
		cost := fmt.Sprintf("$%.4f ($%.5f)", m.usage.CostUSD, m.usage.CostUSD/float64(max(r.torrents, 1)))
		p50, worst := percentiles(m.latencies)
		fmt.Fprintf(w, "%-10s %-17s %-9d %-24s %-28s %-24s %s / %s\n", m.mode, acc, m.calls, changes, tokens, cost,
			p50.Round(10*time.Millisecond), worst.Round(10*time.Millisecond))
	}
	fmt.Fprintln(w)
	for _, m := range r.modes {
		if m.calls > 0 {
			fmt.Fprintf(w, "%s: $%.5f per AI call (%d calls), $%.5f per torrent over all %d.\n",
				m.mode, m.usage.CostUSD/float64(m.calls), m.calls, m.usage.CostUSD/float64(max(r.torrents, 1)), r.torrents)
		}
		if m.alone.videos > 0 {
			a := m.alone
			fmt.Fprintf(w, "%s, the AI's own answers (before MergeAI keeps the rules' high ones): %d/%d videos right (%.1f%%), %d wrong (%d of them high), %d unanswered.\n",
				m.mode, a.right, a.videos, 100*ratio(a.right, a.videos), a.wrong, a.wrongHigh, a.unanswered)
		}
		for _, e := range m.errs {
			fmt.Fprintf(w, "%s error: %s\n", m.mode, e)
		}
	}

	for _, m := range r.modes {
		if len(m.changes) > 0 && (verbose || m.broke > 0) {
			fmt.Fprintf(w, "\n%s: files the AI changed:\n", m.mode)
			for _, c := range m.changes {
				verdict := "still wrong"
				switch {
				case !c.beforeOK && c.afterOK:
					verdict = "FIXED"
				case c.beforeOK && !c.afterOK:
					verdict = "BROKE"
				case c.beforeOK && c.afterOK:
					verdict = "still right"
				}
				fmt.Fprintf(w, "  - %s [%s]\n      %s -> %s\n", c.where, verdict, c.before, c.after)
			}
		}
		if verbose && len(m.misses) > 0 {
			fmt.Fprintf(w, "\n%s: misses (%d):\n", m.mode, len(m.misses))
			for _, s := range m.misses {
				fmt.Fprintf(w, "  - %s\n", s)
			}
		}
		if verbose && len(m.alone.wrongs) > 0 {
			fmt.Fprintf(w, "\n%s: the AI's own wrong or missing answers (%d):\n", m.mode, len(m.alone.wrongs))
			for _, s := range m.alone.wrongs {
				fmt.Fprintf(w, "  - %s\n", s)
			}
		}
	}
}

// gate checks PLAN B3's acceptance: ai_always at least 95%, auto no worse
// than rules. A mode that didn't run isn't checked.
func (r *report) gate(w io.Writer) bool {
	byMode := map[string]modeSummary{}
	for _, m := range r.modes {
		byMode[m.mode] = m
	}
	ok := true
	check := func(name string, pass bool, detail string) {
		verdict := "PASS"
		if !pass {
			verdict, ok = "FAIL", false
		}
		fmt.Fprintf(w, "Gate: %s: %s (%s)\n", name, verdict, detail)
	}
	fmt.Fprintln(w)
	if m, ran := byMode[modeAlways]; ran {
		check("ai_always ≥ 95%", m.accuracy() >= 0.95 && len(m.errs) == 0,
			fmt.Sprintf("%.1f%%, %d errors", 100*m.accuracy(), len(m.errs)))
	}
	rules, ranRules := byMode[modeRules]
	if auto, ran := byMode[modeAuto]; ran && ranRules {
		check("auto ≥ rules", auto.correct >= rules.correct && len(auto.errs) == 0,
			fmt.Sprintf("%.1f%% vs %.1f%%, %d errors", 100*auto.accuracy(), 100*rules.accuracy(), len(auto.errs)))
	}
	return ok
}

func percentiles(ds []time.Duration) (p50, worst time.Duration) {
	if len(ds) == 0 {
		return 0, 0
	}
	sorted := slices.Clone(ds)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[(len(sorted)-1)/2], sorted[len(sorted)-1]
}

// printDryRun prints the requests the AI modes would make, without calling
// the API. Token counts are estimates (about 4 characters a token).
func printDryRun(w io.Writer, runs []caseRun, modes []string, prompt bool) {
	fmt.Fprintf(w, "Dry run: %d golden torrents; no API calls. Tokens are estimated at 4 characters each.\n", len(runs))
	for _, mode := range modes {
		if mode == modeRules {
			continue
		}
		var requests, files, chars int
		fmt.Fprintf(w, "\n%s:\n", mode)
		for _, r := range runs {
			scope := scopeFor(mode, r.rules)
			if len(scope) == 0 {
				fmt.Fprintf(w, "  %-28s no call (nothing in scope)\n", r.c.ID)
				continue
			}
			for i, q := range ai.DescribeRequests(resolveInput(mode, r, scope)) {
				requests++
				files += q.Files
				chars += len(q.User)
				fmt.Fprintf(w, "  %-28s request %d: %2d files, %4d episodes listed, user ~%5d tokens (%d chars), max_tokens %d\n",
					r.c.ID, i+1, q.Files, q.Episodes, len(q.User)/4, len(q.User), q.MaxTokens)
				if prompt {
					fmt.Fprintf(w, "%s\n%s\n", strings.Repeat("-", 72), q.User)
				}
			}
		}
		if requests > 0 {
			sys := ai.DescribeRequests(resolveInput(modeAlways, runs[0], ai.Videos(runs[0].rules)))
			sysTokens := 0
			if len(sys) > 0 {
				sysTokens = sys[0].SystemChars / 4
			}
			fmt.Fprintf(w, "  total: %d requests, %d files, user messages ~%d tokens; system prompt ~%d tokens (cached)\n",
				requests, files, chars/4, sysTokens)
		}
	}
}

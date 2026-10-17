package report

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Markdown sums the report up for a reader: the verdict, the checks, the
// metrics, the repetitions and the last good repetition's sessions.
func (r *Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# streamlab: %s: %s\n\n", r.Scenario.Name, strings.ToUpper(string(r.Verdict)))
	fmt.Fprintf(&b, "**Why:** %s\n\n", r.why())
	if doc := firstParagraph(r.Scenario.Doc); doc != "" {
		fmt.Fprintf(&b, "%s\n\n", doc)
	}
	fmt.Fprintf(&b, "- **Guards:** %s\n", orDash(strings.Join(r.Scenario.Guards, ", ")))
	confirm := ""
	if r.Run.Confirmations > 0 {
		confirm = fmt.Sprintf(" +%d confirmation", r.Run.Confirmations)
	}
	fmt.Fprintf(&b, "- **Run:** %s, %d reps%s (+%d warm-up discarded), %.1f s, seed %d, read_timeout %s, config %s%s\n",
		r.Run.ID, r.Run.Reps, confirm, r.Run.Warmup, r.Run.WallS, r.Scenario.Seed, orDash(r.Scenario.Params["read_timeout"]),
		orDash(r.Config.Fingerprint), configPath(r.Config.Path))
	fmt.Fprintf(&b, "- **Tool:** %s %s, commit %s%s\n", r.Tool.Name, r.Tool.Version, orDash(short(r.Tool.Git.Commit)), dirty(r.Tool.Git.Dirty))
	fmt.Fprintf(&b, "- **Env:** %s\n", r.Env.Line())
	b.WriteString("\n## Checks\n\n| Check | Status | Known | Reps | Evidence |\n|---|---|---|---|---|\n")
	for _, c := range r.Checks {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", c.ID, c.Status, orDash(c.Known), strings.Join(c.Reps, " "), cell(c.Evidence))
	}
	b.WriteString(RacesMarkdown(r.Races))

	b.WriteString("\n## Metrics\n\n| Metric | Kind | Median | p95 | Values | Unit | Better |\n|---|---|---|---|---|---|---|\n")
	names := make([]string, 0, len(r.Metrics))
	for n := range r.Metrics {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int {
		ka, kb := kindOrder(r.Metrics[a].Kind), kindOrder(r.Metrics[b].Kind)
		if ka != kb {
			return ka - kb
		}
		return strings.Compare(a, b)
	})
	for _, n := range names {
		m := r.Metrics[n]
		vals := make([]string, len(m.Values))
		for i, v := range m.Values {
			vals[i] = Num(v)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n", n, m.Kind, Num(m.Median), Num(m.P95), strings.Join(vals, " "), m.Unit, m.Better)
	}

	b.WriteString("\n## Repetitions\n\n| Rep | Status | Wall | Notes |\n|---|---|---|---|\n")
	for _, rep := range r.Reps {
		name := fmt.Sprint(rep.Rep)
		switch {
		case rep.Warmup:
			name += " (warm-up)"
		case rep.Confirm:
			name += " (confirmation)"
		}
		notes := strings.Join(rep.Notes, "; ")
		if rep.Error != "" {
			notes = rep.Error
		}
		fmt.Fprintf(&b, "| %s | %s | %.1f s | %s |\n", name, rep.Status, rep.WallMS/1000, cell(notes))
	}

	if rep := r.lastGood(); rep != nil {
		fmt.Fprintf(&b, "\n## Sessions (rep %d)\n", rep.Rep)
		for _, s := range rep.Sessions {
			fmt.Fprintf(&b, "\n### %s: %s\n\n", s.Device, s.Path)
			if p := s.Player; p != nil {
				fmt.Fprintf(&b, "TTFB %s ms, playable after %s ms, %d stalls (%s ms, rebuffer %.3f), over-read %s, received %s",
					Num(p.TTFBMS), Num(p.TimeToPlayableMS), p.Stalls, Num(p.StallTimeMS), p.RebufferRatio, Bytes(p.OverRead), Bytes(p.Received))
				if len(p.SeekLatenciesMS) > 0 {
					l := make([]string, len(p.SeekLatenciesMS))
					for i, v := range p.SeekLatenciesMS {
						l[i] = Num(v)
					}
					fmt.Fprintf(&b, ", seek latencies %s ms", strings.Join(l, " "))
				}
				b.WriteString(".\n")
				for _, e := range p.ReadErrors {
					fmt.Fprintf(&b, "- read error at %d: %s (%s)\n", e.Offset, e.Err, orDash(e.Diagnosis))
				}
				b.WriteString("\n")
			}
			b.WriteString("| Request | Method | Range | Status | TTFB ms | Bytes | Took ms | Aborted | Lag after abort ms |\n|---|---|---|---|---|---|---|---|---|\n")
			for _, q := range s.Requests {
				lag := "-"
				if q.HandlerLagMS != nil {
					lag = Num(*q.HandlerLagMS)
				}
				fmt.Fprintf(&b, "| %s | %s | %s | %d | %s | %s | %s | %v | %s |\n",
					q.ID, q.Method, orDash(q.Range), q.Status, Num(q.TTFBMS), Bytes(q.Bytes), Num(q.DurationMS), q.Aborted, lag)
			}
		}
		t := rep.Torrent
		fmt.Fprintf(&b, "\n**Torrent:** downloaded %s, served %s (waste ratio %.2f), residual %s in the %s ms after the last close.\n",
			Bytes(t.Downloaded), Bytes(t.Served), t.WasteRatio, Bytes(t.Residual), Num(t.ResidualWindowMS))
		res := rep.Resources
		fmt.Fprintf(&b, "\n**Resources:** peak %d goroutines, %d readers, %s heap; %d leaked goroutine stacks, %d inherited.\n",
			res.PeakGoroutines, res.PeakReaders, Bytes(int64(res.PeakHeapBytes)), len(res.Leaked), res.Inherited)
	}

	b.WriteString("\n## Params\n\n")
	keys := make([]string, 0, len(r.Scenario.Params))
	for k := range r.Scenario.Params {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "- `%s` = `%s`\n", k, r.Scenario.Params[k])
	}
	if len(r.Artifacts) > 0 {
		b.WriteString("\n## Artifacts\n\n")
		for _, a := range r.Artifacts {
			fmt.Fprintf(&b, "- %s\n", a)
		}
	}
	return b.String()
}

// why is the verdict's reason on one line: the failing checks, the
// repetitions that broke and their dumps, the expected failures.
func (r *Report) why() string {
	var fail, xfail, xpass []string
	for _, c := range r.Checks {
		s := c.ID
		if c.Known != "" {
			s += " (" + c.Known + ")"
		}
		switch c.Status {
		case Fail:
			fail = append(fail, s)
		case XFail:
			xfail = append(xfail, s)
		case XPass:
			xpass = append(xpass, s)
		}
	}
	var broken []string
	for _, rep := range r.Reps {
		if rep.Status == RepOK {
			continue
		}
		name := fmt.Sprintf("rep %d", rep.Rep)
		if rep.Warmup {
			name = "the warm-up"
		}
		s := fmt.Sprintf("%s %s", name, rep.Status)
		var dumps []string
		for k, v := range rep.Artifacts {
			if k == "goroutines" || strings.HasPrefix(k, "failure-") {
				dumps = append(dumps, v)
			}
		}
		slices.Sort(dumps)
		if len(dumps) == 0 {
			dumps = append(dumps, rep.Artifacts["log"])
		}
		broken = append(broken, s+" ("+strings.Join(dumps, ", ")+")")
	}
	var parts []string
	if len(fail) > 0 {
		parts = append(parts, "failing: "+strings.Join(fail, ", "))
	}
	if len(broken) > 0 {
		parts = append(parts, "broken: "+strings.Join(broken, "; "))
	}
	if len(parts) == 0 {
		parts = append(parts, "every check passed or is a known issue")
	}
	if len(xfail) > 0 {
		parts = append(parts, "expected failures: "+strings.Join(xfail, ", "))
	}
	if len(xpass) > 0 {
		parts = append(parts, "passing though known to fail (fixed?): "+strings.Join(xpass, ", "))
	}
	for _, rep := range r.Reps {
		for k, v := range rep.Artifacts {
			if rep.Status == RepOK && strings.HasPrefix(k, "failure-") {
				parts = append(parts, "status at a timed-out read: "+v)
			}
		}
	}
	return strings.Join(parts, "; ")
}

func configPath(p string) string {
	if p == "" {
		return ""
	}
	return " from " + p
}

// lastGood is the last measured repetition that ran to the end.
func (r *Report) lastGood() *Rep {
	for i := len(r.Reps) - 1; i >= 0; i-- {
		if rep := &r.Reps[i]; rep.Status == RepOK && !rep.Warmup {
			return rep
		}
	}
	return nil
}

// Line is the env on one line.
func (e Env) Line() string {
	mime := func(ext string) string {
		if t := e.MIME[ext]; t != "" {
			return t
		}
		return "unresolved"
	}
	s := fmt.Sprintf("%s %s/%s, %s, %d CPUs, GOMAXPROCS %d, cgo %v, race %v, uTP %s, .mkv %s, .mp4 %s",
		e.Go, e.OS, e.Arch, orDash(e.CPU), e.NumCPU, e.GOMAXPROCS, e.CGO, e.Race, e.UTP, mime(".mkv"), mime(".mp4"))
	if len(e.Load) == 3 {
		s += fmt.Sprintf(", load %.1f %.1f %.1f", e.Load[0], e.Load[1], e.Load[2])
	}
	return s
}

func kindOrder(k Kind) int {
	switch k {
	case Exact:
		return 0
	case Count:
		return 1
	}
	return 2
}

// Num formats a metric value: integers bare, others with 3 significant
// digits after the point at most.
func Num(v float64) string {
	switch {
	case math.IsNaN(v):
		return "-"
	case v == math.Trunc(v) && math.Abs(v) < 1e15:
		return fmt.Sprintf("%d", int64(v))
	case math.Abs(v) >= 100:
		return fmt.Sprintf("%.0f", v)
	case math.Abs(v) >= 1:
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("%.3g", v)
}

// Bytes formats a byte count in binary units.
func Bytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func firstParagraph(s string) string {
	p, _, _ := strings.Cut(strings.TrimSpace(s), "\n\n")
	return strings.Join(strings.Fields(p), " ")
}

// cell makes s safe in a Markdown table cell: one line, no pipes, short.
func cell(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	if r := []rune(s); len(r) > 400 {
		s = string(r[:400]) + "…"
	}
	return orDash(s)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

func dirty(d bool) string {
	if d {
		return " (dirty)"
	}
	return ""
}

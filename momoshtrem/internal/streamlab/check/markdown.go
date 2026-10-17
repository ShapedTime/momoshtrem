package check

import (
	"fmt"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Markdown is check.md: the verdict and why, the build, the scenarios,
// the races and hangs, and go test -race.
func (s *Summary) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# streamlab check %s: %s\n\n**Why:** %s\n\n", s.Run.ID, strings.ToUpper(string(s.Verdict)), s.Why)
	commit := s.Tool.Git.Commit
	if len(commit) > 12 {
		commit = commit[:12]
	}
	dirty := ""
	if s.Tool.Git.Dirty {
		dirty = " (dirty)"
	}
	fmt.Fprintf(&b, "- **Tool:** %s %s, commit %s%s\n", s.Tool.Name, s.Tool.Version, dash(commit), dirty)
	fmt.Fprintf(&b, "- **Build:** %s, race %v, tags %s, built in %.0f s\n", s.Build.Exe, s.Build.Race, dash(s.Build.Tags), s.Build.TookS)
	fmt.Fprintf(&b, "- **Env:** %s\n", s.Env.Line())
	fmt.Fprintf(&b, "- **Reps:** %d per scenario (+%d warm-up), %.0f s\n", s.Run.Reps, s.Run.Warmup, s.Run.WallS)
	if len(s.Run.Command) > 0 {
		fmt.Fprintf(&b, "- **Command:** `%s`\n", strings.Join(s.Run.Command, " "))
	}
	b.WriteString("\nTimings from a race build get no verdict (PLAN D8): the scenarios' reports keep them only as a record.\n")

	b.WriteString("\n## Stress tier\n\n| Scenario | Verdict | Not passing | Wall |\n|---|---|---|---|\n")
	for _, e := range s.Scenarios {
		not := strings.Join(e.Checks, ", ")
		if e.Error != "" {
			not = strings.TrimSpace(not + " " + e.Error)
		}
		fmt.Fprintf(&b, "| [%s](%s) | %s | %s | %.0f s |\n", e.Name, strings.TrimSuffix(e.Report, ".json")+".md", e.Verdict,
			cell(not), e.WallS)
	}

	fmt.Fprintf(&b, "\n## Hangs (%d)\n\n", len(s.Hangs))
	if len(s.Hangs) == 0 {
		b.WriteString("No repetition outlived its hard timeout.\n")
	}
	for _, h := range s.Hangs {
		fmt.Fprintf(&b, "- %s rep %d: %s; goroutines: %s\n", h.Scenario, h.Rep, h.Error, dash(h.Dump))
	}

	var races []report.Race
	scenarios := map[string]string{}
	for _, r := range s.Races {
		races = append(races, r.Race)
		scenarios[r.Signature] = strings.Join(r.Scenarios, ", ")
	}
	if len(races) == 0 {
		b.WriteString("\n## Data races (0)\n\nThe stress tier reported no data race.\n")
	} else {
		md := report.RacesMarkdown(races)
		for sig, scs := range scenarios {
			md = strings.Replace(md, "`"+sig+"`\n", "`"+sig+"`\n- **Scenarios:** "+scs+"\n", 1)
		}
		b.WriteString(md)
	}

	if t := s.Tests; t != nil {
		fmt.Fprintf(&b, "\n## go test -race: %s\n\n`%s`, %.0f s; output in %s.\n\n", strings.ToUpper(string(t.Verdict)),
			strings.Join(t.Command, " "), t.TookS, t.Output)
		if t.Error != "" {
			fmt.Fprintf(&b, "**Error:** %s\n\n", t.Error)
		}
		b.WriteString("| Package | Status | Elapsed | Failed tests | Races |\n|---|---|---|---|---|\n")
		for _, p := range t.Packages {
			fmt.Fprintf(&b, "| %s | %s | %.1f s | %s | %d |\n", p.Name, p.Status, p.ElapsedS, cell(strings.Join(p.Failed, ", ")), p.Races)
		}
		if len(t.Races) > 0 {
			b.WriteString(strings.Replace(report.RacesMarkdown(t.Races), "## Data races", "### go test's data races", 1))
		}
	}
	return b.String()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func cell(s string) string {
	s = strings.ReplaceAll(strings.Join(strings.Fields(s), " "), "|", "\\|")
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300]) + "…"
	}
	return dash(s)
}

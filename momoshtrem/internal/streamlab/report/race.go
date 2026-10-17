package report

import (
	"fmt"
	"slices"
	"strings"
)

// RacesMarkdown is a "Data races" section listing races: each signature,
// its count and known issue, where it was reported, and its first
// report. "" without races.
func RacesMarkdown(races []Race) string {
	if len(races) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Data races (%d)\n", len(races))
	for i, r := range races {
		known := "new: file it as a streaming issue (PLAN §0)"
		if r.Known != "" {
			known = "known: " + r.Known
		}
		fmt.Fprintf(&b, "\n### %d. %s\n\n- **Signature:** `%s`\n- **Reported:** %d times, in %s\n\n```\n%s\n```\n",
			i+1, known, r.Signature, r.Count, orDash(strings.Join(r.Where, ", ")), strings.TrimSpace(r.Text))
	}
	return b.String()
}

// Race is one data race the race detector reported (a "WARNING: DATA
// RACE" block), with every report of it folded in: streamlab check (S7,
// PLAN D8) collects them from each repetition's GORACE log. Any race is a
// fail; a race known by its exact Signature is an xfail until it's fixed.
type Race struct {
	// Signature names the race: each access's kind, its top frame and its
	// top momoshtrem frames, the two accesses sorted. Reports of one race
	// share it whichever access came first (internal/streamlab/race).
	Signature string `json:"signature"`
	// Count is how many times it was reported.
	Count int `json:"count"`
	// Known is the known issue registered with this exact signature; ""
	// for a race nobody has filed yet.
	Known string `json:"known,omitempty"`
	// Accesses are the two conflicting accesses: the one that found the
	// race, then the earlier one.
	Accesses []RaceAccess `json:"accesses"`
	// Where are the logs it was reported in, relative to the report's
	// folder (rep-1/race.12345, say).
	Where []string `json:"where,omitempty"`
	// Text is the first report as the race detector wrote it.
	Text string `json:"text"`
}

// RaceAccess is one side of a race: what the goroutine did and where.
type RaceAccess struct {
	// Op is "read" or "write", "atomic read" or "atomic write".
	Op string `json:"op"`
	// Goroutine is the goroutine's number, or "main".
	Goroutine string `json:"goroutine"`
	// Frames are the access's stack, innermost first: "func (file:line)".
	Frames []string `json:"frames"`
	// Created is where the goroutine was started, innermost first.
	Created []string `json:"created,omitempty"`
}

// MergeRaces folds lists of races into one, by signature: counts add up,
// the first report's text and accesses stay. The order is the first
// appearance's.
func MergeRaces(lists ...[]Race) []Race {
	var out []Race
	at := map[string]int{}
	for _, list := range lists {
		for _, r := range list {
			i, ok := at[r.Signature]
			if !ok {
				at[r.Signature] = len(out)
				r.Where = slices.Clone(r.Where)
				out = append(out, r)
				continue
			}
			out[i].Count += r.Count
			for _, w := range r.Where {
				if !slices.Contains(out[i].Where, w) {
					out[i].Where = append(out[i].Where, w)
				}
			}
		}
	}
	return out
}

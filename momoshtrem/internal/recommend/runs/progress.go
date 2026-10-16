package runs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
)

// Progress is rec_runs.progress (PLAN §6 "A run's steps"): the steps
// reached so far, in the order reached, each once, at most one active
// (the newest); all done once the run ends.
type Progress struct {
	Steps         []recommend.Step `json:"steps"`
	ShelvesDone   int              `json:"shelves_done"`
	ShelvesTarget int              `json:"shelves_target"`
}

// DecodeProgress reads a stored progress document; an empty or unreadable
// one is no progress.
func DecodeProgress(raw json.RawMessage) Progress {
	var p Progress
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &p)
	}
	if p.Steps == nil {
		p.Steps = []recommend.Step{}
	}
	return p
}

// stepOf is the step a custom tool starts (PLAN §6): history for the
// profile and check_seen, tmdb for the candidates and TMDB lookups, write
// for submitting.
func stepOf(tool string) string {
	switch tool {
	case tools.NameGetTasteProfile, tools.NameCheckSeen:
		return recommend.StepHistory
	case tools.NameGetCandidates, tools.NameTMDBSearch, tools.NameTMDBDetails:
		return recommend.StepTMDB
	case NameSubmitShelf, NameSubmitHeroPicks:
		return recommend.StepWrite
	}
	return ""
}

// tracker follows a run's progress and saves it when it changes. It runs
// on the run's goroutine (the loop's hooks and tools), so it needs no lock.
type tracker struct {
	p    Progress
	save func(Progress)
}

func newTracker(target int, save func(Progress)) *tracker {
	return &tracker{p: Progress{Steps: []recommend.Step{}, ShelvesTarget: target}, save: save}
}

// reach makes step the active one: the step active before is done. A step
// already reached stays where it is (each appears once).
func (t *tracker) reach(step string) {
	if step == "" {
		return
	}
	for _, s := range t.p.Steps {
		if s.Step == step {
			return
		}
	}
	for i := range t.p.Steps {
		t.p.Steps[i].State = recommend.StepDone
	}
	t.p.Steps = append(t.p.Steps, recommend.Step{Step: step, State: recommend.StepActive})
	t.changed()
}

// detail sets a reached step's tally.
func (t *tracker) detail(step, detail string) {
	for i := range t.p.Steps {
		if t.p.Steps[i].Step == step {
			if t.p.Steps[i].Detail != nil && *t.p.Steps[i].Detail == detail {
				return
			}
			d := detail
			t.p.Steps[i].Detail = &d
			t.changed()
			return
		}
	}
}

// webUsage is the run's web searches and fetches so far, as usage counts
// them (server-side filtering may search more than the blocks show): "3
// searches · 1 page".
func (t *tracker) webUsage(searches, fetches int64) {
	if searches <= 0 && fetches <= 0 {
		return
	}
	t.reach(recommend.StepWeb)
	parts := []string{}
	if searches > 0 {
		parts = append(parts, plural(int(searches), "search", "searches"))
	}
	if fetches > 0 {
		parts = append(parts, plural(int(fetches), "page", "pages"))
	}
	t.detail(recommend.StepWeb, strings.Join(parts, " · "))
}

// shelves counts the shelves written: "2 of 5 shelves".
func (t *tracker) shelves(done int) {
	if done == t.p.ShelvesDone {
		return
	}
	t.p.ShelvesDone = done
	t.p.ShelvesTarget = max(t.p.ShelvesTarget, done)
	t.reach(recommend.StepWrite)
	t.detail(recommend.StepWrite, fmt.Sprintf("%d of %d shelves", done, t.p.ShelvesTarget))
}

// finish marks every step done (the run ended).
func (t *tracker) finish() Progress {
	for i := range t.p.Steps {
		t.p.Steps[i].State = recommend.StepDone
	}
	return t.progress()
}

// progress is a copy of the progress.
func (t *tracker) progress() Progress {
	p := t.p
	p.Steps = make([]recommend.Step, len(t.p.Steps))
	for i, s := range t.p.Steps {
		p.Steps[i] = s
		if s.Detail != nil {
			d := *s.Detail
			p.Steps[i].Detail = &d
		}
	}
	return p
}

func (t *tracker) changed() {
	if t.save != nil {
		t.save(t.progress())
	}
}

// plural is "1 search" or "3 searches", the count grouped ("2,184 plays").
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return thousands(n) + " " + many
}

// thousands groups a count's digits: 2184 → "2,184".
func thousands(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + thousands(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

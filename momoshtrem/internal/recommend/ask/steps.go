package ask

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
)

// A turn's steps (PLAN §6 "Ask momoshtrem", status events): a step starts
// with the first tool that belongs to it (history: the profile and
// check_seen; web: web search and fetch; tmdb: the candidates and TMDB
// lookups; a refinement chip's every tool: rerank), and write with
// show_picks or the answer's text. Each step happens at most once a turn,
// in the order reached, with at most one active: starting one ends the
// active one with its detail, the step's tally of what its tools did.
// write is the last: once it's reached no other step starts, and its
// detail is the turn's picks when the turn ends.
//
// A custom tool's step starts when it runs (Hooks.ToolStart), after the
// tools before it in its response have answered, so the step it ends has
// its whole tally; only the first custom tool of a response starts its
// step as its block starts streaming (nothing before it is still waiting).
// A server tool's step starts with its block: it has already run.

// Server tools' names, as their blocks start.
const (
	serverWebSearch = "web_search"
	serverWebFetch  = "web_fetch"
)

// stepOf is the step a tool belongs to; "" for none.
func stepOf(tool string, refine bool) string {
	if tool == NameShowPicks {
		return recommend.StepWrite
	}
	var step string
	switch tool {
	case tools.NameGetTasteProfile, tools.NameCheckSeen:
		step = recommend.StepHistory
	case serverWebSearch, serverWebFetch:
		step = recommend.StepWeb
	case tools.NameGetCandidates, tools.NameTMDBSearch, tools.NameTMDBDetails:
		step = recommend.StepTMDB
	default:
		return ""
	}
	if refine {
		return recommend.StepRerank
	}
	return step
}

// tracker is a turn's steps and what its tools did.
type tracker struct {
	list   []recommend.Step
	active int // index into list; -1 for none
	tally  tally
}

func newTracker() *tracker {
	return &tracker{active: -1, tally: tally{checked: map[library.TitleRef]bool{}}}
}

// reach starts step unless it was reached before or write has started,
// and returns the status events: the active step's end, then the new
// one's start.
func (t *tracker) reach(step string) []recommend.Step {
	if step == "" {
		return nil
	}
	for _, s := range t.list {
		if s.Step == step || s.Step == recommend.StepWrite {
			return nil
		}
	}
	out := t.finish()
	t.list = append(t.list, recommend.Step{Step: step, State: recommend.StepActive})
	t.active = len(t.list) - 1
	return append(out, t.list[t.active])
}

// finish ends the active step with its detail, and returns its event.
func (t *tracker) finish() []recommend.Step {
	if t.active < 0 {
		return nil
	}
	s := &t.list[t.active]
	s.State = recommend.StepDone
	s.Detail = t.tally.detail(s.Step)
	t.active = -1
	return []recommend.Step{*s}
}

// ended is the steps as an ended turn keeps them: the active one is
// dropped (it never finished, and an old turn must never show a spinner).
func (t *tracker) ended() []recommend.Step {
	out := []recommend.Step{}
	for i, s := range t.list {
		if i != t.active {
			out = append(out, s)
		}
	}
	return out
}

// tally counts what a turn's tools did, for the steps' details.
type tally struct {
	// history
	profileRead bool
	plays       int
	loved       []string
	seenChecked int
	// web
	searches, fetches int
	// tmdb
	poolRead       bool
	matched, known int // candidates through the filters; excluded as already seen
	lookups        int
	// rerank: every title the tools looked at
	checked map[library.TitleRef]bool
	// write
	picks int
}

// serverTool counts a server tool call as its block starts.
func (t *tally) serverTool(name string) {
	switch name {
	case serverWebSearch:
		t.searches++
	case serverWebFetch:
		t.fetches++
	}
}

// titleRefs are the titles in a tool's answer.
type titleRefs []struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
}

// toolResult counts what a custom tool answered; errors count nothing.
func (t *tally) toolResult(call aicore.ToolCall, res aicore.ToolResult) {
	if res.IsError {
		return
	}
	var out struct {
		Counts struct {
			Plays int `json:"plays"`
		} `json:"counts"`
		Loved []struct {
			Title string `json:"title"`
		} `json:"loved"`
		Matched *int `json:"matched"`
		Pool    struct {
			Excluded int `json:"excluded"`
		} `json:"pool"`
		Items     titleRefs `json:"items"`
		Results   titleRefs `json:"results"`
		MediaType string    `json:"media_type"`
		TMDBID    int       `json:"tmdb_id"`
	}
	if json.Unmarshal(res.Output, &out) != nil {
		return
	}
	switch call.Name {
	case tools.NameGetTasteProfile:
		t.profileRead = true
		t.plays = out.Counts.Plays
		t.loved = t.loved[:0]
		for _, l := range out.Loved {
			if len(t.loved) == lovedShown {
				break
			}
			if l.Title != "" {
				t.loved = append(t.loved, l.Title)
			}
		}
	case tools.NameCheckSeen:
		t.seenChecked += len(out.Items)
		t.check(out.Items)
	case tools.NameGetCandidates:
		t.poolRead = true
		if out.Matched != nil {
			t.matched = max(t.matched, *out.Matched)
		}
		t.known = out.Pool.Excluded
		t.check(out.Items)
	case tools.NameTMDBSearch:
		t.lookups++
		t.check(out.Results)
	case tools.NameTMDBDetails:
		t.lookups++
		t.check(titleRefs{{MediaType: out.MediaType, TMDBID: out.TMDBID}})
	}
}

func (t *tally) check(refs titleRefs) {
	for _, r := range refs {
		if r.TMDBID > 0 {
			t.checked[library.TitleRef{MediaType: r.MediaType, TMDBID: r.TMDBID}] = true
		}
	}
}

// lovedShown is how many loved titles the history step names.
const lovedShown = 3

// detail is a step's tally, short and concrete: "2,184 plays · you loved
// Severance, The Bear, Ted Lasso", "2 searches · 1 page read", "148
// candidates · 23 already seen", "23 checked", "5 picks". Nil when there's
// nothing to say.
func (t *tally) detail(step string) *string {
	var parts []string
	switch step {
	case recommend.StepHistory:
		if t.profileRead {
			if t.plays > 0 {
				parts = append(parts, count(t.plays, "play", "plays"))
			}
			if len(t.loved) > 0 {
				parts = append(parts, "you loved "+strings.Join(t.loved, ", "))
			}
			if len(parts) == 0 {
				parts = append(parts, "No watch history yet")
			}
		} else if t.seenChecked > 0 {
			parts = append(parts, count(t.seenChecked, "title", "titles")+" checked")
		}
	case recommend.StepWeb:
		if t.searches > 0 {
			parts = append(parts, count(t.searches, "search", "searches"))
		}
		if t.fetches > 0 {
			parts = append(parts, count(t.fetches, "page", "pages")+" read")
		}
	case recommend.StepTMDB:
		switch {
		case t.poolRead:
			parts = append(parts, count(t.matched, "candidate", "candidates"), number(t.known)+" already seen")
		case t.lookups > 0:
			parts = append(parts, count(t.lookups, "lookup", "lookups"))
		}
	case recommend.StepRerank:
		if len(t.checked) > 0 {
			parts = append(parts, number(len(t.checked))+" checked")
		}
	case recommend.StepWrite:
		if t.picks > 0 {
			parts = append(parts, count(t.picks, "pick", "picks"))
		}
	}
	if len(parts) == 0 {
		return nil
	}
	d := strings.Join(parts, " · ")
	return &d
}

// count is "1 pick", "5 picks", "2,184 plays".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return number(n) + " " + many
}

// number groups thousands: 2,184.
func number(n int) string {
	s := strconv.Itoa(n)
	if n < 0 || len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

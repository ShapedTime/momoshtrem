package ask

import (
	"testing"

	"github.com/shapedtime/momoshtrem/internal/recommend"
)

func TestStepOf(t *testing.T) {
	for _, tt := range []struct {
		tool   string
		refine bool
		want   string
	}{
		{"get_taste_profile", false, recommend.StepHistory},
		{"check_seen", false, recommend.StepHistory},
		{"web_search", false, recommend.StepWeb},
		{"web_fetch", false, recommend.StepWeb},
		{"get_candidates", false, recommend.StepTMDB},
		{"tmdb_search", false, recommend.StepTMDB},
		{"tmdb_details", false, recommend.StepTMDB},
		{"show_picks", false, recommend.StepWrite},
		{"get_candidates", true, recommend.StepRerank},
		{"web_search", true, recommend.StepRerank},
		{"show_picks", true, recommend.StepWrite},
		{"code_execution", false, ""},
	} {
		if got := stepOf(tt.tool, tt.refine); got != tt.want {
			t.Errorf("stepOf(%s, %v) = %q, want %q", tt.tool, tt.refine, got, tt.want)
		}
	}
}

// Each step is reached once, in order, with at most one active; an ended
// turn keeps the done ones.
func TestTracker(t *testing.T) {
	tr := newTracker()
	tr.tally.searches, tr.tally.fetches = 2, 1
	var got []string
	for _, step := range []string{"history", "web", "history", "tmdb", "web", "write"} {
		for _, s := range tr.reach(step) {
			d := "-"
			if s.Detail != nil {
				d = *s.Detail
			}
			got = append(got, s.Step+":"+s.State+":"+d)
		}
	}
	want := "history:active:- history:done:- web:active:- web:done:2 searches · 1 page read tmdb:active:- tmdb:done:- write:active:-"
	if g := join(got); g != want {
		t.Errorf("events = %s\nwant %s", g, want)
	}
	if ended := tr.ended(); len(ended) != 3 || ended[2].Step != "tmdb" {
		t.Errorf("ended = %+v, want the active write dropped", ended)
	}
	// write is last: nothing starts after it, and it stays active.
	if after := tr.reach("rerank"); len(after) != 0 || tr.list[tr.active].Step != "write" {
		t.Errorf("a step after write = %+v", after)
	}
	if done := tr.finish(); len(done) != 1 || done[0].Step != "write" || len(tr.ended()) != 4 {
		t.Errorf("finish = %+v", done)
	}
}

func TestDetails(t *testing.T) {
	ta := tally{profileRead: true, plays: 2184, loved: []string{"Severance", "The Bear"}, poolRead: true, matched: 1, known: 0, picks: 1}
	for step, want := range map[string]string{
		recommend.StepHistory: "2,184 plays · you loved Severance, The Bear",
		recommend.StepTMDB:    "1 candidate · 0 already seen",
		recommend.StepWrite:   "1 pick",
	} {
		if got := ta.detail(step); got == nil || *got != want {
			t.Errorf("%s = %v, want %q", step, got, want)
		}
	}
	empty := tally{profileRead: true}
	if got := empty.detail(recommend.StepHistory); got == nil || *got != "No watch history yet" {
		t.Errorf("empty history = %v", got)
	}
	if got := (&tally{lookups: 3}).detail(recommend.StepTMDB); got == nil || *got != "3 lookups" {
		t.Errorf("lookups = %v", got)
	}
	if got := (&tally{}).detail(recommend.StepRerank); got != nil {
		t.Errorf("an empty rerank = %v, want null", *got)
	}
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 2184: "2,184", 1234567: "1,234,567"} {
		if got := number(n); got != want {
			t.Errorf("number(%d) = %s", n, got)
		}
	}
}

func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

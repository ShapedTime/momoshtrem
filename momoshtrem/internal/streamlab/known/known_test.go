package known_test

import (
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

func reps(spec ...string) []known.Outcome {
	var out []known.Outcome
	for _, s := range spec {
		status, id, _ := strings.Cut(s, ":")
		out = append(out, known.Outcome{Status: report.Status(status), Known: id, Evidence: "rep says " + s})
	}
	return out
}

// prod is production's MIME table, in a scenario guarding every suspect.
var prod = known.Env{MIMEResolvesMKV: false, Guards: []string{"F1", "F2", "F3", "F6", "F11", "F12", "F13"}}

// runOf is prod with #195 fired on fired of the run's reps.
func runOf(fired, reps int) known.Env {
	env := prod
	env.Run = map[string]known.Tally{"#195": {Fired: fired, Reps: reps}}
	return env
}

func TestJudge(t *testing.T) {
	tests := []struct {
		name  string
		check string
		reps  []known.Outcome
		env   known.Env
		want  report.Status
		known string
	}{
		{"passing", "integrity", reps("pass", "pass"), prod, report.Pass, ""},
		{"unexplained failure", "integrity", reps("pass", "fail"), prod, report.Fail, ""},
		{"no rep could tell", "propfind-no-load", reps("n/a", "n/a"), prod, report.NA, ""},

		// Deterministic issues: they fire on every rep by nature.
		{"deterministic, every rep", "abort-lag", reps("fail:F12", "fail:F12", "fail:F12"), prod, report.XFail, "F12"},
		{"deterministic, one rep", "abort-lag", reps("fail:F12"), prod, report.XFail, "F12"},
		{"deterministic, fixed", "abort-lag", reps("pass", "pass"), prod, report.XPass, "F12"},
		{"passing where it isn't guarded", "abort-lag", reps("pass", "pass"), known.Env{}, report.Pass, ""},
		{"F11 breaks a playback", "play-ok", reps("fail:F11"), prod, report.XFail, "F11"},
		{"F11's signature where it isn't guarded", "play-ok", reps("fail:F11"), known.Env{Guards: []string{"F12"}}, report.Fail, ""},
		{"play-ok passing in a scenario not guarding F11", "play-ok", reps("pass"), known.Env{Guards: []string{"F12"}}, report.Pass, ""},
		{"deterministic, n/a reps don't count", "propfind-no-load", reps("n/a", "fail:F3"), prod, report.XFail, "F3"},
		{"a failure without the signature", "abort-lag", reps("fail:F12", "fail"), prod, report.Fail, ""},
		{"the wrong issue's signature", "etag-stable", reps("fail:F12"), prod, report.Fail, ""},
		{"an unknown issue", "etag-stable", reps("fail:F99"), prod, report.Fail, ""},

		// F1 only where .mkv doesn't resolve.
		{"F1 in production's image", "no-sniff-reads", reps("fail:F1"), prod, report.XFail, "F1"},
		{"F1 can't happen here", "no-sniff-reads", reps("fail:F1"), known.Env{MIMEResolvesMKV: true, Guards: []string{"F1"}}, report.Fail, ""},
		{"F1 passing here isn't a fix", "no-sniff-reads", reps("pass"), known.Env{MIMEResolvesMKV: true, Guards: []string{"F1"}}, report.Pass, ""},
		{"F1 passing where it applies is", "no-sniff-reads", reps("pass"), prod, report.XPass, "F1"},

		// #195 is intermittent: at most 25% of reps, never all of them.
		{"#195 on 1 of 4 reps", "play-ok", reps("pass", "fail:#195", "pass", "pass"), prod, report.XFail, "#195"},
		{"#195 on 1 of 10 reps", "play-ok", reps("pass", "pass", "pass", "fail:#195", "pass", "pass", "pass", "pass", "pass", "pass"), prod, report.XFail, "#195"},
		{"#195 on 2 of 4 reps", "play-ok", reps("fail:#195", "pass", "fail:#195", "pass"), prod, report.Fail, "#195"},
		{"#195 on every rep", "play-ok", reps("fail:#195", "fail:#195", "fail:#195", "fail:#195", "fail:#195"), prod, report.Fail, "#195"},
		{"#195 on the only rep", "play-ok", reps("fail:#195"), prod, report.Fail, "#195"},
		{"#195 on the only rep, rare in the run", "play-ok", reps("fail:#195"), runOf(1, 14), report.Fail, "#195"},
		{"#195 once in 3 (confirmed)", "play-ok", reps("fail:#195", "pass", "pass"), runOf(1, 16), report.XFail, "#195"},
		{"#195 again on a confirmation", "play-ok", reps("fail:#195", "fail:#195", "pass"), runOf(2, 16), report.Fail, "#195"},
		{"#195 once in 2", "play-ok", reps("fail:#195", "pass"), runOf(1, 28), report.XFail, "#195"},
		{"#195 on the only rep of 4 in 14", "play-ok", reps("fail:#195"), runOf(4, 14), report.Fail, "#195"},
		{"#195 on every rep of the run", "play-ok", reps("fail:#195"), runOf(14, 14), report.Fail, "#195"},
		{"#195 on every rep of a scenario, rare in the run", "play-ok", reps("fail:#195", "fail:#195"), runOf(2, 28), report.Fail, "#195"},
		{"#195 on 1 of 4, rare in the run", "play-ok", reps("pass", "fail:#195", "pass", "pass"), runOf(1, 56), report.XFail, "#195"},
		{"#195 in 3 scenarios of 14, once each (confirmed)", "play-ok", reps("fail:#195", "pass", "pass"), runOf(3, 42), report.XFail, "#195"},
		{"#195 in many scenarios", "play-ok", reps("fail:#195", "pass", "pass"), runOf(12, 42), report.Fail, "#195"},
		{"#195 absent isn't a fix", "play-ok", reps("pass", "pass"), known.Env{}, report.Pass, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := known.Judge(tt.check, tt.reps, tt.env)
			if v.Status != tt.want || v.Known != tt.known {
				t.Errorf("Judge = %s (known %q): %s; want %s (known %q)", v.Status, v.Known, v.Note, tt.want, tt.known)
			}
			if v.Status != report.Pass && v.Note == "" {
				t.Errorf("a %s verdict says nothing", v.Status)
			}
		})
	}
}

func TestFired(t *testing.T) {
	o := func(status, id string) known.Outcome { return known.Outcome{Status: report.Status(status), Known: id} }
	if id, ok := known.Fired(map[string]known.Outcome{"play-ok": o("fail", "#195")}); !ok || id != "#195" {
		t.Errorf("Fired = %q, %v", id, ok)
	}
	for _, rep := range []map[string]known.Outcome{
		{"play-ok": o("pass", "")},
		{"play-ok": o("fail", "")},
		{"abort-lag": o("fail", "F12")}, // deterministic
	} {
		if id, ok := known.Fired(rep); ok {
			t.Errorf("Fired(%v) = %q", rep, id)
		}
	}
}

// The run's tally counts the repetitions that ran a check the issue
// explains, and those where it fired.
func TestTallyRun(t *testing.T) {
	o := func(status, id string) known.Outcome { return known.Outcome{Status: report.Status(status), Known: id} }
	run := []map[string]known.Outcome{
		{"play-ok": o("pass", ""), "integrity": o("pass", "")},
		{"play-ok": o("fail", "#195")},
		{"play-ok": o("fail", "")},   // a failure without the signature isn't a firing
		{"play-ok": o("n/a", "")},    // didn't run
		{"integrity": o("fail", "")}, // no play-ok
	}
	got := known.TallyRun(run)["#195"]
	if got != (known.Tally{Fired: 1, Reps: 3}) {
		t.Errorf("tally = %+v, want 1 of 3", got)
	}
	if _, ok := known.TallyRun(run)["F12"]; ok {
		t.Error("a deterministic issue was tallied")
	}
}

// Every entry explains checks, has a rate in (0, 1] and a doc; IDs are
// unique.
func TestRegistry(t *testing.T) {
	seen := map[string]bool{}
	for _, is := range known.Issues {
		if seen[is.ID] {
			t.Errorf("%s registered twice", is.ID)
		}
		seen[is.ID] = true
		if len(is.Checks) == 0 || is.Doc == "" || is.Title == "" || is.Ref == "" {
			t.Errorf("%s: needs checks, a title, a ref and a doc", is.ID)
		}
		if r := is.Rate(); r <= 0 || r > 1 {
			t.Errorf("%s: rate %v", is.ID, r)
		}
	}
	for _, id := range []string{"F1", "F2", "F3", "F6", "F11", "F12", "F13", "#195", "#225"} {
		if !seen[id] {
			t.Errorf("%s isn't registered", id)
		}
	}
	if is, _ := known.Lookup("#195"); !is.Intermittent() || is.Rate() != known.DefaultMaxRate {
		t.Errorf("#195 must be intermittent at the default rate: %+v", is)
	}
}

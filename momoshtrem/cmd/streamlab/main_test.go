package main

import (
	"bytes"
	"strings"
	"testing"
)

func cli(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestList(t *testing.T) {
	code, out, _ := cli("list")
	if code != exitPass || !strings.Contains(out, "dual-reader-open") || !strings.Contains(out, "F13") {
		t.Errorf("list = %d:\n%s", code, out)
	}
}

// list's summaries are whole words, never cut inside a parenthesis.
func TestFirstSentence(t *testing.T) {
	for _, sc := range []string{"pause-resume", "parallel-ranges", "scrub"} {
		code, out, _ := cli("list")
		if code != exitPass {
			t.Fatal(code)
		}
		for _, line := range strings.Split(out, "\n") {
			if !strings.HasPrefix(line, sc+" ") {
				continue
			}
			if strings.Count(line, "(") != strings.Count(line, ")") {
				t.Errorf("%s's summary is cut inside a parenthesis: %q", sc, line)
			}
			if len(line) > 160 {
				t.Errorf("%s's line is %d long", sc, len(line))
			}
		}
	}
	if got := firstSentence("A short one. Then more."); got != "A short one." {
		t.Errorf("firstSentence = %q", got)
	}
}

func TestExplain(t *testing.T) {
	for name, want := range map[string]string{
		"scrub":                   "abort_lag_max*",
		"handler_lag_after_abort": "metric handler_lag_after_abort: timing",
		"abort-lag":               "Known issue F12",
		"#195":                    "intermittent: an xfail on at most 25% of reps and never all; a firing in a scenario with under 2 reps gets 2 confirmation reps",
		"F2":                      "deterministic",
	} {
		code, out, errOut := cli("explain", name)
		if code != exitPass || !strings.Contains(out, want) {
			t.Errorf("explain %s = %d, want %q in:\n%s%s", name, code, want, out, errOut)
		}
	}
}

// Bad input is a tool error: exit 3.
func TestToolErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"nope"},
		{"explain", "nope"},
		{"explain"},
		{"run", "--scenario", "nope"},
		{"run", "--tier", "nope"},
		{"run", "--tier", "gate", "--scenario", "scrub"},
		{"run", "--scenario", "scrub", "--set", "nope=1"},
		{"run", "--scenario", "scrub", "--set", "seeks=many"},
		{"run", "--scenario", "scrub", "--reps", "0"},
		{"run", "--scenario", "scrub", "--warmup", "-1"},
		{"run", "--scenario", "scrub", "--timeout", "-1s"},
		{"run", "extra"},
		{"compare"},
		{"ab"},
	} {
		if code, _, _ := cli(args...); code != exitTool {
			t.Errorf("streamlab %q = %d, want %d", args, code, exitTool)
		}
	}
}

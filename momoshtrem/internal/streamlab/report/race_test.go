package report

import (
	"runtime/debug"
	"strings"
	"testing"
)

// env.race follows the race build tag, whatever the build settings say:
// compare (S8) refuses timing verdicts for race builds, so it must hold.
func TestCollectEnvRace(t *testing.T) {
	if got := CollectEnv().Race; got != raceBuild {
		t.Errorf("env.race = %v in a binary whose race tag is %v", got, raceBuild)
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "-race" && (s.Value == "true") != raceBuild {
				t.Errorf("build settings -race=%s, race tag %v", s.Value, raceBuild)
			}
		}
	}
}

func TestMergeRaces(t *testing.T) {
	a := Race{Signature: "a", Count: 2, Where: []string{"rep-1/race.1"}, Text: "first"}
	b := Race{Signature: "b", Count: 1, Where: []string{"rep-1/race.1"}}
	a2 := Race{Signature: "a", Count: 3, Where: []string{"rep-2/race.9", "rep-1/race.1"}, Text: "second"}
	got := MergeRaces([]Race{a, b}, []Race{a2})
	if len(got) != 2 || got[0].Signature != "a" || got[0].Count != 5 || got[0].Text != "first" ||
		strings.Join(got[0].Where, " ") != "rep-1/race.1 rep-2/race.9" || got[1].Count != 1 {
		t.Errorf("MergeRaces = %+v", got)
	}
	// The inputs keep their own Where.
	if len(a.Where) != 1 {
		t.Errorf("MergeRaces changed its input: %v", a.Where)
	}
	if MergeRaces() != nil {
		t.Error("merging nothing gave something")
	}
}

func TestRacesMarkdown(t *testing.T) {
	if RacesMarkdown(nil) != "" {
		t.Error("a section without races")
	}
	md := RacesMarkdown([]Race{{Signature: "write x | read y", Count: 2, Known: "#7", Where: []string{"rep-1/race.3"}, Text: "WARNING: DATA RACE"},
		{Signature: "write z | write z", Count: 1, Text: "WARNING: DATA RACE"}})
	for _, want := range []string{"## Data races (2)", "known: #7", "new: file it", "`write x | read y`", "rep-1/race.3"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
}

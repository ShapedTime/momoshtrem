package check

import (
	"github.com/shapedtime/momoshtrem/internal/streamlab/race"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// RaceHooks are what a run of a race build needs beyond the binary, as
// check sets them up: the scenarios with the race check added, each
// repetition's GORACE log in its folder (runner.Options.RepEnv), and its
// races read back into it once it ended (runner.Options.AfterRep).
// streamlab ab --race uses them for both sides.
func RaceHooks(scs []*scenario.Scenario) ([]*scenario.Scenario, func(repDir string) []string, func(repDir string, rep *report.Rep)) {
	return withRaceCheck(scs), func(repDir string) []string { return []string{race.Env(repDir)} }, collectRaces
}

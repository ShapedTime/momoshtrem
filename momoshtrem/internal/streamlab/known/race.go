package known

import (
	"slices"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/race"
)

// RaceCheck is the check streamlab check adds to every scenario it runs:
// the repetition's process reported no data race (PLAN §6: any race is a
// fail).
const RaceCheck = "race"

// RaceIssue is a data race found in production code and not fixed yet.
// It explains a race check's failure only when every race of the
// repetition carries a registered signature exactly (report.Race's
// Signature: both accesses' kinds, their top frames with file and line,
// and their top momoshtrem frames). Any other race, a new one on another
// line of the same functions included, still fails; so does this one once
// its top line moves, until its signature is updated.
type RaceIssue struct {
	Issue
	// Signature is the race's exact signature.
	Signature string
}

// Races are the known races. Each is filed as its own streaming issue,
// linked from #171, and removed by the PR that fixes it.
var Races = []RaceIssue{}

func init() {
	for _, r := range Races {
		is := r.Issue
		is.Checks = []string{RaceCheck}
		Issues = append(Issues, is)
	}
}

// MatchRace returns the known race registered with exactly signature. An
// unparsed report (race.Unparsed) never matches: nothing says what raced.
func MatchRace(signature string) (RaceIssue, bool) {
	if strings.HasPrefix(signature, race.Unparsed) {
		return RaceIssue{}, false
	}
	i := slices.IndexFunc(Races, func(r RaceIssue) bool { return r.Signature == signature })
	if i < 0 {
		return RaceIssue{}, false
	}
	return Races[i], true
}

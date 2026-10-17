package scenario

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
)

// Expectation is a failure a fault is meant to cause: a broken body or a
// failed request of one title, at most Max of them, while it's open. The
// play-ok check leaves those out; one more is a failure like any other.
// A script opens it just before the fault and ends it once the player has
// recovered.
type Expectation struct {
	title *Title
	why   string
	max   int
	// got counts the failures it explained; ended is when End was called.
	got   int
	ended time.Time
}

// Expect expects up to max failures of t, for why, until End.
func (e *Env) Expect(t *Title, max int, why string) *Expectation {
	x := &Expectation{title: t, why: why, max: max}
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	e.rec.expects = append(e.rec.expects, x)
	return x
}

// End closes the expectation and returns how many failures it explained.
func (e *Env) End(x *Expectation) int {
	e.rec.mu.Lock()
	defer e.rec.mu.Unlock()
	if x.ended.IsZero() {
		x.ended = time.Now()
	}
	return x.got
}

// expected is why a failure of t now was expected: an open expectation of
// t with room left takes it. "" when none. Called with r.mu held.
func (r *recorder) expected(t *Title) string {
	for _, x := range r.expects {
		if x.title == t && x.ended.IsZero() && x.got < x.max {
			x.got++
			return x.why
		}
	}
	return ""
}

// unexpected is the failures no expectation explained, and a note of the
// expected ones for the evidence.
func unexpected(fs []failure) ([]failure, string) {
	var out []failure
	n := 0
	for _, f := range fs {
		if f.expected != "" {
			n++
			continue
		}
		out = append(out, f)
	}
	if n == 0 {
		return out, ""
	}
	return out, fmt.Sprintf(" (%d expected failures left out)", n)
}

// knownSep joins known issues (known.Sep), where a local variable
// shadows the package.
const knownSep = known.Sep

// newIntegrity checks bytes of l's files.
func newIntegrity(l *media.Layout) *checks.Integrity { return checks.NewIntegrity(l) }

// integrityKnown is the known issues that explain every mismatched byte,
// joined by known.Sep; "" when any byte is unexplained.
func integrityKnown(unexplained int64, explained map[string]int64) string {
	if unexplained > 0 || len(explained) == 0 {
		return ""
	}
	ids := make([]string, 0, len(explained))
	for id := range explained {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return strings.Join(ids, known.Sep)
}

func explainedNote(explained map[string]int64) string {
	if len(explained) == 0 {
		return ""
	}
	var parts []string
	for id, n := range explained {
		parts = append(parts, fmt.Sprintf("%d with %s's signature", n, id))
	}
	slices.Sort(parts)
	return " (" + strings.Join(parts, ", ") + ")"
}

// joinErrs is the errors' messages, the nil ones left out.
func joinErrs(errs ...error) string {
	return errors.Join(errs...).Error()
}

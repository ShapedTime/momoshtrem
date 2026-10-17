package scenario

import (
	"errors"
	"fmt"
)

// catalog is every scenario, in the order a run takes them.
var catalog = []*Scenario{
	coldStart,
	steadyPlay,
	seekFwd,
	seekBack,
	scrub,
	probeBurst,
	parallelRanges,
	dualReaderClose,
	dualReaderOpen,
	pauseResume,
	reconnect,
	nextEpisode,
	smallCache,
	etagStability,
}

// All returns every scenario.
func All() []*Scenario { return append([]*Scenario(nil), catalog...) }

// Lookup returns the scenario called name.
func Lookup(name string) (*Scenario, bool) {
	for _, sc := range catalog {
		if sc.Name == name {
			return sc, true
		}
	}
	return nil, false
}

// InTier returns the tier's scenarios.
func InTier(t Tier) []*Scenario {
	var out []*Scenario
	for _, sc := range catalog {
		if sc.Tier == t {
			out = append(out, sc)
		}
	}
	return out
}

// Validate checks the catalog: unique names, declared checks and key
// metrics, and params whose defaults parse.
func Validate() error {
	var errs []error
	names := map[string]bool{}
	for _, sc := range catalog {
		if names[sc.Name] {
			errs = append(errs, fmt.Errorf("%s: duplicate name", sc.Name))
		}
		names[sc.Name] = true
		if sc.Script == nil || sc.Doc == "" || sc.Tier == "" {
			errs = append(errs, fmt.Errorf("%s: needs a script, a doc and a tier", sc.Name))
		}
		for _, id := range sc.AllChecks() {
			if _, ok := LookupCheck(id); !ok {
				errs = append(errs, fmt.Errorf("%s: undeclared check %q", sc.Name, id))
			}
		}
		for _, m := range sc.Key {
			if _, ok := LookupMetric(m); !ok {
				errs = append(errs, fmt.Errorf("%s: undeclared key metric %q", sc.Name, m))
			}
		}
		seen := map[string]bool{}
		for _, p := range sc.Params {
			if seen[p.Name] {
				errs = append(errs, fmt.Errorf("%s: param %q declared twice", sc.Name, p.Name))
			}
			seen[p.Name] = true
			for _, c := range CommonParams {
				if c.Name == p.Name && c.Kind != p.Kind {
					errs = append(errs, fmt.Errorf("%s: param %q is a %s, not a %s", sc.Name, p.Name, c.Kind, p.Kind))
				}
			}
		}
		if _, err := sc.Resolve(nil); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

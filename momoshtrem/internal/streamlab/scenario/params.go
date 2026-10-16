package scenario

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Kind is a param's type.
type Kind int

const (
	Int Kind = iota
	Float
	// Bytes is a size: a number with an optional B, KiB, MiB or GiB
	// suffix. A rate is a Bytes param: bytes per second.
	Bytes
	Duration
	Bool
	String
)

func (k Kind) String() string {
	return [...]string{"int", "float", "bytes", "duration", "bool", "string"}[k]
}

// Param is a scenario's parameter. --set name=value overrides Default.
type Param struct {
	Name    string
	Kind    Kind
	Default string
	Doc     string
}

// Params are a run's effective parameters: the declared defaults with the
// --set overrides, every value checked. The accessors panic on a name the
// scenario didn't declare: that's a bug in the scenario.
type Params struct {
	defs map[string]Param
	vals map[string]string
}

// resolve checks sets against defs and returns the effective params.
func resolve(defs []Param, sets map[string]string) (Params, error) {
	p := Params{defs: map[string]Param{}, vals: map[string]string{}}
	for _, d := range defs {
		p.defs[d.Name] = d
		p.vals[d.Name] = d.Default
	}
	for k, v := range sets {
		if _, ok := p.defs[k]; !ok {
			names := make([]string, 0, len(p.defs))
			for n := range p.defs {
				names = append(names, n)
			}
			slices.Sort(names)
			return Params{}, fmt.Errorf("unknown param %q (the scenario's params: %s)", k, strings.Join(names, ", "))
		}
		p.vals[k] = v
	}
	for name, v := range p.vals {
		if err := check(p.defs[name].Kind, v); err != nil {
			return Params{}, fmt.Errorf("param %s=%q: %w", name, v, err)
		}
	}
	return p, nil
}

func check(k Kind, v string) error {
	var err error
	switch k {
	case Int:
		_, err = strconv.ParseInt(v, 10, 64)
	case Float:
		_, err = strconv.ParseFloat(v, 64)
	case Bytes:
		_, err = ParseBytes(v)
	case Duration:
		_, err = time.ParseDuration(v)
	case Bool:
		_, err = strconv.ParseBool(v)
	}
	if err != nil {
		return fmt.Errorf("not a %s: %w", k, err)
	}
	return nil
}

func (p Params) get(name string, k Kind) string {
	d, ok := p.defs[name]
	if !ok || d.Kind != k {
		panic(fmt.Sprintf("scenario: param %q isn't a declared %s", name, k))
	}
	return p.vals[name]
}

// Int is an Int param.
func (p Params) Int(name string) int64 {
	v, _ := strconv.ParseInt(p.get(name, Int), 10, 64)
	return v
}

// Float is a Float param.
func (p Params) Float(name string) float64 {
	v, _ := strconv.ParseFloat(p.get(name, Float), 64)
	return v
}

// Bytes is a Bytes param.
func (p Params) Bytes(name string) int64 {
	v, _ := ParseBytes(p.get(name, Bytes))
	return v
}

// Duration is a Duration param.
func (p Params) Duration(name string) time.Duration {
	v, _ := time.ParseDuration(p.get(name, Duration))
	return v
}

// Bool is a Bool param.
func (p Params) Bool(name string) bool {
	v, _ := strconv.ParseBool(p.get(name, Bool))
	return v
}

// String is a String param.
func (p Params) String(name string) string { return p.get(name, String) }

// Values are the effective values, by name.
func (p Params) Values() map[string]string {
	out := make(map[string]string, len(p.vals))
	for k, v := range p.vals {
		out[k] = v
	}
	return out
}

var byteUnits = []struct {
	suffix string
	mult   int64
}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"B", 1}}

// ParseBytes parses a size: "96MiB", "512KiB", "1GiB", "300B" or "4096".
func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	mult := int64(1)
	for _, u := range byteUnits {
		if strings.HasSuffix(s, u.suffix) {
			s, mult = strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), u.mult
			break
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("size %q: want a number with an optional B, KiB, MiB or GiB", s)
	}
	return int64(f * float64(mult)), nil
}

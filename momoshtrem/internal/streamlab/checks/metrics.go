package checks

import (
	"math"

	"github.com/prometheus/client_golang/prometheus"
)

// LabelValue is the counter or gauge called name whose label equals
// value, summed over its other labels: priority_updates_total{type=
// "initial"}, say. 0 when it has none yet, NaN when the registry can't be
// read.
func LabelValue(g prometheus.Gatherer, name, label, value string) float64 {
	mfs, err := g.Gather()
	if err != nil {
		return math.NaN()
	}
	var sum float64
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == label && lp.GetValue() == value {
					sum += m.GetGauge().GetValue() + m.GetCounter().GetValue()
					break
				}
			}
		}
	}
	return sum
}

// Histogram is the histogram called name's sum and count, over its
// labels: (0, 0) when it has no observation, NaN sum when the registry
// can't be read.
func Histogram(g prometheus.Gatherer, name string) (sum float64, count uint64) {
	mfs, err := g.Gather()
	if err != nil {
		return math.NaN(), 0
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if h := m.GetHistogram(); h != nil {
				sum += h.GetSampleSum()
				count += h.GetSampleCount()
			}
		}
	}
	return sum, count
}

package compare

import (
	"math"
	"math/rand/v2"
	"testing"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestMannWhitney(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b []float64
		want float64
		tol  float64
	}{
		// Complete separation of 5 and 5: 2 of C(10,5) = 252 arrangements.
		{"separated", []float64{1, 2, 3, 4, 5}, []float64{6, 7, 8, 9, 10}, 2.0 / 252, 1e-12},
		{"separated, reversed", []float64{6, 7, 8, 9, 10}, []float64{1, 2, 3, 4, 5}, 2.0 / 252, 1e-12},
		// Interleaved evenly: no evidence at all.
		{"interleaved", []float64{1, 3, 5, 7, 9}, []float64{2, 4, 6, 8, 10}, 1, 0.31},
		{"all equal", []float64{3, 3, 3}, []float64{3, 3, 3}, 1, 0},
		{"empty", nil, []float64{1}, 1, 0},
		// 3 and 3 can't reach 0.05: the smallest p is 2/20.
		{"three a side", []float64{1, 2, 3}, []float64{4, 5, 6}, 0.1, 1e-12},
		// Ties: zeros against ones, every rep; exact over the midranks.
		{"ties separated", []float64{0, 0, 0, 0, 0}, []float64{1, 1, 1, 1, 1}, 2.0 / 252, 1e-12},
	} {
		got := MannWhitney(tc.a, tc.b)
		if !near(got, tc.want, tc.tol) {
			t.Errorf("%s: p = %.6f, want %.6f ± %g", tc.name, got, tc.want, tc.tol)
		}
	}
	// The normal approximation past the exact limit: 20 against 20
	// separated is far below 0.001, and its symmetric twin agrees.
	var a, b []float64
	for i := range 20 {
		a = append(a, float64(i))
		b = append(b, float64(i+20))
	}
	if p := MannWhitney(a, b); p > 1e-6 || !near(p, MannWhitney(b, a), 1e-12) {
		t.Errorf("20 vs 20 separated: p = %g", p)
	}
	// Overlapping heavily: well above 0.05.
	if p := MannWhitney(a, append(a[2:], 100, 101)); p < 0.3 {
		t.Errorf("overlapping: p = %g", p)
	}
}

// The dynamic program agrees with brute force over every split, ties
// included, both tails; the one-sided tail is the screen's direction.
func TestMannWhitneyExact(t *testing.T) {
	brute := func(a, b []float64, greater bool) float64 {
		all := append(append([]float64{}, a...), b...)
		ranks := midranks(all)
		n, k := len(all), len(b)
		obs, mean := 0.0, float64(k)*float64(n+1)/2
		for i := len(a); i < n; i++ {
			obs += ranks[i]
		}
		total, extreme := 0, 0
		var pick func(start, left int, sum float64)
		pick = func(start, left int, sum float64) {
			if left == 0 {
				total++
				if (greater && sum >= obs-1e-9) || (!greater && math.Abs(sum-mean) >= math.Abs(obs-mean)-1e-9) {
					extreme++
				}
				return
			}
			for i := start; i <= n-left; i++ {
				pick(i+1, left-1, sum+ranks[i])
			}
		}
		pick(0, k, 0)
		return float64(extreme) / float64(total)
	}
	rng := rand.New(rand.NewPCG(7, 9))
	for range 200 {
		n1, n2 := 2+rng.IntN(6), 2+rng.IntN(6)
		a, b := make([]float64, n1), make([]float64, n2)
		for i := range a {
			a[i] = float64(rng.IntN(6)) // few values: many ties
		}
		for i := range b {
			b[i] = float64(rng.IntN(6))
		}
		if allEqual(append(append([]float64{}, a...), b...)) {
			continue
		}
		if got, want := MannWhitney(a, b), brute(a, b, false); !near(got, want, 1e-12) {
			t.Fatalf("two-sided %v %v: %g, brute %g", a, b, got, want)
		}
		if got, want := MannWhitneyGreater(a, b), brute(a, b, true); !near(got, want, 1e-12) {
			t.Fatalf("one-sided %v %v: %g, brute %g", a, b, got, want)
		}
	}
	sep := []float64{1, 2, 3, 4, 5}
	hi := []float64{6, 7, 8, 9, 10}
	if p := MannWhitneyGreater(sep, hi); !near(p, 1.0/252, 1e-12) {
		t.Errorf("separated, the right way: %g", p)
	}
	if p := MannWhitneyGreater(hi, sep); p < 0.99 {
		t.Errorf("separated, the wrong way: %g", p)
	}
	// Past exactMax, the normal approximation: close to the exact tail.
	var a, b []float64
	for i := range 31 {
		a = append(a, float64(i))
		b = append(b, float64(i)+4)
	}
	if p, two := MannWhitneyGreater(a, b), MannWhitney(a, b); p <= 0.02 || p >= 0.1 || !near(p, two/2, 0.002) {
		t.Errorf("31 v 31 shifted a little: one-sided %g, two-sided %g", p, two)
	}
}

func TestFisherGreater(t *testing.T) {
	for _, tc := range []struct {
		bk, bn, hk, hn int
		want           float64
	}{
		{0, 5, 5, 5, 1.0 / 252},
		// P(X ≥ 2) of 3 events among 5+5: (C(5,2)C(5,1) + C(5,3)) / C(10,3).
		{1, 5, 2, 5, 60.0 / 120},
		{0, 5, 0, 5, 1},
		{5, 5, 0, 5, 1},
		{0, 0, 2, 2, 1},
	} {
		if got := FisherGreater(tc.bk, tc.bn, tc.hk, tc.hn); !near(got, tc.want, 1e-9) {
			t.Errorf("FisherGreater(%d/%d, %d/%d) = %.6f, want %.6f", tc.bk, tc.bn, tc.hk, tc.hn, got, tc.want)
		}
	}
}

func TestHodgesLehmann(t *testing.T) {
	if got := HodgesLehmann([]float64{1, 2, 3}, []float64{11, 12, 13}); got != 10 {
		t.Errorf("a shift of 10: %g", got)
	}
	// Two clusters mixed alike: the medians are 1900 apart, the shift isn't.
	if got := HodgesLehmann([]float64{100, 2000, 110, 2100, 105}, []float64{2000, 120, 2050, 115, 2200}); got > 100 {
		t.Errorf("two clusters: %g", got)
	}
	if !math.IsNaN(HodgesLehmann(nil, []float64{1})) {
		t.Error("an empty side")
	}
}

func TestCV(t *testing.T) {
	if cv := CV([]float64{1, 2, 3}); !near(cv, 0.5, 1e-12) {
		t.Errorf("CV(1,2,3) = %g", cv)
	}
	for _, v := range [][]float64{{4, 4, 4}, {0, 0}, {7}, nil} {
		if cv := CV(v); cv != 0 {
			t.Errorf("CV(%v) = %g, want 0", v, cv)
		}
	}
}

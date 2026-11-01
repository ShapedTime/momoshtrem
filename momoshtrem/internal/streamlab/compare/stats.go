package compare

import (
	"math"
	"slices"
)

// exactMax is the most values, both sides together, whose rank-sum
// distribution MannWhitney computes exactly (a dynamic program over the
// doubled midranks: about n²·n/2 steps); past it, the normal
// approximation is close enough.
const exactMax = 60

// MannWhitney is the two-sided p-value of the Mann-Whitney U test: how
// likely ranks at least as far apart as a's and b's are when both come
// from one distribution. Ties take their midrank. Up to exactMax values
// it's the exact permutation distribution, ties included; past it the
// normal approximation with the tie and continuity corrections. 1 when
// either side is empty or every value is equal.
func MannWhitney(a, b []float64) float64 {
	return mannWhitney(a, b, twoSided)
}

// MannWhitneyGreater is the one-sided p-value that b's values tend to be
// greater than a's: how likely b's rank sum is at least what it is when
// both come from one distribution. MannWhitneyGreater(b, a) is the other
// way.
func MannWhitneyGreater(a, b []float64) float64 {
	return mannWhitney(a, b, bGreater)
}

type tail int

const (
	twoSided tail = iota
	bGreater
)

func mannWhitney(a, b []float64, t tail) float64 {
	n1, n2 := len(a), len(b)
	if n1 == 0 || n2 == 0 {
		return 1
	}
	all := append(slices.Clone(a), b...)
	if allEqual(all) {
		return 1
	}
	ranks := midranks(all)
	n := n1 + n2
	// b's rank sum, doubled so midranks are integers.
	doubled := make([]int, n)
	obs := 0
	for i, r := range ranks {
		doubled[i] = int(math.Round(2 * r))
		if i >= n1 {
			obs += doubled[i]
		}
	}
	mean := n2 * (n + 1) // doubled: n2(n+1)/2 × 2
	if n <= exactMax {
		dist := rankSumDist(doubled, n2)
		total, extreme := 0.0, 0.0
		dev := abs(obs - mean)
		for s, c := range dist {
			if c == 0 {
				continue
			}
			total += c
			switch t {
			case twoSided:
				if abs(s-mean) >= dev {
					extreme += c
				}
			case bGreater:
				if s >= obs {
					extreme += c
				}
			}
		}
		return math.Min(1, extreme/total)
	}
	// The normal approximation of the rank sum, ties corrected.
	ties := 0.0
	for _, g := range tieGroups(all) {
		ties += float64(g*g*g - g)
	}
	nf := float64(n)
	variance := float64(n1) * float64(n2) / 12 * ((nf + 1) - ties/(nf*(nf-1)))
	if variance <= 0 {
		return 1
	}
	d := float64(obs-mean) / 2 // back to rank-sum units
	switch t {
	case bGreater:
		z := (d - 0.5) / math.Sqrt(variance)
		return math.Min(1, 0.5*math.Erfc(z/math.Sqrt2))
	}
	z := (math.Abs(d) - 0.5) / math.Sqrt(variance)
	if z < 0 {
		return 1
	}
	return math.Min(1, math.Erfc(z/math.Sqrt2))
}

// rankSumDist is the exact null distribution of the sum of k of the
// doubled ranks: dist[s] is how many of the C(n, k) picks sum to s.
func rankSumDist(doubled []int, k int) []float64 {
	total := 0
	for _, r := range doubled {
		total += r
	}
	dp := make([][]float64, k+1)
	for j := range dp {
		dp[j] = make([]float64, total+1)
	}
	dp[0][0] = 1
	for i, r := range doubled {
		for j := min(k, i+1); j >= 1; j-- {
			prev, cur := dp[j-1], dp[j]
			for s := total; s >= r; s-- {
				cur[s] += prev[s-r]
			}
		}
	}
	return dp[k]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// midranks ranks values from 1, ties sharing their mean rank.
func midranks(values []float64) []float64 {
	order := make([]int, len(values))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(x, y int) int {
		switch {
		case values[x] < values[y]:
			return -1
		case values[x] > values[y]:
			return 1
		}
		return 0
	})
	ranks := make([]float64, len(values))
	for i := 0; i < len(order); {
		j := i
		for j+1 < len(order) && values[order[j+1]] == values[order[i]] {
			j++
		}
		r := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			ranks[order[k]] = r
		}
		i = j + 1
	}
	return ranks
}

// tieGroups are the sizes of the groups of equal values.
func tieGroups(values []float64) []int {
	s := slices.Clone(values)
	slices.Sort(s)
	var out []int
	for i := 0; i < len(s); {
		j := i
		for j+1 < len(s) && s[j+1] == s[i] {
			j++
		}
		if j > i {
			out = append(out, j-i+1)
		}
		i = j + 1
	}
	return out
}

func allEqual(values []float64) bool {
	for _, v := range values[1:] {
		if v != values[0] {
			return false
		}
	}
	return true
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

// HodgesLehmann is the shift from a to b that goes with the Mann-Whitney
// test: the median of every difference b[j] − a[i]. Unlike the difference
// of the medians, a few slow reps on either side barely move it, so a
// metric whose reps fall in two clusters (a cold start that connects at
// once or a second later) doesn't look shifted when both sides mix them
// alike. NaN when a side is empty.
func HodgesLehmann(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return math.NaN()
	}
	d := make([]float64, 0, len(a)*len(b))
	for _, x := range a {
		for _, y := range b {
			d = append(d, y-x)
		}
	}
	slices.Sort(d)
	n := len(d)
	if n%2 == 1 {
		return d[n/2]
	}
	return (d[n/2-1] + d[n/2]) / 2
}

// FisherGreater is the one-sided p-value of Fisher's exact test that
// head's rate (headK of headN) is greater than base's (baseK of baseN):
// how likely head gets headK or more of the baseK+headK events when both
// sides share one rate.
func FisherGreater(baseK, baseN, headK, headN int) float64 {
	k := baseK + headK
	n := baseN + headN
	if headN == 0 || k == 0 {
		return 1
	}
	p := 0.0
	for x := headK; x <= min(k, headN); x++ {
		p += math.Exp(logChoose(headN, x) + logChoose(baseN, k-x) - logChoose(n, k))
	}
	return math.Min(1, p)
}

func logChoose(n, k int) float64 {
	if k < 0 || k > n {
		return math.Inf(-1)
	}
	a, _ := math.Lgamma(float64(n + 1))
	b, _ := math.Lgamma(float64(k + 1))
	c, _ := math.Lgamma(float64(n - k + 1))
	return a - b - c
}

// CV is the coefficient of variation of values: the sample standard
// deviation over the mean. 0 for fewer than two values or all equal; 0
// for a zero mean of nonnegative values (all zero).
func CV(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	if mean == 0 {
		return 0
	}
	ss := 0.0
	for _, v := range values {
		ss += (v - mean) * (v - mean)
	}
	return math.Sqrt(ss/float64(len(values)-1)) / math.Abs(mean)
}

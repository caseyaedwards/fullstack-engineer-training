// Package aiassisted holds the example for the AI-Assisted Development
// track: a percentile function whose tests came before its code.
package aiassisted

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// region: percentile

// Percentile returns the p-th percentile of values, for p from 0 to
// 100. It interpolates linearly between the two closest ranks, the
// method that NumPy uses by default and Excel calls PERCENTILE.INC:
// p 0 is the minimum, p 100 the maximum, and p 50 of 10, 20, 30, 40
// is 25. It returns an error for no values or for a p outside 0 to
// 100, NaN included, and it never changes values.
func Percentile(values []float64, p float64) (float64, error) {
	if len(values) == 0 {
		return 0, errors.New("percentile of no values")
	}
	// Every comparison with NaN is false: this form rejects NaN,
	// and p < 0 || p > 100 lets it through.
	if !(p >= 0 && p <= 100) {
		return 0, fmt.Errorf("percentile %v is outside 0 to 100", p)
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	rank := p / 100 * float64(len(sorted)-1)
	lo, hi := int(math.Floor(rank)), int(math.Ceil(rank))
	return sorted[lo] + (rank-float64(lo))*(sorted[hi]-sorted[lo]), nil
}

// endregion: percentile

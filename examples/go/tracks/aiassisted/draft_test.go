package aiassisted

import (
	"slices"
	"sort"
	"testing"
)

// region: draft

// draftPercentile is a first draft of the shape an assistant returns for the
// prompt "write a Go function that returns the p-th percentile of a
// slice of float64". It compiles, and it answers 3 for the median of
// 1, 2, 3, 4, 5.
func draftPercentile(values []float64, p float64) (float64, error) {
	sort.Float64s(values)
	i := int(p / 100 * float64(len(values)))
	return values[i], nil
}

// endregion: draft

// region: caught

// TestDraftFailsTheCases runs the cases written before any code
// against the draft. Seven of the nine fail, each for a reason a
// reviewer can name.
func TestDraftFailsTheCases(t *testing.T) {
	var failed []string
	for _, c := range edgeCases {
		if problem := c.check(draftPercentile); problem != "" {
			failed = append(failed, c.name)
			t.Logf("draft: %s: %s", c.name, problem)
		}
	}
	want := []string{
		"no values is an error",
		"p below 0 is an error",
		"p above 100 is an error",
		"p NaN is an error",
		"p 100 is the maximum",
		"p 50 of an even count interpolates",
		"the input stays unsorted",
	}
	if !slices.Equal(failed, want) {
		t.Errorf("the draft fails %q, want %q", failed, want)
	}
}

// endregion: caught

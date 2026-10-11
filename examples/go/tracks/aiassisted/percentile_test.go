package aiassisted

import (
	"fmt"
	"math"
	"slices"
	"testing"
)

// percentileFunc is the signature under test. The same cases run
// against Percentile and against the draft in draft_test.go.
type percentileFunc func(values []float64, p float64) (float64, error)

// region: run

// run calls f and turns a panic into a problem description, which
// lets a case report a crash as a failure instead of ending the test.
func run(f percentileFunc, values []float64, p float64) (got float64, err error, problem string) {
	defer func() {
		if r := recover(); r != nil {
			problem = fmt.Sprintf("panics: %v", r)
		}
	}()
	got, err = f(values, p)
	return got, err, ""
}

// endregion: run

// region: cases

// edgeCases come first, before any implementation exists. Each check
// returns what went wrong, or "" when f passes.
var edgeCases = []struct {
	name  string
	check func(f percentileFunc) string
}{
	{"no values is an error", func(f percentileFunc) string {
		return wantError(f, nil, 50)
	}},
	{"p below 0 is an error", func(f percentileFunc) string {
		return wantError(f, []float64{1, 2, 3}, -1)
	}},
	{"p above 100 is an error", func(f percentileFunc) string {
		return wantError(f, []float64{1, 2, 3}, 101)
	}},
	{"p NaN is an error", func(f percentileFunc) string {
		return wantError(f, []float64{1, 2, 3}, math.NaN())
	}},
	{"p 0 is the minimum", func(f percentileFunc) string {
		return wantValue(f, []float64{30, 10, 20}, 0, 10)
	}},
	{"p 100 is the maximum", func(f percentileFunc) string {
		return wantValue(f, []float64{30, 10, 20}, 100, 30)
	}},
	{"p 50 of an even count interpolates", func(f percentileFunc) string {
		return wantValue(f, []float64{40, 10, 30, 20}, 50, 25)
	}},
	{"one value is every percentile", func(f percentileFunc) string {
		return wantValue(f, []float64{7}, 90, 7)
	}},
	{"the input stays unsorted", func(f percentileFunc) string {
		values := []float64{30, 10, 20}
		if _, _, problem := run(f, values, 50); problem != "" {
			return problem
		}
		if !slices.Equal(values, []float64{30, 10, 20}) {
			return fmt.Sprintf("input changed to %v", values)
		}
		return ""
	}},
}

// endregion: cases

// region: test

func TestPercentile(t *testing.T) {
	for _, c := range edgeCases {
		t.Run(c.name, func(t *testing.T) {
			if problem := c.check(Percentile); problem != "" {
				t.Error(problem)
			}
		})
	}
}

// endregion: test

func wantError(f percentileFunc, values []float64, p float64) string {
	got, err, problem := run(f, values, p)
	if problem != "" {
		return problem
	}
	if err == nil {
		return fmt.Sprintf("p %v: want an error, got %v and nil", p, got)
	}
	return ""
}

func wantValue(f percentileFunc, values []float64, p, want float64) string {
	got, err, problem := run(f, values, p)
	if problem != "" {
		return problem
	}
	if err != nil {
		return fmt.Sprintf("p %v: unexpected error %v", p, err)
	}
	if math.Abs(got-want) > 1e-9 {
		return fmt.Sprintf("p %v of %v: got %v, want %v", p, values, got, want)
	}
	return ""
}

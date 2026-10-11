package sre

import (
	"math"
	"testing"
	"time"
)

// 1 - 0.999 is 0.0010000000000000009 in float64, not 0.001, which puts every
// ratio a few units in the last place off the decimal answer. The tests
// compare ratios within 1e-9.
func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

const month = 30 * 24 * time.Hour

// region: tests
func TestErrorBudget(t *testing.T) {
	tests := []struct {
		slo    float64
		window time.Duration
		want   time.Duration
	}{
		{0.999, month, 43*time.Minute + 12*time.Second},
		{0.995, month, 3*time.Hour + 36*time.Minute}, // Week 18 availability
		{0.99, month, 7*time.Hour + 12*time.Minute},  // Week 18 freshness
		{0.9999, month, 4*time.Minute + 19*time.Second + 200*time.Millisecond},
		{1, month, 0},
	}
	for _, tt := range tests {
		if got := ErrorBudget(tt.slo, tt.window); got != tt.want {
			t.Errorf("ErrorBudget(%v, %v) = %v, want %v", tt.slo, tt.window, got, tt.want)
		}
	}
}

func TestBurnRate(t *testing.T) {
	tests := []struct {
		slo, errorRatio, want float64
	}{
		{0.999, 0.0144, 14.4}, // the fast-burn page threshold
		{0.999, 0.001, 1},     // spends the budget in exactly the window
		{0.995, 0.036, 7.2},   // Week 18: 3.6% errors against a 0.5% budget
		{0.995, 1, 200},       // total outage, the highest burn rate there is
		{0.999, 0, 0},
		{1, 0, 0},
	}
	for _, tt := range tests {
		if got := BurnRate(tt.slo, tt.errorRatio); !near(got, tt.want) {
			t.Errorf("BurnRate(%v, %v) = %v, want %v", tt.slo, tt.errorRatio, got, tt.want)
		}
	}
	if got := BurnRate(1, 0.0001); !math.IsInf(got, 1) {
		t.Errorf("BurnRate(1, 0.0001) = %v, want +Inf: a 100%% target has no budget", got)
	}
}

func TestBudgetLeft(t *testing.T) {
	tests := []struct {
		slo         float64
		good, total int
		want        float64
	}{
		{0.999, 99_950, 100_000, 0.5},
		{0.999, 100_000, 100_000, 1},
		{0.999, 99_900, 100_000, 0},
		{0.999, 99_800, 100_000, -1}, // overspent: two budgets gone
		{0.995, 0, 0, 1},             // no valid events spend nothing
	}
	for _, tt := range tests {
		if got := BudgetLeft(tt.slo, tt.good, tt.total); !near(got, tt.want) {
			t.Errorf("BudgetLeft(%v, %d, %d) = %v, want %v", tt.slo, tt.good, tt.total, got, tt.want)
		}
	}
}

// endregion: tests

// region: spent-test
// The alert table of the SRE workbook, chapter Alerting on SLOs: each
// threshold fires once this share of a 30-day budget is gone.
func TestBudgetSpent(t *testing.T) {
	tests := []struct {
		burnRate float64
		long     time.Duration
		want     float64
	}{
		{14.4, time.Hour, 0.02},       // page
		{6, 6 * time.Hour, 0.05},      // page
		{1, 3 * 24 * time.Hour, 0.10}, // ticket
	}
	for _, tt := range tests {
		if got := BudgetSpent(tt.burnRate, tt.long, month); !near(got, tt.want) {
			t.Errorf("BudgetSpent(%v, %v) = %v, want %v", tt.burnRate, tt.long, got, tt.want)
		}
	}
}

// endregion: spent-test

// Package sre holds the error budget arithmetic behind an SLO and its
// burn-rate alerts.
package sre

import (
	"math"
	"time"
)

// region: budget

// ErrorBudget returns how much of window an SLO allows to fail: the window
// times 1 - slo. A 99.9% SLO over 30 days allows 43m12s.
func ErrorBudget(slo float64, window time.Duration) time.Duration {
	budget := max(1-slo, 0)
	return time.Duration(math.Round(float64(window) * budget))
}

// endregion: budget

// region: burn

// BurnRate returns how fast an error ratio spends the budget of slo,
// relative to the rate that spends it in exactly the SLO window. At 1, the
// budget lasts the window. At 14.4, it lasts 1/14.4 of it.
func BurnRate(slo, errorRatio float64) float64 {
	budget := 1 - slo
	if budget <= 0 {
		if errorRatio > 0 {
			return math.Inf(1) // a 100% target has no budget to spend
		}
		return 0
	}
	return errorRatio / budget
}

// endregion: burn

// region: left

// BudgetLeft returns the share of the budget still unspent after good of
// total valid events succeeded: 1 is untouched, 0 is spent, below 0 is
// overspent.
func BudgetLeft(slo float64, good, total int) float64 {
	if total == 0 {
		return 1
	}
	errorRatio := float64(total-good) / float64(total)
	return 1 - BurnRate(slo, errorRatio)
}

// endregion: left

// region: spent

// BudgetSpent returns the share of the budget for sloWindow that a constant
// burn rate spends over alertWindow.
func BudgetSpent(burnRate float64, alertWindow, sloWindow time.Duration) float64 {
	return burnRate * float64(alertWindow) / float64(sloWindow)
}

// endregion: spent

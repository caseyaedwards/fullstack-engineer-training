package alert

import (
	"fmt"

	"fsp/examples/week11/your-monitor/internal/monitor"
)

// region: rule

// Action is what the alerter does for one monitor in one round.
type Action int

const (
	Nothing Action = iota
	Open
	Resolve
)

// Decide applies the alert rule to the newest results of one monitor,
// newest first. With no incident open, threshold failed results in a row
// open one. With an incident open, the next result that is up resolves it.
// Fewer results than threshold never open an incident: a new monitor
// needs time to earn one.
func Decide(recent []monitor.Result, open bool, threshold int) Action {
	if len(recent) == 0 {
		return Nothing
	}
	if open {
		if recent[0].Up {
			return Resolve
		}
		return Nothing
	}
	if len(recent) < threshold {
		return Nothing
	}
	for _, r := range recent[:threshold] {
		if r.Up {
			return Nothing
		}
	}
	return Open
}

// Cause describes why a result counts as a failure, for the incident.
func Cause(r monitor.Result) string {
	if r.Error != "" {
		return r.Error
	}
	return fmt.Sprintf("status %d", r.StatusCode)
}

// endregion: rule

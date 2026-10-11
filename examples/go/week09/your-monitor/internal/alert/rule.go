package alert

import (
	"fmt"

	"fsp/examples/week09/your-monitor/internal/monitor"
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

// FirstUp returns the oldest result of the run of successes at the start
// of recent, newest first: the first successful check after the failures,
// as far back as recent reaches. When the alerter falls behind, more than
// one success can wait for it, and the incident ends at the first.
func FirstUp(recent []monitor.Result) monitor.Result {
	i := 0
	for i+1 < len(recent) && recent[i+1].Up {
		i++
	}
	return recent[i]
}

// Cause describes why a result counts as a failure, for the incident.
func Cause(r monitor.Result) string {
	switch {
	case r.Error != nil:
		return *r.Error
	case r.StatusCode != nil:
		return fmt.Sprintf("status %d", *r.StatusCode)
	default:
		return "down"
	}
}

// endregion: rule

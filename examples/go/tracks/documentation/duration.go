// Package docs holds the example for the Documentation track. Its
// folder is documentation, but the go command ignores every file whose
// package clause says documentation, which is why the package is docs.
package docs

import (
	"strings"
	"time"
)

// region: format

// FormatDuration formats d for a log line or the status page: rounded
// to the millisecond, or to the microsecond below one millisecond, and
// written the way [time.Duration.String] writes it, without the zero
// units at the end. 1500 milliseconds gives "1.5s", 90 seconds gives
// "1m30s" and two minutes gives "2m" instead of "2m0s". Zero gives
// "0s", and a negative duration keeps its minus sign.
func FormatDuration(d time.Duration) string {
	unit := time.Millisecond
	if d > -time.Millisecond && d < time.Millisecond {
		unit = time.Microsecond
	}
	s := d.Round(unit).String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// endregion: format

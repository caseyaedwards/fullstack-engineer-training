// Package programming sorts the errors a check returns into classes,
// with errors.Is for a sentinel value and errors.As for an error type.
package programming

import (
	"errors"
	"fmt"
)

// region: types

// ErrTimeout is a sentinel error: one value that callers compare against.
var ErrTimeout = errors.New("timeout")

// StatusError is an error type: it carries the HTTP status code
// of a response that the check counts as a failure. Error has a
// pointer receiver, which makes *StatusError the error, and a plain
// StatusError value does not compile where an error goes.
type StatusError struct {
	Code int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("HTTP status %d", e.Code)
}

// endregion: types

// region: classify

// Classify names the class of err for a log line or a metric label:
// ok, timeout, http-4xx, http-5xx or other. It looks through every
// layer of wrapping that fmt.Errorf with %w adds.
func Classify(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, ErrTimeout) {
		return "timeout"
	}
	var se *StatusError
	if errors.As(err, &se) {
		switch {
		case se.Code >= 500 && se.Code <= 599:
			return "http-5xx"
		case se.Code >= 400 && se.Code <= 499:
			return "http-4xx"
		}
	}
	return "other"
}

// endregion: classify

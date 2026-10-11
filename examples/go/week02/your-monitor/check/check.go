// region: imports

// Package check checks whether web sites answer, and reads the list of
// URLs to check from a file.
package check

import (
	"fmt"
	"net/http"
	"time"
)

// endregion: imports

// region: getter

// Getter sends a GET request. *http.Client has this method, so main
// passes a real client. Tests pass a fake.
type Getter interface {
	Get(url string) (*http.Response, error)
}

// endregion: getter

// region: result

// Result is the outcome of checking one URL.
type Result struct {
	URL     string
	Status  int           // 0 when no response arrived
	Latency time.Duration // from sending the request to the headers arriving
	Err     error         // nil when a response arrived
}

// Up reports whether a response arrived with a status code below 400.
func (r Result) Up() bool {
	return r.Err == nil && r.Status < 400
}

// String formats r as one line of key=value pairs.
func (r Result) String() string {
	ms := r.Latency.Milliseconds()
	switch {
	case r.Err != nil:
		return fmt.Sprintf("down url=%s latency_ms=%d error=%q", r.URL, ms, r.Err)
	case r.Status >= 400:
		return fmt.Sprintf("down url=%s status=%d latency_ms=%d", r.URL, r.Status, ms)
	default:
		return fmt.Sprintf("up url=%s status=%d latency_ms=%d", r.URL, r.Status, ms)
	}
}

// endregion: result

// region: url

// URL sends a GET request to target with client and reports the status
// code and the latency. Result.Err is not nil when no response arrives.
func URL(client Getter, target string) Result {
	start := time.Now()
	resp, err := client.Get(target)
	latency := time.Since(start)
	if err != nil {
		return Result{URL: target, Latency: latency, Err: err}
	}
	defer resp.Body.Close()
	return Result{URL: target, Status: resp.StatusCode, Latency: latency}
}

// endregion: url

// region: testsite

// Command testsite is a local web site for trying the monitor against
// many URLs. Paths under /slow/ answer only after -slow, paths under
// /fail/ answer 500, and every other path answers 200. Once a second, it
// logs how many requests arrived and the most it held at once.
//
//	go run ./cmd/testsite -addr 127.0.0.1:8000
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8000", "address to listen on")
	slow := flag.Duration("slow", 10*time.Second, "how long /slow/ paths take to answer")
	flag.Parse()
	log.SetOutput(os.Stdout)

	var site stats
	go site.report(time.Second)
	log.Printf("listening on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, site.handler(*slow)))
}

// stats counts requests. Handlers run on many goroutines at once, which is
// why every counter is atomic.
type stats struct {
	requests, inFlight, peak atomic.Int64
}

func (s *stats) handler(slow time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		n := s.inFlight.Add(1)
		defer s.inFlight.Add(-1)
		for {
			p := s.peak.Load()
			if n <= p || s.peak.CompareAndSwap(p, n) {
				break
			}
		}

		switch {
		case strings.HasPrefix(r.URL.Path, "/slow/"):
			select {
			case <-time.After(slow):
			case <-r.Context().Done(): // the client gave up
				return
			}
		case strings.HasPrefix(r.URL.Path, "/fail/"):
			http.Error(w, "fail", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok\n"))
	})
}

// report logs the requests of the last interval and the peak in flight,
// for every interval that had requests.
func (s *stats) report(interval time.Duration) {
	for range time.Tick(interval) {
		if n := s.requests.Swap(0); n > 0 {
			log.Printf("requests=%d peak_in_flight=%d", n, s.peak.Swap(s.inFlight.Load()))
		}
	}
}

// endregion: testsite

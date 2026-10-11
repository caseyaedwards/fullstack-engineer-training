// Command your-monitor checks one URL and prints its HTTP status code and
// latency. It exits with 0 when the site is up, 1 when it is down and 2
// when it is run without a URL. Run it from the examples/go directory:
//
//	go run ./week01/your-monitor -url https://example.com

// region: imports
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

// endregion: imports

// region: check

// checkURL sends a GET request to url and reports the status code and
// the latency: the time from sending the request until the response
// headers arrive. err is not nil when no response arrives at all.
func checkURL(client *http.Client, url string) (status int, latency time.Duration, err error) {
	start := time.Now()
	resp, err := client.Get(url)
	latency = time.Since(start)
	if err != nil {
		return 0, latency, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, latency, nil
}

// endregion: check

// region: main

func main() {
	url := flag.String("url", "", "URL to check, for example https://example.com")
	timeout := flag.Duration("timeout", 5*time.Second, "how long to wait for a response")
	flag.Parse()
	if *url == "" {
		fmt.Fprintln(os.Stderr, "missing -url")
		flag.Usage()
		os.Exit(2)
	}

	client := &http.Client{Timeout: *timeout}
	status, latency, err := checkURL(client, *url)
	if err != nil {
		fmt.Printf("down url=%s latency_ms=%d error=%q\n", *url, latency.Milliseconds(), err)
		os.Exit(1)
	}
	if status >= 400 {
		fmt.Printf("down url=%s status=%d latency_ms=%d\n", *url, status, latency.Milliseconds())
		os.Exit(1)
	}
	fmt.Printf("up url=%s status=%d latency_ms=%d\n", *url, status, latency.Milliseconds())
}

// endregion: main

// Command your-monitor checks every URL in a file, one after the other,
// and prints one line per URL. It exits with 0 when every site is up, 1
// when any site is down and 2 when the command or the file is wrong. Run
// it from the examples/go directory:
//
//	go run ./week02/your-monitor -file week02/your-monitor/urls.txt

// region: imports
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"fsp/examples/week02/your-monitor/check"
)

// endregion: imports

// region: main

func main() {
	file := flag.String("file", "", "file with one URL per line, for example urls.txt")
	timeout := flag.Duration("timeout", 5*time.Second, "how long to wait for each response")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "missing -file")
		flag.Usage()
		os.Exit(2)
	}

	targets, err := readList(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	client := &http.Client{Timeout: *timeout}
	down := 0
	for _, target := range targets {
		result := check.URL(client, target)
		fmt.Println(result)
		if !result.Up() {
			down++
		}
	}
	if down > 0 {
		os.Exit(1)
	}
}

// endregion: main

// region: read

// readList opens the file at path and parses it with check.ParseList.
func readList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	targets, err := check.ParseList(f)
	if err != nil {
		return nil, fmt.Errorf("%s:\n%w", path, err)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("%s: no URLs to check", path)
	}
	return targets, nil
}

// endregion: read

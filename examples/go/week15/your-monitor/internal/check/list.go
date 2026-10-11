// region: imports

package check

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// endregion: imports

// region: parse

// ParseList reads one URL per line from r. It skips blank lines and lines
// that start with #. When any line is not an http or https URL, it returns
// a nil slice and an error that names every such line by its number.
func ParseList(r io.Reader) ([]string, error) {
	var urls []string
	var errs []error
	scanner := bufio.NewScanner(r)
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !valid(line) {
			errs = append(errs, fmt.Errorf("line %d: %q is not an http or https URL", n, line))
			continue
		}
		urls = append(urls, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return urls, nil
}

// valid reports whether s parses as a URL with an http or https scheme
// and a host.
func valid(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// endregion: parse

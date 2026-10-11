package check_test

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"fsp/examples/week02/your-monitor/check"
)

// region: fuzz

func FuzzParseList(f *testing.F) {
	f.Add("https://example.com\n# comment\n\nhttp://example.org/health\n")
	f.Add("example.com\r\nftp://example.org\n")
	f.Fuzz(func(t *testing.T, input string) {
		urls, err := check.ParseList(strings.NewReader(input))
		if err != nil {
			return // invalid input is fine, as long as ParseList does not panic
		}
		for _, s := range urls {
			u, err := url.Parse(s)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				t.Errorf("ParseList accepted %q", s)
			}
		}
		// Writing the URLs back out, one per line, gives the same list.
		again, err := check.ParseList(strings.NewReader(strings.Join(urls, "\n")))
		if err != nil || !slices.Equal(again, urls) {
			t.Errorf("round trip: got %q, %v; want %q", again, err, urls)
		}
	})
}

// endregion: fuzz

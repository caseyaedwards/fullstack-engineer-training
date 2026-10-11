package check_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"fsp/examples/week02/your-monitor/check"
)

// region: helper

// assertError fails the test unless err matches want. An empty want
// means no error.
func assertError(t *testing.T, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Fatalf("unexpected error: %v", err)
	case want != "" && err == nil:
		t.Fatalf("got no error, want %q", want)
	case want != "" && err.Error() != want:
		t.Fatalf("error = %q\nwant    %q", err, want)
	}
}

// endregion: helper

// region: parse

func TestParseList(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr string
	}{
		{
			name:  "one url per line",
			input: "https://example.com\nhttp://example.org/health\n",
			want:  []string{"https://example.com", "http://example.org/health"},
		},
		{
			name:  "blank lines and comments",
			input: "# production\nhttps://example.com\n\n   \n  # staging\nhttps://example.org\n",
			want:  []string{"https://example.com", "https://example.org"},
		},
		{
			name:  "spaces around a url",
			input: "  https://example.com\t\n",
			want:  []string{"https://example.com"},
		},
		{
			name:  "no newline at the end",
			input: "https://example.com",
			want:  []string{"https://example.com"},
		},
		{
			name:  "windows line endings",
			input: "https://example.com\r\nhttps://example.org\r\n",
			want:  []string{"https://example.com", "https://example.org"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:    "missing scheme",
			input:   "https://example.com\nexample.org\n",
			wantErr: `line 2: "example.org" is not an http or https URL`,
		},
		{
			name:  "every bad line is reported",
			input: "ftp://example.com\n\nhttps://\nhttps://example.org # main site\n",
			wantErr: `line 1: "ftp://example.com" is not an http or https URL` + "\n" +
				`line 3: "https://" is not an http or https URL` + "\n" +
				`line 4: "https://example.org # main site" is not an http or https URL`,
		},
		{
			name:    "a line longer than the scanner buffer",
			input:   "https://example.com/" + strings.Repeat("a", 70_000) + "\n",
			wantErr: "bufio.Scanner: token too long",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := check.ParseList(strings.NewReader(tt.input))
			assertError(t, err, tt.wantErr)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q\nwant %q", got, tt.want)
			}
		})
	}
}

// endregion: parse

// region: file

func TestParseListReadsAFile(t *testing.T) {
	f, err := os.Open("testdata/urls.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	got, err := check.ParseList(f)
	assertError(t, err, "")
	want := []string{"https://example.com", "https://example.org/health"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

// endregion: file

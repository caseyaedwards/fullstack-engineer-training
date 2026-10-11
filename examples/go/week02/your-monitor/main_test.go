package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// server starts a test server that answers every request with status.
func server(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// writeFile writes content to a new file in a temporary folder and
// returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "urls.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestProgram builds the program and runs it the way the page does, to check
// what it prints and the exit code a shell sees.
func TestProgram(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "your-monitor")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	up := server(t, http.StatusOK)
	down := server(t, http.StatusInternalServerError)

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  []string // lines that must appear, in this order
	}{
		{
			name:     "every site up",
			args:     []string{"-file", writeFile(t, "# two sites\n"+up.URL+"\n\n"+up.URL+"/health\n")},
			wantCode: 0,
			wantOut:  []string{"up url=" + up.URL + " status=200 latency_ms=", "up url=" + up.URL + "/health status=200 latency_ms="},
		},
		{
			name:     "one site down",
			args:     []string{"-file", writeFile(t, up.URL+"\n"+down.URL+"\n")},
			wantCode: 1,
			wantOut:  []string{"up url=" + up.URL + " status=200", "down url=" + down.URL + " status=500"},
		},
		{
			name:     "invalid lines",
			args:     []string{"-file", writeFile(t, up.URL+"\nexample.org\n\nftp://example.net\n")},
			wantCode: 2,
			wantOut:  []string{"urls.txt:", `line 2: "example.org" is not an http or https URL`, `line 4: "ftp://example.net" is not an http or https URL`},
		},
		{
			name:     "only comments",
			args:     []string{"-file", writeFile(t, "# nothing yet\n")},
			wantCode: 2,
			wantOut:  []string{"urls.txt: no URLs to check"},
		},
		{
			name:     "no such file",
			args:     []string{"-file", "missing.txt"},
			wantCode: 2,
			wantOut:  []string{"open missing.txt: no such file or directory"},
		},
		{
			name:     "no file given",
			args:     nil,
			wantCode: 2,
			wantOut:  []string{"missing -file", "-file string"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := exec.Command(bin, tt.args...)
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()
			code := 0
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				code = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("run: %v", err)
			}
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\n%s", code, tt.wantCode, out.String())
			}
			rest := out.String()
			for _, want := range tt.wantOut {
				i := strings.Index(rest, want)
				if i < 0 {
					t.Fatalf("output = %q, want it to contain %q after the earlier lines", out.String(), want)
				}
				rest = rest[i+len(want):]
			}
			if tt.wantCode == 2 && strings.Contains(out.String(), "url=") {
				t.Errorf("output = %q, want no checks when the command or file is wrong", out.String())
			}
		})
	}
}

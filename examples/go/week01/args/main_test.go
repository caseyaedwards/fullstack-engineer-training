package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestArgs builds the program and runs it with and without a URL.
func TestArgs(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "your-monitor")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	out, err := exec.Command(bin, "https://example.com", "extra").CombinedOutput()
	if err != nil {
		t.Fatalf("run with a URL: %v\n%s", err, out)
	}
	want := "os.Args[0] = " + bin + "\n" +
		"os.Args[1] = https://example.com\n" +
		"os.Args[2] = extra\n" +
		"checking https://example.com\n"
	if string(out) != want {
		t.Errorf("output:\n%s\nwant:\n%s", out, want)
	}

	out, err = exec.Command(bin).CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Errorf("run without a URL: err = %v, want exit status 2", err)
	}
	if string(out) != "usage: your-monitor URL\n" {
		t.Errorf("run without a URL printed %q, want the usage line", out)
	}
}

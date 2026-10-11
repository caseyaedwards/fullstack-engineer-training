package automation

import (
	"os"
	"path/filepath"
	"testing"
)

// region: ensureline-test

// TestEnsureLineIsIdempotent runs EnsureLine twice with the same line. The
// first run changes the file and the second finds nothing to do, which is
// what lets you run it again and again without fear.
func TestEnsureLineIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chrony.conf")
	if err := os.WriteFile(path, []byte("driftfile /var/lib/chrony/chrony.drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const line = "makestep 1 3"

	for i, want := range []bool{true, false} {
		changed, err := EnsureLine(path, line)
		if err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
		if changed != want {
			t.Errorf("run %d: changed = %v, want %v", i+1, changed, want)
		}
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), "driftfile /var/lib/chrony/chrony.drift\nmakestep 1 3\n"; got != want {
		t.Errorf("file holds %q, want %q", got, want)
	}
}

// endregion: ensureline-test

// TestEnsureLineCases covers the edges: a missing file, a last line without
// a newline, and a line that only contains the one asked for.
func TestEnsureLineCases(t *testing.T) {
	tests := []struct {
		name    string
		before  *string // nil: the file does not exist
		line    string
		changed bool
		after   string
	}{
		{"missing file", nil, "rtcsync", true, "rtcsync\n"},
		{"empty file", ptr(""), "rtcsync", true, "rtcsync\n"},
		{"no final newline", ptr("makestep 1 3"), "rtcsync", true, "makestep 1 3\nrtcsync\n"},
		{"present in the middle", ptr("a\nrtcsync\nb\n"), "rtcsync", false, "a\nrtcsync\nb\n"},
		{"present without final newline", ptr("a\nrtcsync"), "rtcsync", false, "a\nrtcsync"},
		{"only a longer line", ptr("rtcsync off\n"), "rtcsync", true, "rtcsync off\nrtcsync\n"},
		{"windows line ending is a different line", ptr("rtcsync\r\n"), "rtcsync", true, "rtcsync\r\nrtcsync\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.conf")
			if tt.before != nil {
				if err := os.WriteFile(path, []byte(*tt.before), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			changed, err := EnsureLine(path, tt.line)
			if err != nil {
				t.Fatal(err)
			}
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tt.after {
				t.Errorf("file holds %q, want %q", b, tt.after)
			}
		})
	}
}

// TestEnsureLineKeepsTheMode checks that a rewrite keeps the permission
// bits of the file it replaces: a key file that is 0600 stays 0600.
func TestEnsureLineKeepsTheMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chrony.keys")
	if err := os.WriteFile(path, []byte("1 SHA256 HEX:00\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureLine(path, "2 SHA256 HEX:11"); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode after EnsureLine is %v, want -rw-------", fi.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".ensureline-*")); len(left) != 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
}

// TestEnsureLineRejectsBadLines refuses a line that holds a newline, which
// can never match a single line, and an empty line, which matches the empty
// element after the last newline of any file and is never written.
func TestEnsureLineRejectsBadLines(t *testing.T) {
	for _, line := range []string{"a\nb", "a\r", ""} {
		path := filepath.Join(t.TempDir(), "f.conf")
		if _, err := EnsureLine(path, line); err == nil {
			t.Errorf("EnsureLine accepts the line %q", line)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("EnsureLine(%q) created the file anyway: %v", line, err)
		}
	}
}

func ptr(s string) *string { return &s }

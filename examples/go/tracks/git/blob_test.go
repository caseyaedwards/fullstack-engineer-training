package git

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// region: test

func TestBlobHash(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"one line", "hello\n", "ce013625030ba8dba906f756967f9e9ca394464a"},
		{"empty file", "", "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BlobHash([]byte(tt.content)); got != tt.want {
				t.Errorf("BlobHash(%q) = %s, want %s", tt.content, got, tt.want)
			}
		})
	}
}

// endregion: test

// region: against-git

// TestBlobHashMatchesGit asks the real git for the same IDs. It
// runs in a fresh SHA-1 repository, because git hash-object inside
// a repository uses the hash function of that repository.
func TestBlobHashMatchesGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		need(t, "git is not on PATH")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", "--object-format=sha1", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	inputs := [][]byte{
		[]byte("hello\n"),
		{},
		[]byte("no newline at the end"),
		{0x00, 0xff, 0x10, '\n', 0x00},
		bytes.Repeat([]byte("monitor "), 10_000),
	}
	for _, in := range inputs {
		cmd := exec.Command("git", "hash-object", "--stdin")
		cmd.Dir = dir
		cmd.Stdin = bytes.NewReader(in)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git hash-object: %v", err)
		}
		want := strings.TrimSpace(string(out))
		if got := BlobHash(in); got != want {
			t.Errorf("BlobHash of %d bytes = %s, git says %s", len(in), got, want)
		}
	}
}

// endregion: against-git

// need fails the test in CI and skips it elsewhere: CI must run every
// test, a laptop without the tool may not.
func need(t *testing.T, why string) {
	t.Helper()
	if os.Getenv("CI") == "true" {
		t.Fatal(why)
	}
	t.Skip(why)
}

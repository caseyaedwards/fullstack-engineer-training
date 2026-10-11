package storage

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// names lists the entries of dir, to catch a temp file left behind.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// region: test
func TestWriteFileAtomicReplacesAFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "urls.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomic(path, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Errorf("content = %q, want %q", got, "new\n")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want -rw-------", info.Mode().Perm())
	}
	if n := names(t, dir); len(n) != 1 {
		t.Errorf("directory holds %v, want only urls.txt", n)
	}
}

func TestWriteFileAtomicMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "urls.txt")

	err := WriteFileAtomic(path, []byte("new\n"), 0o600)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist", err)
	}
	if n := names(t, dir); len(n) != 0 {
		t.Errorf("directory holds %v, want nothing", n)
	}
}

// endregion: test

// When the rename fails, here because a directory sits at path, the temp
// file goes away and the directory stays as it is.
func TestWriteFileAtomicFailedRenameLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomic(path, []byte("new\n"), 0o600); err == nil {
		t.Fatal("WriteFileAtomic over a directory: err = nil, want an error")
	}
	if n := names(t, dir); len(n) != 1 || n[0] != "target" {
		t.Errorf("directory holds %v, want only target", n)
	}
}

// A reader that opens the file while writers replace it sees the whole old
// content or the whole new content, never a mix and never a short file.
func TestWriteFileAtomicReadersNeverSeeAPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	a := bytes.Repeat([]byte("a"), 256<<10)
	b := bytes.Repeat([]byte("b"), 256<<10)
	if err := WriteFileAtomic(path, a, 0o644); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 20 {
			data := a
			if i%2 == 0 {
				data = b
			}
			if err := WriteFileAtomic(path, data, 0o644); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for range 100 {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, a) && !bytes.Equal(got, b) {
			t.Fatalf("read %d bytes that are neither version", len(got))
		}
	}
	wg.Wait()
}

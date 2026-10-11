// Package backups writes a checksum manifest for a folder of backup files and
// verifies a copy of the folder against it, the check to run after every
// copy and before every restore.
package backups

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// region: manifest

// Manifest maps the path of each file, relative to the folder and with
// forward slashes, to the SHA-256 of its content in hexadecimal.
type Manifest map[string]string

// ManifestName is the file WriteManifest writes into the folder. Its format
// is the one of sha256sum: `sha256sum -c SHA256SUMS` checks the same files.
const ManifestName = "SHA256SUMS"

// WriteManifest hashes every regular file under dir, writes the result to
// dir/SHA256SUMS and returns it. Symbolic links and other special files are
// left out, and so is an earlier SHA256SUMS.
func WriteManifest(dir string) (Manifest, error) {
	paths, err := files(dir)
	if err != nil {
		return nil, err
	}
	m := Manifest{}
	var out strings.Builder
	for _, p := range paths {
		sum, err := hashFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			return nil, err
		}
		m[p] = sum
		fmt.Fprintf(&out, "%s  %s\n", sum, p) // two spaces: binary-safe text mode
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte(out.String()), 0o644); err != nil {
		return nil, err
	}
	return m, nil
}

// hashFile streams the file through SHA-256, which keeps memory flat for a
// dump of any size.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// endregion: manifest

// region: verify

// Problem is one difference between a folder and its manifest. Kind is
// missing (listed, not on disk), changed (on disk with another hash, or
// unreadable) or extra (on disk, not listed).
type Problem struct {
	Path, Kind string
}

// Verify compares the files under dir with m and returns every problem,
// sorted by path. No problems means the folder holds exactly the files of
// the manifest, byte for byte.
func Verify(dir string, m Manifest) []Problem {
	var problems []Problem
	onDisk, _ := files(dir) // what cannot be listed counts as missing
	seen := map[string]bool{}
	for _, p := range onDisk {
		seen[p] = true
		if _, listed := m[p]; !listed {
			problems = append(problems, Problem{p, "extra"})
		}
	}
	for p, want := range m {
		if !seen[p] {
			problems = append(problems, Problem{p, "missing"})
			continue
		}
		got, err := hashFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil || got != want {
			problems = append(problems, Problem{p, "changed"})
		}
	}
	slices.SortFunc(problems, func(a, b Problem) int { return cmp.Compare(a.Path, b.Path) })
	return problems
}

// endregion: verify

// files returns the relative, slash-separated paths of the regular files
// under dir, sorted, without the manifest itself. It skips what it cannot
// read and returns the first such error with the rest of the list.
func files(dir string) ([]string, error) {
	var paths []string
	var first error
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			first = cmp.Or(first, err)
			return nil // a directory that cannot be read: skip it, keep walking
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != ManifestName {
			paths = append(paths, rel)
		}
		return nil
	})
	slices.Sort(paths)
	return paths, errors.Join(first, err)
}

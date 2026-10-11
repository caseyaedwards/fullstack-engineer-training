// Package automation holds the examples of the Automation track:
// playbook.yml, an Ansible playbook that keeps time with chrony, and
// EnsureLine, the idea behind the Ansible lineinfile module in a few lines
// of Go.
package automation

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// region: ensureline

// EnsureLine makes sure that the file at path holds line as one whole
// line. When the line is already there, it changes nothing and returns
// false. Otherwise it appends the line, creating the file with mode 0644
// when it does not exist, and returns true. Run it twice, and the second
// run reports no change: it describes a state, not a step.
func EnsureLine(path, line string) (changed bool, err error) {
	if strings.ContainsAny(line, "\r\n") {
		return false, errors.New("ensureline: line must not hold a line break")
	}
	if line == "" {
		return false, errors.New("ensureline: line must not be empty")
	}
	data, err := os.ReadFile(path)
	mode := fs.FileMode(0o644)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		data = nil
	case err != nil:
		return false, err
	default:
		fi, err := os.Stat(path)
		if err != nil {
			return false, err
		}
		mode = fi.Mode().Perm()
	}

	for _, l := range bytes.Split(data, []byte("\n")) {
		if string(l) == line {
			return false, nil
		}
	}

	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	data = append(data, line+"\n"...)
	return true, replace(path, data, mode)
}

// endregion: ensureline

// region: replace

// replace writes data to a temporary file next to path, then renames it
// over path. A rename within one file system is atomic: a reader sees the
// old file or the new one, never half of each, even when the write fails.
func replace(path string, data []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ensureline-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // fails harmlessly after the rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// endregion: replace

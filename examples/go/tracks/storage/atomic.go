// Package storage replaces a file atomically and durably: a reader sees
// the old content or the new content, and a crash loses neither.
package storage

import (
	"io/fs"
	"os"
	"path/filepath"
)

// region: atomic

// WriteFileAtomic replaces the file at path with data and sets its mode
// to perm. It writes a temp file in the same directory, flushes it to
// disk, and renames it over path. On an error before the rename, it
// removes the temp file and leaves path as it was. An error after the
// rename, from opening or syncing the directory, means path already holds
// the new data, but its directory entry is not yet durable.
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	// The same directory keeps the rename on one filesystem, where
	// rename replaces the target in one step.
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()

	if _, err = f.Write(data); err != nil {
		return err
	}
	// Sync waits until the data is on the disk, not only in the page cache.
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp, perm); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	// The new directory entry lives in the directory, which needs its
	// own Sync to survive a power cut.
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// endregion: atomic

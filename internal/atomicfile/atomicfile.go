// Package atomicfile writes files so that readers see either the old or the
// complete new content, never a partial write.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write writes data to path via a uniquely named temporary file in the same
// directory and a rename, so concurrent writers of the same path cannot
// interleave.
func Write(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

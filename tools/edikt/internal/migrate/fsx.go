package migrate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// AtomicWriteNoFollow writes data to path atomically (tmp + rename), with a
// pre-rename check that path has not become a symlink. Shared by the lifter
// and the install migration in cmd/migrate.go.
func AtomicWriteNoFollow(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%d", filepath.Base(path), os.Getpid()))

	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	// Sync.
	f, err := os.Open(tmp)
	if err == nil {
		_ = f.Sync()
		f.Close()
	}

	// Pre-rename check: ensure path is still not a symlink.
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		os.Remove(tmp)
		return fmt.Errorf("%s became a symlink during migration — aborting", path)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

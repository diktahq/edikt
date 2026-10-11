// Package projectroot is the one place that decides where an edikt project
// begins: the nearest directory, walking up from a starting point, that holds
// .edikt/config.yaml. Callers: `doctor --contract-state`, `phasea plan` and
// `gov reextract`. It is NOT the only finder in the binary: cmd/verify.go keeps
// its own, used by `verify`, `sidecar approve` and `dev hookcorpus`; those
// commands do not go through this package.
package projectroot

import (
	"os"
	"path/filepath"
)

// Find walks up from dir looking for .edikt/config.yaml and returns the
// directory that holds .edikt/. found is false when no ancestor (dir included)
// has one; callers decide what "no project" means for them.
func Find(dir string) (root string, found bool) {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".edikt", "config.yaml")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// FindFromCwd is Find starting at the working directory. err is non-nil only
// when the working directory cannot be read.
func FindFromCwd() (root string, found bool, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, err
	}
	root, found = Find(cwd)
	return root, found, nil
}

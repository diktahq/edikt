// Package projectpaths resolves where a project keeps the artifact trees
// edikt reads — currently its PRDs and its specs.
//
// It exists so there is ONE definition of that mapping. The rule lives in
// `.edikt/config.yaml` under `paths:`, and every consumer that re-derives it
// from a hard-coded default is a second definition that can drift from the
// first — the same shape as an ungated schema copy. `doctor` needed it to
// report drift, and the criteria loader needs it to resolve the spec a plan
// declares against; both call this.
package projectpaths

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Default locations, used when `.edikt/config.yaml` says nothing.
const (
	DefaultPRDsDir  = "docs/product/prds"
	DefaultSpecsDir = "docs/product/specs"
)

// ProductDirs returns the absolute PRD and spec directories for projectRoot,
// reading `paths.prds` / `paths.specs` from `.edikt/config.yaml` and falling
// back to the defaults above.
//
// A missing or unreadable config is NOT an error: a project that has never
// customised these paths has no `paths:` block, and the defaults are the
// answer. What it must never do is report a directory it did not resolve, so
// the fallbacks are the documented defaults rather than an empty string.
func ProductDirs(projectRoot string) (prdsDir, specsDir string) {
	prdsDir = filepath.Join(projectRoot, DefaultPRDsDir)
	specsDir = filepath.Join(projectRoot, DefaultSpecsDir)

	data, err := os.ReadFile(filepath.Join(projectRoot, ".edikt", "config.yaml"))
	if err != nil {
		return prdsDir, specsDir
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	inPaths := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "paths:") {
			inPaths = true
			continue
		}
		if !inPaths {
			continue
		}
		if !(strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "#") {
				break // a new top-level key ended the paths: block
			}
			continue
		}
		ts := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(ts, "prds:"); ok {
			prdsDir = filepath.Join(projectRoot, strings.TrimSpace(rest))
		} else if rest, ok := strings.CutPrefix(ts, "specs:"); ok {
			specsDir = filepath.Join(projectRoot, strings.TrimSpace(rest))
		}
	}
	return prdsDir, specsDir
}

// FindRoot walks up from start and returns the first directory that looks like
// an edikt project root, or "" when there is none.
//
// It returns "" rather than a guess on purpose. A caller that needs the root
// to validate something must be able to tell "resolved" from "assumed" — a
// fabricated root would send every subsequent lookup to a directory that does
// not exist and report the resulting absence as the answer.
func FindRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, ".edikt")); err == nil && st.IsDir() {
			return dir
		}
		if st, err := os.Stat(filepath.Join(dir, DefaultSpecsDir)); err == nil && st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

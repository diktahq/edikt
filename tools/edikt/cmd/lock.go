package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LockFile represents the top-level fields of lock.yaml.
type LockFile struct {
	Active       string        `yaml:"active"`
	Previous     string        `yaml:"previous,omitempty"`
	Pinned       string        `yaml:"pinned,omitempty"`
	InstalledVia string        `yaml:"installed_via,omitempty"`
	History      []LockHistory `yaml:"history,omitempty"`
}

// LockHistory is a single history entry in lock.yaml.
type LockHistory struct {
	Version      string `yaml:"version"`
	InstalledAt  string `yaml:"installed_at,omitempty"`
	ActivatedAt  string `yaml:"activated_at,omitempty"`
	InstalledVia string `yaml:"installed_via,omitempty"`
}

// readLock reads $EDIKT_ROOT/lock.yaml. Returns a zero-value LockFile
// (not an error) when the file does not exist.
func readLock(ediktRoot string) (LockFile, error) {
	p := filepath.Join(ediktRoot, "lock.yaml")
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return LockFile{}, nil
	}
	if err != nil {
		return LockFile{}, fmt.Errorf("reading lock.yaml: %w", err)
	}
	var lf LockFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return LockFile{}, fmt.Errorf("parsing lock.yaml: %w", err)
	}
	return lf, nil
}

// writeLock atomically writes lock.yaml at $EDIKT_ROOT/lock.yaml.
// newActive is the version being activated; installedVia is a short label
// ("launcher", "dev-link", etc.). The previous field is set from the current
// active (if different). History is preserved up to 50 entries.
func writeLock(ediktRoot, newActive, installedVia string) error {
	existing, _ := readLock(ediktRoot)

	lf := LockFile{
		Active:       normalizeTag(newActive),
		InstalledVia: installedVia,
	}
	// Preserve or advance previous.
	if existing.Active != "" && existing.Active != newActive {
		lf.Previous = existing.Active
	} else if existing.Previous != "" {
		lf.Previous = existing.Previous
	}

	// Carry forward history (cap at 49 to leave room for new entry).
	hist := existing.History
	if len(hist) > 49 {
		hist = hist[len(hist)-49:]
	}
	hist = append(hist, LockHistory{
		Version:      normalizeTag(newActive),
		InstalledVia: installedVia,
	})
	lf.History = hist

	data, err := yaml.Marshal(lf)
	if err != nil {
		return fmt.Errorf("marshalling lock.yaml: %w", err)
	}

	p := filepath.Join(ediktRoot, "lock.yaml")
	tmp := p + fmt.Sprintf(".tmp.%d", os.Getpid())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing lock.yaml tmp: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming lock.yaml: %w", err)
	}
	return nil
}

// normalizeTag strips a leading "v" from a version string.
func normalizeTag(tag string) string {
	return strings.TrimPrefix(tag, "v")
}

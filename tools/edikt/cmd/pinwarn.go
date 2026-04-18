package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// mutatingCommands are subject to global lock acquisition via PersistentPreRunE.
// Commands NOT listed here either have their own internal locking (migrate)
// or don't mutate shared state (list, version, doctor, help, completion).
var mutatingCommands = map[string]bool{
	"install":   true,
	"use":       true,
	"upgrade":   true,
	"rollback":  true,
	"prune":     true,
	"uninstall": true,
}

// pinWarnExempt returns true if cmd name is exempt from pin warning.
func pinWarnExempt(name string) bool {
	exempt := map[string]bool{
		"list":        true,
		"version":     true,
		"doctor":      true,
		"help":        true,
		"completion":  true,
		"upgrade-pin": true,
		"gov":         true,
	}
	return exempt[name]
}

// emitPinWarn checks the current working directory's ancestor tree for an
// .edikt/config.yaml with an edikt_version pin that differs from the active
// version. If found, prints a warning to stderr.
func emitPinWarn(ediktRoot string) {
	configPath := findProjectConfig()
	if configPath == "" {
		return
	}
	pinned := readPinnedVersion(configPath)
	if pinned == "" {
		return
	}
	lf, err := readLock(ediktRoot)
	if err != nil || lf.Active == "" {
		return
	}
	active := normalizeTag(lf.Active)
	if normalizeTag(pinned) == active {
		return
	}
	fmt.Fprintf(os.Stderr, "warn: this project pins edikt %s but the active version is %s\n",
		pinned, active)
	fmt.Fprintf(os.Stderr, "      Run `edikt upgrade-pin` inside the project to update the pin.\n")
}

// findProjectConfig walks from CWD to HOME looking for .edikt/config.yaml.
// Returns the path if found, empty string otherwise.
func findProjectConfig() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	home := os.Getenv("HOME")
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, ".edikt", "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			// Don't treat the global ~/.edikt/config.yaml as a project config.
			if home != "" && filepath.Clean(dir) == filepath.Clean(home) {
				break
			}
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if home != "" && filepath.Clean(dir) == filepath.Clean(home) {
			break
		}
		dir = parent
	}
	return ""
}

// readPinnedVersion reads the edikt_version field from a config.yaml.
func readPinnedVersion(configPath string) string {
	f, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "edikt_version:") {
			continue
		}
		val := strings.TrimPrefix(line, "edikt_version:")
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		return val
	}
	return ""
}


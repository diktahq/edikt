package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/versioncmp"
	"github.com/spf13/cobra"
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
		// Hooks run `edikt hook <name>` many times per turn, and their stderr
		// is host-surfaced hook output where no one can act on a version
		// mismatch. `edikt doctor` reports pin drift instead (#11, item 8).
		"hook": true,
	}
	return exempt[name]
}

// emitPinWarn checks the current working directory's ancestor tree for an
// .edikt/config.yaml with an edikt_version pin that differs from the active
// version. If found, prints a warning to stderr.
func emitPinWarn(ediktRoot string) {
	emitPinWarnTo(os.Stderr, ediktRoot)
}

// emitPinWarnTo is emitPinWarn writing to w, so the warning's presence can be
// asserted rather than inferred.
func emitPinWarnTo(w io.Writer, ediktRoot string) {
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
	// A channel install (`dev`) against a release pin, or the reverse, has no
	// direction and so no remedy — pinRemedy's own text for the case says so.
	// Printing it on every command told every developer of this project
	// something they could do nothing about. Silent here; `edikt doctor`
	// reports it as drift, where a standing condition belongs (#11, item 8).
	if versioncmp.Compare(pinned, active) == versioncmp.Incomparable {
		return
	}
	fmt.Fprintf(w, "warn: this project pins edikt %s but the active version is %s\n",
		pinned, active)

	fmt.Fprint(w, pinRemedy(pinned, active))
}

// pinWarnExemptChain reports whether cmd or any ancestor is exempt. The
// policy is "any command under `gov` (or `hook`) is exempt", not "only the
// literal leaf", so the walk covers every subcommand without listing it.
func pinWarnExemptChain(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if pinWarnExempt(c.Name()) {
			return true
		}
	}
	return false
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

// pinRemedy returns the advice line(s) for a pin/active mismatch.
//
// THE REMEDY DEPENDS ON THE DIRECTION, AND IT USED NOT TO.
//
// This warning always said "run `edikt upgrade-pin`". That is correct when the
// pin is BEHIND the binary: the project declares an older version than the one
// running, and moving the pin forward is the fix.
//
// When the pin is AHEAD, the same advice is actively wrong. `upgrade-pin`
// rewrites the pin to match whatever is installed, so following it moves the
// project's declared version BACKWARD to agree with an older binary —
// resolving the warning by discarding the very fact it was reporting. A
// colleague hit exactly this on a project pinned 0.4.5 with 0.4.3 installed,
// and only noticed because he reasoned about the direction himself. The
// version gate cannot catch it either: it is directional by construction and
// refuses only in the other direction.
//
// AND THERE IS A FOURTH ANSWER, WHICH IS THE ONE EVERY DEVELOPER HERE GOT.
// The direction was read from a truncating parser that turned `dev` into
// [0,0,0], so a dev checkout compared BEHIND every pin and this function told
// every developer on this project that their pin was AHEAD of their binary —
// then advised against `upgrade-pin`, the one command that would have cleared
// it. A channel name has no position in a release ordering. The truthful
// output is that the two are NOT COMPARABLE, not a corrected direction between
// them, and that is now its own branch.
func pinRemedy(pinned, active string) string {
	switch versioncmp.Compare(pinned, active) {
	case versioncmp.LeftNewer:
		return "      The pin is AHEAD of what is installed: this project expects a newer\n" +
			"      edikt than the one running. Upgrade the binary — do NOT run\n" +
			"      `edikt upgrade-pin`, which would move the pin backward to match the\n" +
			"      older install and hide the mismatch.\n"
	case versioncmp.RightNewer:
		return "      The pin is behind the installed version. Run `edikt upgrade-pin`\n" +
			"      inside the project to move it forward.\n"
	case versioncmp.Incomparable:
		return "      The pin and the installed version are not comparable: one of them is\n" +
			"      a channel name such as `dev` rather than a release, and a channel has\n" +
			"      no position in a release ordering. Neither moving the pin nor upgrading\n" +
			"      the binary follows from this — the two are different kinds of thing.\n"
	default:
		// The same point in the ordering reached by two different strings —
		// `v0.8.0` against `0.8.0`, say. Nothing is stale in either direction,
		// so the generic remedy is the honest one.
		return "      Run `edikt upgrade-pin` inside the project to update the pin.\n"
	}
}

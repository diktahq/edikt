package cmd

// doctor_pin_drift.go — the standing report for a project pin that differs
// from the active edikt version (#11 comment item 8).
//
// The per-invocation pin warning (pinwarn.go) is silent where it has no
// remedy: on an Incomparable pair such as a `dev` install against a release
// pin, and inside every hook. Silencing it there must not hide the drift, so
// doctor reports EVERY pin != active, Incomparable included, once, with the
// remedy that matches the direction. It is a WARN: drift is a state to know
// about, not a broken install.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/diktahq/edikt/tools/edikt/internal/versioncmp"
)

// runPinDriftCheck compares projectRoot's edikt_version pin with the active
// version recorded in ediktRoot's lock.yaml. Returns 1 if it warned, 0
// otherwise. No pin, or a project root that is $HOME (whose .edikt/ is the
// global home, not a project), is silent: there is nothing to compare. A pin
// with no readable active version prints an UNMEASURED line and counts
// nothing — it is neither drift nor its absence: a control that observed
// nothing must say so rather than report clean.
func runPinDriftCheck(projectRoot, ediktRoot string, out io.Writer) int {
	if home := os.Getenv("HOME"); home != "" && samePath(projectRoot, home) {
		return 0
	}
	pinned := readPinnedVersion(filepath.Join(projectRoot, ".edikt", "config.yaml"))
	if pinned == "" {
		return 0
	}
	lf, err := readLock(ediktRoot)
	if err != nil || lf.Active == "" {
		fmt.Fprintf(out, "  INFO: pin drift UNMEASURED — edikt_version pins %s but no active version is recorded in %s\n",
			pinned, filepath.Join(ediktRoot, "lock.yaml"))
		return 0
	}
	active := normalizeTag(lf.Active)
	if normalizeTag(pinned) == active {
		return 0
	}
	var remedy string
	switch versioncmp.Compare(pinned, active) {
	case versioncmp.RightNewer:
		remedy = "the pin is behind the install — run `edikt upgrade-pin` inside the project to move it forward"
	case versioncmp.LeftNewer:
		remedy = "the pin is ahead of the install — upgrade the binary; do NOT run `edikt upgrade-pin`, which would move the pin backward"
	case versioncmp.Incomparable:
		remedy = "the two are not comparable (one is a channel such as `dev`, which has no place in a release ordering); nothing follows from it, and it is reported here instead of on every command"
	default:
		remedy = "run `edikt upgrade-pin` inside the project to record the active spelling"
	}
	fmt.Fprintf(out, "  WARN: edikt_version pins %s but the active version is %s — %s\n", pinned, active, remedy)
	return 1
}

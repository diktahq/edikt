package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// versionLineFloor is the major.minor of the current architecture line.
// v0.6.0 introduced sidecar architecture — a hard break — so edikt v0.6.0+
// refuses to operate on a project whose pinned edikt_version is below this
// line. Bump this when a future release makes another breaking change.
//
// Raised to 0.7 by SPEC-011 stage 0: gov-sidecar.v2 replaces the singular  edikt-guard:allow
// source_excerpt with a 1..N source_excerpts array. A v0.6 loader cannot read
// a v2 sidecar (KnownFields rejects the array key) and a v0.6 project's v1
// sidecars are not what the v2 extractor writes, so the two lines cannot share
// a corpus.
//
// WHAT THIS GATE DOES AND DOES NOT BUY (recorded 2026-08-12 — the plan's
// atomicity note says the pin bump makes the gate "refuse older binaries
// cleanly rather than surfacing a generic KnownFields error", and that is only
// half true, so the half that is not is written down rather than assumed):
//
// The gate is DIRECTIONAL. It runs inside the binary and refuses a project
// whose pin is BELOW the running binary's floor — a new binary refusing an old
// project. It cannot refuse in the other direction, because the check would
// have to live in a binary that already shipped. A released v0.6 binary meeting
// a v2 corpus therefore still fails with the generic KnownFields error; nothing
// in this release can retrofit a check into it.
//
// What the bump does buy is the direction that is reachable: the moment an
// operator runs a 0.7 binary, a project still pinned on the 0.6 line is refused
// with the upgrade path named, instead of half-working against a corpus shape
// it cannot represent.
//
// Raised to 0.8 by SPEC-015/PRD-007 (Phase 11, ADR-042's 2026-08-20  edikt-guard:allow
// amendment). This second bump is NOT paired with a new corpus-shape break —
// v0.8.0 introduces no schema_version bump — so it differs from the 0.6->0.7
// bump this comment otherwise describes. It is paired instead with SP-006's
// new N-1 upgrade-line gate in upgrade.go: that gate ships INSIDE the v0.8.0
// binary and only ever runs when a v0.8.0+ binary is doing the resolving, so
// it cannot catch a user still running a v0.7.x binary (that binary executes
// v0.7.x's OWN same-major check and never reaches the new code at all). This
// floor is what actually catches that user — from the OTHER direction, a new
// binary refusing an old project, which is reachable regardless of which
// binary the user still has installed. See upgrade.go's N-1 gate comment for
// the fuller argument.
const versionLineFloor = "0.8"

// payloadLineFloor is the OLDEST payload the launcher will activate, which
// is a different question from versionLineFloor's.
//
// versionLineFloor gates a PROJECT: a binary refuses to operate on a project
// whose pinned edikt_version predates the current architecture line
// (ADR-042). payloadLineFloor gates an INSTALLED RELEASE: which versions the
// launcher will flip `current` to.
//
// These were one constant until PLAN-v080 Phase 11 raised the project floor
// to 0.8 and silently took the payload floor with it. That made `edikt use`
// and therefore `edikt rollback` refuse every 0.7.x payload — a released
// version could be installed but never activated, so rollback had nothing to
// roll back to. It also made Phase 11's own acceptance criterion
// (test_upgrade_0_7_to_0_8.sh, "a 0.7.x layout upgrades to 0.8.0 end to
// end") impossible to even set up, and left use.go's comment describing a
// v0.6 line the code no longer implemented.
//
// The payload floor is v0.7: the Go-launcher era technically starts at
// v0.6.0 (v0.5.x was retracted and never released), but v0.6 is two lines
// back and nothing in the tree activates a 0.6 payload — the sole v0.6.0
// reference is a --dry-run that never reaches activation. Owner decision
// 2026-08-21: rollback must reach the last released line (0.7.x) and no
// further.
//
// Raise this only when an older payload genuinely cannot be activated by
// this launcher, and check first what still installs one — this constant is
// what makes `edikt rollback` able to reach anything at all.
const payloadLineFloor = "0.7"

// versionLineFloorReason names, in one clause, WHY the current floor is where
// it is. It travels with the floor into the refusal message so an operator
// reading the error learns what changed, not merely that something did.
// The reason travels into the refusal an ADOPTER reads, so it says what
// changed in their terms. It used to name "SPEC-015 ... (SP-006)" —
// identifiers that exist only in edikt's own internal specs, which the
// person hitting the error cannot open.
const versionLineFloorReason = "edikt only upgrades one minor line at a time, and refuses a project pinned further back than that"

// ensureVersionLine fail-closes any project-operating command when the
// project's pinned edikt_version predates the current version line (ADR-042).  // edikt-guard:allow
// After v0.6.0 edikt does not run across version lines: the operator must
// upgrade the project (migrates it onto the current line) or downgrade the
// edikt binary to match. EDIKT_SKIP_VERSION_GATE=1 overrides (test harness,
// emergency).
func ensureVersionLine() error {
	if os.Getenv("EDIKT_SKIP_VERSION_GATE") == "1" {
		return nil
	}
	configPath := findProjectConfig()
	if configPath == "" {
		return nil // no project config (fresh / global invocation) — nothing to gate
	}
	pinned := readPinnedVersion(configPath)
	if pinned == "" {
		return nil // unpinned — fresh project; the project adopts the running line
	}
	if !versionLineBelowFloor(pinned) {
		return nil // on or ahead of the current line
	}
	// The floor is interpolated rather than written into the sentence. It has
	// now moved once, and the message that named "v0.6.0" in prose would have
	// gone on saying so at floor 0.7 — telling the operator a version number
	// that no longer matches the check that produced the message.
	return &exitCodeError{code: 4, msg: fmt.Sprintf(
		"this project was set up with edikt v%s, which predates the v%s line "+
			"(%s). edikt does not run across version lines.\n"+
			"  Upgrade the project:  run /edikt:upgrade (migrates it onto the current line)\n"+
			"  Or match the project: install edikt v%s (edikt rollback, or install --ref v%s)\n"+
			"  Override (not recommended): EDIKT_SKIP_VERSION_GATE=1",
		pinned, versionLineFloor, versionLineFloorReason, pinned, pinned)}
}

// versionLineBelowFloor reports whether v's major.minor is below the floor.
// Prerelease/patch are ignored, so 0.6.0-rc4 is "on the 0.6 line".
func versionLineBelowFloor(v string) bool { return belowFloor(v, versionLineFloor) }

// payloadLineBelow reports whether an installed payload predates the oldest
// line this launcher will activate.
func payloadLineBelow(v string) bool { return belowFloor(v, payloadLineFloor) }

func belowFloor(v, floor string) bool {
	pm, pn, ok := majorMinor(v)
	if !ok {
		return false // unparseable — do not gate on garbage
	}
	fm, fn, _ := majorMinor(floor)
	if pm != fm {
		return pm < fm
	}
	return pn < fn
}

// majorMinor extracts the (major, minor) integers from a version like
// "v0.6.0-rc4" / "0.5.1" / "0.6". Returns ok=false if it can't parse two ints.
func majorMinor(v string) (int, int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	maj, e1 := strconv.Atoi(parts[0])
	min, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil {
		return 0, 0, false
	}
	return maj, min, true
}

// versionGateExempt names the commands that must run even on a cross-line
// project: the version-management verbs that FIX the mismatch, plus read-only
// / meta commands. Everything else (gov, verify, sidecar, …) is gated.
func versionGateExempt(name string) bool {
	return map[string]bool{
		"install": true, "use": true, "upgrade": true, "rollback": true,
		"uninstall": true, "prune": true, "migrate": true,
		"version": true, "doctor": true, "list": true, "help": true,
		"completion": true, "upgrade-pin": true,
	}[name]
}

// versionGateApplies walks the command's parent chain (excluding root) and
// returns false if any ancestor is exempt — so `migrate sidecars` inherits
// `migrate`'s exemption while `gov compile` stays gated.
func versionGateApplies(cmd *cobra.Command) bool {
	if !cmd.HasParent() {
		return false // root itself
	}
	for c := cmd; c != nil && c.HasParent(); c = c.Parent() {
		if versionGateExempt(c.Name()) {
			return false
		}
	}
	return true
}

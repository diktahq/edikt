package cmd

// doctor_verify_gate.go — SPEC-009 Plan B Phase 5.  // edikt-guard:allow
//
// Adds a `Verify Gate` section to `edikt doctor` reporting the posture of
// the PreToolUse verify-gate hook (ADR-038), plus a count of recent bypass  // edikt-guard:allow
// events from `.edikt/state/verify-gate.jsonl`.
//
// Posture is ENABLED, BYPASSED, NOT INSTALLED, or UNMEASURED. The last two
// were added because the section reported ENABLED whenever the bypass env
// var was unset, without ever checking that the hook existed — so a
// project with no `.claude/` read as gated. ADR-038's checklist calls for
// ENABLED or BYPASSED to be reported; both still are, in the cases where
// either is true.
//
// The output is informational and never increments errN. NOT INSTALLED and
// UNMEASURED do increment warns: doctor cannot claim the gate is active,
// and staying silent about that is the failure this section had.

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
)

// verifyGateHook is the basename ADR-038 registers under PreToolUse.  // edikt-guard:allow
const verifyGateHook = "verify-gate.sh"

// runVerifyGateCheck reports verify-gate posture and a tail-count of
// recent bypass events. Returns (warns, ran). ran is always true so the
// section is always rendered when doctor visits a project directory.
//
// Posture is one of ENABLED, BYPASSED, NOT INSTALLED, or UNMEASURED. The
// last two exist because absence must not render as a pass: an
// uninstalled gate and an unparseable settings file are both states in
// which doctor cannot say the gate is active, and neither is the same
// claim as "active".
func runVerifyGateCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	io.WriteString(w, "  ── Verify Gate ────────────────────────────────\n")

	// Is the hook actually installed?
	//
	// This line used to branch solely on EDIKT_DISABLE_VERIFY_GATE, so
	// "unset" printed "ENABLED (PreToolUse hook active)" — including for a
	// directory with no .claude/ at all. The one thing the line asserts is
	// the one thing it never checked, and a project that had never
	// installed the gate read as protected by it.
	//
	// Registration is read via hookBasenames, the same reader doctor's
	// dual-registration check uses. A second parser here would be a second
	// derivation of one fact, free to drift from the first.
	registered := false
	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")
	reg, err := hookBasenames(settingsPath)
	switch {
	case err == nil:
		registered = reg["PreToolUse"][verifyGateHook]
	case os.IsNotExist(err):
		// No settings file: not installed. Definite, not unknown.
	default:
		// The file exists and would not parse. That is not "installed"
		// and not "absent" — it is unmeasured, and saying either would
		// be a claim doctor cannot support.
		io.WriteString(w, "  Verify Gate: UNMEASURED — .claude/settings.json could not be read ("+
			err.Error()+"); hook registration is unknown\n")
		return warns + 1, true
	}

	// Tier B bypass: EDIKT_DISABLE_VERIFY_GATE is the ad-hoc operator
	// escape hatch documented in ADR-038 §5. When set, the hook  // edikt-guard:allow
	// short-circuits to allow on every call. Checked after registration
	// because bypassing something uninstalled is not a posture.
	switch {
	case !registered:
		io.WriteString(w, "  Verify Gate: NOT INSTALLED — no "+verifyGateHook+
			" registered under PreToolUse in .claude/settings.json; "+
			"completion claims are ungated. Run `/edikt:upgrade` to install it.\n")
		warns++
	case os.Getenv("EDIKT_DISABLE_VERIFY_GATE") == "1":
		io.WriteString(w, "  Verify Gate: BYPASSED (EDIKT_DISABLE_VERIFY_GATE=1 is set)\n")
	default:
		io.WriteString(w, "  Verify Gate: ENABLED ("+verifyGateHook+" registered under PreToolUse)\n")
	}

	// Tail-count recent bypass events. Audit log is at
	// .edikt/state/verify-gate.jsonl (one JSON object per line).
	logPath := filepath.Join(projectRoot, ".edikt", "state", "verify-gate.jsonl")
	f, err := os.Open(logPath)
	if err != nil {
		// No log file yet — fine; the gate just hasn't fired any bypasses.
		return warns, true
	}
	defer f.Close()

	count := 0
	sc := bufio.NewScanner(f)
	// Bump the scanner buffer to handle long JSON lines if the payload ever
	// grows. 1 MiB is well above any plausible single-event size.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		count++
	}
	if count > 0 {
		io.WriteString(w, "  Verify Gate audit log: "+itoa(count)+
			" bypass event(s) recorded at .edikt/state/verify-gate.jsonl\n")
	}
	return warns, true
}

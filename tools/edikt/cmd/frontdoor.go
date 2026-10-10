package cmd

// frontdoor.go — the EDIKT_INVOKER front door. Steering, not enforcement:
// a walled verb invoked from a non-terminal with no EDIKT_INVOKER
// declared refuses with exit 4 naming the remedy; the same verb from
// a real terminal always runs. EDIKT_INVOKER is a self-declared signal
// any session can set — it measures whether callers identify
// themselves, not whether they are trustworthy, and this file MUST
// NEVER be described as a control that prevents anything.
//
// WALLED SET: every audience:user verb in the permit list
// (internal/permits, mirroring templates/permits/tier2-verbs.yaml)
// minus a small, stable, named exempt set — read-only diagnostics,
// the whole hook/phasea/util groups, and the launcher verbs the
// installer itself invokes. Derived from the permit list rather than
// classified leaf-by-leaf as mutating/read-only: the alternative needs
// ongoing curation as subcommands are added and drifts the moment
// someone forgets. The purpose this design settles on is a consistent,
// attributable invocation path — not narrowly gating mutation — so a
// read-only verb like `gov score` being walled is coherent with that
// purpose, not an over-broad accident: any real caller clears it
// trivially (a terminal, or the EDIKT_INVOKER every tier-1 command
// sets), and the value is provenance, not restriction.

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/permits"
	"github.com/spf13/cobra"
	"golang.org/x/text/unicode/norm"
)

// normalizeInvoker applies NFKC + whitespace-strip before the
// EDIKT_INVOKER allowlist check, mirroring internal/idvalidate's own
// normalize step — a Unicode lookalike or trailing whitespace cannot
// forge a value that reads as a valid invoker name.
func normalizeInvoker(s string) string {
	return strings.TrimSpace(norm.NFKC.String(s))
}

// frontDoorExemptTop is the named exempt set: top-level verb names
// (the immediate child of root) never walled regardless of the permit
// list. Read-only diagnostics (doctor, verify, next-id, version),
// internal orchestration primitives invoked many times per turn by
// tier-1 markdown (hook, phasea, util), and the launcher verbs the
// installer itself calls (use, install, rollback, dev — dev is
// audience:dev and not even in the shipped permit list, named here
// defensively anyway).
var frontDoorExemptTop = map[string]bool{
	"doctor":   true,
	"verify":   true,
	"next-id":  true,
	"version":  true,
	"hook":     true,
	"phasea":   true,
	"util":     true,
	"use":      true,
	"install":  true,
	"rollback": true,
	"dev":      true,
}

// frontDoorInvokerRe validates EDIKT_INVOKER after NFKC + whitespace
// strip: a slash-command name, or one of the two literal machinery
// values ("ci" for test/run.sh and CI, "bench" for the benchmark
// runner) — anchored, so no suffix/prefix injection reads as a name.
var frontDoorInvokerRe = regexp.MustCompile(`^(/edikt:[a-z][a-z:-]*|ci|bench)$`)

// isTerminalStdin reports whether stdin is a real terminal. A package
// var, not a call inline, so tests can force the non-terminal branch
// without needing an actual pty/pipe distinction under `go test`.
var isTerminalStdin = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// frontDoorTopLevelName returns the name of cmd's ancestor immediately
// under root — the group name both the permit list and the exempt set
// are keyed on.
func frontDoorTopLevelName(cmd *cobra.Command) *cobra.Command {
	top := cmd
	for top.HasParent() && top.Parent().HasParent() {
		top = top.Parent()
	}
	return top
}

// frontDoorWalled reports whether cmd's top-level verb is walled.
// Returns false (never walled) on any permit-list read failure — a
// front door that can fail closed on a parse error would block every
// command in the binary over a broken embed, which is worse than the
// steering signal it exists to provide.
func frontDoorWalled(cmd *cobra.Command) bool {
	if !cmd.HasParent() {
		return false // root itself
	}
	top := frontDoorTopLevelName(cmd).Name()
	if frontDoorExemptTop[top] {
		return false
	}
	verbs, err := permits.UserVerbs()
	if err != nil {
		return false
	}
	for _, v := range verbs {
		if v == top {
			return true
		}
	}
	return false
}

// frontDoorCommandName renders cmd's full path as a slash-command
// name for the refusal's remedy, e.g. "edikt gov compile" -> "/edikt:gov:compile".
func frontDoorCommandName(cmd *cobra.Command) string {
	path := cmd.CommandPath()
	path = strings.TrimPrefix(path, "edikt")
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, " ", ":")
	return "/edikt:" + path
}

// checkFrontDoor is called from PersistentPreRunE for every command.
// Returns nil to proceed (not walled, a real terminal, or a validated
// EDIKT_INVOKER), or an *exitCodeError (code 4) to refuse.
//
// invoked_by is journaled for EVERY walled verb that proceeds — the
// validated EDIKT_INVOKER value, or the literal "direct" when none was
// set (a real terminal with no invoker declared). Recording "direct"
// rather than skipping the journal entry is what lets doctor's count
// (AC-6.5) measure "N of M walled calls had no declared invoker" —
// skipping direct calls would make every terminal session invisible
// to the exact metric this steering signal exists to report.
func checkFrontDoor(cmd *cobra.Command, ediktRoot string) error {
	if !frontDoorWalled(cmd) {
		return nil
	}
	invoker := normalizeInvoker(os.Getenv("EDIKT_INVOKER"))
	valid := frontDoorInvokerRe.MatchString(invoker)

	if !valid && !isTerminalStdin() {
		name := frontDoorCommandName(cmd)
		return &exitCodeError{code: 4, msg: fmt.Sprintf(
			"%s was called directly, without identifying its caller.\n"+
				"  Run %s instead, or set EDIKT_INVOKER=%s if this is machinery (CI, a script).",
			frontDoorTopLevelName(cmd).Name(), name, name)}
	}

	journaled := invoker
	if !valid {
		journaled = "direct"
	}
	if ediktRoot != "" {
		emitEvent(ediktRoot, "verb_invoked", map[string]interface{}{
			"verb":       frontDoorTopLevelName(cmd).Name(),
			"invoked_by": journaled,
		})
	}
	return nil
}

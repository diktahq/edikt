package verify

import (
	"os"
	"strconv"
)

// Re-entry containment for the verify runner.
//
// A sidecar `verify:` command is arbitrary shell, so nothing stops it from
// invoking `edikt verify` again. When the addressed artifact is the one being
// verified (or two artifacts address each other) the runner re-enters itself
// without bound: each level forks one child per self-referencing entry, so the
// process count grows exponentially. This is not hypothetical — INV-009's own
// sidecar carried `verify: bin/edikt verify gov INV-009` at directives[6] and
// verification[3], and the first compile run that reached the post-Phase-B
// gate produced ~1300 processes and exhausted the process table.
//
// Two independent containments, because they fail differently:
//
//  1. DEPTH (this file) bounds how many times the runner may re-enter itself.
//     It is what stops unbounded growth.
//  2. PROCESS-GROUP KILL (runner.go) reaps the tree a timed-out verify already
//     forked. The doctrine line from the incident: fail-closed on TIME does not
//     imply fail-closed on FANOUT. CommandContext kills the direct child only;
//     every grandchild it already spawned survives and keeps spawning.
//
// MaxDepth allows exactly one nested runner invocation. That is deliberate,
// not arbitrary: a verify that checks a DIFFERENT artifact's status is
// legitimate, non-circular evidence (ADR-038 directives[53] runs
// `edikt verify spec SPEC-009` to read SR-004's state), and the e2e suites
// scaffold throwaway projects and run the binary against them. A flat refusal
// would break both. What is never legitimate is a second nesting, which no
// honest check needs and which is the signature of a cycle.
const (
	// EnvDepth carries the current runner nesting level to child processes.
	EnvDepth = "EDIKT_VERIFY_DEPTH"

	// MaxDepth is the highest nesting level at which the runner may start.
	// Level 0 is a user-invoked run; level 1 is a run spawned from inside a
	// verify command. A run attempting to start at level 2 is refused.
	MaxDepth = 2
)

// CurrentDepth reports the nesting level this process is running at. A missing
// or unparseable value reads as 0 (top level) — an attacker-controlled or
// corrupt value must not be able to raise the cap, so negatives clamp to 0 and
// anything unparseable is treated as top level rather than trusted.
func CurrentDepth() int {
	raw := os.Getenv(EnvDepth)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// DepthExceeded reports whether a runner starting now would exceed MaxDepth.
// Callers refuse the run when this is true.
func DepthExceeded() bool { return CurrentDepth() >= MaxDepth }

// childDepthEnv returns the EDIKT_VERIFY_DEPTH assignment for a spawned verify
// command: one level deeper than this process.
func childDepthEnv() string {
	return EnvDepth + "=" + strconv.Itoa(CurrentDepth()+1)
}

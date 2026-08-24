// Package hookcorpus is the characterization-corpus recorder and the
// migration gate that reads what it records (SPEC-014 FR-002, ADR-063
// Decision §2).
//
// # WHAT THIS PACKAGE IS FOR
//
// Twenty-two bash hooks are being rewritten in Go, one at a time. The whole
// behaviour-equivalence argument for that migration rests on ONE mechanical
// claim: for a curated matrix of inputs, the Go replacement produces the same
// exit code, the same stdout, the same stderr and the same filesystem effect
// as the bash body it replaces. This package makes that claim checkable.
//
// The hook itself is the oracle. Nothing here hand-authors an expectation —
// ADR-063 is explicit that the hand-work is matrix curation, not expectation
// authoring. The recorder executes the bash hook and writes down what happened;
// the gate re-runs the same scenario against the Go implementation and compares.
//
// # THE FOUR WAYS THIS COULD BE GAMED, AND WHAT STOPS EACH
//
// A gate that cannot fail is worse than no gate, because it converts an
// unexamined rewrite into recorded evidence of equivalence. Four specific
// routes to a meaningless green exist, and each has a named defence:
//
//  1. RECORD AGAINST THE REPLACEMENT. Capture the corpus after swapping in the
//     Go shim and the diff compares Go against Go — trivially green, proving
//     nothing. Defence: the recorder derives `oracle` from
//     templates/hooks/registry.yaml's own `status` field rather than from a
//     flag, REFUSES to write oracle "bash" for a row marked `migrated`,
//     REFUSES to write it for a file whose shape is the ≤10-line FR-004 exec
//     shim, and pins `oracle_sha256` over the executed bytes so a reviewer can
//     confirm the claim out of git history. Diff --require-go then refuses to
//     treat a bash-against-bash self-diff as a migration gate at all.
//
//  2. EDIT THE INPUT UNDER THE RECORDING. Change a payload fixture and the
//     recording's bytes do not move, so the gate keeps comparing against an
//     oracle recorded from input that no longer exists. Defence:
//     scenario_input_sha256, recomputed from the scenario's CURRENT inputs at
//     diff time and refused on mismatch.
//
//  3. NORMALIZE THE FAILURE AWAY. A stubborn diff goes green if you add one
//     more normalization pass. Defence: normalizer_version, refused on
//     mismatch, plus TestNormalizerPassListIsPinned — a digest over every
//     pass's token and pattern, so changing what a pass strips fails a test
//     that names the version bump and the re-record it forces.
//
//  4. COMPARE NOTHING. An empty matrix, a scenario filter that matches no
//     scenario, or a coverage check that examines no axis all exit 0 by
//     default. Defence: INV-013 — every entry point counts its subjects and
//     fails when the count is zero.
//
// # SSP-002: THIS PACKAGE AND internal/hook DO NOT IMPORT EACH OTHER
//
// Not a style rule. Both link into ONE ~11MB binary, and that binary is
// re-executed on every hook fire. Anything this package did at package-init
// time — a compiled regexp, a parsed file, a populated map — would be paid by
// every PreToolUse in every session, for tooling that only ever runs offline.
// So: no func init(), no non-trivial package-level state, and no import edge in
// either direction. TestPackageIsolation asserts all three by parsing the
// source of both packages.
//
// The cost is a second reader for templates/hooks/registry.yaml (see
// registry.go). That is deliberate and the shared source of truth is the FILE,
// not a Go type — the two readers are bound by the same schema and by
// TestRegistryReaderAgreesWithFile, not by a shared struct.
//
// # HERMETICITY (INV-007)
//
// The hooks this package executes write to $HOME. session-start.sh TRUNCATES
// $HOME/.edikt/gate-overrides.jsonl on line 28; six hooks append under
// $HOME/.edikt/; session-start.sh reads the maintainer's private auto-memory
// out of $HOME/.claude/projects/<encoded>/memory/MEMORY.md. Recording must
// therefore never run them against the developer's real home. Every scenario
// gets HOME redirected into its own sandbox, the fake home is captured in the
// SAME fs_diff as the project tree (so a $HOME write is an observable, not an
// invisible one), the child environment is built from an allowlist rather than
// inherited, and every artifact is scanned for the real home path before it is
// written to disk.
package hookcorpus

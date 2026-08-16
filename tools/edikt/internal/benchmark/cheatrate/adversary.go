// Adversary request/result contract for the cheat-rate benchmark.
//
// This file used to hold the ADR-040 §6.4 dispatcher itself. ADR-044  // edikt-guard:allow
// removed it: a tier-2 binary must never spawn an LLM CLI (INV-012), so
// the cheat-rate-adversary subagent (template at
// `templates/agents/cheat-rate-adversary.md`) is now dispatched from
// tier-1 via the host's Task primitive.
//
// What remains is the seam and the guardrails around it:
//
//   - AdversaryRequest / AdversaryResult — the shapes crossing the
//     Dispatcher boundary declared in run.go. The caller supplies the
//     dispatch func; this package never performs one.
//   - Validate — INV-006 hygiene on every author-controlled field.  // edikt-guard:allow
//     Still required, and arguably more so: the values now flow into a
//     prompt assembled outside this package, so this is the last place
//     that can reject null bytes, ASCII control characters (except tab
//     and newline), and oversize prompt-bomb inputs.
//
// Nothing here decides cheated / not_cheated; that is the verdict
// layer's job. Nothing here runs a subprocess, builds an argv, or reads
// the agent template. AdversaryResult's ExitCode / RawStdout /
// RawStderr / TimedOut fields describe what the INJECTED dispatcher
// reports back, not anything this package observes.
package cheatrate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// DefaultAdversaryBudget is the per-run wall-clock budget an injected
// Dispatcher is expected to apply when the caller's context carries no
// shorter deadline. ADR-040 §6.4 mandates 5 minutes. This package no  // edikt-guard:allow
// longer enforces it — it publishes the number the dispatcher honours.
const DefaultAdversaryBudget = 5 * time.Minute

// Author-controlled text fields are NFKC-normalized before length
// checks. These caps are generous enough for real intent /
// falsifying_observation prose (which the cheatability rule encourages
// to be one-line) and verify shell commands, but small enough to bound
// the prompt size and reject prompt-bomb inputs.
const (
	maxIntentLen                = 2000
	maxFalsifyingObservationLen = 2000
	maxVerifyCommandLen         = 4000
)

var (
	// SidecarID — same shape as idvalidate.ArtifactID (ADR-NNN /
	// INV-NNN / a guideline slug). Local copy because cheatrate is
	// a benchmark concern and idvalidate is scoped to the
	// extractor/migrate dispatch path; keeping them independent
	// avoids cross-package coupling that would break the no-LLM
	// boundary if idvalidate ever gained dispatch-shaped helpers.
	sidecarIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,80}$`)

	// AdversaryModel — Anthropic model id shape. Lowercase alpha
	// followed by lowercase alphanumeric or hyphen. The default
	// "claude-opus-4-7" is ADR-040 §7's locked value; the regex  // edikt-guard:allow
	// keeps the field tight enough that argv injection through a
	// rogue --model= string cannot escape.
	adversaryModelPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,40}$`)
)

// AdversaryRequest carries the inputs DispatchAdversary needs to dispatch
// a single adversary run for one verify entry under one sidecar.
type AdversaryRequest struct {
	// SidecarID identifies the sidecar under test ("ADR-NNN",
	// "INV-NNN", or a guideline slug). Used only for trace
	// filename naming; never interpolated into the adversary
	// prompt body. Required.
	SidecarID string

	// VerifyIdx is the 0-indexed position of the verify within
	// the sidecar's verifies array. Used only for trace filename
	// naming and per-run sandbox subpath disambiguation.
	VerifyIdx int

	// SandboxPath is the absolute path to the per-run hermetic
	// sandbox the adversary may edit. Must be absolute, must not
	// contain "..", must not include shell metacharacters.
	// Becomes cmd.Dir for the adversary subprocess and is also
	// interpolated into the prompt template as {{SANDBOX_PATH}}.
	SandboxPath string

	// Intent is the directive's `intent:` field — the one-line
	// semantic claim the verify is meant to falsify. Interpolated
	// into the prompt as {{DIRECTIVE_INTENT}}.
	Intent string

	// FalsifyingObservation is the directive's
	// `falsifying_observation:` field — what a violation looks
	// like in the world. Interpolated as
	// {{FALSIFYING_OBSERVATION}}.
	FalsifyingObservation string

	// VerifyCommand is the exact shell command the verify executes.
	// Interpolated as {{VERIFY_COMMAND}}.
	VerifyCommand string

	// AdversaryModel is the Claude model id passed to `claude --model`.
	// Defaults to ADR-040's locked value ("claude-opus-4-7") if empty.  // edikt-guard:allow
	// Override only by the operator via --adversary-model.
	AdversaryModel string

	// TemplatePath is the absolute path to the adversary template
	// (canonical: `templates/agents/cheat-rate-adversary.md`). The
	// dispatcher reads this file and performs string replacement
	// on the four interpolation markers; nothing else.
	TemplatePath string
}

// AdversaryResult captures the outcome of one adversary run. The
// dispatcher does NOT classify cheated / not_cheated — that is the
// verdict layer's job, which inspects ExitCode + the contents of
// SandboxPath + the negative fixture.
type AdversaryResult struct {
	// ExitCode is the adversary subprocess exit code. 0 in the
	// common case (the subagent emitted its JSON verdict and
	// returned cleanly). Non-zero indicates a subprocess error
	// (e.g., claude CLI not authenticated, network failure).
	// Meaningful only when TimedOut is false.
	ExitCode int

	// TracePath is the absolute path to the trace file the
	// dispatcher wrote. The file contains both stdout (the
	// adversary's JSON verdict, if any) and stderr (logs, errors)
	// for post-hoc inspection.
	TracePath string

	// ElapsedMs is the wall-clock duration of the subprocess in
	// milliseconds (from cmd.Start to cmd.Wait).
	ElapsedMs int64

	// TimedOut is true iff the per-run budget elapsed before the
	// subprocess finished. When true, ExitCode is meaningless and
	// the per-verify orchestration layer should treat this run
	// as inconclusive for majority-vote purposes (ADR-040 §6.6).  // edikt-guard:allow
	TimedOut bool

	// RawStdout is the captured stdout of the subprocess. Useful
	// to the verdict layer for parsing the JSON verdict envelope.
	RawStdout string

	// RawStderr is the captured stderr of the subprocess. Useful
	// for diagnostics when ExitCode != 0 or TimedOut is true.
	RawStderr string
}

// Validate enforces INV-006 on every field that flows into argv or  // edikt-guard:allow
// into the prompt body. The caller (DispatchAdversary) calls this
// first; if it returns an error, no subprocess is spawned.
//
// Rejection rules:
//
//   - SidecarID must match the canonical artifact ID shape.
//   - SandboxPath must be absolute, must not contain "..", must
//     contain only path-safe characters.
//   - AdversaryModel must match the narrow model-id allowlist.
//   - TemplatePath must be absolute and the file must be readable.
//   - Intent / FalsifyingObservation / VerifyCommand: not empty,
//     no null bytes, no ASCII control chars (except tab and newline),
//     and post-NFKC length within their per-field cap.
func (req AdversaryRequest) Validate() error {
	if !sidecarIDPattern.MatchString(req.SidecarID) {
		return fmt.Errorf("adversary request: invalid sidecar_id %q: must match %s",
			req.SidecarID, sidecarIDPattern.String())
	}
	if req.VerifyIdx < 0 {
		return fmt.Errorf("adversary request: verify_idx must be >= 0, got %d", req.VerifyIdx)
	}
	if err := validateSandboxPath(req.SandboxPath); err != nil {
		return err
	}
	model := req.AdversaryModel
	if model == "" {
		// Empty is allowed at the request level — the dispatcher
		// substitutes the ADR-040 default. We still validate the  // edikt-guard:allow
		// substituted value below by checking the regex after
		// normalization.
		model = "claude-opus-4-7"
	}
	if !adversaryModelPattern.MatchString(model) {
		return fmt.Errorf("adversary request: invalid adversary_model %q: must match %s",
			req.AdversaryModel, adversaryModelPattern.String())
	}
	if req.TemplatePath == "" {
		return fmt.Errorf("adversary request: template_path required")
	}
	if !filepath.IsAbs(req.TemplatePath) {
		return fmt.Errorf("adversary request: template_path must be absolute, got %q", req.TemplatePath)
	}
	if err := validateAuthorText(req.Intent, "intent", maxIntentLen); err != nil {
		return err
	}
	if err := validateAuthorText(req.FalsifyingObservation, "falsifying_observation", maxFalsifyingObservationLen); err != nil {
		return err
	}
	if err := validateAuthorText(req.VerifyCommand, "verify_command", maxVerifyCommandLen); err != nil {
		return err
	}
	return nil
}

// validateSandboxPath enforces INV-007's sandbox-path discipline plus  // edikt-guard:allow
// INV-006's no-shell-meta rule.  // edikt-guard:allow
func validateSandboxPath(p string) error {
	if p == "" {
		return fmt.Errorf("adversary request: sandbox_path required")
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("adversary request: sandbox_path must be absolute, got %q", p)
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("adversary request: sandbox_path must not contain '..' (got %q)", p)
	}
	// Reject shell metacharacters and most non-printable input. Path
	// is allowed to contain ASCII letters, digits, '/', '_', '-',
	// '.', and space. Note we still disallow ".." above; the dot
	// allowance here is for "." files (e.g. .edikt) inside the
	// sandbox, which is fine because the sandbox creation primitive
	// (CreateSandbox in cheatrate.go) refuses to copy them in.
	for _, r := range p {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '/' || r == '_' || r == '-' || r == '.' || r == ' ':
		default:
			return fmt.Errorf("adversary request: sandbox_path contains disallowed character %q (in %q)", r, p)
		}
	}
	return nil
}

// validateAuthorText runs the INV-006 hygiene check against an  // edikt-guard:allow
// author-controlled text field. The three fields covered (intent,
// falsifying_observation, verify_command) are not allowlist-restricted
// at the character level — they're meant to be prose / shell — but they
// must not contain null bytes, must not contain ASCII control
// characters other than tab/newline, and must fit within their cap
// after NFKC normalization.
func validateAuthorText(s, fieldName string, maxLen int) error {
	if s == "" {
		return fmt.Errorf("adversary request: %s required", fieldName)
	}
	if strings.ContainsRune(s, 0) {
		return fmt.Errorf("adversary request: %s contains null byte", fieldName)
	}
	normalized := norm.NFKC.String(s)
	if len(normalized) > maxLen {
		return fmt.Errorf("adversary request: %s exceeds %d bytes after NFKC normalization (got %d)",
			fieldName, maxLen, len(normalized))
	}
	for _, r := range normalized {
		if r < 0x20 && r != '\t' && r != '\n' {
			return fmt.Errorf("adversary request: %s contains disallowed control character U+%04X", fieldName, r)
		}
	}
	return nil
}

// DispatchAdversary was removed by ADR-044.
//
// It shelled out to `claude -p` from inside the tier-2 binary, which INV-012
// forbids: a tier-2 binary never spawns an LLM CLI. Hard-coding `claude` also
// forecloses every other host agent, which is the portability INV-012 exists
// to protect.
//
// The adversary is now dispatched from tier-1 (commands/gov/benchmark.md) via
// the host's Task primitive. What stays here is deterministic and testable:
// the request/result shapes, request validation, and verdict aggregation. The
// binary plans the work and scores the outcome; it does not summon the judge.
//
// buildAdversaryPrompt and assembleTrace went with it. Both existed only to
// serve the subprocess: the prompt builder stripped the template's YAML
// frontmatter because a leading `---` made the CLI read the prompt as an
// unknown flag, and the trace assembler formatted the subprocess's stdout,
// stderr, and exec error. Under Task dispatch there is no CLI to confuse and
// no subprocess streams to record, so neither had an input left to take.
// Tier-1 owns prompt construction now.
//
// buildTracePath followed them. It computed the AC-1.5 trace filename for
// a trace this package no longer writes — assembleTrace was its only
// consumer. staticcheck never flagged it because a test called it, and
// that test asserted nothing else: its whole function was to keep the
// function reachable. A test that exists only to hold dead code up turns
// U1000 from a signal into a formality, so it went with the code.

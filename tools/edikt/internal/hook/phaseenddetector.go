package hook

// phaseenddetector.go — the Go half of templates/hooks/phase-end-detector.sh
// (PLAN-v080-reliability-core Phase 17).
//
// THIS HOOK IS THE ONE THE ADR-063 AMENDMENT WAS WRITTEN FOR
//
// Twenty-one of the twenty-two hooks are a Go handler behind a ten-line exec
// shim. This one is not, and the reason is an invariant rather than a
// preference: phase-end-detector is the only hook in the tier that execs the
// host's model CLI, at three sites, and INV-012 forbids a tier-2 binary from
// doing that. So the work is split down the middle —
//
//	Go   detection, every byte of JSON, every verdict, every artifact
//	bash the three dispatches, and the state handoff between them
//
// — and neither side crosses. There is no model invocation anywhere in this
// file, and there is no JSON construction anywhere in the caller.
//
// # THE STEP PROTOCOL
//
// The caller runs `hook run phase-end-detector --step N`, and a step either
// returns the final reply (exit 0) or asks for a dispatch (exit 10, with the
// path of a state file on stdout and a prompt file beside it). Four steps:
//
//	1 detect        the whole hook up to the first thing needing a dispatch
//	2 post-rebuild  consumes the sidecar-regeneration exit; builds the evaluator
//	3 post-eval     consumes the evaluator output; verdict, gates, kill-switches
//	4 post-flight   consumes the post-flight output; telemetry and the reply
//
// `--step` absent means step 1, so the corpus recorder's own bare
// `hook run phase-end-detector` executes the same first leg the caller does
// rather than a second entry point nothing else uses.
//
// # WHY THE STATE FILE CARRIES A DIGEST
//
// The state file is new mutable cross-step state, and it is the one thing this
// shape adds that the exec-shim shape does not have. A step 3 that ran against
// a state file left by a DIFFERENT invocation would compute a verdict over the
// wrong phase and persist it — silently, since every field would look
// well-formed. Every step therefore recomputes the payload digest from its own
// stdin and REFUSES a state file that does not match. The prefix is distinct
// from the dedup guard's so the two digests can never be confused for each
// other.
//
// # WHAT THE COMMITTED CORPUS CANNOT REACH
//
// The shared sandbox carries no plans directory (seeding one would rewrite
// eighteen other hooks' recordings and change stop-hook's own behaviour), and
// this hook resolves its plan file before every remaining branch. So the
// corpus characterizes the first fifth of the hook and nothing after it. The
// rest — the plan ladder, the detector, the sidecar regeneration, the evidence
// gate, the kill-switch hierarchy, the binary ladder, the scope journal — is
// covered by phaseenddetector_test.go with positive controls, and each gap is
// named in test/fixtures/hook-corpus/phase-end-detector/matrix.yaml's header
// beside the test that closes it.
//
// # PORTED, NOT IMPROVED
//
// The charter is byte-for-byte parity with the bash. Where the bash does
// something odd, this file does the same odd thing and says so. The
// divergences that could NOT be preserved are collected at the bottom of this
// comment block rather than left to be discovered:
//
//   - A malformed `meta` in an evaluator verdict makes python raise mid-write
//     and abandon the reply entirely. ADR-063 requires the Go tier to fail
//     open, so a non-object meta is replaced rather than fatal.
//   - The evaluator's output is truncated to 200 lines HERE rather than by a
//     `| head -200` in the caller. Identical bytes; it also removes the
//     pipeline's SIGPIPE case, where a >200-line evaluator could make the
//     recorded exit status 141 instead of the model CLI's own.
//   - A dispatch the caller could not run because the tool is absent arrives
//     as exit 127 with `--dispatch-available=false`, and renders as "<tool>
//     CLI not installed" — the bash's own `command -v` wording. A tool that
//     exists and genuinely exits 127 therefore reads the same way.
//   - The state file for an EXISTING plan-eval JSON is rewritten in this
//     file's field order rather than in the order the previous writer used.
//     Its consumer is a parser, and no gate compares its bytes.
//   - The config opt-out no longer depends on `grep` being on PATH. The bash
//     asked its two questions by running grep, so a stripped PATH made BOTH
//     return 127, the guard degraded OPEN, and a project that had explicitly
//     disabled phase-end evaluation ran anyway. Measured, not theorised: a
//     candidate corpus scenario combining the config opt-out with the
//     no-python3 PATH recorded the bash emitting the interpreter banner for a
//     project that had opted out. That scenario was dropped rather than
//     recorded — pinning it would freeze a guard failing open — and the
//     ordering it was meant to pin is covered by no-python3-skip-phase-eval,
//     whose opt-out is a pure shell test with no external command.
//   - The missing-evaluator-template message no longer advises an install
//     command tracking a moving ref. See phaseEndReinstallHint.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("phase-end-detector", phaseEndDetector)
}

// phaseEndDispatchExit is the "dispatch and call me back" status. Any other
// non-zero exit is a real failure and the caller fails open, exactly as the
// bash hook does today.
const phaseEndDispatchExit = 10

// phaseEndDedupTTL and phaseEndDedupPrune mirror the two literals in the
// duplicate-invocation guard: `-lt 15` seconds and `find … -mmin +5`.
const (
	phaseEndDedupTTL   = 15 * time.Second
	phaseEndDedupPrune = 5 * time.Minute
)

// phaseEndNoPython3Message is the literal text the preflight printf carries.
// The wire shape around it is rendered by adapter.NoPython3ContinueBanner.
const phaseEndNoPython3Message = "⚠ edikt: python3 is missing — phase-completion evaluation was skipped this turn. If you just claimed a phase is complete, that claim was NOT checked against verification evidence (INV-009)."

// phaseEndStateDigestPrefix is the digest domain for the cross-step state
// file. DISTINCT from the dedup guard's `<name>:` prefix on purpose: the two
// hash the same bytes for different questions, and a shared prefix would let a
// dedup token name collide with a state-file identity check.
const phaseEndStateDigestPrefix = "phase-end-detector:step-state:"

var (
	// The in-progress progress-table row: `grep -iE '\| *(Phase )?[0-9]+ *\|.*in[_ -]progress'`.
	phaseEndRowRe = regexp.MustCompile(`(?i)\| *(Phase )?[0-9]+ *\|.*in[_ -]progress`)
	// The digit runs `grep -oE '[0-9]+'` extracts from the phase cell.
	phaseEndDigitsRe = regexp.MustCompile(`[0-9]+`)
	// The plan-stem allowlist gate (INV-006): `''|*[!A-Za-z0-9._-]*`.
	phaseEndStemRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	// The criteria-sidecar block splitter and its two field probes.
	phaseEndBlockSplitRe  = regexp.MustCompile(`(?m)^\s*-\s+`)
	phaseEndSidecarIDRe   = regexp.MustCompile(`\bid:\s*['"]?([A-Za-z0-9_.-]+)`)
	phaseEndSidecarVerRe  = regexp.MustCompile(`\bverify:\s*['"]?([^'"\n]+)`)
	phaseEndTestCommandRe = regexp.MustCompile(`\b(pytest|bash|make |npm test|\./test/|uv run)\b`)
	// The two post-flight field validators. Both are anchored with \A/\z and
	// paired with a one-newline trim at the call site, because python's `$`
	// also matches before a single trailing newline and Go's does not.
	phaseEndReportPathRe  = regexp.MustCompile(`\A[A-Za-z0-9._/-]{1,256}\z`)
	phaseEndStatusTokenRe = regexp.MustCompile(`\A[A-Za-z][A-Za-z0-9_-]{0,23}\z`)
	// The evaluator-model lookup: `grep -E '^\s*model:'` within `grep -A10 '^evaluator:'`.
	phaseEndModelRe = regexp.MustCompile(`^\s*model:`)
)

// phaseEndCleanStatuses is the renderer's `_CLEAN_STATUSES` set. A post-flight
// whose L2/L3/synthesis status is anything else must not render as a bare
// "completed" — that was absence-of-output displayed as clean (INV-011).
var phaseEndCleanStatuses = map[string]bool{
	"pass": true, "passed": true, "ok": true, "complete": true, "completed": true,
}

// phaseEndAuthMarkers are the substrings that make a missing verdict an
// evaluator AUTH failure rather than a generic one, in the order the bash
// tests them.
var phaseEndAuthMarkers = []string{
	"not logged in", "please run /login", "invalid api key",
	"authentication_error", "oauth token has expired",
}

// ── the cross-step state ────────────────────────────────────────────────────

// phaseEndState is everything a later step needs and cannot recompute.
//
// Emit is the stdout the run has ALREADY produced. The bash prints the
// sidecar-rebuild notice and the missing-history warning as they happen, ahead
// of a dispatch that may take minutes; the caller cannot hold them (holding
// them would mean parsing something) so they ride in the state and are flushed
// in front of the final reply. The host reads the stream once at exit, so the
// bytes and their order are identical either way.
type phaseEndState struct {
	SchemaVersion int    `json:"schema_version"`
	PayloadDigest string `json:"payload_digest"`
	Emit          string `json:"emit"`
	Root          string `json:"root"`
	Base          string `json:"base"`
	PlanDir       string `json:"plan_dir"`
	PlanFile      string `json:"plan_file"`
	PlanStem      string `json:"plan_stem"`
	PhaseNum      string `json:"phase_num"`
	Sidecar       string `json:"sidecar"`
	SidecarGen    string `json:"sidecar_gen_status"`
	EvalTS        string `json:"eval_ts"`
	L1Output      string `json:"l1_output"`
	PostFlightT0  int64  `json:"post_flight_t0"`
	PostFlightTMO string `json:"post_flight_timeout"`
}

// phaseEndRun is one step's working context.
type phaseEndRun struct {
	inv    *Invocation
	digest string
	state  phaseEndState
	runDir string
}

// phaseEndDetector dispatches to the requested step.
func phaseEndDetector(inv *Invocation) Result {
	step := 1
	if inv.Step != nil && inv.Step.Step > 0 {
		step = inv.Step.Step
	}
	switch step {
	case 1:
		return phaseEndStep1(inv)
	case 2, 3, 4:
		r, res, ok := phaseEndResume(inv, step)
		if !ok {
			return res
		}
		switch step {
		case 2:
			return phaseEndStep2(r)
		case 3:
			return phaseEndStep3(r)
		default:
			return phaseEndStep4(r)
		}
	}
	fmt.Fprintf(inv.Stderr,
		"edikt: phase-end-detector — --step %d is not a step of this protocol (want 1-4); nothing ran.\n", step)
	return Result{ExitCode: 3}
}

// phaseEndResume loads the state file for steps 2-4 and REFUSES one whose
// digest does not match this invocation's own payload.
//
// A mismatch exits non-zero WITHOUT a reply, which the caller reads as a real
// failure and fails open on. That is deliberately louder than continuing: a
// verdict computed over a different phase's run would be persisted to disk and
// look entirely well-formed.
func phaseEndResume(inv *Invocation, step int) (*phaseEndRun, Result, bool) {
	if inv.Step == nil || inv.Step.StatePath == "" {
		fmt.Fprintf(inv.Stderr,
			"edikt: phase-end-detector — step %d requires --state; nothing ran.\n", step)
		return nil, Result{ExitCode: 3}, false
	}
	raw, err := os.ReadFile(inv.Step.StatePath) //nolint:gosec // caller-supplied step-protocol path
	if err != nil {
		fmt.Fprintf(inv.Stderr,
			"edikt: phase-end-detector — step %d could not read its state file %s: %v; nothing ran.\n",
			step, inv.Step.StatePath, err)
		return nil, Result{ExitCode: 3}, false
	}
	var st phaseEndState
	if uerr := json.Unmarshal(raw, &st); uerr != nil {
		fmt.Fprintf(inv.Stderr,
			"edikt: phase-end-detector — step %d could not decode its state file %s: %v; nothing ran.\n",
			step, inv.Step.StatePath, uerr)
		return nil, Result{ExitCode: 3}, false
	}
	digest := inv.Payload.PayloadDigest(phaseEndStateDigestPrefix)
	if st.PayloadDigest != digest {
		fmt.Fprintf(inv.Stderr,
			"edikt: phase-end-detector — step %d REFUSED its state file: %s was written for a\n"+
				"  DIFFERENT payload (state %s, this invocation %s). Continuing would compute a\n"+
				"  verdict over another run's phase and persist it. Nothing ran.\n",
			step, inv.Step.StatePath, phaseEndShortDigest(st.PayloadDigest), phaseEndShortDigest(digest))
		return nil, Result{ExitCode: 3}, false
	}
	return &phaseEndRun{
		inv:    inv,
		digest: digest,
		state:  st,
		runDir: filepath.Dir(inv.Step.StatePath),
	}, Result{}, true
}

func phaseEndShortDigest(d string) string {
	if len(d) > 12 {
		return d[:12]
	}
	if d == "" {
		return "(none)"
	}
	return d
}

// ── step 1: detect ──────────────────────────────────────────────────────────

func phaseEndStep1(inv *Invocation) Result {
	// Every path in this hook is relative to the PROCESS's working directory,
	// never the one the host reported — the foreign-cwd scenario pins it.
	root, err := os.Getwd()
	if err != nil {
		return Result{}
	}
	configPath := filepath.Join(root, ".edikt", "config.yaml")

	// ── guard 1: only run in edikt projects ─────────────────────────────
	if !fileExists(configPath) {
		return Result{}
	}
	configBytes, _ := os.ReadFile(configPath) //nolint:gosec // project-local config
	config := string(configBytes)

	// ── guard 2: the documented opt-out ─────────────────────────────────
	// `phase-end: true` is asked FIRST and the false-check is only consulted
	// when the answer is no; both anchor at start-of-line, so a commented-out
	// line never matches. phase-end-true-and-false and phase-end-commented
	// pin the two halves of that.
	if !phaseEndConfigHas(config, `^\s*phase-end:\s*true`) {
		if phaseEndConfigHas(config, `^\s*phase-end:\s*false`) {
			return Result{}
		}
	}

	// ── guard 3: the test/debug override ────────────────────────────────
	// An exact match on "1", not a set-and-non-empty test. skip-phase-eval
	// and skip-phase-eval-zero are the two halves.
	if os.Getenv("EDIKT_SKIP_PHASE_EVAL") == "1" {
		return Result{}
	}

	// ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────
	// This branch is BEHAVIOUR, not an implementation detail the Go rewrite
	// gets to drop: a project whose python3 is gone gets a banner naming
	// INV-009 rather than the silence that used to let a real completion
	// claim pass unevaluated. It runs AFTER stdin is read and after the three
	// guards above, which is observable — the corpus pins both facts.
	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr,
			"edikt: phase-end-detector — python3 not found; phase-completion evaluation SKIPPED for this turn")
		return Result{Response: adapter.Response{Text: adapter.NoPython3ContinueBanner(phaseEndNoPython3Message)}}
	}

	// ── duplicate-invocation guard (bok-services field bug #4) ──────────
	if phaseEndDedupSuppresses(inv, root) {
		return Result{}
	}

	// ── re-entrancy guard ───────────────────────────────────────────────
	if inv.Payload.StopActive {
		return Result{}
	}

	lastMsg := strings.TrimSpace(inv.Payload.LastAssistantMessage)
	if lastMsg == "" {
		return Result{}
	}

	// ── find the active plan ────────────────────────────────────────────
	base := phaseEndConfigField(config, `^base:`)
	if base == "" {
		base = "docs"
	}
	planDir := phaseEndConfigField(config, `^  plans:`)
	if planDir == "" {
		planDir = base + "/plans"
	}

	planFile := phaseEndLatestPlan(root, phaseEndPlanDirs(base, planDir))
	if planFile == "" {
		return phaseEndContinue()
	}

	planBytes, rerr := os.ReadFile(filepath.Join(root, planFile)) //nolint:gosec // config-declared plan path
	if rerr != nil {
		return phaseEndContinue()
	}
	phaseLine := phaseEndFirstMatch(string(planBytes), phaseEndRowRe)
	if phaseLine == "" {
		return phaseEndContinue()
	}
	phaseNum := phaseEndPhaseNumber(phaseLine)
	if phaseNum == "" {
		return phaseEndContinue()
	}

	// ── detect a completion signal for THIS phase ───────────────────────
	if !phaseEndCompletionDetected(lastMsg, phaseNum) {
		return phaseEndContinue()
	}

	// ── ownership guard (security review Finding 4) ─────────────────────
	// Everything past here MAY dispatch the model CLI against repo-controlled
	// plan and criteria content. Refuse to arm that when the project's config
	// belongs to somebody else — a config planted by another user on a shared
	// host, not a repo you cloned yourself, which is the trust gate's job.
	if owned, known := phaseEndOwnedByUs(configPath); known && !owned {
		return phaseEndContinue()
	}

	planStem := strings.TrimSuffix(filepath.Base(planFile), ".md")
	if !phaseEndStemRe.MatchString(planStem) {
		// INV-003: the untrusted value is passed as data to the serializer,
		// never concatenated into a JSON template.
		msg := "edikt: plan filename " + planStem +
			" has an unsafe shape (must match [A-Za-z0-9._-]+) — phase-end-detector aborting."
		return phaseEndReply(phaseEndContinueMessage(msg), false)
	}

	sidecar := phaseEndFindSidecar(root, phaseEndPlanDirs(base, planDir), planStem)

	// ── log the detection, whatever happens next ────────────────────────
	phaseEndLogEvent(phaseEndDetectEvent{
		TS:    phaseEndTimestamp(),
		Event: "phase_completion_detected",
		Plan:  filepath.Base(planFile),
		Phase: phaseEndNumberOrString(phaseNum),
	})

	st := phaseEndState{
		SchemaVersion: 1,
		PayloadDigest: inv.Payload.PayloadDigest(phaseEndStateDigestPrefix),
		Root:          root,
		Base:          base,
		PlanDir:       planDir,
		PlanFile:      planFile,
		PlanStem:      planStem,
		PhaseNum:      phaseNum,
		Sidecar:       sidecar,
		SidecarGen:    "not_attempted",
	}
	r := &phaseEndRun{inv: inv, digest: st.PayloadDigest, state: st}

	// ── auto-generate the criteria sidecar if it is missing ─────────────
	dryRun := os.Getenv("EDIKT_EVALUATOR_DRY_RUN") == "1"
	if sidecar == "" &&
		phaseEndToolAvailable(inv) &&
		!dryRun &&
		os.Getenv("EDIKT_SKIP_SIDECAR_REGEN") != "1" {
		r.state.Emit += "🔧  Evaluation history missing for " + planStem + ".md — rebuilding it now...\n"
		return phaseEndRequestDispatch(r, phaseEndDispatch{
			Prompt: "/edikt:sdlc:plan --sidecar-only " + planStem,
		})
	}
	return phaseEndAfterRebuild(r)
}

// ── step 2: after the sidecar-rebuild dispatch ──────────────────────────────

func phaseEndStep2(r *phaseEndRun) Result {
	if r.inv.Step.DispatchExit == 0 {
		r.state.SidecarGen = "attempted"
		if found := phaseEndFindSidecar(r.state.Root,
			phaseEndPlanDirs(r.state.Base, r.state.PlanDir), r.state.PlanStem); found != "" {
			r.state.Sidecar = found
			r.state.SidecarGen = "success"
		}
	} else {
		r.state.SidecarGen = "failed"
	}
	return phaseEndAfterRebuild(r)
}

// phaseEndAfterRebuild is the shared tail of steps 1 and 2 — the point the
// bash reaches whether or not it attempted a regeneration.
func phaseEndAfterRebuild(r *phaseEndRun) Result {
	if r.state.Sidecar == "" {
		r.state.Emit += phaseEndMissingSidecarWarning(
			r.state.PhaseNum, filepath.Base(r.state.PlanFile), r.state.SidecarGen) + "\n"
	}

	// ── dry-run mode (for testing) ──────────────────────────────────────
	if os.Getenv("EDIKT_EVALUATOR_DRY_RUN") == "1" {
		sc := r.state.Sidecar
		if sc == "" {
			sc = "(none)"
		}
		msg := "⚙️  Phase " + r.state.PhaseNum + " completion detected (dry-run).\n" +
			"    Plan: " + r.state.PlanFile + "\n" +
			"    Sidecar: " + sc
		return phaseEndReply(r.state.Emit+phaseEndSystemMessage(msg), false)
	}

	// ── resolve the evaluator template ──────────────────────────────────
	// Project-mode canonical path first, this repo's own dogfooding
	// convention second, the global install root last. The global rung reads
	// literal $HOME, never EDIKT_ROOT/EDIKT_HOME: those are set independently
	// of HOME by real overrides AND by this repo's test sandbox, and
	// preferring them here would stop finding a real, populated $HOME/.edikt.
	tmpl := phaseEndEvaluatorTemplate(r.state.Root)
	if tmpl == "" {
		msg := "⚠️  Phase completion detected but evaluator template missing.\n" +
			"    Expected: .edikt/current/templates/agents/evaluator-headless.md\n" +
			"    Re-run: " + phaseEndReinstallHint()
		return phaseEndReply(r.state.Emit+phaseEndSystemMessage(msg), false)
	}

	sys, rerr := os.ReadFile(tmpl) //nolint:gosec // resolved from a fixed candidate ladder
	if rerr != nil {
		sys = nil
	}

	prompt := phaseEndEvaluatorPrompt(r.state.PhaseNum, r.state.PlanFile, r.state.Sidecar)

	// ── the evaluator model, allowlisted before it reaches argv ─────────
	model := phaseEndConfiguredModel(r.state.Root)
	if model == "" {
		model = adapter.EvaluatorModelDefault
	}
	if !adapter.EvaluatorModelAllowed(model) {
		r.state.Emit += phaseEndSystemMessage(
			"edikt: evaluator.model " + pythonRepr(model) +
				" is not in the allowlist (opus/sonnet/haiku and full model IDs). Falling back to sonnet.")
		model = adapter.EvaluatorModelDefault
	}

	r.state.EvalTS = phaseEndTimestamp()
	return phaseEndRequestDispatch(r, phaseEndDispatch{
		Prompt:       prompt,
		SystemPrompt: string(sys),
		HasSystem:    true,
		Model:        model,
		Timeout:      phaseEndEvalTimeout(),
	})
}

// ── step 3: after the evaluator dispatch ────────────────────────────────────

func phaseEndStep3(r *phaseEndRun) Result {
	rawOut := phaseEndHeadLines(phaseEndReadDispatch(r.inv.Step.DispatchOut), 200)

	phaseEndLogEvent(phaseEndEvalEvent{
		TS:    r.state.EvalTS,
		Event: "phase_evaluation",
		Plan:  filepath.Base(r.state.PlanFile),
		Phase: phaseEndNumberOrString(r.state.PhaseNum),
		Exit:  r.inv.Step.DispatchExit,
	})

	l1 := phaseEndL1Output(r, rawOut)
	r.state.L1Output = l1

	// ── kill-switch 1: the env var, highest precedence ──────────────────
	enabled := os.Getenv("EDIKT_DISABLE_POST_FLIGHT") != "1"

	// ── kill-switch 2: the config gate, via the tier-2 scope query ──────
	if enabled {
		enabled = phaseEndPostFlightScopeEnabled(r.state.Root)
	}

	verdict := phaseEndL1Verdict(l1)
	if !enabled || verdict != "PASS" {
		// The byte-identical L1 contract: `printf '%s'`, no trailing newline.
		if r.state.Emit == "" && l1 == "" {
			return Result{}
		}
		return phaseEndReply(r.state.Emit+l1, true)
	}

	r.state.PostFlightT0 = time.Now().Unix()
	r.state.PostFlightTMO = phaseEndPostFlightTimeout()
	if !phaseEndToolAvailable(r.inv) {
		// The bash checks `command -v` before dispatching at all; the caller
		// makes the same check and reports it, so this leg still runs.
		return phaseEndStep4Reason(r, phaseEndToolName(r.inv)+" CLI not installed", "")
	}
	return phaseEndRequestDispatch(r, phaseEndDispatch{
		Prompt:  "/edikt:sdlc:post-flight " + r.state.PlanStem + " --phase " + r.state.PhaseNum,
		Timeout: r.state.PostFlightTMO,
	})
}

// ── step 4: after the post-flight dispatch ──────────────────────────────────

func phaseEndStep4(r *phaseEndRun) Result {
	out := phaseEndReadDispatch(r.inv.Step.DispatchOut)
	out = strings.TrimRight(out, "\n")
	code := r.inv.Step.DispatchExit
	tool := phaseEndToolName(r.inv)

	reason := ""
	switch {
	case code == 124:
		reason = "timed out after " + r.state.PostFlightTMO + "s"
	case code == 127 && !r.inv.Step.DispatchAvailable:
		reason = tool + " CLI not installed"
	case code != 0:
		reason = tool + " exited " + strconv.Itoa(code)
	case out == "":
		reason = "no response"
	case !json.Valid([]byte(out)):
		reason = "malformed response"
	}
	return phaseEndStep4Reason(r, reason, out)
}

// phaseEndStep4Reason renders the telemetry line and the combined reply.
// Shared with step 3's tool-absent short circuit, which never dispatches and
// therefore has no output to inspect.
func phaseEndStep4Reason(r *phaseEndRun, reason, out string) Result {
	elapsed := (time.Now().Unix() - r.state.PostFlightT0) * 1000
	phaseEndAppendMetrics(r.state, reason, elapsed)
	return phaseEndReply(r.state.Emit+phaseEndCombined(r.state.L1Output, reason, out), false)
}

// ── the dispatch request ────────────────────────────────────────────────────

// phaseEndDispatch is everything the caller needs to perform one dispatch that
// this file may legitimately supply: the DATA. The argv shape — which flags,
// which tool, which redirection — belongs to the caller and appears nowhere
// here, which is what keeps INV-012 satisfied with the exemption list
// untouched.
type phaseEndDispatch struct {
	Prompt       string
	SystemPrompt string
	HasSystem    bool
	Model        string
	Timeout      string
}

// phaseEndRequestDispatch writes the run directory and returns exit 10 with
// the state file's path as a bare line on stdout.
//
// A failure to write ANY of it is a real failure, not a silent skip: the
// caller would otherwise dispatch against a stale or absent prompt. It exits
// non-zero without a reply and the caller fails open.
func phaseEndRequestDispatch(r *phaseEndRun, d phaseEndDispatch) Result {
	dir := r.runDir
	if dir == "" {
		parent := filepath.Join(r.state.Root, ".edikt", "state", "phase-end")
		if err := os.MkdirAll(parent, 0o755); err != nil {
			fmt.Fprintf(r.inv.Stderr, "edikt: phase-end-detector — cannot create %s: %v\n", parent, err)
			return Result{ExitCode: 4}
		}
		phaseEndPruneRuns(parent)
		made, err := os.MkdirTemp(parent, "run-")
		if err != nil {
			fmt.Fprintf(r.inv.Stderr, "edikt: phase-end-detector — cannot create a run directory: %v\n", err)
			return Result{ExitCode: 4}
		}
		dir = made
		r.runDir = made
	}

	write := func(name, body string) error {
		return os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600)
	}
	if err := write("prompt", d.Prompt); err != nil {
		fmt.Fprintf(r.inv.Stderr, "edikt: phase-end-detector — cannot write the dispatch prompt: %v\n", err)
		return Result{ExitCode: 4}
	}
	if d.HasSystem {
		if err := write("sysprompt", d.SystemPrompt); err != nil {
			fmt.Fprintf(r.inv.Stderr, "edikt: phase-end-detector — cannot write the system prompt: %v\n", err)
			return Result{ExitCode: 4}
		}
	}
	if d.Model != "" {
		if err := write("model", d.Model); err != nil {
			fmt.Fprintf(r.inv.Stderr, "edikt: phase-end-detector — cannot write the model token: %v\n", err)
			return Result{ExitCode: 4}
		}
	}
	if d.Timeout != "" {
		if err := write("timeout", d.Timeout); err != nil {
			fmt.Fprintf(r.inv.Stderr, "edikt: phase-end-detector — cannot write the timeout: %v\n", err)
			return Result{ExitCode: 4}
		}
	}

	stateBytes, err := json.Marshal(r.state)
	if err != nil {
		fmt.Fprintf(r.inv.Stderr, "edikt: phase-end-detector — cannot encode step state: %v\n", err)
		return Result{ExitCode: 4}
	}
	statePath := filepath.Join(dir, "state.json")
	if werr := os.WriteFile(statePath, stateBytes, 0o600); werr != nil {
		fmt.Fprintf(r.inv.Stderr, "edikt: phase-end-detector — cannot write %s: %v\n", statePath, werr)
		return Result{ExitCode: 4}
	}
	return Result{
		Response: adapter.Response{Text: statePath},
		ExitCode: phaseEndDispatchExit,
	}
}

// phaseEndPruneRuns removes run directories a previous invocation abandoned.
// Best-effort: a directory that cannot be removed is left alone rather than
// turned into a failure of the hook that found it.
func phaseEndPruneRuns(parent string) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "run-") {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil || time.Since(info.ModTime()) < time.Hour {
			continue
		}
		_ = os.RemoveAll(filepath.Join(parent, e.Name()))
	}
}

// ── replies ─────────────────────────────────────────────────────────────────

// phaseEndReply wraps already-serialized bytes as the hook's stdout.
//
// The text is JSON that adapter.MarshalPythonCompact produced; it is carried
// on Response.Text because this hook emits, on some paths, MORE THAN ONE JSON
// object — the missing-history warning followed by the verdict — which no
// single Response value can express. noNewline reproduces the L1-only path's
// `printf '%s'`, which sends no trailing byte at all.
func phaseEndReply(text string, noNewline bool) Result {
	if text == "" {
		return Result{}
	}
	return Result{Response: adapter.Response{Text: text, NoTrailingNewline: noNewline}}
}

func phaseEndContinue() Result {
	return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
}

// phaseEndSystemMessage renders the operator-message reply with a trailing
// newline, which is what python's print() adds. The wire shape itself is the
// adapter's business (SP-004); this function owns only the newline.
func phaseEndSystemMessage(msg string) string {
	b, err := adapter.SystemMessageReply(msg)
	if err != nil {
		return ""
	}
	return string(b) + "\n"
}

func phaseEndContinueMessage(msg string) string {
	b, err := adapter.ContinueSystemMessageReply(msg)
	if err != nil {
		return ""
	}
	return string(b) + "\n"
}

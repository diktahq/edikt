// Package phasea implements the conditional resync phase of two-phase
// gov:compile. It dispatches stale-sidecar regenerations to
// the locked sidecar-extractor agent, one fresh subagent context per
// artifact, parallel up to a concurrency cap, continue-on-error.
package phasea

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// Runner regenerates the sidecar for one artifact. Implementations are
// expected to be deterministic w.r.t. the parent prose body — actual LLM
// drift is bounded by the locked extractor prompt and per-artifact context
// isolation.
//
// The interface exists so tests can inject a fake runner without paying
// real-claude latency or requiring CLI auth in hermetic sandboxes
// .
type Runner interface {
	Resync(ctx context.Context, t Task) error

	// Preflight reports whether dispatch can succeed at all, before any
	// task is fanned out. Returning non-nil lets the caller emit one
	// actionable error instead of N identical per-task failures.
	Preflight() error
}

// Task describes one Phase A unit of work.
type Task struct {
	ArtifactType string // "adr", "invariant", "guideline"
	ArtifactID   string // "ADR-NNN", "INV-NNN", or guideline slug
	ParentPath   string
	SidecarPath  string
	// ProjectRoot is the tree the dispatch must write into. It exists
	// because the prompt carries only an artifact ID: the subagent resolves
	// that ID against its OWN working directory's .edikt/config.yaml, so
	// without this the write lands wherever the binary happened to be
	// invoked from, no matter which root the caller targeted. Measured
	// 2026-08-13 (F-029): `gov reextract .measure/runA --only ADR-010`  edikt-guard:allow
	// rewrote the LIVE corpus and then reported "wrote no sidecar",
	// because the writer and the check were looking at different trees.
	//
	// Empty means "the current working directory", which is correct for
	// the ordinary in-repo compile.
	ProjectRoot string
}

// DefaultExtractorModel is the model the extractor dispatch pins when
// nothing overrides it.
//
// Before this pin existed the dispatch passed no --model at all, so every
// extraction — including the banked greenfield baseline — was produced by
// whatever the CLI defaulted to at that moment. That made every
// extraction-quality measurement unattributable: a difference between two
// runs could not be assigned to the prompt, the corpus, or a model that
// changed underneath both.
//
// Pinning does not retroactively attribute the baseline; it makes runs
// from here on attributable, which is what measuring a prompt change
// requires. The corollary is that the first run after this pin may differ
// from the baseline for model reasons rather than prompt reasons — treat
// a baseline taken before it as un-comparable until re-taken.
//
// Changing this value changes what every future measurement means, so
// treat it as a governance decision rather than a version bump — ADR-040  // edikt-guard:allow
// required an amending ADR to change the benchmark adversary's model for
// the same reason.
const DefaultExtractorModel = "claude-opus-5"

// ExtractorModelEnv overrides DefaultExtractorModel for one run.
const ExtractorModelEnv = "EDIKT_EXTRACTOR_MODEL"

// ResolveExtractorModel returns the model id the extractor dispatch should
// pin, in precedence order: explicit argument, then ExtractorModelEnv,
// then DefaultExtractorModel. The result is validated before return, so a
// caller that gets a value back can pass it to argv unchecked.
//
// The environment value is operator-supplied and reaches an argv element,
// which is INV-006's definition of externally-controlled — hence the  // edikt-guard:allow
// validation here rather than trust at the call site.
func ResolveExtractorModel(explicit string) (string, error) {
	model := explicit
	if model == "" {
		model = os.Getenv(ExtractorModelEnv)
	}
	if model == "" {
		model = DefaultExtractorModel
	}
	if err := idvalidate.ModelID(model); err != nil {
		return "", err
	}
	return model, nil
}

// ClaudeRunner shells out to `claude code` headless and asks it to run
// the per-artifact :compile slash command. It is the production runner;
// tests use FakeRunner instead.
type ClaudeRunner struct {
	Binary string // override for the claude CLI path; defaults to "claude"

	// Model pins the CLI's --model. Empty resolves through
	// ResolveExtractorModel, so the zero value is pinned rather than
	// unpinned — an unset field must never mean "whatever the CLI picks",
	// which is the defect this field exists to close.
	Model string
}

// Preflight resolves the claude CLI on PATH. It does not invoke it — an
// auth failure still surfaces per-task — but a missing binary is a whole-run
// condition and is reported as one error before any dispatch happens.
func (r *ClaudeRunner) Preflight() error {
	bin := r.Binary
	if bin == "" {
		bin = "claude"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%q not found on PATH", bin)
	}
	return nil
}

// Resync invokes `claude -p "/edikt:<type>:compile <id>"` and returns
// non-nil on a non-zero exit. Captured combined output is folded into the
// returned error so the dispatcher can log it.
//
// ArtifactType and ArtifactID are re-validated
// at the dispatcher boundary even though upstream callers (govrun.RunTwoPhase,
// migrate sidecars) validate before constructing the Task. A defense-in-depth
// check here means a future caller that forgets to validate cannot inject
// shell-meta or instruction-injection text into the claude prompt.
func (r *ClaudeRunner) Resync(ctx context.Context, t Task) error {
	if err := idvalidate.ArtifactType(t.ArtifactType); err != nil {
		return fmt.Errorf("phasea.Resync refused dispatch: %w", err)
	}
	if err := idvalidate.ArtifactID(t.ArtifactID); err != nil {
		return fmt.Errorf("phasea.Resync refused dispatch: %w", err)
	}
	bin := r.Binary
	if bin == "" {
		bin = "claude"
	}

	// Record the sidecar's pre-dispatch state so success can be verified
	// against the FILESYSTEM, not the agent's exit status. Field failure
	// mode: a stale sidecar-extractor agent definition (cached by Claude
	// Code at session start) exits 0 with an empty final response and
	// writes nothing — two full 20+-agent dispatch rounds produced zero
	// files with no error surfaced anywhere.
	before, beforeErr := os.Stat(t.SidecarPath)

	// Keep the prior bytes so a corrupt extraction can be rolled back.
	//
	// The agent owns the Write tool, so Go cannot stop a bad file being
	// written — it can only refuse to let one SURVIVE. Restoring the
	// previous content (or removing the file when there was none) makes
	// "the write does not happen" true in effect: after a failed dispatch
	// the tree is byte-identical to before it.
	var priorBytes []byte
	if beforeErr == nil {
		priorBytes, _ = os.ReadFile(t.SidecarPath)
	}

	// Resolve and validate the model before building argv, so an invalid
	// id refuses the dispatch rather than reaching the CLI.
	model, merr := ResolveExtractorModel(r.Model)
	if merr != nil {
		return fmt.Errorf("phasea.Resync refused dispatch: %w", merr)
	}

	prompt := fmt.Sprintf("/edikt:%s:compile %s", t.ArtifactType, t.ArtifactID)
	// --model and its value are separate argv elements, never interpolated
	// into a string that is then evaluated (INV-006).  // edikt-guard:allow
	cmd := exec.CommandContext(ctx, bin, "--model", model, "-p", prompt)
	// F-029. The prompt carries an artifact ID and nothing else, so the
	// subagent resolves it against ITS OWN working directory's config. With
	// Dir unset the child inherited the parent's cwd and wrote to whichever
	// tree the binary was invoked from — which is how a run targeting a
	// scratch root rewrote the live corpus while this function reported
	// "wrote no sidecar". The writer and the checker were looking at
	// different trees; this is the line that makes them agree.
	cmd.Dir = t.ProjectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude exit: %w; output: %s", err, truncate(string(out), 500))
	}

	// State what was OBSERVED and what was EXPECTED. Never name a cause that
	// was not verified: the previous text asserted "a stale sidecar-extractor
	// agent definition is the known cause", which had not been checked and
	// was wrong — it sent the investigation into session caching for two
	// rounds while the actual defect was the unset working directory above.
	// A diagnosis with no observation behind it is worse than no diagnosis,
	// because it redirects the search.
	where := t.ProjectRoot
	if where == "" {
		where = "the current working directory"
	}
	after, afterErr := os.Stat(t.SidecarPath)
	if afterErr != nil {
		return fmt.Errorf(
			"extractor exited 0 but no sidecar exists at %s — zero-file dispatch "+
				"(working directory: %s). Expected the extractor to create it. Two things worth "+
				"checking, neither of them asserted here: whether a sidecar was written elsewhere "+
				"in the tree, and whether the extractor agent definition is current (`edikt doctor`)",
			t.SidecarPath, where)
	}
	if beforeErr == nil && after.ModTime().Equal(before.ModTime()) && after.Size() == before.Size() {
		return fmt.Errorf(
			"extractor exited 0 but %s is unchanged — same mtime and size as before dispatch, "+
				"zero-file dispatch (working directory: %s). Expected the extractor to rewrite it. "+
				"Two things worth checking, neither of them asserted here: whether a sidecar was "+
				"written elsewhere in the tree, and whether the extractor agent definition is "+
				"current (`edikt doctor`)",
			t.SidecarPath, where)
	}

	// D20: the file exists and changed — but "a file was written" is not
	// "a sidecar was written". The extractor intermittently emits a
	// backtick escape inside a double-quoted scalar, which is not valid
	// YAML; the resulting sidecar cannot be loaded and fails compile for
	// the WHOLE project, not just this artifact.
	//
	// Nothing between the agent and the file checked that the output was
	// even parseable. INV-011 says stat the promised artifact rather than
	// trust the reported success; for a sidecar that has to mean PARSE it,
	// because an unparseable file passes every stat a caller can make.
	// D45 call site (a) — THE GENERATION BOUNDARY. Parse proves it is YAML;
	// the schema proves it CONFORMS. Both roll back.
	//
	// This is the boundary phase 1's acceptance names, and it is deliberately
	// NOT sidecar.Load: `v12_test.go:128` records that the loader returns nil
	// for schema-invalid input because rejection belongs at a gate, not at
	// load. That decision stands — Load stays permissive and the test pinning
	// it is untouched.
	//
	// Same ValidateRawAgainstSchema, same mirrored bytes as the corpus-wide
	// check. NOT a third definition of valid: D48 is what a genuine third
	// definition looked like, and it is now gated by test-schema-copies.sh.
	//
	// ADR-056 adds the third rung: PARSE proves it is YAML, SCHEMA proves it  edikt-guard:allow
	// conforms, ANCHORS prove it actually quotes the artifact it claims to.
	// A sidecar can clear the first two while every anchor points at prose
	// that is not there — three prompt revisions failed to drive that rate to
	// zero, so it is decided here instead of requested there.
	gateErr := func() error {
		if _, loadErr := sidecar.Load(t.SidecarPath); loadErr != nil {
			return loadErr
		}
		raw, readErr := os.ReadFile(t.SidecarPath)
		if readErr != nil {
			return readErr
		}
		// Validate against the schema the DOCUMENT declares. Hardcoding a
		// version here is what made a freshly-migrated v2 corpus read as 70
		// of 82 sidecars failing "the authoritative schema".
		if schemaErr := sidecar.ValidateRawAgainstDeclaredSchema(raw); schemaErr != nil {
			return schemaErr
		}
		return verifyAnchorsAgainstParent(t.SidecarPath, t.ParentPath)
	}()
	if gateErr != nil {
		loadErr := gateErr
		restoreErr := rollbackSidecar(t.SidecarPath, priorBytes, beforeErr == nil)
		msg := fmt.Sprintf(
			"extractor wrote an unloadable sidecar at %s: %v", t.SidecarPath, loadErr)
		if restoreErr != nil {
			// Worse than the parse failure: a corrupt sidecar is now on
			// disk and could not be reverted. Say so loudly — a silent
			// half-rollback is how a broken compile gets blamed on the
			// next change instead of this one.
			return fmt.Errorf("%s; ROLLBACK ALSO FAILED (%v) — %s is corrupt on disk and must be restored by hand",
				msg, restoreErr, t.SidecarPath)
		}
		return fmt.Errorf("%s (rolled back; the file is unchanged from before this dispatch)", msg)
	}
	return nil
}

// rollbackSidecar restores a sidecar to its pre-dispatch state: the prior
// bytes when the file existed, otherwise removal.
func rollbackSidecar(path string, prior []byte, existedBefore bool) error {
	if !existedBefore {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if prior == nil {
		// The file existed but was unreadable pre-dispatch, so there is
		// nothing to restore TO. Leaving the corrupt file is still wrong,
		// but inventing content would be worse — report instead.
		return fmt.Errorf("no pre-dispatch content was captured")
	}
	return os.WriteFile(path, prior, 0o644)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// verifyAnchorsAgainstParent is the ADR-056 gate: every anchor in a  edikt-guard:allow
// freshly-written sidecar must quote the parent artifact byte-exactly at the
// line range it records.
//
// It runs at the GENERATION BOUNDARY, per dispatch, alongside the parse and
// schema rungs — and it fails the dispatch rather than warning. A warning here
// would be the absence-rendering-as-pass shape INV-013 forbids: the compile  edikt-guard:allow
// continues, the corpus takes on an anchor pointing at prose that is not there,
// and nothing downstream distinguishes it from a verified one.
//
// The failure names every offending anchor with its recorded range AND what
// actually sits at that range. An anchor error is invisible from the quote
// alone — the text is real prose from the artifact — so a message that omits
// the actual lines leaves the reader unable to see what is wrong.
func verifyAnchorsAgainstParent(sidecarPath, parentPath string) error {
	sc, err := sidecar.Load(sidecarPath)
	if err != nil {
		return err
	}

	// No items means nothing to verify — a measured zero, not a skipped
	// check. The parent is not read in this case, because reading it would
	// only manufacture an error for a sidecar that has no anchors to be wrong
	// about (a roadmap-only ADR legitimately compiles to `directives: []`).
	if len(sc.Directives) == 0 && len(sc.Prohibitions) == 0 {
		return nil
	}

	body, err := os.ReadFile(parentPath)
	if err != nil {
		// The parent is the oracle. There ARE anchors here and none of them
		// could be checked — that is unmeasured, and unmeasured must not read
		// as a clean bill of health (INV-011).
		return fmt.Errorf("cannot read parent %s to verify %d item(s) of anchors: %w",
			parentPath, len(sc.Directives)+len(sc.Prohibitions), err)
	}

	v := sidecar.VerifyAnchors(sc, string(body))
	if v.OK() {
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "extractor wrote %d invalid anchor(s) of %d across %d item(s) — every anchor must quote %s byte-exactly at the line range it records",
		len(v.Faults), v.Anchors, v.Items, filepath.Base(parentPath))
	for _, f := range v.Faults {
		fmt.Fprintf(&b, "\n    %s", f.String())
	}
	return fmt.Errorf("%s", b.String())
}

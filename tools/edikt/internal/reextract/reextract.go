// Package reextract drives a deliberate, whole-corpus regeneration of
// sidecars through the locked extractor.
//
// # WHY THIS EXISTS SEPARATELY FROM COMPILE
//
// Phase A regenerates a sidecar when it is STALE, and staleness is measured
// from anchors. After a prompt-contract change every anchor still matches, so
// nothing is stale and nothing regenerates — while every sidecar in the corpus
// was written by a contract that no longer applies. There was no forcing
// mechanism at all: `agent_prompt_version` is forbidden at sidecar root
// (ADR-027), and deleting sidecars to force a rebuild destroys exactly the  edikt-guard:allow
// human-pinned state (manual_directives, suppressed_directives,
// human_approved_at, approved paths) that must survive.
//
// The owner ruling is that re-extraction is NEVER an implicit side-effect of
// staleness: it runs behind an explicit force flag, or it does not run.
//
// # RESUMABILITY IS THE POINT, NOT A CONVENIENCE
//
// A whole-corpus batch is ~60 LLM dispatches over tens of minutes. A kill in
// the middle — a context boundary, a lost terminal, a cancelled CI job — must
// not cost the work already done, and re-invoking must not re-dispatch
// artifacts already regenerated. The ledger records each completion as it
// happens (not at the end), so what survives a kill is what actually finished.
//
// A ledger entry is keyed by the extractor's prompt version. Two sidecars
// written under different contracts are not comparable, so a contract change
// starts a new batch instead of reporting old work as current.
package reextract

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// LedgerRelPath is where a batch's progress is recorded, relative to the
// project root.
const LedgerRelPath = ".edikt/state/reextract-ledger.json"

// FixtureProofRelPath is where the AUTHORITATIVE, human-edited
// fixture-validation record lives in this repo — the file a maintainer edits
// to declare "prompt_version vN is validated on the frozen fixture set".
// Named here for tooling and error messages, but production code never reads
// it from opts.ProjectRoot: see fixtureProofYAML.
const FixtureProofRelPath = "test/fixtures/extractor-validation.yaml"

// fixtureProofYAML is a BYTE-IDENTICAL MIRROR of FixtureProofRelPath,
// embedded into the binary at build time.
//
// WHY A MIRROR AND NOT A PROJECT-RELATIVE READ. The original design read this
// file from opts.ProjectRoot — the CONSUMER's project, not edikt's own repo.
// "Was this extraction contract validated on the frozen fixture set" is a
// fact about the release that shipped the contract, not about whatever
// project the binary happens to run against — and test/ is never distributed
// (PUBLIC-REPO-PUBLISH-MANIFEST.md), so no consumer project could ever
// satisfy this gate. Confirmed against a real consumer project (no
// test/fixtures/ directory at all): every invocation refused unconditionally,
// citing an override flag (--skip-fixture-proof) that did not exist either.
//
// The go:embed directive's patterns cannot contain "..", and the authoritative file lives
// outside this Go module — so this mirrors the FILE verbatim, gated by
// TestFixtureProofMirrorIsByteIdentical, the same pattern
// internal/sidecar/schemavalidate.go uses for the sidecar JSON schemas.
//
//go:embed fixtureproof/extractor-validation.yaml
var fixtureProofYAML []byte

// LedgerSchemaVersion is the ledger file's own format version.
const LedgerSchemaVersion = 1

// Status values for a ledger entry.
const (
	StatusDone   = "done"
	StatusFailed = "failed"
)

// Entry records one artifact's outcome within a batch.
type Entry struct {
	Status string `json:"status"`
	// SidecarSHA256 is the hash of the sidecar this batch produced. It makes
	// "already regenerated" a checkable claim rather than a remembered one:
	// a sidecar edited after its dispatch no longer matches what the ledger
	// says was written.
	SidecarSHA256 string `json:"sidecar_sha256,omitempty"`
	CompletedAt   string `json:"completed_at"`
	Err           string `json:"error,omitempty"`
}

// Ledger is the on-disk batch record.
type Ledger struct {
	SchemaVersion int              `json:"schema_version"`
	PromptVersion string           `json:"prompt_version"`
	StartedAt     string           `json:"started_at"`
	Artifacts     map[string]Entry `json:"artifacts"`
}

// Options configures one re-extraction invocation.
type Options struct {
	ProjectRoot string
	// Force must be true. A false value is refused rather than defaulted,
	// which is the whole contract: re-extraction is explicit or it does not
	// happen.
	Force bool
	// Only restricts the batch to these artifact IDs. Empty means the whole
	// live corpus.
	Only        []string
	Concurrency int
	// Runner is injectable so tests can count dispatches without paying LLM
	// latency. Defaults to the production ClaudeRunner.
	Runner phasea.Runner
	// RequireCleanTree refuses to start when the working tree carries
	// uncommitted changes, so the batch can land as one reviewable commit.
	RequireCleanTree bool
	// SkipFixtureProof bypasses the fixture-validation precondition. Explicit
	// and named, so a bypass is visible in the invocation rather than implied
	// by an absent file.
	SkipFixtureProof bool
	// PromptVersion overrides the value read from the installed agent.
	// Tests set it; production leaves it empty.
	PromptVersion string
	LedgerPath    string
	Stderr        io.Writer
	Now           func() time.Time
	// DispatchTimeout bounds a single artifact's extraction. Zero means no
	// per-task bound.
	DispatchTimeout time.Duration
}

// Result summarizes an invocation.
type Result struct {
	PromptVersion string   `json:"prompt_version"`
	Eligible      int      `json:"eligible"`
	AlreadyDone   int      `json:"already_done"`
	Dispatched    int      `json:"dispatched"`
	Succeeded     int      `json:"succeeded"`
	Failed        int      `json:"failed"`
	FailedIDs     []string `json:"failed_ids,omitempty"`

	// PinsRestored counts human-pinned fields carried across regeneration,
	// and Unrestorable names every one that could not be. The extractor never
	// sees the previous sidecar (deliberately), so a regeneration wipes what a
	// human pinned unless this path puts it back.
	PinsRestored int           `json:"pins_restored"`
	Unrestorable []PinnedField `json:"unrestorable,omitempty"`
	// LoadFailedIDs names every artifact whose PRIOR sidecar existed but
	// failed to load — a categorically different case from "no prior
	// sidecar", surfaced separately so a caller (or a human reading the run
	// summary) cannot mistake it for the ordinary bootstrap path. Each ID
	// here also contributes entries to Unrestorable, one per at-risk field
	// (see pinnedFieldNamesAtRisk), for the per-artifact review ceremony.
	LoadFailedIDs []string `json:"load_failed_ids,omitempty"`
	Wall          string   `json:"wall,omitempty"`
}

// ErrNotForced is returned when Force was not set.
var ErrNotForced = fmt.Errorf("re-extraction requires an explicit force flag; it is never an implicit staleness side-effect")

// Run executes (or resumes) a re-extraction batch.
func Run(opts Options) (*Result, error) {
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.Runner == nil {
		opts.Runner = &phasea.ClaudeRunner{}
	}
	if opts.LedgerPath == "" {
		opts.LedgerPath = filepath.Join(opts.ProjectRoot, LedgerRelPath)
	}

	if !opts.Force {
		return nil, ErrNotForced
	}

	promptVersion := opts.PromptVersion
	if promptVersion == "" {
		v, err := phasea.ResolveExtractorPromptVersion(opts.ProjectRoot)
		if err != nil {
			// A batch whose contract cannot be named cannot be resumed,
			// compared, or attributed. UNKNOWN is reported, never
			// substituted with a plausible version.
			return nil, fmt.Errorf("cannot determine the extraction contract version (%s): %w", v, err)
		}
		promptVersion = v
	}

	// FIXTURE PROOF BEFORE THE CORPUS. A prompt_version bump reaches the corpus
	// through this verb and no other, so this is where an unvalidated contract
	// is stopped. Prompt v5 captured its target case correctly and dropped
	// eleven bodies across five fixture artifacts; it would have shipped on the
	// strength of one artifact looking better.
	//
	// THE RECORD IS A RELEASE PROPERTY, NOT A PROJECT ONE. It was originally
	// read from FixtureProofRelPath resolved against opts.ProjectRoot — which
	// meant it only existed in edikt-dev's own tree (test/ is never
	// distributed) and this gate could not be satisfied by any consumer
	// project ever. "Was this extraction contract validated on the frozen
	// fixture set" is a fact about the binary that shipped it, not about
	// whatever repo it's being run against, so it now ships INSIDE the binary
	// (see fixtureProofYAML below) instead of being read from disk at
	// opts.ProjectRoot. SkipFixtureProof remains as a real, flag-reachable
	// escape hatch (see --skip-fixture-proof) for the rare case a maintainer
	// is certain a contract is fine despite the shipped record.
	if !opts.SkipFixtureProof {
		// PARSE THE FIELD, do not grep the file. A substring match found "v4"
		// inside the PROSE of the rejected-v5 entry and let an unrecorded
		// contract through — a check matching the shape of the answer rather
		// than the answer, which is this release's signature defect.
		var proof struct {
			Validated []struct {
				PromptVersion string `yaml:"prompt_version"`
			} `yaml:"validated"`
		}
		if yerr := yaml.Unmarshal(fixtureProofYAML, &proof); yerr != nil {
			return nil, fmt.Errorf("refusing to dispatch: the embedded fixture-validation record could not "+
				"be parsed (%v) — an unreadable validation record is not a validation", yerr)
		}
		found := false
		for _, v := range proof.Validated {
			if v.PromptVersion == promptVersion {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf(
				"refusing to dispatch: this build's embedded fixture-validation record has no entry for "+
					"%s (the installed extraction contract). Either this binary predates that contract's "+
					"validation, or the contract was never proven on the frozen fixture set by bodies. "+
					"Pass --skip-fixture-proof if you are certain this contract is already validated",
				promptVersion)
		}
	}

	if opts.RequireCleanTree {
		if dirty, err := gitDirty(opts.ProjectRoot); err != nil {
			return nil, fmt.Errorf("clean-tree precondition could not be measured: %w", err)
		} else if len(dirty) > 0 {
			fmt.Fprintf(opts.Stderr, "error: working tree is not clean — %d path(s) modified:\n", len(dirty))
			for _, p := range dirty {
				fmt.Fprintf(opts.Stderr, "  %s\n", p)
			}
			return nil, fmt.Errorf("re-extraction requires a clean tree so the batch lands as one reviewable commit")
		}
	}

	pairs, err := discoverEligible(opts.ProjectRoot)
	if err != nil {
		return nil, err
	}
	if len(opts.Only) > 0 {
		want := map[string]bool{}
		for _, id := range opts.Only {
			want[strings.ToUpper(id)] = true
		}
		var filtered []sidecar.Pair
		for _, p := range pairs {
			if want[strings.ToUpper(p.ArtifactID)] {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("--only matched no live artifact in the corpus: %s", strings.Join(opts.Only, ", "))
		}
		pairs = filtered
	}

	ledger := loadLedger(opts.LedgerPath)
	if ledger.PromptVersion != promptVersion {
		// A different contract wrote the previous batch. Its completions say
		// nothing about this one.
		if ledger.PromptVersion != "" {
			fmt.Fprintf(opts.Stderr,
				"note: ledger records prompt %s, current contract is %s — starting a new batch (previous completions do not carry over)\n",
				ledger.PromptVersion, promptVersion)
		}
		ledger = Ledger{
			SchemaVersion: LedgerSchemaVersion,
			PromptVersion: promptVersion,
			StartedAt:     opts.Now().UTC().Format(time.RFC3339),
			Artifacts:     map[string]Entry{},
		}
	}
	if ledger.Artifacts == nil {
		ledger.Artifacts = map[string]Entry{}
	}

	res := &Result{PromptVersion: promptVersion, Eligible: len(pairs)}

	var tasks []phasea.Task
	for _, p := range pairs {
		if e, ok := ledger.Artifacts[p.ArtifactID]; ok && e.Status == StatusDone && sidecarUnchanged(p.SidecarPath, e.SidecarSHA256) {
			res.AlreadyDone++
			continue
		}
		if err := idvalidate.ArtifactType(p.Kind); err != nil {
			return nil, fmt.Errorf("%s: %w — refusing dispatch", p.ParentPath, err)
		}
		if err := idvalidate.ArtifactID(p.ArtifactID); err != nil {
			return nil, fmt.Errorf("%s: %w — refusing dispatch", p.ParentPath, err)
		}
		tasks = append(tasks, phasea.Task{
			ArtifactType: p.Kind,
			ArtifactID:   p.ArtifactID,
			ParentPath:   p.ParentPath,
			SidecarPath:  p.SidecarPath,
			// F-029: this verb takes an explicit root, so it is the one that
			// MUST pass it through. Omitting it is what let a targeted run
			// rewrite the live corpus.
			ProjectRoot: opts.ProjectRoot,
		})
	}
	res.Dispatched = len(tasks)

	fmt.Fprintf(opts.Stderr, "re-extraction batch %s: %d eligible, %d already regenerated, %d to dispatch\n",
		promptVersion, res.Eligible, res.AlreadyDone, res.Dispatched)

	if len(tasks) == 0 {
		// Persist even a zero-dispatch run so the batch identity is on disk
		// after the first invocation that found nothing to do.
		if err := saveLedger(opts.LedgerPath, ledger); err != nil {
			return res, err
		}
		return res, nil
	}

	if err := opts.Runner.Preflight(); err != nil {
		return res, fmt.Errorf("cannot dispatch the extractor for %d artifact(s): %w", len(tasks), err)
	}

	// Capture pinned state BEFORE any dispatch. Reading it afterwards would
	// read what the extractor just overwrote.
	//
	// loadFailedBefore is captured in the SAME loop, for the same reason:
	// this is the last point p.LoadErr reflects the pre-dispatch state. A
	// nil p.Sidecar means one of two entirely different things — "no prior
	// sidecar existed" (nothing pinned, the ordinary bootstrap case) or
	// "a prior sidecar existed and failed to load" (unknown pinned state,
	// discarded by treating it as the first case). Discover only sets one of
	// p.Sidecar/p.LoadErr, never both, but this loop keeps them in separate
	// maps rather than folding a load failure into "absent" — the fold is
	// exactly the swallow PreservePinned's beforeLoadErr parameter exists to
	// refuse (F-115/A1: paths, verify, verify_kind, human_approved_at, and
	// both fixture-path pins were silently discarded this way on a real
	// corpus, with nothing distinguishing it from a clean bootstrap).
	pinnedBefore := map[string]*sidecar.Sidecar{}
	loadFailedBefore := map[string]error{}
	for _, p := range pairs {
		if p.Sidecar != nil {
			pinnedBefore[p.ArtifactID] = p.Sidecar
		} else if p.LoadErr != nil {
			loadFailedBefore[p.ArtifactID] = p.LoadErr
		}
	}

	// Snapshot pre-rewrite sidecar bytes for the artifacts actually being
	// dispatched this run — the git-independent review baseline (Q3,
	// option a). Same loop as pinnedBefore, same reason: this is the last
	// point the on-disk bytes are still what extraction is about to
	// overwrite. Scoped to `tasks`, not all `pairs` — snapshotting an
	// artifact that's already done and not being re-dispatched would
	// record a baseline nobody reviews against.
	for _, t := range tasks {
		if err := writeSnapshot(opts.ProjectRoot, t.ArtifactID, t.SidecarPath); err != nil {
			// Non-fatal: a snapshot failure must not block re-extraction.
			// A git-based review remains available as a fallback if the
			// tree happens to be clean; the snapshot is a strictly better
			// baseline when it succeeds, not a required one.
			fmt.Fprintf(opts.Stderr, "  warn: %s — could not write pre-rewrite snapshot: %v (review will need a clean git tree instead)\n",
				t.ArtifactID, err)
		}
	}

	var mu sync.Mutex
	recorder := &recordingRunner{
		inner:   opts.Runner,
		timeout: opts.DispatchTimeout,
		preserve: func(t phasea.Task) (*PreserveResult, error) {
			pr, err := PreservePinned(t.ArtifactID, pinnedBefore[t.ArtifactID], loadFailedBefore[t.ArtifactID], t.SidecarPath)
			if err != nil {
				return nil, err
			}
			mu.Lock()
			defer mu.Unlock()
			res.PinsRestored += pr.DirectivePins
			if pr.PathsRestored {
				res.PinsRestored++
			}
			res.Unrestorable = append(res.Unrestorable, pr.Unrestorable...)
			if pr.LoadFailed {
				// One salient line for the artifact, not seven identical
				// UNRESTORABLE lines (one per at-risk field) — the per-field
				// detail still lands in res.Unrestorable for the per-artifact
				// review ceremony (commands/gov/reextract.md Step 5) to walk
				// through; this line is what makes the run's own stderr
				// output impossible to miss.
				res.LoadFailedIDs = append(res.LoadFailedIDs, t.ArtifactID)
				fmt.Fprintf(opts.Stderr,
					"  REFUSED %s: prior sidecar existed but failed to load (%v) — could not check for pinned state (paths, verify, human_approved_at, and others); this artifact's regeneration is NOT treated as clean, review required\n",
					t.ArtifactID, loadFailedBefore[t.ArtifactID])
			}
			if pr.SidecarRewritten {
				fmt.Fprintf(opts.Stderr, "  restored %s: %d directive pin(s)%s\n",
					t.ArtifactID, pr.DirectivePins,
					map[bool]string{true: ", approved paths", false: ""}[pr.PathsRestored])
			}
			if !pr.LoadFailed {
				for _, u := range pr.Unrestorable {
					fmt.Fprintf(opts.Stderr, "  UNRESTORABLE %s\n", u.String())
				}
			}
			return pr, nil
		},
		onDone: func(t phasea.Task, err error) {
			// Persist per completion, not at the end. What survives a kill
			// must be what actually finished.
			mu.Lock()
			defer mu.Unlock()
			e := Entry{CompletedAt: opts.Now().UTC().Format(time.RFC3339)}
			if err != nil {
				e.Status = StatusFailed
				e.Err = oneLine(err.Error())
			} else {
				// ADR-053 — stamp the body-drift baseline (F-027).  edikt-guard:allow
				//
				// RunTwoPhase stamped; this path did not, so a forced
				// re-extraction — the one operation that rewrites every
				// sidecar and therefore most needs a baseline — was the only
				// one that recorded none. The corpus sat at 0-of-71 measured
				// after a full 65-artifact run.
				//
				// Here for the same reason it belongs in RunTwoPhase: the
				// extractor has just read this parent and produced this
				// sidecar from it, so the digest is genuinely "what
				// extraction saw". A backfill pass would write a baseline
				// claiming an extraction that never happened, converting
				// "may be incomplete" into a confident lie.
				//
				// BEFORE the hash, not after: stamping rewrites the sidecar,
				// so hashing first records a digest the file no longer has
				// and resume re-dispatches work that succeeded.
				if serr := sidecar.StampBodyDigest(t.SidecarPath, t.ParentPath,
					os.ReadFile,
					func(p string, b []byte) error { return os.WriteFile(p, b, 0o644) },
				); serr != nil {
					// Non-fatal, as in RunTwoPhase: the extraction succeeded
					// and a missing baseline degrades to UNMEASURED, which is
					// a safe state. Failing to stamp must not discard good
					// extraction work — but it must not be silent either.
					fmt.Fprintf(opts.Stderr,
						"  warn: %s — could not record body-drift baseline: %v (reported as unmeasured)\n",
						t.ArtifactID, serr)
				}
				e.Status = StatusDone
				e.SidecarSHA256 = fileSHA256(t.SidecarPath)
			}
			ledger.Artifacts[t.ArtifactID] = e
			if serr := saveLedger(opts.LedgerPath, ledger); serr != nil {
				fmt.Fprintf(opts.Stderr, "  warn: ledger write failed for %s: %v\n", t.ArtifactID, serr)
			}
		},
	}

	dis := &phasea.Dispatcher{
		Runner:       recorder,
		Concurrency:  opts.Concurrency,
		ProgressOut:  opts.Stderr,
		ErrorLogPath: filepath.Join(opts.ProjectRoot, ".edikt", "state", "compile-errors.log"),
	}
	start := opts.Now()
	out := dis.Run(context.Background(), tasks)
	res.Wall = opts.Now().Sub(start).Round(time.Second).String()
	res.Failed = len(out.Failures)
	res.Succeeded = res.Dispatched - res.Failed
	for _, f := range out.Failures {
		res.FailedIDs = append(res.FailedIDs, f.Task.ArtifactID)
	}
	sort.Strings(res.FailedIDs)

	if err := saveLedger(opts.LedgerPath, ledger); err != nil {
		return res, err
	}
	if res.Failed > 0 {
		return res, fmt.Errorf("%d of %d dispatch(es) failed: %s (re-run to resume; succeeded artifacts are not re-dispatched)",
			res.Failed, res.Dispatched, strings.Join(res.FailedIDs, ", "))
	}
	return res, nil
}

// RestampLedger updates the recorded hash for artifacts whose sidecar was
// rewritten AFTER their dispatch, so a legitimate post-dispatch edit does not
// read as "never regenerated".
//
// This exists because the one-shot pinned-state repair rewrites sidecars behind
// the ledger's back: 43 restored files immediately dropped out of the completed
// set, and the next --force would have re-dispatched them — paying for
// extraction that had already happened and, worse, throwing away the restore.
//
// It re-stamps ONLY entries already marked done. An artifact that never
// completed does not become complete because something edited its file.
func RestampLedger(root string, artifactSidecars map[string]string) (int, error) {
	path := filepath.Join(root, LedgerRelPath)
	ledger := loadLedger(path)
	if ledger.Artifacts == nil {
		return 0, nil
	}
	n := 0
	for id, scPath := range artifactSidecars {
		e, ok := ledger.Artifacts[id]
		if !ok || e.Status != StatusDone {
			continue
		}
		sum := fileSHA256(scPath)
		if sum == "" || sum == e.SidecarSHA256 {
			continue
		}
		e.SidecarSHA256 = sum
		ledger.Artifacts[id] = e
		n++
	}
	if n == 0 {
		return 0, nil
	}
	return n, saveLedger(path, ledger)
}

// DoneSidecarPaths maps every artifact the ledger marks done to its sidecar
// path, for a caller that needs to re-stamp after a deliberate repair.
func DoneSidecarPaths(root string) (map[string]string, error) {
	pairs, err := discoverEligible(root)
	if err != nil {
		return nil, err
	}
	ledger := loadLedger(filepath.Join(root, LedgerRelPath))
	out := map[string]string{}
	for _, p := range pairs {
		if e, ok := ledger.Artifacts[p.ArtifactID]; ok && e.Status == StatusDone {
			out[p.ArtifactID] = p.SidecarPath
		}
	}
	return out, nil
}

// StatusReport is the ledger's state read without dispatching anything.
//
// It exists because a batch that spans a context boundary needs an answer to
// "what is left" that does not involve starting work to find out.
type StatusReport struct {
	PromptVersion string   `json:"prompt_version"`
	LedgerPrompt  string   `json:"ledger_prompt_version,omitempty"`
	Eligible      int      `json:"eligible"`
	Done          int      `json:"done"`
	Remaining     int      `json:"remaining"`
	Failed        int      `json:"failed"`
	RemainingIDs  []string `json:"remaining_ids,omitempty"`
	FailedIDs     []string `json:"failed_ids,omitempty"`
}

// Status reports what a resumed batch would do, without dispatching.
//
// An unresolvable extraction contract version degrades PromptVersion to
// ExtractorPromptVersionUnknown rather than failing the whole report — the
// same way the real dispatch path (govrun/twophase.go) already treats the
// identical resolution failure as non-fatal. Before this, Status() was the
// one caller that turned "the agent resolves, just not project-locally"
// into a hard error, which upgrade.md §7's own contract then reads as "the
// status check failed" and silently skips its re-extraction offer entirely
// (INV-013: a control with a subject it could not fully observe must say  edikt-guard:allow
// so, not fail outright — see F4,
// docs/internal/issues/agentmodel-resolver-no-global-fallback.md).
func Status(root string) (*StatusReport, error) {
	promptVersion, err := phasea.ResolveExtractorPromptVersion(root)
	if err != nil {
		promptVersion = phasea.ExtractorPromptVersionUnknown
	}
	pairs, err := discoverEligible(root)
	if err != nil {
		return nil, err
	}
	ledger := loadLedger(filepath.Join(root, LedgerRelPath))
	st := &StatusReport{
		PromptVersion: promptVersion,
		LedgerPrompt:  ledger.PromptVersion,
		Eligible:      len(pairs),
	}
	// A ledger written under a different contract carries no completions for
	// this one. Reporting its counts here would be the unmeasured-as-measured
	// shape: work that happened, but not this work.
	sameBatch := ledger.PromptVersion == promptVersion
	for _, p := range pairs {
		e, ok := ledger.Artifacts[p.ArtifactID]
		if sameBatch && ok && e.Status == StatusDone && sidecarUnchanged(p.SidecarPath, e.SidecarSHA256) {
			st.Done++
			continue
		}
		if sameBatch && ok && e.Status == StatusFailed {
			st.Failed++
			st.FailedIDs = append(st.FailedIDs, p.ArtifactID)
		}
		st.Remaining++
		st.RemainingIDs = append(st.RemainingIDs, p.ArtifactID)
	}
	sort.Strings(st.RemainingIDs)
	sort.Strings(st.FailedIDs)
	return st, nil
}

// recordingRunner wraps a Runner so each completion reaches the ledger as it
// happens. The Dispatcher has no completion hook, and adding one there would
// give every caller a persistence concern that only this one has.
type recordingRunner struct {
	inner   phasea.Runner
	onDone  func(phasea.Task, error)
	timeout time.Duration

	// preserve carries human-pinned state across the regeneration. It runs
	// between the dispatch and the ledger write, so a task is only recorded
	// done once its pins are back on disk.
	preserve func(phasea.Task) (*PreserveResult, error)
}

func (r *recordingRunner) Preflight() error { return r.inner.Preflight() }

func (r *recordingRunner) Resync(ctx context.Context, t phasea.Task) error {
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}
	err := r.inner.Resync(ctx, t)
	if err == nil && r.preserve != nil {
		if _, perr := r.preserve(t); perr != nil {
			// A regeneration whose pins could not be restored is a FAILED
			// regeneration, not a successful one with a warning: the sidecar
			// on disk is now missing state a human put there, and recording it
			// done would take it out of the resume set.
			err = fmt.Errorf("regenerated %s but could not restore pinned state: %w", t.ArtifactID, perr)
		}
	}
	r.onDone(t, err)
	return err
}

// discoverEligible returns the live artifact pairs a batch covers: every
// non-skipped artifact in the configured governance directories.
//
// A load error is NOT a reason to exclude an artifact — a sidecar that fails
// to load is precisely one that needs regenerating. Excluding it would let the
// batch report full coverage while silently skipping its worst cases.
func discoverEligible(root string) ([]sidecar.Pair, error) {
	dirs := govrun.GovernanceDirs(root)
	pairs, err := sidecar.Discover(root, dirs)
	if err != nil {
		return nil, fmt.Errorf("discover sidecars: %w", err)
	}
	var out []sidecar.Pair
	for _, p := range pairs {
		if p.Skip {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ArtifactID < out[j].ArtifactID })
	return out, nil
}

func sidecarUnchanged(path, recorded string) bool {
	if recorded == "" {
		return false
	}
	return fileSHA256(path) == recorded
}

func fileSHA256(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func loadLedger(path string) Ledger {
	var l Ledger
	b, err := os.ReadFile(path)
	if err != nil {
		return l
	}
	if err := json.Unmarshal(b, &l); err != nil {
		// An unreadable ledger is treated as no ledger: the batch re-dispatches
		// rather than trusting a file it could not parse.
		return Ledger{}
	}
	return l
}

func saveLedger(path string, l Ledger) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// gitDirty returns the porcelain status lines of a working tree.
func gitDirty(root string) ([]string, error) {
	cmd := exec.Command("git", "-C", root, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	return lines, nil
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

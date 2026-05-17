package govrun

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/internal/phaseb"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/model"
)

// TwoPhaseOptions configures one invocation of the sidecar-aware compile.
type TwoPhaseOptions struct {
	ProjectRoot string
	CheckOnly   bool
	NoWait      bool
	JSONMode    bool          // suppress prose UI on stdout; caller emits JSON
	Runner      phasea.Runner // injectable for tests; defaults to ClaudeRunner
	Concurrency int           // defaults to 8 per ADR-028
	Stderr      io.Writer
	Stdout      io.Writer
}

// TwoPhaseResult summarizes the run for both prose UI and --json output.
type TwoPhaseResult struct {
	StaleSidecars  []string         `json:"stale_sidecars,omitempty"`
	PhaseADone     bool             `json:"phase_a_done"`
	PhaseAFailures int              `json:"phase_a_failures"`
	PhaseAErrors   []PhaseAErrorRec `json:"phase_a_errors,omitempty"`
	PhaseB         *phaseb.Result   `json:"phase_b,omitempty"`
}

// PhaseAErrorRec is a JSON-friendly view of a Phase A subagent failure.
type PhaseAErrorRec struct {
	ArtifactID   string `json:"artifact_id"`
	ArtifactType string `json:"artifact_type"`
	Err          string `json:"error"`
}

// RunTwoPhase is the sidecar-aware compile path per ADR-027 + ADR-028.
//
// Order of operations:
//  1. acquire .edikt/state/compile.lock (blocks unless --no-wait)
//  2. discover artifact pairs and detect stale sidecars
//  3. if --check && stale → emit actionable error, exit 1, no LLM dispatch
//  4. if stale && !--check → run Phase A (parallel resync, continue-on-error)
//  5. if Phase A had failures → exit 1, do NOT run Phase B (per ADR-028)
//  6. run Phase B (deterministic merge)
func RunTwoPhase(opts TwoPhaseOptions, clk model.Clock) (*TwoPhaseResult, error) {
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 8
	}
	if opts.Runner == nil {
		opts.Runner = &phasea.ClaudeRunner{}
	}

	cfg, err := loadConfig(opts.ProjectRoot)
	if err != nil {
		return nil, err
	}
	dirs := []string{cfg.Paths.Decisions, cfg.Paths.Invariants, cfg.Paths.Guidelines}

	release, err := acquireCompileLock(opts.ProjectRoot, opts.NoWait)
	if err != nil {
		return nil, err
	}
	defer release()

	pairs, err := sidecar.Discover(opts.ProjectRoot, dirs)
	if err != nil {
		return nil, fmt.Errorf("discover sidecars: %w", err)
	}

	res := &TwoPhaseResult{}
	var tasks []phasea.Task
	var loadErrs []string

	for _, p := range pairs {
		if p.Skip {
			// Superseded ADRs and migration:skip-marked artifacts opt out of
			// sidecar coverage. INV-002-compliant: no body edit required to
			// suppress them — the supersession status line was present at
			// acceptance time. Skip silently from compile.
			continue
		}
		if p.LoadErr != nil {
			loadErrs = append(loadErrs, fmt.Sprintf("  %s: %v", p.SidecarPath, p.LoadErr))
			continue
		}
		if p.Sidecar == nil {
			loadErrs = append(loadErrs, fmt.Sprintf("  %s: sidecar missing — run /edikt:%s:compile", p.ParentPath, artifactTypeFromPath(p.ParentPath, dirs)))
			continue
		}
		stale, reason, err := p.Sidecar.IsStale(opts.ProjectRoot)
		if err != nil {
			loadErrs = append(loadErrs, fmt.Sprintf("  %s: %v", p.SidecarPath, err))
			continue
		}
		if stale {
			// INV-006: validate artifact ID + type at the dispatch boundary
			// before they flow into a `claude -p` argv string. A name that
			// slipped past discovery (unusual filename, future glob) MUST
			// NOT reach the locked extractor prompt.
			artifactType := artifactTypeFromPath(p.ParentPath, dirs)
			if err := idvalidate.ArtifactType(artifactType); err != nil {
				loadErrs = append(loadErrs, fmt.Sprintf("  %s: %v — refusing dispatch", p.ParentPath, err))
				continue
			}
			if err := idvalidate.ArtifactID(p.ArtifactID); err != nil {
				loadErrs = append(loadErrs, fmt.Sprintf("  %s: %v — refusing dispatch", p.ParentPath, err))
				continue
			}
			res.StaleSidecars = append(res.StaleSidecars, p.ArtifactID)
			tasks = append(tasks, phasea.Task{
				ArtifactType: artifactType,
				ArtifactID:   p.ArtifactID,
				ParentPath:   p.ParentPath,
				SidecarPath:  p.SidecarPath,
			})
			fmt.Fprintf(opts.Stderr, "  stale: %s — %s\n", p.ArtifactID, reason)
		}
	}

	if len(loadErrs) > 0 {
		fmt.Fprintln(opts.Stderr, "Sidecar errors:")
		for _, e := range loadErrs {
			fmt.Fprintln(opts.Stderr, e)
		}
		return res, fmt.Errorf("%d sidecar error(s)", len(loadErrs))
	}

	if opts.CheckOnly && len(tasks) > 0 {
		fmt.Fprintf(opts.Stderr, "error: %d sidecar(s) stale — run 'edikt gov compile' to resync\n", len(tasks))
		return res, fmt.Errorf("stale sidecars in --check mode")
	}

	if len(tasks) > 0 {
		ctx := context.Background()
		dis := &phasea.Dispatcher{
			Runner:       opts.Runner,
			Concurrency:  opts.Concurrency,
			ProgressOut:  opts.Stderr,
			ErrorLogPath: filepath.Join(opts.ProjectRoot, ".edikt", "state", "compile-errors.log"),
		}
		paRes := dis.Run(ctx, tasks)
		res.PhaseADone = true
		res.PhaseAFailures = len(paRes.Failures)
		fmt.Fprintf(opts.Stderr, "Phase A done: %d ok, %d failed in %s\n",
			paRes.TaskCount-len(paRes.Failures), len(paRes.Failures), paRes.Wall.Round(time.Millisecond))

		if len(paRes.Failures) > 0 {
			fmt.Fprintf(opts.Stderr, "Phase A failures (logged to .edikt/state/compile-errors.log):\n")
			for _, f := range paRes.Failures {
				fmt.Fprintf(opts.Stderr, "  %s: %v\n", f.Task.ArtifactID, f.Err)
				res.PhaseAErrors = append(res.PhaseAErrors, PhaseAErrorRec{
					ArtifactID:   f.Task.ArtifactID,
					ArtifactType: f.Task.ArtifactType,
					Err:          f.Err.Error(),
				})
			}
			return res, fmt.Errorf("phase A: %d failure(s); not running phase B", len(paRes.Failures))
		}

		// Reload sidecars now that subagents have regenerated them. Two
		// paths (Phase 7 of PLAN-sidecar-review-fixes #41):
		//
		//   • Default — full re-Discover. Conservative; matches v0.6.0
		//     RC behavior. Stays the fallback so any incremental-mode
		//     regression can be ruled out by clearing the env var.
		//
		//   • EDIKT_PHASE_B_INCREMENTAL=1 — reload only the sidecars
		//     Phase A actually rewrote. Phase A never adds new pairs to
		//     the set (the dispatch loop above filters out
		//     `p.Sidecar == nil` cases as load errors before any task
		//     is built), so the original `pairs` slice is still
		//     authoritative for membership; only contents need
		//     refreshing. Reuse-with-incremental-reload is opt-in for
		//     v0.6.0 and slated to default on in v0.6.1 once soak data
		//     is in.
		if os.Getenv("EDIKT_PHASE_B_INCREMENTAL") == "1" {
			staleIDs := make(map[string]bool, len(tasks))
			for _, t := range tasks {
				staleIDs[t.ArtifactID] = true
			}
			for i := range pairs {
				if !staleIDs[pairs[i].ArtifactID] {
					continue
				}
				sc, lerr := sidecar.Load(pairs[i].SidecarPath)
				if lerr != nil {
					pairs[i].Sidecar = nil
					pairs[i].LoadErr = lerr
					continue
				}
				pairs[i].Sidecar = sc
				pairs[i].LoadErr = nil
			}
		} else {
			pairs, err = sidecar.Discover(opts.ProjectRoot, dirs)
			if err != nil {
				return res, fmt.Errorf("rediscover after phase A: %w", err)
			}
		}
	}

	if opts.CheckOnly {
		// Emit a verdict line even when no stale sidecars were found —
		// silent exit-0 in --check mode (regression rc≤7) made CI checks
		// indistinguishable from "compile is broken" vs "all good".
		if !opts.JSONMode {
			fmt.Fprintf(opts.Stderr, "edikt gov compile --check: up-to-date (%d sidecar(s), 0 stale)\n", len(pairs))
		}
		return res, nil
	}

	pbRes, err := phaseb.Merge(opts.ProjectRoot, pairs, phaseb.Options{
		CompiledAt:      clk.Now().Format(time.RFC3339),
		CompilerVersion: CompilerVersion,
	})
	if err != nil {
		return res, fmt.Errorf("phase B: %w", err)
	}
	res.PhaseB = pbRes

	// In JSON mode the caller emits a single JSON object on stdout; the
	// prose summary line is routed to stderr at a lower verbosity to avoid
	// breaking machine-readable consumers.
	summaryOut := opts.Stdout
	if opts.JSONMode {
		summaryOut = opts.Stderr
	}
	fmt.Fprintf(summaryOut, "Phase B — merged %d sidecar(s) into %d topic file(s) (%d rendered, %d unchanged), index %s\n",
		len(pairs),
		len(pbRes.TopicsRendered)+len(pbRes.TopicsUnchanged),
		len(pbRes.TopicsRendered),
		len(pbRes.TopicsUnchanged),
		writtenLabel(pbRes.IndexWritten),
	)
	return res, nil
}

// artifactTypeFromPath returns "adr", "invariant", or "guideline" based on
// which configured dir the parent .md sits under.
func artifactTypeFromPath(parentPath string, dirs []string) string {
	abs, _ := filepath.Abs(parentPath)
	if strings.Contains(abs, string(os.PathSeparator)+"decisions"+string(os.PathSeparator)) {
		return "adr"
	}
	if strings.Contains(abs, string(os.PathSeparator)+"invariants"+string(os.PathSeparator)) {
		return "invariant"
	}
	if strings.Contains(abs, string(os.PathSeparator)+"guidelines"+string(os.PathSeparator)) {
		return "guideline"
	}
	for _, d := range dirs {
		if strings.Contains(parentPath, d) {
			if strings.Contains(d, "decisions") {
				return "adr"
			}
			if strings.Contains(d, "invariants") {
				return "invariant"
			}
			if strings.Contains(d, "guidelines") {
				return "guideline"
			}
		}
	}
	return "adr"
}

func writtenLabel(b bool) string {
	if b {
		return "rewritten"
	}
	return "unchanged"
}

// acquireCompileLock takes an advisory flock on .edikt/state/compile.lock.
// When noWait is true, contention returns an error immediately rather than
// blocking. The returned func releases the lock and closes the file
// descriptor; the lock file itself is intentionally left in place
// (advisory locks survive across runs and the file is purely a lock
// token — no consumer reads its contents). Removing it after release
// would race against any concurrent process that just opened the same
// path; benign today on POSIX but a real bug if the file ever doubles
// as a PID record.
func acquireCompileLock(projectRoot string, noWait bool) (func(), error) {
	lockDir := filepath.Join(projectRoot, ".edikt", "state")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir lock dir: %w", err)
	}
	lockPath := filepath.Join(lockDir, "compile.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	how := syscall.LOCK_EX
	if noWait {
		how |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		if noWait {
			return nil, fmt.Errorf("compile already running (lock held); --no-wait set, exiting")
		}
		return nil, fmt.Errorf("acquire compile.lock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

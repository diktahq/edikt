// Package phasea implements the conditional resync phase of two-phase
// gov:compile per ADR-028. It dispatches stale-sidecar regenerations to
// the locked sidecar-extractor agent, one fresh subagent context per
// artifact, parallel up to a concurrency cap, continue-on-error.
package phasea

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
)

// Runner regenerates the sidecar for one artifact. Implementations are
// expected to be deterministic w.r.t. the parent prose body — actual LLM
// drift is bounded by the locked extractor prompt and per-artifact context
// isolation (ADR-027).
//
// The interface exists so tests can inject a fake runner without paying
// real-claude latency or requiring CLI auth in hermetic sandboxes
// (INV-007).
type Runner interface {
	Resync(ctx context.Context, t Task) error
}

// Task describes one Phase A unit of work.
type Task struct {
	ArtifactType string // "adr", "invariant", "guideline"
	ArtifactID   string // "ADR-001", "INV-005", or guideline slug
	ParentPath   string
	SidecarPath  string
}

// ClaudeRunner shells out to `claude code` headless and asks it to run
// the per-artifact :compile slash command. It is the production runner;
// tests use FakeRunner instead.
type ClaudeRunner struct {
	Binary string // override for the claude CLI path; defaults to "claude"
}

// Resync invokes `claude -p "/edikt:<type>:compile <id>"` and returns
// non-nil on a non-zero exit. Captured combined output is folded into the
// returned error so the dispatcher can log it.
//
// INV-006 belt-and-suspenders: ArtifactType and ArtifactID are re-validated
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
	prompt := fmt.Sprintf("/edikt:%s:compile %s", t.ArtifactType, t.ArtifactID)
	cmd := exec.CommandContext(ctx, bin, "-p", prompt)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude exit: %w; output: %s", err, truncate(string(out), 500))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

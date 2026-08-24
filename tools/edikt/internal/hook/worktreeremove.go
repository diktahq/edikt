package hook

// worktreeremove.go — the Go replacement for
// templates/hooks/worktree-remove.sh (PLAN-v080-reliability-core Phase 14).
// Logs the teardown. Idempotent: tolerates missing worktree state. Reads NO
// .edikt/config.yaml at all — not even for existence — unlike almost every
// other hook in this tier.

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("worktree-remove", worktreeRemove)
}

const worktreeRemoveNoPython3Message = "⚠ edikt: python3 is missing — worktree teardown was not logged."

func worktreeRemove(inv *Invocation) Result {
	if os.Getenv("EDIKT_WORKTREE_SKIP") == "1" {
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}

	// worktree-remove.sh's own `INPUT=$(cat)` runs BEFORE its python3
	// preflight, same ordering as subagent-start.sh/worktree-create.sh.
	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: worktree-remove — python3 not found; this teardown was not logged")
		return Result{Response: adapter.Response{Text: adapter.NoPython3ContinueBanner(worktreeRemoveNoPython3Message)}}
	}

	// `d.get("worktree_path", "") or d.get("path", "")` — both defaults are
	// "", so presence never changes the outcome.
	path := inv.Payload.WorktreePath
	if path == "" {
		path = inv.Payload.TopLevelPath
	}

	// `"${WORKTREE_PATH:-unknown}"` — bash's `:-` triggers on EMPTY as well
	// as unset, so an extracted-but-empty path also logs as "unknown".
	logPath := path
	if logPath == "" {
		logPath = "unknown"
	}

	home := os.Getenv("HOME")
	_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	if body, encErr := adapter.WorktreeEventJSON(ts, "worktree_remove", logPath); encErr == nil {
		appendEventLine(filepath.Join(home, ".edikt", "events.jsonl"), body)
	}

	return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
}

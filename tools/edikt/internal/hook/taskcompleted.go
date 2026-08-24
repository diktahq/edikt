package hook

// taskcompleted.go — the Go replacement for
// templates/hooks/task-completed.sh (PLAN-v080-reliability-core Phase 14).
// Closes the loop on TaskCreated: logs a structured event to
// $HOME/.edikt/events.jsonl. No plan-phase logic — that is task-created.go's
// own job.

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("task-completed", taskCompleted)
}

const taskCompletedNoPython3Message = "⚠ edikt: python3 is missing — task completion was not logged. Install python3 to restore task/plan-phase tracking."

func taskCompleted(inv *Invocation) Result {
	if os.Getenv("EDIKT_TASK_COMPLETED_SKIP") == "1" {
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}

	cwd, err := os.Getwd()
	if err != nil || !fileExists(filepath.Join(cwd, ".edikt", "config.yaml")) {
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}

	// ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────
	// task-completed.sh's own preflight runs BEFORE it reads stdin at all.
	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: task-completed — python3 not found; this completion was not logged")
		return Result{Response: adapter.Response{Text: adapter.NoPython3ContinueBanner(taskCompletedNoPython3Message)}}
	}

	taskID := inv.Payload.TaskID
	if taskID == "" {
		taskID = inv.Payload.ID
	}
	taskName := inv.Payload.TaskName
	if taskName == "" {
		taskName = inv.Payload.Name
	}
	if taskName == "" {
		// Unlike task-created.sh, TaskCompleted's final fallback default is
		// "" too — no Present() dependency, since every step of this chain
		// shares the same empty-string default.
		taskName = inv.Payload.Subject
	}

	status := inv.Payload.Status
	if status == "" {
		// `d.get("status", "") or d.get("result", "completed")` — the
		// SECOND get's own default ("completed") applies only when
		// "result" is genuinely absent, not present-and-empty
		// (adapter.Payload.Result's own doc comment).
		if inv.Payload.Present("Result") {
			status = inv.Payload.Result
		} else {
			status = "completed"
		}
	}

	home := os.Getenv("HOME")
	_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	if body, encErr := adapter.TaskCompletedEventJSON(ts, taskID, taskName, status); encErr == nil {
		appendEventLine(filepath.Join(home, ".edikt", "events.jsonl"), body)
	}

	return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
}

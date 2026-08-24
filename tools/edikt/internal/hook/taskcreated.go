package hook

// taskcreated.go — the Go replacement for templates/hooks/task-created.sh
// (PLAN-v080-reliability-core Phase 14). Logs task creation with the active
// plan-phase context to $HOME/.edikt/events.jsonl. Pairs with
// task-completed.sh to reconstruct plan progress.
//
// UNLIKE its file-changed/cwd-changed siblings, this hook's stdout is NEVER
// empty: every reachable branch (missing config, missing python3, the
// normal path) ends in a `{"continue": true}` reply — the missing-config and
// success paths both print the SAME bare literal via a hand-built printf,
// never python3 json.dumps, so both go through adapter.ContinueTrueReply
// rather than Result{}'s "write nothing" contract.
//
// NOT EXERCISED BY test/fixtures/hook-corpus/task-created/matrix.yaml: the
// PLAN_FILE resolution (find "PLAN-*.md" across PLANS_DIR / "$BASE/product/
// plans" / "docs/product/plans", picking the newest by mtime). The shared
// _base tree carries no plans directory at all (see that matrix's own
// header comment for why one was not added). Still ported faithfully below;
// end-to-end coverage lives in taskcreated_test.go, seeding a real
// PLAN-*.md the way userpromptsubmit_test.go's own end-to-end test does.

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("task-created", taskCreated)
}

const taskCreatedNoPython3Message = "⚠ edikt: python3 is missing — task creation was not logged. Install python3 to restore task/plan-phase tracking."

func taskCreated(inv *Invocation) Result {
	cwd, err := os.Getwd()
	if err != nil || !fileExists(filepath.Join(cwd, ".edikt", "config.yaml")) {
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}

	// ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────
	// task-created.sh's own preflight runs BEFORE it reads stdin at all —
	// unlike subagent-start.sh/worktree-create.sh/worktree-remove.sh below,
	// whose `INPUT=$(cat)` comes first.
	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: task-created — python3 not found; this task was not logged (plan-phase attribution lost)")
		return Result{Response: adapter.Response{Text: adapter.NoPython3ContinueBanner(taskCreatedNoPython3Message)}}
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
		// `d.get("subject", "unknown")` — the FINAL fallback's own default is
		// "unknown", which fires only when "subject" is genuinely absent, not
		// present-and-empty (adapter.Payload.Subject's own doc comment).
		if inv.Payload.Present("Subject") {
			taskName = inv.Payload.Subject
		} else {
			taskName = "unknown"
		}
	}

	configBytes, _ := os.ReadFile(filepath.Join(cwd, ".edikt", "config.yaml")) //nolint:gosec // project-local config
	base := configBase(configBytes)
	planDir := configPlanDir(configBytes, base)

	var planFile string
	for _, dir := range []string{planDir, filepath.Join(base, "product", "plans"), filepath.Join("docs", "product", "plans")} {
		if cand := mostRecentPlanFilePrefixed(dir, "PLAN-"); cand != "" {
			planFile = cand
			break
		}
	}

	phase := ""
	if planFile != "" {
		if body, rerr := os.ReadFile(planFile); rerr == nil { //nolint:gosec // project-local plan file
			if fields, ok := planPhaseRow(body); ok {
				phase = phaseNumDigits(phaseField(fields, 2))
			}
		}
	}
	plan := ""
	if planFile != "" {
		plan = filepath.Base(planFile)
	}

	home := os.Getenv("HOME")
	_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	if body, encErr := adapter.TaskCreatedEventJSON(ts, taskID, taskName, phase, plan); encErr == nil {
		appendEventLine(filepath.Join(home, ".edikt", "events.jsonl"), body)
	}

	return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
}

// mostRecentPlanFilePrefixed mirrors:
//
//	find "$dir" -maxdepth 1 -type f -name 'PLAN-*.md' | while read f; do
//	  printf "%s\t%s\n" "$(date -r "$f" +%s || stat -f %m "$f")" "$f"
//	done | sort -rn | head -1 | cut -f2
//
// Unlike planphase.go's mostRecentPlanFile (which post-compact.sh/
// user-prompt-submit.sh use — ANY *.md file, via `ls -t "$PLAN_DIR"/*.md`),
// task-created.sh's own `find` additionally requires the "PLAN-" prefix —
// a genuinely different bash pattern, not a redundant second copy of the
// same one. os.Stat().ModTime() is a single portable syscall wrapper with
// no GNU/BSD stat-flag ambiguity (F-059/D23), so there is no `date -r` /
// `stat -f` / `stat -c` fallback chain to reproduce here at all.
func mostRecentPlanFilePrefixed(dir, prefix string) string {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var (
		best     string
		bestTime int64
		found    bool
	)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || len(name) < len(prefix)+3 {
			continue
		}
		if name[:len(prefix)] != prefix || name[len(name)-3:] != ".md" {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		mt := info.ModTime().UnixNano()
		if !found || mt > bestTime {
			best, bestTime, found = filepath.Join(dir, name), mt, true
		}
	}
	return best
}

// appendEventLine appends body plus a trailing newline to path, creating
// parent directories as needed. Errors are swallowed, matching bash's own
// `>> "$EVENTS" 2>/dev/null || true`.
func appendEventLine(path string, body []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // matches bash's own log file mode
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(body)
	_, _ = f.Write([]byte("\n"))
}

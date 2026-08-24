package hook

// sessionend.go — the Go replacement for templates/hooks/session-end.sh
// (PLAN-v080-reliability-core Phase 12). Symmetric counterpart to
// session-start.go: flushes a per-session summary to $HOME/.edikt/state/
// last-session.json and always replies `{"continue": true}`.

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("session-end", sessionEnd)
}

const sessionEndNoPython3Message = "⚠ edikt: python3 is missing — the session summary was not written."

func sessionEnd(inv *Invocation) Result {
	// Fast opt-out — checked before even the .edikt/config.yaml test, exactly
	// as session-end.sh's own first line does.
	if os.Getenv("EDIKT_SESSION_END_SKIP") == "1" {
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}
	if !fileExists(filepath.Join(cwd, ".edikt", "config.yaml")) {
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}

	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: session-end — python3 not found; the session summary was not written")
		return Result{Response: adapter.Response{
			Text: adapter.NoPython3ContinueBanner(sessionEndNoPython3Message),
		}}
	}

	home := os.Getenv("HOME")
	_ = os.MkdirAll(filepath.Join(home, ".edikt", "state"), 0o755)
	summaryFile := filepath.Join(home, ".edikt", "state", "last-session.json")
	eventsFile := filepath.Join(home, ".edikt", "events.jsonl")

	// Rotate events.jsonl if over 10MB.
	if fi, statErr := os.Stat(eventsFile); statErr == nil {
		const tenMB = 10 * 1024 * 1024
		if fi.Size() > tenMB {
			date := time.Now().Format("20060102")
			_ = os.Rename(eventsFile, eventsFile+"."+date)
		}
	}

	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	body, encErr := adapter.LastSessionSummary(ts, inv.Payload.SessionID, cwd)
	if encErr != nil {
		// Mirrors the python except-block's own fallback write. Best-effort,
		// exactly as bash's trailing `|| true` treats it — a summary write
		// failure never changes the hook's own reply.
		if fb, ferr := adapter.LastSessionSummaryError(ts, encErr.Error()); ferr == nil {
			_ = os.WriteFile(summaryFile, fb, 0o644) //nolint:gosec // matches bash's own summary file mode
		}
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}
	_ = os.WriteFile(summaryFile, body, 0o644) //nolint:gosec // matches bash's own summary file mode

	return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
}

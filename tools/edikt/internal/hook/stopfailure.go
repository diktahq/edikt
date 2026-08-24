package hook

// stopfailure.go — the Go replacement for templates/hooks/stop-failure.sh
// (PLAN-v080-reliability-core Phase 12). Logs an API-error event to
// $HOME/.edikt/events.jsonl via the same append-only shape
// templates/hooks/event-log.sh's edikt_log_event() builds. This hook has no
// stdout output on any path — its whole job is the log write (or a stderr
// diagnostic when python3 is missing, a pattern this migration removes: the
// Go binary needs no python3 to build this JSON).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("stop-failure", stopFailure)
}

// stopFailureEvent mirrors edikt_log_event's field-insertion order exactly:
// the base dict starts as {error_type, message} (stop-failure.sh's own
// EVENT_JSON), then gains type, at, by via assignment — python dict
// insertion order, which json.MarshalIndent's struct FIELD order reproduces.
type stopFailureEvent struct {
	ErrorType string `json:"error_type"`
	Message   string `json:"message"`
	Type      string `json:"type"`
	At        string `json:"at"`
	By        string `json:"by"`
}

func stopFailure(inv *Invocation) Result {
	cwd, err := os.Getwd()
	if err != nil {
		return Result{}
	}
	if !fileExists(filepath.Join(cwd, ".edikt", "config.yaml")) {
		return Result{}
	}

	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: stop-failure — python3 not found; this API error was not logged")
		return Result{}
	}

	// `d.get('error',{}).get('type','unknown')` / `.get('message','')[:200]` —
	// "unknown"/"" only when the KEY is absent, not when it is present-and-empty
	// (Payload.Present distinguishes the two; see adapter.claudePayload's own
	// comment on why `error` is decoded leniently).
	errType := "unknown"
	if inv.Payload.Present("ErrorType") {
		errType = inv.Payload.ErrorType
	}
	errMsg := ""
	if inv.Payload.Present("ErrorMessage") {
		errMsg = truncateRunes(inv.Payload.ErrorMessage, 200)
	}

	home := os.Getenv("HOME")
	logEvent(home, "stop_failure", stopFailureEvent{ErrorType: errType, Message: errMsg})
	return Result{}
}

// truncateRunes mirrors python's `s[:n]` — sliced by CODE POINT, not byte,
// matching python3 str indexing exactly (a str is a sequence of codepoints).
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// logEvent mirrors templates/hooks/event-log.sh's edikt_log_event(): create
// the log file with 0600 if absent (re-tighten its mode if present — MED-8),
// append one JSON line built with `type`/`at`/`by` folded in.
func logEvent(home, eventType string, base stopFailureEvent) {
	logFile := filepath.Join(home, ".edikt", "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		return
	}
	if _, statErr := os.Stat(logFile); statErr != nil {
		f, cerr := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // matches event-log.sh's own MED-8 tightened mode
		if cerr != nil {
			return
		}
		_ = f.Close()
	} else {
		_ = os.Chmod(logFile, 0o600) //nolint:gosec // matches event-log.sh's own MED-8 tightened mode
	}

	base.Type = eventType
	base.At = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	base.By = gitUserEmail()

	body, err := adapter.MarshalPythonCompact(base)
	if err != nil {
		return
	}

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // matches event-log.sh's own mode
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(body)
	_, _ = f.Write([]byte("\n"))
}

// gitUserEmail mirrors: git config user.email 2>/dev/null || echo "unknown".
func gitUserEmail() string {
	out, err := exec.Command("git", "config", "user.email").Output() //nolint:gosec // fixed argv
	if err != nil {
		return "unknown"
	}
	email := string(out)
	for len(email) > 0 && (email[len(email)-1] == '\n' || email[len(email)-1] == '\r') {
		email = email[:len(email)-1]
	}
	// bash's fallback fires on a NON-ZERO exit only; `git config` succeeding
	// with genuinely empty stdout is left as "", not upgraded to "unknown" —
	// matching `user=$(git config user.email 2>/dev/null || echo "unknown")`
	// exactly rather than defensively.
	return email
}

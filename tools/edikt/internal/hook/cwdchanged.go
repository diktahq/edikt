package hook

// cwdchanged.go — the Go replacement for templates/hooks/cwd-changed.sh
// (PLAN-v080-reliability-core Phase 13). Logs a directory switch (monorepo
// awareness) to $HOME/.edikt/session-signals.log. Never writes to stdout on
// any path — CwdChanged has no established stdout-JSON contract, matching
// the original bash's own comment on why its no-python3 path only writes to
// stderr.

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func init() {
	Register("cwd-changed", cwdChanged)
}

func cwdChanged(inv *Invocation) Result {
	cwd, err := os.Getwd()
	if err != nil {
		return Result{}
	}
	if !fileExists(filepath.Join(cwd, ".edikt", "config.yaml")) {
		return Result{}
	}

	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: cwd-changed — python3 not found; this directory switch was not logged")
		return Result{}
	}

	newCwd := inv.Payload.Cwd
	if newCwd == "" {
		return Result{}
	}

	home := os.Getenv("HOME")
	_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
	logFile := filepath.Join(home, ".edikt", "session-signals.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // matches bash's own log file mode
	if err == nil {
		timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
		fmt.Fprintf(f, "%s CWD_CHANGED %s\n", timestamp, newCwd)
		_ = f.Close()
	}
	return Result{}
}

package hook

// filechanged.go — the Go replacement for templates/hooks/file-changed.sh
// (PLAN-v080-reliability-core Phase 13). Detects a governance file changed
// OUTSIDE Claude (another editor, git) and warns via systemMessage.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("file-changed", fileChanged)
}

func fileChanged(inv *Invocation) Result {
	cwd, err := os.Getwd()
	if err != nil {
		return Result{}
	}
	if !fileExists(filepath.Join(cwd, ".edikt", "config.yaml")) {
		return Result{}
	}

	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: file-changed — python3 not found; could not check whether the changed file was a governance file")
		return Result{Response: adapter.Response{
			Text: adapter.NoPython3SystemMessageBanner(adapter.FileChangedNoPython3Message),
		}}
	}

	changedFile := inv.Payload.FilePath
	if changedFile == "" {
		return Result{}
	}

	if !isGovernanceFilePath(changedFile) {
		return Result{}
	}

	home := os.Getenv("HOME")
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
	logFile := filepath.Join(home, ".edikt", "session-signals.log")
	f, ferr := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // matches bash's own log file mode
	if ferr == nil {
		fmt.Fprintf(f, "%s FILE_CHANGED_EXTERNAL %s\n", timestamp, changedFile)
		_ = f.Close()
	}

	msg := fmt.Sprintf("⚠ Governance file modified externally: %s. Run /edikt:gov:compile if this affects ADRs or invariants.", changedFile)
	// adapter.Response.Message is the host-neutral carrier for exactly this
	// `{"systemMessage": "..."}` shape — the dispatcher's own writeReply ->
	// adapter.Encode path renders it, so the wire field name "systemMessage"
	// never needs spelling out here (SP-004/SAC-003). Unlike the fixed-literal
	// NoPython3SystemMessageBanner above, msg is caller-built from a
	// payload-derived file path, so it goes through Encode's proper
	// JSON-escaping rather than a hand-built string.
	return Result{Response: adapter.Response{Message: msg}}
}

// isGovernanceFilePath mirrors file-changed.sh's own
// `case "$CHANGED_FILE" in *.claude/rules/*|*.edikt/*|*docs/architecture/decisions/*|*docs/architecture/invariants/*)`
// — a bash glob `*` matches any sequence including '/', so each arm is a
// plain substring test, not a path-segment-aware or prefix-anchored one.
// edikt's own directory-naming convention, not host wire vocabulary, so this
// lives in the handler rather than the adapter (SP-004/SAC-003 scope only
// the HOST'S spellings, not edikt's own).
func isGovernanceFilePath(path string) bool {
	for _, seg := range []string{
		adapter.RulesDirSegment,
		".edikt/",
		"docs/architecture/decisions/",
		"docs/architecture/invariants/",
	} {
		if strings.Contains(path, seg) {
			return true
		}
	}
	return false
}

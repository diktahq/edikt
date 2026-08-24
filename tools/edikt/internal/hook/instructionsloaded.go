package hook

// instructionsloaded.go — the Go replacement for
// templates/hooks/instructions-loaded.sh (PLAN-v080-reliability-core Phase
// 12). Logs which rule packs are active to $HOME/.edikt/session-signals.log.
//
// F-055 (docs/internal/plans/PLAN-SPEC-011-progressive-governance-delivery.md):
// the FILE variable this hook keys off is read from
// CLAUDE_TOOL_INPUT_FILE_PATH / CLAUDE_TOOL_INPUT_PATH — environment
// variables the real stdin-payload protocol never sets. Confirmed live: this
// hook is a no-op on every real invocation today. That is preserved exactly,
// not fixed — this migration's charter is byte-for-byte behavioural parity,
// and "make the hook do something a real payload never lets it do" is a
// product decision for a different change, not something a migration PR
// gets to smuggle in. See adapter.ToolInputFilePath's own doc comment for
// the same point from the wire-vocabulary side.
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("instructions-loaded", instructionsLoaded)
}

func instructionsLoaded(inv *Invocation) Result {
	cwd, err := os.Getwd()
	if err != nil {
		return Result{}
	}
	if !fileExists(filepath.Join(cwd, ".edikt", "config.yaml")) {
		return Result{}
	}

	file := adapter.ToolInputFilePath()
	if file == "" {
		return Result{}
	}

	if !adapter.IsRulesFilePath(file) {
		return Result{}
	}

	ruleName := strings.TrimSuffix(filepath.Base(file), ".md")
	home := os.Getenv("HOME")
	_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
	logFile := filepath.Join(home, ".edikt", "session-signals.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // matches bash's own log file mode
	if err == nil {
		timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
		fmt.Fprintf(f, "%s RULE_LOADED %s\n", timestamp, ruleName)
		_ = f.Close()
	}
	return Result{}
}

package hook

// posttooluse.go — the Go replacement for templates/hooks/post-tool-use.sh
// (PLAN-v080-reliability-core Phase 13). Auto-formats a written file, silently,
// via whatever formatter is installed for its language. Zero python3 calls in
// the bash original (python3_baseline_count: 0) — this hook exists purely to
// retire the interpreter-free shell body behind the same FR-004 shim shape.
//
// FILE comes ONLY from CLAUDE_TOOL_INPUT_FILE_PATH / CLAUDE_TOOL_INPUT_PATH —
// the same env-var channel instructions-loaded.sh reads (F-055) — never from
// the payload's own tool_input, matching the bash original's own FILE
// resolution exactly (it never reads stdin at all).
//
// FORMATTER STDOUT PASSES THROUGH UNREDIRECTED, matching the bash original:
// every arm redirects the formatter's STDERR to /dev/null (`2>/dev/null`) but
// none redirects its stdout, so whatever the formatter prints goes straight to
// the hook's own stdout — a pre-existing behaviour this migration preserves
// rather than fixes. cmd.Stdout = os.Stdout reproduces that fd inheritance;
// cmd.Stderr = nil (Go's own os/exec contract: nil connects to /dev/null)
// reproduces `2>/dev/null`.
import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("post-tool-use", postToolUse)
}

// postToolUseValidFileRe mirrors bash's `case "$FILE" in *[!A-Za-z0-9_./-]*)
// exit 0 ;; esac`: a FILE containing ANY byte outside this set is rejected
// outright, before any formatter ever sees it.
var postToolUseValidFileRe = regexp.MustCompile(`[^A-Za-z0-9_./-]`)

func postToolUse(inv *Invocation) Result {
	if os.Getenv("EDIKT_FORMAT_SKIP") == "1" {
		return Result{}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return Result{}
	}
	configPath := filepath.Join(cwd, ".edikt", "config.yaml")
	configBytes, cerr := os.ReadFile(configPath) //nolint:gosec // project-local config
	if cerr != nil {
		return Result{}
	}
	// `grep -q "auto-format: false"` — a plain substring test, not anchored,
	// not YAML-aware. Matches the bash original exactly.
	if strings.Contains(string(configBytes), "auto-format: false") {
		return Result{}
	}

	file := adapter.ToolInputFilePath()
	if file == "" {
		return Result{}
	}
	if !fileExists(file) {
		return Result{}
	}
	if postToolUseValidFileRe.MatchString(file) {
		return Result{}
	}

	ext := file
	if i := strings.LastIndex(file, "."); i >= 0 {
		ext = file[i+1:]
	}

	switch ext {
	case "go":
		if commandAvailable("gofmt") {
			runFormatter("gofmt", "-w", file)
		}
	case "ts", "tsx", "js", "jsx":
		if fileExists("node_modules/.bin/prettier") {
			runFormatter("node_modules/.bin/prettier", "--write", file)
		} else if commandAvailable("prettier") {
			runFormatter("prettier", "--write", file)
		}
	case "py":
		if commandAvailable("black") {
			runFormatter("black", file)
		} else if commandAvailable("ruff") {
			runFormatter("ruff", "format", file)
		}
	case "rb":
		if commandAvailable("rubocop") {
			runFormatter("rubocop", "-A", file)
		}
	case "php":
		if commandAvailable("php-cs-fixer") {
			runFormatter("php-cs-fixer", "fix", file)
		}
	case "rs":
		if commandAvailable("rustfmt") {
			runFormatter("rustfmt", file)
		}
	}

	return Result{}
}

// runFormatter execs a formatter exactly as the bash original did: its own
// stdout inherited (never redirected there), its stderr discarded
// (`2>/dev/null`), and any failure swallowed (`|| true` / an unchecked
// command in bash both leave the hook's own exit code untouched).
func runFormatter(name string, args ...string) {
	cmd := exec.Command(name, args...) //nolint:gosec // formatter name is one of a fixed switch of literals; args is the validated FILE
	cmd.Stdout = os.Stdout
	cmd.Stderr = nil // matches bash's `2>/dev/null`
	_ = cmd.Run()
}

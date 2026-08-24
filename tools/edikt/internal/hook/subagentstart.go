package hook

// subagentstart.go — the Go replacement for
// templates/hooks/subagent-start.sh (PLAN-v080-reliability-core Phase 14).
// Injects a governance heads-up (ADR/INV counts) into every subagent spawn.
// Pairs with the existing subagent-stop.sh.
//
// UNLIKE task-created.go/task-completed.go, subagent-start.sh's own
// `INPUT=$(cat)` runs BEFORE its python3 preflight — the same ordering
// worktree-create.sh/worktree-remove.sh share, which is why their no-python3
// corpus scenarios all need the `.corpus-bin` cat stand-in rather than a
// plain empty PATH.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("subagent-start", subagentStart)
}

const subagentStartNoPython3Message = "⚠ edikt: python3 is missing on this host — the governance heads-up (ADR/INV count and citation reminder) could not be generated for this subagent. Proceed without it; do not assume no governance applies."

// subagentStartFrontmatterAcceptedRe / subagentStartBoldAcceptedRe mirror:
//
//	grep -l -E '^status:[[:space:]]*accepted|^\*\*Status:\*\*[[:space:]]+Accepted'
//
// grep -E with no -i flag: case-SENSITIVE, exactly as written.
var (
	subagentStartFrontmatterAcceptedRe = regexp.MustCompile(`^status:[ \t]*accepted`)
	subagentStartBoldAcceptedRe        = regexp.MustCompile(`^\*\*Status:\*\*[ \t]+Accepted`)
	subagentStartFrontmatterActiveRe   = regexp.MustCompile(`^status:[ \t]*active`)
	subagentStartBoldActiveRe          = regexp.MustCompile(`^\*\*Status:\*\*[ \t]+Active`)
)

func subagentStart(inv *Invocation) Result {
	if os.Getenv("EDIKT_SUBAGENT_START_SKIP") == "1" {
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}

	cwd, err := os.Getwd()
	if err != nil || !fileExists(filepath.Join(cwd, ".edikt", "config.yaml")) {
		return Result{Response: adapter.Response{Text: adapter.ContinueTrueReply}}
	}

	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: subagent-start — python3 not found; governance heads-up not delivered to this subagent")
		banner := adapter.NoPython3HookSpecificBanner("SubagentStart", subagentStartNoPython3Message)
		return Result{Response: adapter.Response{Text: banner}}
	}

	subagentType := inv.Payload.SubagentTypeField
	if subagentType == "" {
		subagentType = inv.Payload.AgentField
	}

	adrCount := countGovernanceFiles(filepath.Join(cwd, "docs", "architecture", "decisions"), "ADR-",
		subagentStartFrontmatterAcceptedRe, subagentStartBoldAcceptedRe)
	invCount := countGovernanceFiles(filepath.Join(cwd, "docs", "architecture", "invariants"), "INV-",
		subagentStartFrontmatterActiveRe, subagentStartBoldActiveRe)

	msg := adapter.SubagentStartMessage(subagentType, adrCount, invCount)
	return Result{Response: adapter.Response{Context: msg}}
}

// countGovernanceFiles mirrors:
//
//	grep -l -E '<pattern>' "$DIR"/<prefix>*.md | wc -l
//
// Counts FILES with at least one matching line, not matching lines.
// Returns 0 (not an error) when dir does not exist, matching bash's own
// `[ -d "$DIR" ]` guard, which leaves *_COUNT at its default 0.
func countGovernanceFiles(dir, prefix string, patterns ...*regexp.Regexp) int {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || len(name) < len(prefix)+3 || name[:len(prefix)] != prefix || name[len(name)-3:] != ".md" {
			continue
		}
		body, rerr := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // project-local governance file
		if rerr != nil {
			continue
		}
		if fileHasAnyLineMatch(body, patterns) {
			count++
		}
	}
	return count
}

func fileHasAnyLineMatch(body []byte, patterns []*regexp.Regexp) bool {
	lines := splitLines(body)
	for _, line := range lines {
		for _, re := range patterns {
			if re.MatchString(line) {
				return true
			}
		}
	}
	return false
}

func splitLines(body []byte) []string {
	var lines []string
	start := 0
	for i, b := range body {
		if b == '\n' {
			lines = append(lines, string(body[start:i]))
			start = i + 1
		}
	}
	if start < len(body) {
		lines = append(lines, string(body[start:]))
	}
	return lines
}

package hook

// sessionstart.go — the Go replacement for templates/hooks/session-start.sh
// (PLAN-v080-reliability-core Phase 12).
//
// Ports the bash body verbatim, including its dead code: every reachable
// path terminates by calling bash's `emit` (which always ends in `exit 0`),
// so the "surface the most recent unresolved gate finding" block at the tail
// of session-start.sh is UNREACHABLE — every branch above it already exits.
// This is not ported. Skipping unreachable code changes no observable
// behaviour (which is the whole bar this migration holds itself to); porting
// it would risk MAKING it reachable by accident, which is a behaviour change
// the corpus could not have caught (its bash oracle can never take that
// branch either).
//
// NOT EXERCISED BY test/fixtures/hook-corpus/session-start/matrix.yaml, BUT
// STILL IMPLEMENTED HERE: the memory-file-present branches (stale warning,
// fresh-with-git-classification). MEMORY.md's mtime comes from git checkout,
// not from a fixture author, so a scenario asserting a specific age would
// drift the moment the repo is re-cloned — see the matrix's own header
// comment. The logic below is still complete, because real projects (with a
// real memory file and a real git history) exercise all of it.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("session-start", sessionStart)
}

// sessionStartNoPython3Message is the literal text session-start.sh's own
// python3-preflight printf carries. The wire shape it goes into
// (hookSpecificOutput/hookEventName/additionalContext) is built by
// adapter.NoPython3HookSpecificBanner, never spelled out here (SP-004/SAC-003).
const sessionStartNoPython3Message = "⚠ edikt: python3 is missing on this host — the session summary (changed files, relevant agents, memory age) and last-session gate-finding check could not run. Governance rules still load normally; only this summary is affected."

const (
	sessionStartMsgDetected       = "📋 edikt project detected. Run /edikt:context to load project context before writing code."
	sessionStartMsgStaleFmt       = "⚠️  edikt memory is %dd old. Run /edikt:context to refresh."
	sessionStartMsgSummaryOffFmt  = "📋 edikt project — memory %dd old. Run /edikt:context to load context."
	sessionStartMsgFallbackFmt    = "📋 edikt — %dd since last session. Run /edikt:context to load context."
	sessionStartMsgAgentsFmtBlock = "📋 edikt — since your last session (%dd ago):\n   %s changed\n   Relevant agents: %s\n   Run /edikt:context to load full project context."
)

var (
	sessionStartMigrationRe = regexp.MustCompile(`(?i)migration|schema|\.sql`)
	sessionStartInfraRe     = regexp.MustCompile(`(?i)docker|compose|\.tf|helm|k8s|Dockerfile`)
	sessionStartSecurityRe  = regexp.MustCompile(`(?i)auth|jwt|oauth|payment|token|secret`)
	sessionStartAPIRe       = regexp.MustCompile(`(?i)route|handler|controller|api|endpoint`)
)

func sessionStart(inv *Invocation) Result {
	// Only run in edikt projects — the process's OWN cwd, exactly as bash
	// checks the RELATIVE path '.edikt/config.yaml' against its own $PWD,
	// never the host-reported payload.cwd (which can legitimately differ from
	// where the process actually runs, e.g. every hook-corpus sandbox).
	cwd, err := os.Getwd()
	if err != nil {
		return Result{}
	}
	configPath := filepath.Join(cwd, ".edikt", "config.yaml")
	configBytes, err := os.ReadFile(configPath) //nolint:gosec // project-local config, same trust boundary bash's `.edikt/config.yaml` read carries
	if err != nil {
		return Result{}
	}

	// ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────
	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: session-start — python3 not found; session summary and gate-finding check skipped")
		banner := adapter.NoPython3HookSpecificBanner("SessionStart", sessionStartNoPython3Message)
		return Result{Response: adapter.Response{Text: banner}}
	}

	home := os.Getenv("HOME")

	// Clear gate overrides from previous session. bash's `> file 2>/dev/null
	// || true` truncates-or-creates but FAILS SILENTLY if the parent
	// directory does not exist yet — and it runs BEFORE the mkdir -p below,
	// so that failure mode is real, not theoretical.
	_ = truncateOrCreate(filepath.Join(home, ".edikt", "gate-overrides.jsonl"))

	// Rotate session signals log — archive previous session, start fresh.
	_ = os.MkdirAll(filepath.Join(home, ".edikt"), 0o755)
	logFile := filepath.Join(home, ".edikt", "session-signals.log")
	if fileExists(logFile) {
		_ = os.Rename(logFile, logFile+".prev")
	}

	memory := adapter.MemoryPath(home, cwd)

	age := int64(0)
	memoryPresent := false
	if fi, statErr := os.Stat(memory); statErr == nil && !fi.IsDir() {
		memoryPresent = true
		age = (time.Now().Unix() - fi.ModTime().Unix()) / 86400
	}

	if bytes.Contains(configBytes, []byte("session-summary: false")) {
		switch {
		case !memoryPresent:
			return emitText(sessionStartMsgDetected)
		case age > 7:
			return emitText(fmt.Sprintf(sessionStartMsgStaleFmt, age))
		default:
			return emitText(fmt.Sprintf(sessionStartMsgSummaryOffFmt, age))
		}
	}

	if !memoryPresent {
		return emitText(sessionStartMsgDetected)
	}
	if age > 7 {
		return emitText(fmt.Sprintf(sessionStartMsgStaleFmt, age))
	}

	changed := sessionStartChangedFiles(cwd, memory)
	if len(changed) == 0 {
		return emitText(fmt.Sprintf(sessionStartMsgFallbackFmt, age))
	}

	summary, agents := sessionStartClassify(changed)
	if agents == "" {
		return emitText(fmt.Sprintf(sessionStartMsgFallbackFmt, age))
	}
	msg := fmt.Sprintf(sessionStartMsgAgentsFmtBlock, age, summary, agents)
	return emitText(msg)
}

// emitText wraps msg the way session-start.sh's emit() does: a python3
// json.dumps({"hookSpecificOutput": {...}}) call, reproduced by the shared
// dispatcher/adapter encoder (which now matches json.dumps' own formatting —
// see adapter.MarshalPythonCompact).
func emitText(msg string) Result {
	return Result{Response: adapter.Response{Context: msg}}
}

// commandAvailable mirrors bash's `command -v <name> >/dev/null 2>&1`.
func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// truncateOrCreate mirrors bash's `> path`: create-or-truncate to zero bytes,
// failing (silently, at the call site) if the parent directory is absent —
// it does NOT create parent directories, unlike os.WriteFile combined with
// MkdirAll would.
func truncateOrCreate(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // matches bash `>` semantics
	if err != nil {
		return err
	}
	return f.Close()
}

// sessionStartChangedFiles mirrors:
//
//	git log --since="$MTIME" --name-only --pretty=format: 2>/dev/null | grep -v '^$' | sort -u
//
// MTIME is the memory file's own mtime, formatted the way `date -r "$MEMORY"
// '+%Y-%m-%dT%H:%M:%S'` renders it (local time, no zone suffix — git accepts
// this form for --since). Errors (not a git repo, git absent) are swallowed
// exactly as bash's `2>/dev/null` swallows them: an empty change set, not a
// crash.
func sessionStartChangedFiles(cwd, memory string) []string {
	fi, err := os.Stat(memory)
	if err != nil {
		return nil
	}
	mtime := fi.ModTime().Format("2006-01-02T15:04:05")
	cmd := exec.Command("git", "log", "--since="+mtime, "--name-only", "--pretty=format:") //nolint:gosec // fixed argv, no interpolated shell
	cmd.Dir = cwd
	out, _ := cmd.Output()
	seen := map[string]bool{}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		files = append(files, line)
	}
	sort.Strings(files) // `sort -u` — already deduped above, this supplies the sort
	return files
}

// sessionStartClassify mirrors the four `grep -ciE` category counts and the
// SUMMARY/AGENTS string-building that follows them. grep -c counts MATCHING
// LINES, not total occurrences, so each category is a per-file boolean, not a
// per-file match count.
func sessionStartClassify(changed []string) (summary, agents string) {
	nMigration, nInfra, nSecurity, nAPI := 0, 0, 0, 0
	for _, f := range changed {
		if sessionStartMigrationRe.MatchString(f) {
			nMigration++
		}
		if sessionStartInfraRe.MatchString(f) {
			nInfra++
		}
		if sessionStartSecurityRe.MatchString(f) {
			nSecurity++
		}
		if sessionStartAPIRe.MatchString(f) {
			nAPI++
		}
	}
	var sb, ab strings.Builder
	if nMigration > 0 {
		fmt.Fprintf(&sb, "%d migration/schema file(s), ", nMigration)
		ab.WriteString("dba, ")
	}
	if nInfra > 0 {
		fmt.Fprintf(&sb, "%d infra file(s), ", nInfra)
		ab.WriteString("sre, ")
	}
	if nSecurity > 0 {
		fmt.Fprintf(&sb, "%d security file(s), ", nSecurity)
		ab.WriteString("security, ")
	}
	if nAPI > 0 {
		fmt.Fprintf(&sb, "%d API file(s), ", nAPI)
		ab.WriteString("api, ")
	}
	summary = strings.TrimSuffix(sb.String(), ", ")
	agents = strings.TrimSuffix(ab.String(), ", ")
	return summary, agents
}

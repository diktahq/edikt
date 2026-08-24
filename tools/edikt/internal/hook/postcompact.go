package hook

// postcompact.go — the Go replacement for templates/hooks/post-compact.sh
// (PLAN-v080-reliability-core Phase 12). Re-injects the active plan phase and
// invariants after context compaction.
//
// NOT EXERCISED BY test/fixtures/hook-corpus/post-compact/matrix.yaml, BUT
// STILL IMPLEMENTED HERE: the plan-phase-table branch (planPhaseRow,
// EVAL_STATE parsing, "Context Needed" extraction). The base fixture tree's
// docs/plans/ is empty by design (see _base/README.md), so PLAN_DIR is never
// a real directory in any recorded scenario — see matrix.yaml's own header
// comment for why inventing a phase-table fixture was judged not worth the
// risk of encoding an unverified table convention as gate-tested truth. The
// invariants branch (the OTHER half of this hook's job) IS exercised, via
// _base/project/docs/architecture/invariants/'s one fixture file.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("post-compact", postCompact)
}

const postCompactNoPython3Message = "edikt: python3 is missing on this host — the active plan phase and invariants could not be re-injected after context compaction. Run /edikt:context to recover manually."

func postCompact(inv *Invocation) Result {
	cwd, err := os.Getwd()
	if err != nil {
		return Result{}
	}
	configPath := filepath.Join(cwd, ".edikt", "config.yaml")
	configBytes, err := os.ReadFile(configPath) //nolint:gosec // project-local config
	if err != nil {
		return Result{}
	}

	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: post-compact — python3 not found; plan phase and invariants were not re-injected after compaction")
		banner := adapter.NoPython3HookSpecificBanner("PostCompact", postCompactNoPython3Message)
		return Result{Response: adapter.Response{Text: banner}}
	}

	base := configBase(configBytes)
	planDir := configPlanDir(configBytes, base)
	planMsg := postCompactPlanMessage(cwd, planDir)
	invMsg := postCompactInvariantsMessage(cwd, base)

	if planMsg == "" && invMsg == "" {
		return Result{}
	}

	msg := "Context recovered after compaction."
	if planMsg != "" {
		msg += " " + planMsg
	}
	if invMsg != "" {
		msg += " " + invMsg
	}
	return Result{Response: adapter.Response{Context: msg}}
}

func postCompactPlanMessage(cwd, planDir string) string {
	// mostRecentPlanFile is handed the RELATIVE planDir, not
	// filepath.Join(cwd, planDir): the process's actual working directory is
	// always cwd (that is what os.Getwd() returned it as), so a relative
	// path resolves identically — and, unlike an absolute one, matches what
	// bash's own `ls -t "$PLAN_DIR"/*.md` produced (PLAN_DIR was never
	// resolved to an absolute path there either). Joining cwd in produced an
	// absolute planPath in the "Re-read <path> for full phase details."
	// message — an observable behaviour change ADR-063's byte-for-byte
	// parity charter forbids, caught by test/unit/hooks/test_post_compact.sh's
	// mid-plan fixture once EDIKT_ROOT made that fixture reach the real
	// binary instead of silently no-opping on binary resolution.
	planPath := mostRecentPlanFile(planDir)
	if planPath == "" {
		return ""
	}
	body, err := os.ReadFile(planPath) //nolint:gosec // project-local plan file
	if err != nil {
		return ""
	}
	fields, ok := planPhaseRow(body)
	if !ok {
		return ""
	}
	phaseNum := phaseNumDigits(phaseField(fields, 2))
	phaseTheme := phaseField(fields, 3)
	planName := planNameFromHeading(body)
	attempt := phaseField(fields, 4)
	if !attemptFormatRe.MatchString(attempt) {
		attempt = ""
	}

	evalState := filepath.Join(cwd, ".edikt", "state", "plan-eval", strings.TrimSuffix(filepath.Base(planPath), ".md")+"-eval.json")
	failMsg := postCompactFailMessage(evalState)
	contextMsg := postCompactContextMessage(body, phaseNum)

	msg := fmt.Sprintf("Active plan: %s. Phase %s", planName, phaseNum)
	if phaseTheme != "" {
		msg += " — " + phaseTheme
	}
	if attempt != "" {
		msg += fmt.Sprintf(" (attempt %s)", attempt)
	}
	msg += "."
	if failMsg != "" {
		msg += " " + failMsg + "."
	}
	if contextMsg != "" {
		msg += " " + contextMsg
	}
	msg += fmt.Sprintf(" Re-read %s for full phase details.", planPath)
	return msg
}

// postCompactFailMessage mirrors the EVAL_STATE python heredoc: collect
// every criterion with status "fail" across every phase, and the FIRST
// non-empty fail_reason encountered.
func postCompactFailMessage(evalStatePath string) string {
	raw, err := os.ReadFile(evalStatePath) //nolint:gosec // project-local state file
	if err != nil {
		return ""
	}
	var state struct {
		Phases []struct {
			Criteria []struct {
				ID         json.RawMessage `json:"id"`
				Status     string          `json:"status"`
				FailReason json.RawMessage `json:"fail_reason"`
			} `json:"criteria"`
		} `json:"phases"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return ""
	}
	var ids []string
	reason := ""
	for _, p := range state.Phases {
		for _, c := range p.Criteria {
			if c.Status != "fail" {
				continue
			}
			ids = append(ids, jsonScalarToString(c.ID, "?"))
			if reason == "" {
				if r := jsonScalarToString(c.FailReason, ""); r != "" {
					reason = r
				}
			}
		}
	}
	if len(ids) == 0 {
		return ""
	}
	msg := "Last failing criteria: " + strings.Join(ids, ",")
	if reason != "" {
		msg += fmt.Sprintf(" (%s)", reason)
	}
	return msg
}

// jsonScalarToString stringifies a raw JSON scalar (string or number) the
// way python's str() does for the same decoded value; def is returned for a
// missing/null field.
func jsonScalarToString(raw json.RawMessage, def string) string {
	if len(raw) == 0 || string(raw) == "null" {
		return def
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return def
}

// postCompactInvariantsMessage mirrors:
//
//	for dir in "$BASE/invariants" "$BASE/architecture/invariants"; do [ -d "$dir" ] && INV_DIR="$dir" && break; done
//	INV_COUNT=$(ls "$INV_DIR"/*.md | wc -l)
//	INV_NAMES=$(ls "$INV_DIR"/*.md | xargs -I{} head -1 {} | sed 's/^# //' | paste -sd', ' -)
func postCompactInvariantsMessage(cwd, base string) string {
	var invDir string
	for _, d := range []string{filepath.Join(base, "invariants"), filepath.Join(base, "architecture", "invariants")} {
		full := filepath.Join(cwd, d)
		if fi, err := os.Stat(full); err == nil && fi.IsDir() {
			invDir = full
			break
		}
	}
	if invDir == "" {
		return ""
	}
	entries, err := os.ReadDir(invDir)
	if err != nil {
		return ""
	}
	var mdFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			mdFiles = append(mdFiles, e.Name())
		}
	}
	if len(mdFiles) == 0 {
		return ""
	}
	sort.Strings(mdFiles) // matches `ls`'s default alphabetical order
	var names []string
	for _, f := range mdFiles {
		body, rerr := os.ReadFile(filepath.Join(invDir, f)) //nolint:gosec // project-local invariant file
		if rerr != nil {
			continue
		}
		names = append(names, strings.TrimPrefix(firstLine(body), "# "))
	}
	return fmt.Sprintf("Invariants (%d): %s. These are hard constraints — never violate them.",
		len(mdFiles), strings.Join(names, ", "))
}

func firstLine(b []byte) string {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}

// postCompactContextMessage mirrors the sed/grep "Context Needed:" bullet
// extraction. Best-effort: this branch is unreachable from the committed
// corpus (see this file's own header comment).
func postCompactContextMessage(planBody []byte, phaseNum string) string {
	lines := extractContextNeededBullets(planBody, phaseNum)
	if len(lines) == 0 {
		return ""
	}
	return "Before continuing, read:\n  " + strings.Join(lines, "\n")
}

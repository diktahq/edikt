package hook

// userpromptsubmit.go — the Go replacement for
// templates/hooks/user-prompt-submit.sh (PLAN-v080-reliability-core Phase
// 12). Injects the active plan phase into every prompt as additionalContext
// (model-facing, never systemMessage — F-045/ADR-061 §1).
//
// NOT EXERCISED BY test/fixtures/hook-corpus/user-prompt-submit/matrix.yaml:
// the phase-detection branch. See that matrix's own header comment for why —
// short version: the base tree's docs/plans/ is empty by design, and adding a
// plan fixture now would silently invalidate post-compact's
// already-recorded, already-diff-verified baseline (the two hooks share one
// _base tree; scenario_input_sha256 does not pin the base tree's own
// contents). Still ported faithfully below, including its own two-tier phase
// regex, which is NOT the same pattern post-compact.sh (and its Go
// counterpart, planPhaseRow) use.

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func init() {
	Register("user-prompt-submit", userPromptSubmit)
}

const userPromptSubmitNoPython3Message = "edikt: python3 is missing on this host — the active plan phase is not being injected into your prompts. Run /edikt:context to check phase state manually."

// userPromptSubmitStrictPhaseRe is user-prompt-submit.sh's PRIMARY phase
// pattern — anchored at line start, case-SENSITIVE, no "Phase " word option,
// and "in.progress" ('.' = any single char) rather than post-compact.sh's
// "in[_ -]progress". Deliberately a different regexp.Regexp value from
// planPhaseRe: the two hooks' bash bodies carry genuinely different patterns,
// and merging them would be a behaviour change dressed as a refactor.
var userPromptSubmitStrictPhaseRe = regexp.MustCompile(`^\| *[0-9]+ *\|.*in.progress`)

func userPromptSubmit(inv *Invocation) Result {
	cwd, err := os.Getwd()
	if err != nil {
		return Result{}
	}
	configPath := filepath.Join(cwd, ".edikt", "config.yaml")
	configBytes, err := os.ReadFile(configPath) //nolint:gosec // project-local config
	if err != nil {
		return Result{}
	}
	if bytes.Contains(configBytes, []byte("plan-injection: false")) {
		return Result{}
	}

	if !commandAvailable("python3") {
		fmt.Fprintln(inv.Stderr, "edikt: user-prompt-submit — python3 not found; active plan phase was not injected into this prompt")
		banner := adapter.NoPython3HookSpecificBanner("UserPromptSubmit", userPromptSubmitNoPython3Message)
		return Result{Response: adapter.Response{Text: banner}}
	}

	base := configBase(configBytes)
	planDir := configPlanDir(configBytes, base)
	if fi, statErr := os.Stat(planDir); statErr != nil || !fi.IsDir() {
		return Result{}
	}
	// mostRecentPlanFile gets the RELATIVE planDir, not filepath.Join(cwd,
	// planDir): the process's actual working directory is always cwd, so a
	// relative path resolves identically — and, unlike an absolute one,
	// matches what bash's own `find "$PLAN_DIR" -maxdepth 1 -name '*.md'`
	// produced (PLAN_DIR was never resolved to an absolute path there
	// either). See postcompact.go's postCompactPlanMessage for the same fix
	// and the fixture that caught the drift.
	planPath := mostRecentPlanFile(planDir)
	if planPath == "" {
		return Result{}
	}
	body, err := os.ReadFile(planPath) //nolint:gosec // project-local plan file
	if err != nil {
		return Result{}
	}

	fields, ok := userPromptSubmitPhaseRow(body)
	if !ok {
		return Result{}
	}
	phaseNum := phaseNumDigits(phaseField(fields, 2))
	phaseTheme := phaseField(fields, 3)
	planName := planNameFromHeading(body)

	msg := fmt.Sprintf("Active plan: %s. Current phase: %s", planName, phaseNum)
	if phaseTheme != "" {
		msg += " — " + phaseTheme
	}
	msg += fmt.Sprintf(". Read %s for full context if needed.", planPath)
	return Result{Response: adapter.Response{Context: msg}}
}

// userPromptSubmitPhaseRow mirrors:
//
//	PHASE=$(grep -E '^\| *[0-9]+ *\|.*in.progress' "$PLAN" | head -1)
//	if [ -z "$PHASE" ]; then
//	  PHASE=$(grep -iE '\| *(Phase )?[0-9]+ *\|.*in[_ -]progress' "$PLAN" | head -1)
//	fi
func userPromptSubmitPhaseRow(planBody []byte) (fields []string, ok bool) {
	sc := bufio.NewScanner(bytes.NewReader(planBody))
	for sc.Scan() {
		line := sc.Text()
		if userPromptSubmitStrictPhaseRe.MatchString(line) {
			return strings.Split(line, "|"), true
		}
	}
	return planPhaseRow(planBody)
}

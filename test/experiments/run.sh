#!/usr/bin/env bash
# edikt experiment runner — v0.3.0 Phase 6
#
# Orchestrates a pre-registered experiment per the ADR-008/ADR-009 validation
# methodology. Mocks nothing — invokes the real `claude` CLI against fixture
# projects with and without an invariant loaded into context, then runs the
# committed assertion script to classify each run as pass/violation.
#
# Usage:
#   ./test/experiments/run.sh 01-multi-tenancy
#   ./test/experiments/run.sh 02-money-precision
#   ./test/experiments/run.sh 03-timezone-awareness
#   ./test/experiments/run.sh all
#   ./test/experiments/run.sh 01-multi-tenancy --dry-run
#
# Per the pre-registration (docs/architecture/proposals/PROPOSAL-001-spec/experiments/):
# N=10 per condition, model version recorded, full transcripts preserved, no
# silent deletions, negative results honestly reported.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
FIXTURES_DIR="$SCRIPT_DIR/fixtures"
RESULTS_DIR="$SCRIPT_DIR/results"

N_RUNS="${EDIKT_EXP_N:-10}"
DRY_RUN=false

# ---------- colors ----------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
DIM='\033[2m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { echo -e "${GREEN}>${RESET} $1"; }
warn()  { echo -e "${YELLOW}!${RESET} $1"; }
error() { echo -e "${RED}error:${RESET} $1" >&2; exit 1; }

# ---------- arg parsing ----------
if [ $# -eq 0 ]; then
    error "Usage: $0 <experiment-id | all> [--dry-run]

Available experiments:
  01-multi-tenancy
  02-money-precision
  03-timezone-awareness
  all

See test/experiments/README.md for details."
fi

EXPERIMENT="$1"
shift
while [ $# -gt 0 ]; do
    case "$1" in
        --dry-run) DRY_RUN=true ;;
        *) error "unknown flag: $1" ;;
    esac
    shift
done

# ---------- preflight ----------
if ! command -v claude >/dev/null 2>&1; then
    error "\`claude\` CLI not found. Install Claude Code first: https://docs.claude.com/en/docs/claude-code"
fi

if ! command -v shasum >/dev/null 2>&1 && ! command -v sha256sum >/dev/null 2>&1; then
    error "neither \`shasum\` nor \`sha256sum\` is available — install one of them"
fi

SHA256_CMD="shasum -a 256"
if ! command -v shasum >/dev/null 2>&1; then
    SHA256_CMD="sha256sum"
fi

# ---------- metadata capture ----------
capture_metadata() {
    local output_file="$1"
    local experiment_id="$2"
    local fixture_dir="$3"

    {
        echo "experiment: $experiment_id"
        echo "run_date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
        echo "claude_code_version: $(claude --version 2>/dev/null || echo 'unknown')"
        echo "edikt_git_sha: $(cd "$REPO_ROOT" && git rev-parse HEAD 2>/dev/null || echo 'unknown')"
        echo "fixture_dir: $fixture_dir"
        echo "n_per_condition: $N_RUNS"
        echo "prompt_hash: $($SHA256_CMD "$fixture_dir/prompt.txt" | awk '{print $1}')"
        echo "invariant_hash: $($SHA256_CMD "$fixture_dir/invariant.md" | awk '{print $1}')"
        echo "assertion_hash: $($SHA256_CMD "$fixture_dir/assertion.sh" | awk '{print $1}')"
        echo "runner_hash: $($SHA256_CMD "$SCRIPT_DIR/run.sh" | awk '{print $1}')"
    } > "$output_file"
}

# ---------- single run ----------
# Copies the fixture project to a tmp dir, optionally loads the invariant into
# Claude's context via .claude/rules/, invokes `claude -p "$prompt"`, captures
# the transcript, and runs the assertion script on the result.
run_one() {
    local fixture_dir="$1"
    local condition="$2"  # "baseline" or "invariant-loaded"
    local run_number="$3"
    local output_dir="$4"

    local tmpdir
    tmpdir="$(mktemp -d -t edikt-exp.XXXXXX)"
    # NOTE: we clean up manually at the end of the function instead of using
    # `trap ... RETURN` because the RETURN trap fires after local variables
    # leave scope, and `set -u` (nounset) errors on the now-undefined $tmpdir.

    # Copy fixture project to tmp location
    cp -R "$fixture_dir/project"/* "$tmpdir/"
    # Also copy hidden files (.claude, .edikt) if present
    if [ -d "$fixture_dir/project/.claude" ]; then
        cp -R "$fixture_dir/project/.claude" "$tmpdir/.claude"
    fi
    if [ -d "$fixture_dir/project/.edikt" ]; then
        cp -R "$fixture_dir/project/.edikt" "$tmpdir/.edikt"
    fi

    # For invariant-loaded condition, place the invariant file where Claude Code
    # will read it. We use a .claude/rules/ location because Claude Code
    # auto-loads files from there as context.
    if [ "$condition" = "invariant-loaded" ]; then
        mkdir -p "$tmpdir/.claude/rules"
        cp "$fixture_dir/invariant.md" "$tmpdir/.claude/rules/experiment-invariant.md"
    fi

    local prompt
    prompt="$(cat "$fixture_dir/prompt.txt")"
    local transcript_file="$output_dir/${condition}/run-$(printf '%02d' "$run_number").txt"
    local verdict_file="$output_dir/${condition}/run-$(printf '%02d' "$run_number")-verdict.txt"

    if $DRY_RUN; then
        echo "DRY: would invoke claude -p in $tmpdir with prompt: $prompt" > "$transcript_file"
        echo "DRY: pass" > "$verdict_file"
        return 0
    fi

    # Invoke Claude Code headless. -p / --print runs a single prompt and exits.
    # --output-format text keeps the output uncluttered for archival.
    local claude_output
    set +e
    claude_output=$(cd "$tmpdir" && claude -p "$prompt" --output-format text 2>&1)
    local claude_exit=$?
    set -e

    # Save the raw transcript
    {
        echo "=== prompt ==="
        cat "$fixture_dir/prompt.txt"
        echo ""
        echo "=== claude exit: $claude_exit ==="
        echo ""
        echo "=== claude output ==="
        echo "$claude_output"
        echo ""
        echo "=== files modified ==="
        (cd "$tmpdir" && find . -type f -newer "$fixture_dir/prompt.txt" 2>/dev/null \
            | grep -v '^\./\.claude' \
            | grep -v '^\./\.edikt' \
            | head -20)
    } > "$transcript_file"

    # Run the assertion script on the tmp project
    local assertion_result
    set +e
    assertion_result=$(cd "$tmpdir" && bash "$fixture_dir/assertion.sh" 2>&1)
    local assertion_exit=$?
    set -e

    {
        echo "exit: $assertion_exit"
        if [ $assertion_exit -eq 0 ]; then
            echo "verdict: PASS"
        else
            echo "verdict: VIOLATION"
        fi
        echo "details:"
        echo "$assertion_result"
    } > "$verdict_file"

    if [ $assertion_exit -eq 0 ]; then
        echo -e "  ${DIM}run $(printf '%02d' "$run_number"): ${GREEN}pass${RESET}"
    else
        echo -e "  ${DIM}run $(printf '%02d' "$run_number"): ${RED}violation${RESET}"
    fi

    # Manual cleanup (see note at top of function about not using trap RETURN)
    rm -rf "$tmpdir"
}

# ---------- single experiment ----------
run_experiment() {
    local experiment_id="$1"
    local fixture_dir="$FIXTURES_DIR/$experiment_id"

    if [ ! -d "$fixture_dir" ]; then
        error "experiment fixture not found: $fixture_dir"
    fi

    for required in project prompt.txt invariant.md assertion.sh; do
        if [ ! -e "$fixture_dir/$required" ]; then
            error "fixture missing required file: $fixture_dir/$required"
        fi
    done

    local date_slug
    date_slug="$(date -u +%Y-%m-%d)"
    local output_dir="$RESULTS_DIR/${experiment_id}-${date_slug}"
    mkdir -p "$output_dir/baseline" "$output_dir/invariant-loaded"

    info "Running experiment: $experiment_id"
    info "Output directory:   ${output_dir#$REPO_ROOT/}"
    info "N per condition:    $N_RUNS"

    capture_metadata "$output_dir/metadata.txt" "$experiment_id" "$fixture_dir"

    # --- baseline condition ---
    echo ""
    echo -e "${BOLD}Condition A: baseline (no invariant loaded)${RESET}"
    local baseline_violations=0
    for i in $(seq 1 "$N_RUNS"); do
        run_one "$fixture_dir" "baseline" "$i" "$output_dir"
        if grep -q "VIOLATION" "$output_dir/baseline/run-$(printf '%02d' "$i")-verdict.txt"; then
            baseline_violations=$((baseline_violations + 1))
        fi
    done

    # --- invariant-loaded condition ---
    echo ""
    echo -e "${BOLD}Condition B: invariant-loaded${RESET}"
    local invariant_violations=0
    for i in $(seq 1 "$N_RUNS"); do
        run_one "$fixture_dir" "invariant-loaded" "$i" "$output_dir"
        if grep -q "VIOLATION" "$output_dir/invariant-loaded/run-$(printf '%02d' "$i")-verdict.txt"; then
            invariant_violations=$((invariant_violations + 1))
        fi
    done

    # --- summary ---
    echo ""
    echo -e "${BOLD}Summary:${RESET}"
    echo "  Baseline violations:          $baseline_violations / $N_RUNS"
    echo "  Invariant-loaded violations:  $invariant_violations / $N_RUNS"
    echo "  Delta:                        $((baseline_violations - invariant_violations)) (improvement if positive)"

    {
        echo "# Results — $experiment_id"
        echo ""
        echo "**Date:** $(date -u +%Y-%m-%d)"
        echo "**Claude Code version:** $(claude --version 2>/dev/null || echo 'unknown')"
        echo "**edikt commit:** $(cd "$REPO_ROOT" && git rev-parse HEAD 2>/dev/null || echo 'unknown')"
        echo "**N per condition:** $N_RUNS"
        echo ""
        echo "## Results"
        echo ""
        echo "- **Baseline (no invariant):** $baseline_violations / $N_RUNS violations"
        echo "- **Invariant-loaded:** $invariant_violations / $N_RUNS violations"
        echo "- **Delta:** $((baseline_violations - invariant_violations)) (positive = invariant helped)"
        echo ""
        echo "## Hypothesis verdict"
        echo ""
        local half=$((N_RUNS / 2))
        if [ "$baseline_violations" -ge "$half" ] && [ "$invariant_violations" -le 1 ]; then
            echo "✅ **Effect confirmed**"
            echo ""
            echo "Baseline failure rate exceeded the threshold ($half+) and invariant-loaded rate dropped to ≤1."
        elif [ "$baseline_violations" -ge "$half" ] && [ "$invariant_violations" -lt "$baseline_violations" ]; then
            echo "⚠ **Weak effect**"
            echo ""
            echo "Baseline failure rate exceeded threshold but invariant-loaded rate is still above 1."
            echo "Consider running with higher N or investigating the remaining violations."
        elif [ "$baseline_violations" -lt "$half" ]; then
            echo "❌ **Effect absent**"
            echo ""
            echo "Baseline failure rate was below threshold — Claude already handles this well."
            echo "The 'Claude blind spot' hypothesis does not hold for this invariant on this model."
        elif [ "$invariant_violations" -gt "$baseline_violations" ]; then
            echo "🔄 **Effect inverted**"
            echo ""
            echo "Invariant-loaded runs had MORE violations than baseline. Investigate."
        fi
        echo ""
        echo "## Limitations"
        echo ""
        echo "- Context-size confound not controlled for."
        echo "- N=$N_RUNS is small; results are directional."
        echo "- Single fixture, single prompt, single Claude model version."
        echo ""
        echo "## Transcripts"
        echo ""
        echo "Full per-run outputs in \`baseline/\` and \`invariant-loaded/\` subdirectories."
    } > "$output_dir/summary.md"

    info "Summary written to: ${output_dir#$REPO_ROOT/}/summary.md"
}

# ---------- dispatch ----------
case "$EXPERIMENT" in
    all)
        for exp in 01-multi-tenancy 02-money-precision 03-timezone-awareness; do
            run_experiment "$exp"
            echo ""
        done
        ;;
    01-multi-tenancy|02-money-precision|03-timezone-awareness)
        run_experiment "$EXPERIMENT"
        ;;
    *)
        error "unknown experiment: $EXPERIMENT"
        ;;
esac

info "Done. Commit results: git add test/experiments/results/"

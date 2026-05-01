#!/usr/bin/env bash
# Phase 11b characterization: post-tool-use.sh
# Hook auto-formats files via CLAUDE_TOOL_INPUT_FILE_PATH env var.
# Characterization: env var absent in harness → exit 0 silently → {}.
# Format invocations (gofmt, prettier, etc.) are not exercised here —
# those require the target file on disk and the formatter installed.
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"
. "$PROJECT_ROOT/test/unit/hooks/_staged_runner.sh"

HOOK="post-tool-use.sh"
STAGED="$STAGED_PROJECTS/edikt-project"
FIXTURES=(post-tool-use-go post-tool-use-ts post-tool-use-unknown-ext)

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: $HOOK — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

FAIL=0
for f in "${FIXTURES[@]}"; do
    run_staged_fixture "$HOOK" "$f" "$STAGED" || FAIL=1
done
exit "$FAIL"

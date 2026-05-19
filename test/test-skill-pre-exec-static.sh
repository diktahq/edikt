#!/usr/bin/env bash
# Lint: slash-command pre-exec blocks (!`...`) must be statically analyzable
# by Claude Code's permission analyzer — that means single binary invocations,
# not multi-stage shell pipelines.
#
# This caught the rc≤8 regression where 5 slash commands shipped with
# !`bash -c '...long pipeline...'` blocks that Claude Code blocked at the
# permission gate, breaking /edikt:sdlc:{spec,prd,plan}, /edikt:adr:new,
# and /edikt:invariant:new.
#
# Rule: any !`...` block must NOT contain shell metacharacters that imply
# a pipeline or compound command: |, ;, &&, $(, `bash -c`. Single binary
# calls (e.g. !`${HOME}/.edikt/bin/edikt next-id spec`) pass cleanly.

set -euo pipefail
PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
cd "$PROJECT_ROOT"

found_violations=0
while IFS=: read -r file line content; do
    case "$content" in
        *'!`'*'bash -c'*)
            echo "FAIL: $file:$line — !\`bash -c ...\` pipeline (analyzer-blocked)"
            found_violations=$((found_violations + 1))
            ;;
        *'!`'*' | '*|*'!`'*'$('*|*'!`'*' && '*|*'!`'*'; '*)
            echo "FAIL: $file:$line — !\`...\` contains pipe/subst/compound (analyzer-blocked)"
            found_violations=$((found_violations + 1))
            ;;
    esac
done < <(grep -nE '^!`' commands/**/*.md commands/*.md 2>/dev/null || true)

if [ "$found_violations" -gt 0 ]; then
    echo ""
    echo "Refactor to a single-binary invocation: !\`\${HOME}/.edikt/bin/edikt <subcommand> <args>\`"
    echo "If you need new functionality, add a Go subcommand under tools/edikt/cmd/ first."
    exit 1
fi

echo "test-skill-pre-exec-static: OK (0 analyzer-blocked patterns in commands/*.md)"

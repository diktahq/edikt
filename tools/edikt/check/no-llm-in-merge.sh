#!/usr/bin/env bash
# no-llm-in-merge.sh — Phase B purity static check (ADR-028).
#
# Phase B (tools/edikt/internal/phaseb/) is the deterministic merge step of
# gov:compile. It MUST NOT dispatch subagents, shell out, or make network
# calls. CI runs this as a build gate so accidental LLM creep into the
# merge path fails fast.
#
# The check uses `go list -deps` so it sees the transitive import closure
# rather than just the package's own source — that catches a forbidden
# symbol leaking in via a helper package too.
#
# Usage:  tools/edikt/check/no-llm-in-merge.sh [--quiet]

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
TOOLS_DIR="$ROOT/tools/edikt"
PKG_PATH="github.com/diktahq/edikt/tools/edikt/internal/phaseb"

if [[ ! -d "$TOOLS_DIR/internal/phaseb" ]]; then
  echo "no-llm-in-merge: $TOOLS_DIR/internal/phaseb not found" >&2
  exit 2
fi

QUIET=0
[[ "${1:-}" == "--quiet" ]] && QUIET=1

# Forbidden transitive imports. Each line is a Go import path that must not
# appear in the closure of phaseb. The list is conservative: any package
# that is plausibly LLM- or subprocess-related is on it.
forbidden_imports=(
  "os/exec"
  "net/http"
  "net/rpc"
  "github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

cd "$TOOLS_DIR"
deps=$(go list -deps -f '{{.ImportPath}}' "$PKG_PATH" 2>/dev/null)

violations=0
for imp in "${forbidden_imports[@]}"; do
  if grep -Fx "$imp" <<<"$deps" >/dev/null; then
    if [[ $QUIET -eq 0 ]]; then
      echo "VIOLATION: '$imp' is a transitive import of $PKG_PATH" >&2
    fi
    violations=$((violations + 1))
  fi
done

# Source-level grep for explicit subprocess/LLM call sites (in case a future
# refactor moves them under a non-flagged import path). Comments and string
# literals are filtered out by ignoring lines that start with `//` after
# trimming and lines whose match sits inside a `"..."` literal.
src_patterns=(
  '\bexec\.Command\b'
  '\bexec\.CommandContext\b'
)

PHASEB_DIR="$TOOLS_DIR/internal/phaseb"
for pat in "${src_patterns[@]}"; do
  # Recursive grep over the package dir; --include filters to non-test
  # source files. Quoting the dir keeps paths-with-spaces working
  # (no risk in the current tree, but the script ships in tools/ and
  # is callable from anywhere).
  hits=$(grep -REn --include='*.go' --exclude='*_test.go' "$pat" "$PHASEB_DIR" 2>/dev/null || true)
  if [[ -n "$hits" ]]; then
    # Strip pure-comment lines before flagging.
    real_hits=$(awk -F: 'NF>=3 { line=$3; sub(/^[ \t]+/, "", line); if (substr(line,1,2) != "//") print }' <<<"$hits")
    if [[ -n "$real_hits" ]]; then
      if [[ $QUIET -eq 0 ]]; then
        echo "VIOLATION: '$pat' found in non-comment source of phaseb:" >&2
        echo "$real_hits" >&2
      fi
      violations=$((violations + 1))
    fi
  fi
done

if [[ $violations -gt 0 ]]; then
  echo "no-llm-in-merge: $violations violation(s) — Phase B must remain pure (ADR-028)" >&2
  exit 1
fi

[[ $QUIET -eq 0 ]] && echo "no-llm-in-merge: phaseb is pure (no LLM/subprocess imports)"
exit 0

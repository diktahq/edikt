#!/usr/bin/env bash
# Phase 2 of PLAN-v060-governance-accuracy — sidecar-extractor smoke test.
#
# Invokes the live sidecar-extractor agent (via `claude -p` headless) on a
# curated v0.4.3-era ADR fixture. Asserts the four extractor prompt rules
# A–D produced the expected output:
#
#   Rule A (paths inference)         — paths[] populated with directory globs
#   Rule B (scope defaults)          — scope[] includes 'design' or 'review'
#   Rule C (prohibition synthesis)   — prohibitions[] has ≥1 entry derived
#                                       from a rejected option in
#                                       ## Considered Options
#   Rule D (modality preservation)   — no `directives[].text` containing the
#                                       substring "Fallback:" was promoted
#                                       to MUST
#
# Gating:
#   EDIKT_SKIP_LLM_TESTS=1 — skip the test (CI default for cost control;
#                            INV-007 sandbox rule for hermetic tests).
#
# Exit codes:
#   0  — all assertions pass OR test was skipped
#   1  — at least one assertion failed
#   77 — claude CLI not on PATH and EDIKT_SKIP_LLM_TESTS not set
#        (autotools convention for "test cannot run in this environment")

set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$PROJECT_ROOT"

GREEN='\033[0;32m'
RED='\033[0;31m'
DIM='\033[0;90m'
RESET='\033[0m'

if [ "${EDIKT_SKIP_LLM_TESTS:-0}" = "1" ]; then
  echo -e "${DIM}extractor-smoke.sh: skipped (EDIKT_SKIP_LLM_TESTS=1)${RESET}"
  exit 0
fi

if ! command -v claude >/dev/null 2>&1; then
  echo -e "${DIM}extractor-smoke.sh: claude CLI not on PATH; cannot run live extractor.${RESET}"
  echo -e "${DIM}  Set EDIKT_SKIP_LLM_TESTS=1 to suppress this check.${RESET}"
  exit 77
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Stage a curated v0.4.3-era ADR with rejected options + Fallback: prose to
# exercise Rules C and D specifically.
cat >"$WORK/ADR-099-test.md" <<'EOF'
---
type: adr
id: ADR-099
title: Test ADR for extractor smoke
status: Accepted
date: 2026-05-03
---

# ADR-099 — Test ADR

## Context

Test fixture for the sidecar-extractor smoke test. References specific
file paths in `internal/stt/` and `internal/voice/` so Rule A produces
paths[] entries.

## Considered Options

### A. Single-stage Gemini Live
- Pros: low latency
- Cons: no reliable speaker diarization (prompt-based, not native)

### B. Two-stage Deepgram + Claude
- Pros: real-time streaming diarization, Go SDK
- Cons: two services to manage

## Decision

Two-stage pipeline: Deepgram Nova-3 for transcription + diarization,
Claude for DDD extraction.

Stage 1 (Deepgram): WebSocket streaming → diarized segments via
`internal/stt/provider.go`.

Stage 2 (Claude): processes segments via `internal/voice/extractor.go`.

Fallback: OpenAI gpt-4o-transcribe-diarize for transcription if Deepgram
is unreachable.
EOF

# Compose the extractor prompt from the agent template + the input path.
AGENT_PROMPT="$(cat templates/agents/sidecar-extractor.md)"
INPUT_PATH="$WORK/ADR-099-test.md"
OUTPUT_PATH="$WORK/ADR-099-test.edikt.yaml"

echo "extractor-smoke.sh: invoking claude -p (this takes 30–60s)..."

# Use claude -p with the agent template loaded as the system prompt.
# Allowed tools restricted per the agent's frontmatter (Read, Write).
USER_MSG="Extract a sidecar from this artifact: $INPUT_PATH

The sidecar must be written to: $OUTPUT_PATH

Apply Rules A (paths), B (scope), C (prohibition synthesis from rejected options A), and D (modality preservation for the 'Fallback:' line)."

if ! echo "$USER_MSG" | claude -p \
  --system-prompt "$AGENT_PROMPT" \
  --allowedTools "Read,Write" \
  --disallowedTools "Edit,Bash,Agent,Task" \
  --bare \
  >"$WORK/claude-stdout.log" 2>"$WORK/claude-stderr.log"; then
  echo -e "${RED}claude -p invocation failed${RESET}" >&2
  cat "$WORK/claude-stderr.log" >&2
  exit 1
fi

if [ ! -f "$OUTPUT_PATH" ]; then
  echo -e "${RED}extractor did not produce $OUTPUT_PATH${RESET}" >&2
  echo "claude stdout:" >&2
  head -40 "$WORK/claude-stdout.log" >&2
  exit 1
fi

# Assertions.
pass_count=0
fail_count=0

check() {
  local label="$1"
  local cmd="$2"
  if eval "$cmd" >/dev/null 2>&1; then
    echo -e "  ${GREEN}+${RESET} $label"
    pass_count=$((pass_count + 1))
  else
    echo -e "  ${RED}x${RESET} $label"
    fail_count=$((fail_count + 1))
  fi
}

echo "Phase 2 — sidecar-extractor smoke test"
check "Rule A: paths[] populated" "grep -E '^paths:' '$OUTPUT_PATH'"
check "Rule A: paths[] includes internal/stt or internal/voice glob" "grep -E '(internal/stt|internal/voice)' '$OUTPUT_PATH'"
check "Rule B: scope[] populated" "grep -E '^scope:' '$OUTPUT_PATH'"
check "Rule C: prohibitions[] has ≥1 entry" "grep -E '^prohibitions:' '$OUTPUT_PATH' && grep -E 'derived_from:|MUST NOT' '$OUTPUT_PATH'"
check "Rule C: prohibition references Gemini / single-stage" "grep -iE 'gemini|single.stage|diarization' '$OUTPUT_PATH'"
check "Rule D: no MUST directive contains literal 'Fallback:'" "! grep -E 'text:.*MUST.*Fallback:' '$OUTPUT_PATH'"
check "Rule D: Fallback line preserved as MAY (or omitted)" "grep -iE 'fallback.*MAY|MAY.*OpenAI' '$OUTPUT_PATH' || ! grep -iE 'OpenAI.*MUST|MUST.*OpenAI' '$OUTPUT_PATH'"

echo
echo -e "${DIM}${pass_count} passed, ${fail_count} failed.${RESET}"

if [ "$fail_count" -gt 0 ]; then
  echo
  echo -e "${DIM}Generated sidecar (for debugging):${RESET}"
  cat "$OUTPUT_PATH"
  exit 1
fi
exit 0

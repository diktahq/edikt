#!/usr/bin/env bash
# edikt: statusLine hook — emit governance health summary
#
# Opt-in: only emits output when .edikt/config.yaml has
#   features:
#     statusline: true
#
# Output format: plain text (not JSON). Claude Code renders statusLine
# output directly, so `{"systemMessage": ...}` wrapping is NOT used here.
#
# Emits:  ADRs: N | INVs: M | Drift: K
# Where:
#   N = count of accepted ADRs
#   M = count of active invariants
#   K = count of open drift findings (reads .edikt/state/drift-report.json
#       if present, else 0)
#
# Performance: cached to $HOME/.edikt/state/statusline-cache on a 60s TTL
# to avoid fork/find overhead on every refreshInterval tick. Opt-out via
# EDIKT_STATUSLINE_SKIP=1 environment variable.

# Fast opt-out (for users who want to suppress the statusline tick entirely)
[ "${EDIKT_STATUSLINE_SKIP:-0}" = "1" ] && exit 0

# Walk up from $PWD to find .edikt/config.yaml (cwd-independent per
# docs/internal/preprocessor-contract.md). Fast-fail if cwd is outside any
# edikt project — avoids the grep/find cost on every tick.
CFG=""
D="$PWD"
while [ "$D" != "/" ]; do
    [ -f "$D/.edikt/config.yaml" ] && CFG="$D/.edikt/config.yaml" && break
    D=$(dirname "$D")
done
[ -z "$CFG" ] && exit 0  # no config, emit nothing

# Cache layer: emit cached result if within TTL
CACHE_DIR="$HOME/.edikt/state"
CACHE_FILE="$CACHE_DIR/statusline-cache"
CACHE_TTL=60
if [ -f "$CACHE_FILE" ]; then
    AGE=$(( $(date +%s) - $(stat -f %m "$CACHE_FILE" 2>/dev/null || stat -c %Y "$CACHE_FILE" 2>/dev/null || echo 0) ))
    if [ "$AGE" -lt "$CACHE_TTL" ]; then
        cat "$CACHE_FILE" 2>/dev/null
        exit 0
    fi
fi

# Check opt-in feature flag
STATUSLINE_ENABLED=$(awk '
    /^features:/ { in_features = 1; next }
    in_features && /^[[:space:]]+statusline:/ {
        sub(/.*statusline:[[:space:]]*/, "")
        sub(/[[:space:]]*#.*$/, "")
        print
        exit
    }
    /^[^[:space:]]/ && in_features { in_features = 0 }
' "$CFG" 2>/dev/null)

if [ "$STATUSLINE_ENABLED" != "true" ]; then
    exit 0  # opt-in; produce no output when disabled
fi

PROOT=$(dirname "$(dirname "$CFG")")

# Resolve paths (use same pattern as the preprocessor contract)
resolve_path() {
    local key="$1" default_sub="$2"
    local rel
    rel=$(grep "^  ${key}:" "$CFG" 2>/dev/null | awk '{print $2}' | tr -d '"')
    if [ -z "$rel" ]; then
        local base
        base=$(grep '^base:' "$CFG" 2>/dev/null | awk '{print $2}' | tr -d '"')
        base="${base:-docs}"
        rel="$base/$default_sub"
    fi
    case "$rel" in
        /*) echo "$rel" ;;
        *) echo "$PROOT/$rel" ;;
    esac
}

DECISIONS_DIR=$(resolve_path decisions architecture/decisions)
INVARIANTS_DIR=$(resolve_path invariants architecture/invariants)

# Count accepted ADRs (match both YAML frontmatter and legacy bolded prose)
ADR_COUNT=0
if [ -d "$DECISIONS_DIR" ]; then
    ADR_COUNT=$(grep -l -E '^status:[[:space:]]*accepted|^\*\*Status:\*\*[[:space:]]+Accepted' "$DECISIONS_DIR"/ADR-*.md 2>/dev/null | wc -l | tr -d ' ')
fi

# Count active invariants
INV_COUNT=0
if [ -d "$INVARIANTS_DIR" ]; then
    INV_COUNT=$(grep -l -E '^status:[[:space:]]*active|^\*\*Status:\*\*[[:space:]]+Active' "$INVARIANTS_DIR"/INV-*.md 2>/dev/null | wc -l | tr -d ' ')
fi

# Count drift findings (if cached)
DRIFT_COUNT=0
DRIFT_REPORT="$PROOT/.edikt/state/drift-report.json"
if [ -f "$DRIFT_REPORT" ]; then
    # Extract `findings_open` field from simple JSON; fall back to 0
    DRIFT_COUNT=$(grep -oE '"findings_open"[[:space:]]*:[[:space:]]*[0-9]+' "$DRIFT_REPORT" 2>/dev/null | grep -oE '[0-9]+' | head -1)
    DRIFT_COUNT="${DRIFT_COUNT:-0}"
fi

OUTPUT=$(printf "ADRs: %s | INVs: %s | Drift: %s" "$ADR_COUNT" "$INV_COUNT" "$DRIFT_COUNT")
# Write to cache and emit
mkdir -p "$CACHE_DIR" 2>/dev/null || true
printf '%s' "$OUTPUT" > "$CACHE_FILE" 2>/dev/null || true
printf '%s' "$OUTPUT"

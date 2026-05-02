#!/usr/bin/env bash
# Phase 7b: stop-hook sidecar drift detection.
#
# Asserts the hook:
#   1. emits a fixed-template systemMessage warning with a count when any
#      <artifact>.edikt.yaml's directive quotes no longer match the parent
#      .md (per ADR-027/028 drift contract);
#   2. writes the stale artifact-ID list to .edikt/state/stale-sidecars.log
#      so /edikt:gov:compile can consume it out-of-band;
#   3. NEVER interpolates filenames or excerpts into the systemMessage
#      (INV-004 — the warning carries cardinality only);
#   4. emits {"continue": true} when there is no drift, and clears any
#      stale stale-sidecars.log left from a previous session;
#   5. degrades softly if PyYAML is unavailable (existing signals still emit).
#
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
HOOK="$PROJECT_ROOT/templates/hooks/stop-hook.sh"

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: stop-hook (sidecar drift) — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

if ! command -v python3 >/dev/null 2>&1; then
    echo "  SKIP: stop-hook (sidecar drift) — python3 not on PATH"
    exit 0
fi

if ! python3 -c 'import yaml' 2>/dev/null; then
    echo "  SKIP: stop-hook (sidecar drift) — PyYAML not installed; drift check soft-degrades"
    exit 0
fi

FAIL=0
TMPROOT=$(mktemp -d -t edikt-stop-sidecar-XXXXXX)
trap 'rm -rf "$TMPROOT"' EXIT INT TERM

# ─── Fixture builders ────────────────────────────────────────────────────────

# Drifted fixture: ADR-001's body no longer contains the quote the sidecar
# recorded at line_start..line_end. Triggers stale detection.
build_drifted() {
    local d="$1"
    mkdir -p "$d/.edikt/state" "$d/docs/architecture/decisions"
    cat > "$d/.edikt/config.yaml" <<'EOF'
base: docs
edikt_version: "0.6.0"
EOF
    cat > "$d/docs/architecture/decisions/ADR-001-test.md" <<'EOF'
# ADR-001 — Test ADR

## Decision

The body has been edited and the original directive sentence is gone.
EOF
    cat > "$d/docs/architecture/decisions/ADR-001-test.edikt.yaml" <<'EOF'
schema_version: 1
topic: test-topic
path: docs/architecture/decisions/ADR-001-test.md
signals: []
directives:
  - text: "MUST do the original thing"
    source_excerpt:
      line_start: 5
      line_end: 5
      quote: "MUST do the original thing"
EOF
}

# Clean fixture: sidecar's quote matches the body exactly. No drift.
build_clean() {
    local d="$1"
    mkdir -p "$d/.edikt/state" "$d/docs/architecture/decisions"
    cat > "$d/.edikt/config.yaml" <<'EOF'
base: docs
edikt_version: "0.6.0"
EOF
    cat > "$d/docs/architecture/decisions/ADR-002-clean.md" <<'EOF'
# ADR-002 — Clean ADR

## Decision

MUST always do the right thing.
EOF
    cat > "$d/docs/architecture/decisions/ADR-002-clean.edikt.yaml" <<'EOF'
schema_version: 1
topic: clean-topic
path: docs/architecture/decisions/ADR-002-clean.md
signals: []
directives:
  - text: "MUST always do the right thing"
    source_excerpt:
      line_start: 5
      line_end: 5
      quote: "MUST always do the right thing"
EOF
}

# Bare project: .edikt/config.yaml only, no governance dirs at all.
build_bare() {
    local d="$1"
    mkdir -p "$d/.edikt/state"
    cat > "$d/.edikt/config.yaml" <<'EOF'
base: docs
edikt_version: "0.6.0"
EOF
}

# Stop payload that does NOT trigger any of the regex signals so the only
# possible signal is sidecar drift.
NEUTRAL_PAYLOAD='{"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"Refactored helper.","cwd":"/tmp"}'

# ─── Test cases ─────────────────────────────────────────────────────────────

case_drift_detected() {
    local d="$TMPROOT/case_drift"
    build_drifted "$d"
    local out
    out=$(cd "$d" && echo "$NEUTRAL_PAYLOAD" | bash "$HOOK" 2>/dev/null)

    local msg
    msg=$(printf '%s' "$out" | python3 -c 'import json,sys; d=json.loads(sys.stdin.read()); print(d.get("systemMessage",""))' 2>/dev/null)

    case "$msg" in
        *"stale sidecars"*) ;;
        *)
            echo "    expected systemMessage to mention stale sidecars"
            echo "    got: $out"
            return 1
            ;;
    esac

    case "$msg" in
        *"Affected: 1"*) ;;
        *)
            echo "    expected 'Affected: 1' in systemMessage"
            echo "    got: $msg"
            return 1
            ;;
    esac

    if [ ! -f "$d/.edikt/state/stale-sidecars.log" ]; then
        echo "    expected $d/.edikt/state/stale-sidecars.log to exist"
        return 1
    fi
    if ! grep -q '^ADR-001$' "$d/.edikt/state/stale-sidecars.log"; then
        echo "    expected log to contain 'ADR-001' on its own line"
        echo "    got: $(cat "$d/.edikt/state/stale-sidecars.log")"
        return 1
    fi
    return 0
}

case_no_filename_in_message() {
    local d="$TMPROOT/case_no_fname"
    build_drifted "$d"
    local out
    out=$(cd "$d" && echo "$NEUTRAL_PAYLOAD" | bash "$HOOK" 2>/dev/null)

    local msg
    msg=$(printf '%s' "$out" | python3 -c 'import json,sys; d=json.loads(sys.stdin.read()); print(d.get("systemMessage",""))' 2>/dev/null)

    # INV-004: filename / artifact ID must not appear in the user-visible message.
    case "$msg" in
        *"ADR-001-test"*|*"ADR-001.md"*|*".edikt.yaml"*)
            echo "    INV-004 violation: systemMessage contains filename or sidecar path"
            echo "    msg: $msg"
            return 1
            ;;
    esac
    # The bare artifact ID "ADR-001" is also off-limits — it would be
    # attacker-influenceable if the prose were attacker-controlled.
    case "$msg" in
        *"ADR-001"*)
            echo "    INV-004 violation: systemMessage contains artifact ID"
            echo "    msg: $msg"
            return 1
            ;;
    esac
    return 0
}

case_clean_no_drift() {
    local d="$TMPROOT/case_clean"
    build_clean "$d"
    local out
    out=$(cd "$d" && echo "$NEUTRAL_PAYLOAD" | bash "$HOOK" 2>/dev/null)

    # No signals expected → {"continue": true}.
    if ! printf '%s' "$out" | python3 -c 'import json,sys; d=json.loads(sys.stdin.read()); sys.exit(0 if d.get("continue") is True else 1)' 2>/dev/null; then
        echo "    expected {\"continue\": true}"
        echo "    got: $out"
        return 1
    fi

    if [ -f "$d/.edikt/state/stale-sidecars.log" ]; then
        echo "    expected no stale-sidecars.log when no drift"
        echo "    got: $(cat "$d/.edikt/state/stale-sidecars.log")"
        return 1
    fi
    return 0
}

case_clears_stale_log_on_clean_run() {
    local d="$TMPROOT/case_clear"
    build_clean "$d"
    # Pre-seed a stale log from a prior session.
    printf 'ADR-999\n' > "$d/.edikt/state/stale-sidecars.log"

    cd "$d" && echo "$NEUTRAL_PAYLOAD" | bash "$HOOK" >/dev/null 2>&1

    if [ -f "$d/.edikt/state/stale-sidecars.log" ]; then
        echo "    expected stale-sidecars.log to be deleted on clean run"
        echo "    got: $(cat "$d/.edikt/state/stale-sidecars.log")"
        return 1
    fi
    return 0
}

case_bare_project() {
    local d="$TMPROOT/case_bare"
    build_bare "$d"
    local out
    out=$(cd "$d" && echo "$NEUTRAL_PAYLOAD" | bash "$HOOK" 2>/dev/null)

    if ! printf '%s' "$out" | python3 -c 'import json,sys; d=json.loads(sys.stdin.read()); sys.exit(0 if d.get("continue") is True else 1)' 2>/dev/null; then
        echo "    expected {\"continue\": true} for project with no governance dirs"
        echo "    got: $out"
        return 1
    fi
    if [ -f "$d/.edikt/state/stale-sidecars.log" ]; then
        echo "    bare project should not produce stale-sidecars.log"
        return 1
    fi
    return 0
}

run_case() {
    local name="$1"
    if "$2"; then
        echo "  PASS: $name"
    else
        echo "  FAIL: $name"
        FAIL=1
    fi
}

run_case "drift-detected"             case_drift_detected
run_case "no-filename-in-message"     case_no_filename_in_message
run_case "clean-no-drift"             case_clean_no_drift
run_case "clears-stale-log-on-clean"  case_clears_stale_log_on_clean_run
run_case "bare-project"               case_bare_project

exit "$FAIL"

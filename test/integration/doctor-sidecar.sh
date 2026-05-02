#!/usr/bin/env bash
# Phase 7 integration: `edikt doctor` Sidecar Health checks.
#
# Six fixture matrices (clean + the five failure modes):
#   1. clean              — exit 0, "All sidecar checks passed."
#   2. orphan             — exit 2, ORPHAN diagnostic
#   3. missing            — exit 2, MISSING diagnostic + adr:compile hint
#   4. path-mismatch      — exit 2, PATH MISMATCH diagnostic
#   5. schema-invalid     — exit 2, SCHEMA INVALID diagnostic
#   6. empty-directives   — exit 1 (warn-only), NEEDS REVIEW diagnostic
#
# Each matrix runs in its own tmp project root with a sandboxed EDIKT_ROOT
# containing the minimum doctor expects (current symlink + manifest). That
# way the existing top-level doctor checks pass and the per-matrix
# assertions only depend on the sidecar checks under test.

set -uo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"

EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .) || {
        echo "build failed"; exit 1
    }
fi

GREEN='\033[0;32m'
RED='\033[0;31m'
DIM='\033[0;90m'
RESET='\033[0m'

pass_count=0
fail_count=0

assert() {
    local label="$1"
    local cmd="$2"
    if eval "$cmd" >/dev/null 2>&1; then
        echo -e "  ${GREEN}✓${RESET} $label"
        pass_count=$((pass_count + 1))
    else
        echo -e "  ${RED}✗${RESET} $label"
        echo -e "    ${DIM}cmd: $cmd${RESET}"
        fail_count=$((fail_count + 1))
    fi
}

# ─── Fixture builders ───────────────────────────────────────────────────────

# new_project <work>: lays down docs tree + .edikt/config.yaml + a minimal
# EDIKT_ROOT skeleton that satisfies the top-level doctor checks (current
# symlink + commands dir). Returns paths via globals WORK/EDIKT_ROOT.
new_project() {
    WORK="$(mktemp -d -t doctor-sidecar-XXXXXX)"
    EDIKT_ROOT="$WORK/.edikt-state"
    mkdir -p "$WORK/docs/architecture/decisions"
    mkdir -p "$WORK/docs/architecture/invariants"
    mkdir -p "$WORK/docs/guidelines"
    mkdir -p "$WORK/.edikt/state"
    cat > "$WORK/.edikt/config.yaml" <<EOF
edikt_version: "0.6.0"
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
EOF

    # Minimal EDIKT_ROOT layout so the existing doctor checks (current
    # symlink, manifest) don't add their own errors.
    mkdir -p "$EDIKT_ROOT/versions/0.6.0/commands"
    mkdir -p "$EDIKT_ROOT/versions/0.6.0/templates"
    mkdir -p "$EDIKT_ROOT/state"
    : > "$EDIKT_ROOT/versions/0.6.0/manifest.yaml"
    ln -s "versions/0.6.0" "$EDIKT_ROOT/current"
    ln -s "current/templates" "$EDIKT_ROOT/templates"
    cat > "$EDIKT_ROOT/state/lock.json" <<EOF
{"active_tag":"0.6.0","activated_at":"2026-05-02T00:00:00Z","activated_by":"test"}
EOF
}

write_md() {
    local p="$1"
    cat > "$p" <<'EOF'
# Sample

## Decision

Hooks must emit JSON.
EOF
}

write_valid_sidecar() {
    local p="$1"
    local rel_md="$2"
    cat > "$p" <<EOF
schema_version: 1
topic: hooks
path: $rel_md
signals:
  - hook
directives:
  - text: "Hooks must emit JSON. (ref: INV-003)"
    source_excerpt:
      line_start: 1
      line_end: 1
      quote: "Hooks must emit JSON."
EOF
}

write_empty_dir_sidecar() {
    local p="$1"
    local rel_md="$2"
    cat > "$p" <<EOF
schema_version: 1
topic: prose
path: $rel_md
signals: []
directives: []
EOF
}

write_invalid_schema_sidecar() {
    local p="$1"
    local rel_md="$2"
    # Missing required `topic` — fails sidecar.Load() validation.
    cat > "$p" <<EOF
schema_version: 1
path: $rel_md
signals: []
directives: []
EOF
}

# run_doctor: runs `edikt doctor` against the current $WORK / $EDIKT_ROOT and
# captures combined output to $WORK/doctor.out. Returns the doctor exit code
# in $DOCTOR_EXIT.
run_doctor() {
    (
        cd "$WORK"
        EDIKT_ROOT="$EDIKT_ROOT" CLAUDE_HOME="$WORK/.claude-home" \
            "$EDIKT_BIN" doctor > "$WORK/doctor.out" 2>&1
    )
    DOCTOR_EXIT=$?
}

# Cleanup all WORK dirs at exit.
ALL_WORKS=()
trap 'for w in "${ALL_WORKS[@]:-}"; do rm -rf "$w"; done; rm -f "$EDIKT_BIN" 2>/dev/null || true' EXIT

echo "Phase 7 — doctor Sidecar Health (six fixture matrices)"

# ─── 1. clean ───────────────────────────────────────────────────────────────
new_project
ALL_WORKS+=("$WORK")
write_md "$WORK/docs/architecture/decisions/ADR-100-x.md"
write_valid_sidecar \
    "$WORK/docs/architecture/decisions/ADR-100-x.edikt.yaml" \
    "docs/architecture/decisions/ADR-100-x.md"
run_doctor
assert "clean: doctor exit 0" "[ '$DOCTOR_EXIT' = '0' ]"
assert "clean: 'All sidecar checks passed.' present" \
    "grep -qF 'All sidecar checks passed.' '$WORK/doctor.out'"

# ─── 2. orphan ──────────────────────────────────────────────────────────────
new_project
ALL_WORKS+=("$WORK")
# Sidecar with no sibling .md.
write_valid_sidecar \
    "$WORK/docs/architecture/decisions/ADR-200-orph.edikt.yaml" \
    "docs/architecture/decisions/ADR-200-orph.md"
run_doctor
assert "orphan: doctor exit 2 (errors)" "[ '$DOCTOR_EXIT' = '2' ]"
assert "orphan: ORPHAN diagnostic emitted" \
    "grep -q 'ORPHAN: docs/architecture/decisions/ADR-200-orph.edikt.yaml' '$WORK/doctor.out'"

# ─── 3. missing ─────────────────────────────────────────────────────────────
new_project
ALL_WORKS+=("$WORK")
write_md "$WORK/docs/architecture/decisions/ADR-300-mis.md"
run_doctor
assert "missing: doctor exit 2 (errors)" "[ '$DOCTOR_EXIT' = '2' ]"
assert "missing: MISSING diagnostic emitted" \
    "grep -q 'MISSING: docs/architecture/decisions/ADR-300-mis.md' '$WORK/doctor.out'"
assert "missing: adr:compile hint emitted" \
    "grep -qF '/edikt:adr:compile' '$WORK/doctor.out'"

# ─── 3b. missing skip-list ──────────────────────────────────────────────────
new_project
ALL_WORKS+=("$WORK")
# Skip-listed files (ADR-008/009/SPEC-) must NOT trigger MISSING.
write_md "$WORK/docs/architecture/decisions/ADR-008-skip.md"
write_md "$WORK/docs/architecture/decisions/ADR-009-skip.md"
write_md "$WORK/docs/architecture/decisions/SPEC-001-skip.md"
run_doctor
assert "skip-list: doctor exit 0 for ADR-008/009/SPEC-" "[ '$DOCTOR_EXIT' = '0' ]"
assert "skip-list: no MISSING diagnostic for skip-list" \
    "! grep -q 'MISSING:' '$WORK/doctor.out'"

# ─── 4. path mismatch ───────────────────────────────────────────────────────
new_project
ALL_WORKS+=("$WORK")
write_md "$WORK/docs/architecture/decisions/ADR-400-pm.md"
write_valid_sidecar \
    "$WORK/docs/architecture/decisions/ADR-400-pm.edikt.yaml" \
    "docs/architecture/decisions/SOMETHING-ELSE.md"
run_doctor
assert "path-mismatch: doctor exit 2 (errors)" "[ '$DOCTOR_EXIT' = '2' ]"
assert "path-mismatch: PATH MISMATCH diagnostic emitted" \
    "grep -q 'PATH MISMATCH' '$WORK/doctor.out'"

# ─── 5. schema invalid ──────────────────────────────────────────────────────
new_project
ALL_WORKS+=("$WORK")
write_md "$WORK/docs/architecture/decisions/ADR-500-si.md"
write_invalid_schema_sidecar \
    "$WORK/docs/architecture/decisions/ADR-500-si.edikt.yaml" \
    "docs/architecture/decisions/ADR-500-si.md"
run_doctor
assert "schema-invalid: doctor exit 2 (errors)" "[ '$DOCTOR_EXIT' = '2' ]"
assert "schema-invalid: SCHEMA INVALID diagnostic emitted" \
    "grep -q 'SCHEMA INVALID' '$WORK/doctor.out'"

# ─── 6. empty directives (soft warning) ─────────────────────────────────────
new_project
ALL_WORKS+=("$WORK")
write_md "$WORK/docs/architecture/decisions/ADR-600-em.md"
write_empty_dir_sidecar \
    "$WORK/docs/architecture/decisions/ADR-600-em.edikt.yaml" \
    "docs/architecture/decisions/ADR-600-em.md"
run_doctor
# Empty directives is a soft warning only: doctor exits 1 (warnings), not 2.
assert "empty-directives: doctor exit 1 (warnings, not errors)" "[ '$DOCTOR_EXIT' = '1' ]"
assert "empty-directives: NEEDS REVIEW soft warning emitted" \
    "grep -q 'NEEDS REVIEW' '$WORK/doctor.out'"
assert "empty-directives: no SCHEMA INVALID / ORPHAN / MISSING / PATH MISMATCH" \
    "! grep -qE '(SCHEMA INVALID|ORPHAN:|MISSING:|PATH MISMATCH)' '$WORK/doctor.out'"

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"

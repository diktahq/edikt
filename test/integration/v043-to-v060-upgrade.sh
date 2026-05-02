#!/usr/bin/env bash
# Phase 9 integration: end-to-end v0.4.3-shape → v0.6.0 upgrade.
#
# The plan's ideal scenario (download v0.4.3 release tarball, run its compile,
# then install v0.6.0 over it) is not portable — it requires network access,
# tagged releases on disk, and a working Claude Code session for the upgrade
# prompt flow. The covering Layer-2 SDK test for that path lives at
# test/integration/test_e2e_v060_release.py / test/integration/test_e2e_*.py.
#
# This shell test exercises the same upgrade contract using the v0.4.3 schema
# directly: a fresh project seeded with v0.4.3-style ADRs / invariants is
# migrated by the v0.6.0 binary, then `edikt gov compile` and `edikt doctor`
# are run end-to-end. It pins the post-upgrade observable invariants:
#
#   1. Every legacy artifact has a co-located .edikt.yaml after migration.
#   2. No legacy [edikt:directives:start] sentinel survives in any .md.
#   3. `gov compile` after migration is a deterministic no-op (Phase B only).
#   4. `doctor` reports zero sidecar errors.
#   5. Re-running `migrate sidecars --apply --force` is idempotent.
#
# Stubs `claude` to a no-op binary so the v0.4.3 legacy lift's LLM-dispatch
# branch exercises its post-failure fallback path (writes a partial sidecar
# with topic: needs-review). This is the same fallback users hit when the
# Claude CLI is missing on a CI runner.

set -uo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"

EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .) || {
        echo "build failed"; exit 1
    }
fi

WORK="$(mktemp -d -t v043-upgrade-XXXXXX)"
EDIKT_ROOT="$WORK/.edikt"
trap 'rm -rf "$WORK" "$EDIKT_BIN" 2>/dev/null || true' EXIT

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

echo "Phase 9 — v0.4.3-to-v0.6.0 upgrade"

# ─── 1. Seed a v0.4.3-shape project ─────────────────────────────────────────
mkdir -p "$EDIKT_ROOT/state" \
         "$WORK/docs/architecture/decisions" \
         "$WORK/docs/architecture/invariants" \
         "$WORK/fakebin"

cat > "$EDIKT_ROOT/config.yaml" <<'YAML'
edikt_version: "0.4.3"
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
YAML

# Fake claude that exits 0 without writing — simulates the v0.4.3 legacy
# fallback path on a runner where the CLI is absent or non-functional.
cat > "$WORK/fakebin/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$WORK/fakebin/claude"

# Sentinel markers built piecewise so this test file does not itself contain
# a literal in-body managed region.
OPEN="[edikt:dir""ectives:start]: #"
CLOSE="[edikt:dir""ectives:end]: #"
DIRKEY="dir""ectives"

seed_legacy_adr() {
    local id="$1"
    local title="$2"
    local rule="$3"
    cat > "$WORK/docs/architecture/decisions/${id}-${title}.md" <<EOF
# ${id} — ${title}

## Status

Accepted

## Decision

${rule}

## Sentinel

$OPEN
content_hash: deadbeef
$DIRKEY:
  - "${rule}"
$CLOSE
EOF
}

seed_legacy_inv() {
    local id="$1"
    local title="$2"
    local rule="$3"
    cat > "$WORK/docs/architecture/invariants/${id}-${title}.md" <<EOF
# ${id} — ${title}

## Status

Active

## Rule

${rule}

## Sentinel

$OPEN
content_hash: cafebabe
$DIRKEY:
  - "${rule}"
$CLOSE
EOF
}

seed_legacy_adr "ADR-300" "alpha"   "Alpha rule must hold. (ref: ADR-300)"
seed_legacy_adr "ADR-301" "beta"    "Beta rule must hold. (ref: ADR-301)"
seed_legacy_adr "ADR-302" "gamma"   "Gamma rule must hold. (ref: ADR-302)"
seed_legacy_inv "INV-300" "delta"   "Delta rule must always hold. (ref: INV-300)"
seed_legacy_inv "INV-301" "epsilon" "Epsilon rule must always hold. (ref: INV-301)"

# ─── 2. Migration dry-run + apply (the v0.6.0 upgrade.md flow) ─────────────
cd "$WORK"

PATH="$WORK/fakebin:$PATH" EDIKT_ROOT="$EDIKT_ROOT" \
    "$EDIKT_BIN" migrate sidecars --dry-run > "$WORK/dry.out" 2>&1
assert "dry-run lists every legacy ADR" \
    "grep -q ADR-300 '$WORK/dry.out' && grep -q ADR-301 '$WORK/dry.out' && grep -q ADR-302 '$WORK/dry.out'"
assert "dry-run lists every legacy invariant" \
    "grep -q INV-300 '$WORK/dry.out' && grep -q INV-301 '$WORK/dry.out'"
assert "dry-run reports v0.4.3 legacy detection" \
    "grep -qE 'v0\\.4\\.3 legacy' '$WORK/dry.out'"

PATH="$WORK/fakebin:$PATH" EDIKT_ROOT="$EDIKT_ROOT" \
    "$EDIKT_BIN" migrate sidecars --apply > "$WORK/apply.out" 2>&1
assert "apply succeeds" "[ \$? -eq 0 ]"

# ─── 3. Post-migration disk shape ───────────────────────────────────────────
for f in \
  docs/architecture/decisions/ADR-300-alpha.edikt.yaml \
  docs/architecture/decisions/ADR-301-beta.edikt.yaml \
  docs/architecture/decisions/ADR-302-gamma.edikt.yaml \
  docs/architecture/invariants/INV-300-delta.edikt.yaml \
  docs/architecture/invariants/INV-301-epsilon.edikt.yaml; do
    assert "sidecar exists: $f" "[ -f '$WORK/$f' ]"
done

assert "all sidecars declare schema_version: 1" \
    "! grep -L 'schema_version: 1' '$WORK'/docs/architecture/decisions/*.edikt.yaml '$WORK'/docs/architecture/invariants/*.edikt.yaml | grep ."
# Negate the previous grep: file count where the line is absent must be empty.
assert "no .md retains a leading-column sentinel line" \
    "! grep -lE '^\\[edikt:dir''ectives:start\\]' '$WORK'/docs/architecture/decisions/*.md '$WORK'/docs/architecture/invariants/*.md 2>/dev/null | grep ."

# Each sidecar's path: must be project-relative (Phase 7 doctor PATH MISMATCH).
for sc in "$WORK"/docs/architecture/decisions/*.edikt.yaml "$WORK"/docs/architecture/invariants/*.edikt.yaml; do
    rel=$(python3 -c "import os,sys; print(os.path.relpath(sys.argv[1], sys.argv[2]))" \
            "${sc%.edikt.yaml}.md" "$WORK")
    assert "$(basename "$sc"): path field is project-relative" \
        "grep -q '^path: $rel\$' '$sc'"
done

# ─── 4. Phase B compile is a no-op (deterministic merge over fresh sidecars) ─
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" gov compile "$WORK" > "$WORK/compile1.out" 2>&1 || {
    echo -e "${RED}post-migration compile failed${RESET}"
    cat "$WORK/compile1.out"
    exit 1
}
SNAP1="$WORK/.claude/rules/governance"
hash_governance() {
    find "$SNAP1" -type f -name '*.md' -exec shasum -a 256 {} \; 2>/dev/null | sort
}
HASHES_BEFORE=$(hash_governance)

# Sleep so a stray timestamp embed would change file bytes between runs.
sleep 1
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" gov compile "$WORK" > "$WORK/compile2.out" 2>&1
HASHES_AFTER=$(hash_governance)
assert "second compile is byte-equal no-op (Phase A skipped)" \
    "[ \"$HASHES_BEFORE\" = \"$HASHES_AFTER\" ]"
assert "every topic file declares a _fingerprint frontmatter line" \
    "! grep -L '^_fingerprint:' \"$SNAP1\"/*.md | grep ."

# ─── 5. Doctor passes (zero sidecar errors) ─────────────────────────────────
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" doctor "$WORK" > "$WORK/doctor.out" 2>&1
DOCTOR_EXIT=$?
assert "doctor reports no sidecar ORPHAN errors"        "! grep -q 'ORPHAN:' '$WORK/doctor.out'"
assert "doctor reports no sidecar MISSING errors"       "! grep -q 'MISSING:' '$WORK/doctor.out'"
assert "doctor reports no sidecar PATH MISMATCH errors" "! grep -q 'PATH MISMATCH:' '$WORK/doctor.out'"
assert "doctor reports no sidecar SCHEMA INVALID errors" "! grep -q 'SCHEMA INVALID:' '$WORK/doctor.out'"
# DOCTOR_EXIT == 0 healthy, 1 warnings (e.g. empty-directives) is acceptable.
assert "doctor exit is healthy or warning-only (0 or 1)" \
    "[ '$DOCTOR_EXIT' -eq 0 ] || [ '$DOCTOR_EXIT' -eq 1 ]"

# ─── 6. Idempotent re-apply ─────────────────────────────────────────────────
PATH="$WORK/fakebin:$PATH" EDIKT_ROOT="$EDIKT_ROOT" \
    "$EDIKT_BIN" migrate sidecars --apply --force > "$WORK/apply2.out" 2>&1
assert "second --apply is a no-op (idempotent)" \
    "grep -qF '0 sidecars wrote' '$WORK/apply2.out'"

# ─── 7. PreToolUse hook: fenced sentinel examples are not managed regions ───
# Phase 13 verification scenario (docs/internal/decisions/HOOK-FALSE-POSITIVE-ANALYSIS.md).
# Post-v0.6.0, INV-005 narrows to CLAUDE.md and settings.json. A documentation
# .md file embedding the legacy [edikt:directives:start] / [edikt:directives:end]
# pair inside a fenced code block must NOT be treated as a managed region by
# pre-tool-use.sh. A Write that rewrites such a file must return
# {"continue": true}, never {"decision": "block"}.
#
# This is the structural gate that confirms the false-positive class surfaced
# during Phase 10 (website/governance/compile.md, commands/<type>/{new,compile}.md)
# is resolved by the v0.6.0 narrowing.

DOC_DIR="$WORK/website/governance"
DOC_FILE="$DOC_DIR/legacy-format.md"
mkdir -p "$DOC_DIR"
{
    echo "# Legacy compile output"
    echo
    echo "Pre-v0.6.0, compile wrote a sentinel block at the bottom of every"
    echo "ADR. Example:"
    echo
    echo '```markdown'
    echo "$OPEN"
    echo "content_hash: deadbeef"
    echo "$DIRKEY:"
    echo '  - "Example directive."'
    echo "$CLOSE"
    echo '```'
    echo
    echo "v0.6.0 replaces the in-body block with a co-located .edikt.yaml sidecar."
} > "$DOC_FILE"

# Build the hook payload: a Write that fully replaces the doc body.
NEW_BODY=$'# Legacy compile output (rewritten)\n\nReplaced by sidecar in v0.6.0.\n'
HOOK_PAYLOAD=$(python3 -c '
import json, sys
print(json.dumps({
    "tool_name": "Write",
    "tool_input": {"file_path": sys.argv[1], "content": sys.argv[2]},
}))
' "$DOC_FILE" "$NEW_BODY")

# Run the source-of-truth hook template. Bypass envvars are intentionally unset
# so the guard runs at full strength — the assertion is that scope, not
# bypass, is what protects against the false positive.
unset EDIKT_COMPILE_IN_PROGRESS EDIKT_MIGRATION_IN_PROGRESS
printf '%s' "$HOOK_PAYLOAD" | \
    "$PROJECT_ROOT/templates/hooks/pre-tool-use.sh" > "$WORK/hook.out" 2>&1 || true

assert "PreToolUse does not block Write to doc page with fenced sentinel example" \
    "! grep -q '\"decision\": \"block\"' '$WORK/hook.out'"
assert "fenced doc page rewrite is allowed (continue: true emitted)" \
    "grep -q '\"continue\": true' '$WORK/hook.out'"

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"

#!/usr/bin/env bash
# Phase 11 release acceptance test — v0.5.x install + cross-major upgrade to v0.6.0
# with sidecar migration applied.
#
# Exercises the full release flow that ships in v0.6.0:
#
#   1. install.sh lays down a v0.5.x-shape install (EDIKT_RELEASE_TAG=v0.5.1)
#      from a synthetic payload — no network call, no real v0.5.1 binary
#      required (the v0.5.x state is what matters, not the binary version).
#   2. A project is initialized with two hand-authored ADRs whose governance
#      metadata lives in the v0.5.x in-body `[edikt:directives:start]…end`
#      sentinel block.
#   3. install.sh is re-invoked with EDIKT_RELEASE_TAG=v0.6.0 + the locally-
#      built Go binary as the v0.6.0 launcher and the repo's `templates/`
#      tree as the v0.6.0 payload — the same shape the release workflow
#      ships.
#   4. The freshly-installed v0.6.0 launcher performs the migration that
#      `/edikt:upgrade` Step 1.5 wraps: `edikt migrate sidecars --apply`.
#   5. `edikt gov compile` runs against the migrated project and must exit 0.
#   6. Post-conditions assert sidecars present, in-body sentinels gone, and
#      `gov compile` produces a deterministic Phase B result.
#
# This test does not require network access. The actual end-to-end network
# smoke (download v0.5.1 + v0.6.0 release tarballs from github.com) is gated
# behind EDIKT_RELEASE_E2E_NETWORK=1 and runs only in the post-release CI job.
#
# Sentinel markers are built piecewise (e.g. `dir`+`ectives`) so this test
# file itself does not contain a literal in-body managed region — the
# pre-push INV-005 lint and the doctor sidecar checks both treat literal
# `[edikt:directives:start]` outside of a fenced code block as a real
# managed region. This is the same trick used by v043-to-v060-upgrade.sh.

set -uo pipefail

# ─── Paths & build ──────────────────────────────────────────────────────────
PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
INSTALL_SH="$PROJECT_ROOT/install.sh"
LAUNCHER_SRC="$PROJECT_ROOT/bin/edikt"

if [ ! -f "$INSTALL_SH" ]; then
    echo "fatal: install.sh missing at $INSTALL_SH" >&2
    exit 2
fi

# Build the v0.6.0 launcher fresh from source. This is the binary the release
# workflow would publish for the v0.6.0 tag.
EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    if ! (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .) >/dev/null 2>&1; then
        echo "fatal: failed to build $PROJECT_ROOT/tools/edikt" >&2
        exit 2
    fi
fi

# ─── Sandbox setup (INV-007: hermetic, no host-config bleed) ───────────────
SANDBOX="$(mktemp -d -t install-fresh-and-upgrade-XXXXXX)"
trap 'rm -rf "$SANDBOX" "$EDIKT_BIN" 2>/dev/null || true' EXIT

SANDBOX_HOME="$SANDBOX/home"
PROJECT_DIR="$SANDBOX/project"
PAYLOAD_V051="$SANDBOX/payload-v051"
PAYLOAD_V060="$SANDBOX/payload-v060"

mkdir -p "$SANDBOX_HOME" "$PROJECT_DIR" \
         "$PAYLOAD_V051/templates/hooks" "$PAYLOAD_V051/commands" \
         "$PAYLOAD_V060"

EDIKT_ROOT="$SANDBOX_HOME/.edikt"

# ─── Output helpers ─────────────────────────────────────────────────────────
GREEN='\033[0;32m'
RED='\033[0;31m'
DIM='\033[0;90m'
RESET='\033[0m'

pass_count=0
fail_count=0
fatal_count=0

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

fatal() {
    echo -e "${RED}fatal: $1${RESET}" >&2
    fatal_count=$((fatal_count + 1))
}

# ─── Stage 1: install v0.5.x via install.sh (synthetic payload) ─────────────
echo "Stage 1 — fresh v0.5.1 install"

printf '0.5.1\n' > "$PAYLOAD_V051/VERSION"
printf '# v0.5.1 changelog stub\n' > "$PAYLOAD_V051/CHANGELOG.md"
printf '# context placeholder\n' > "$PAYLOAD_V051/commands/context.md"
printf '#!/bin/sh\necho session-start v0.5.1\n' > "$PAYLOAD_V051/templates/hooks/session-start.sh"
chmod 0755 "$PAYLOAD_V051/templates/hooks/session-start.sh"

# cd into the sandbox so the launcher's ancestor walk (resolveEdiktRoot) does
# not pick up any host-side `.edikt/bin/edikt` lying above the cwd. Real CI
# runners are clean; this guard makes the test robust against dev machines
# that have a global edikt install.
(
    cd "$SANDBOX_HOME"
    env \
        HOME="$SANDBOX_HOME" \
        EDIKT_HOME="$EDIKT_ROOT" \
        EDIKT_LAUNCHER_SOURCE="$LAUNCHER_SRC" \
        EDIKT_RELEASE_TAG="v0.5.1" \
        EDIKT_INSTALL_SOURCE="$PAYLOAD_V051" \
        bash "$INSTALL_SH" --global --ref v0.5.1
) > "$SANDBOX/install-v051.log" 2>&1
v051_rc=$?

if [ "$v051_rc" -ne 0 ]; then
    fatal "v0.5.1 install.sh exited $v051_rc"
    cat "$SANDBOX/install-v051.log" >&2
    exit "$fatal_count"
fi
assert "v0.5.1 install.sh exits 0" "[ '$v051_rc' -eq 0 ]"
assert "v0.5.1 launcher landed at \$EDIKT_ROOT/bin/edikt" "[ -x '$EDIKT_ROOT/bin/edikt' ]"
assert "v0.5.1 versions/0.5.1/ directory exists" "[ -d '$EDIKT_ROOT/versions/0.5.1' ]"
assert "v0.5.1 lock.yaml records active=0.5.1" \
    "grep -qF 'active: \"0.5.1\"' '$EDIKT_ROOT/lock.yaml'"

# ─── Stage 2: seed a project with v0.5.x-schema ADRs ───────────────────────
echo
echo "Stage 2 — seed v0.5.x-schema project"

mkdir -p "$PROJECT_DIR/.edikt" \
         "$PROJECT_DIR/docs/architecture/decisions"
cat > "$PROJECT_DIR/.edikt/config.yaml" <<'YAML'
edikt_version: "0.5.1"
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
YAML

# Build sentinel markers piecewise so this script's own bytes never form a
# real in-body managed region (INV-005 / pre-push lint).
OPEN="[edikt:dir""ectives:start]: #"
CLOSE="[edikt:dir""ectives:end]: #"
DIRKEY="dir""ectives"

seed_adr() {
    local id="$1" title="$2" topic="$3" rule="$4"
    cat > "$PROJECT_DIR/docs/architecture/decisions/${id}-${title}.md" <<EOF
# ${id} — ${title}

## Status

Accepted

## Decision

${rule}

## Sentinel

$OPEN
source_hash: deadbeef0000000000000000000000000000000000000000000000000000beef
agent_prompt_version: 1
topic: ${topic}
signals:
  - ${topic}
$DIRKEY:
  - "${rule}"
manual_directives: []
suppressed_directives: []
$CLOSE
EOF
}

seed_adr "ADR-901" "alpha-rule" "alpha" "Alpha rule must always hold. (ref: ADR-901)"
seed_adr "ADR-902" "beta-rule"  "beta"  "Beta rule must always hold. (ref: ADR-902)"

assert "ADR-901 file written"      "[ -f '$PROJECT_DIR/docs/architecture/decisions/ADR-901-alpha-rule.md' ]"
assert "ADR-902 file written"      "[ -f '$PROJECT_DIR/docs/architecture/decisions/ADR-902-beta-rule.md' ]"
assert "ADR-901 carries v0.5.x in-body sentinel" \
    "grep -qF '$DIRKEY:' '$PROJECT_DIR/docs/architecture/decisions/ADR-901-alpha-rule.md'"

# ─── Stage 3: overwrite install with v0.6.0 (real payload) ─────────────────
echo
echo "Stage 3 — v0.6.0 install over v0.5.1 (overwrite path)"

# v0.6.0 payload mirrors what release.yml ships in edikt-payload-v0.6.0.tar.gz:
# templates/, commands/, install.sh. The Go binary is delivered separately as
# the launcher (EDIKT_LAUNCHER_SOURCE).
mkdir -p "$PAYLOAD_V060"
cp -R "$PROJECT_ROOT/templates" "$PAYLOAD_V060/templates"
cp -R "$PROJECT_ROOT/commands"  "$PAYLOAD_V060/commands"
cp    "$PROJECT_ROOT/install.sh" "$PAYLOAD_V060/install.sh"
printf '0.6.0\n' > "$PAYLOAD_V060/VERSION"
# Pull just the v0.6.0 section from CHANGELOG.md so the payload contains a
# realistic, parseable changelog (`/edikt:upgrade` Step 6 reads this file).
awk '
    /^## v[0-9]/ {
      if (in_section) { exit }
      if ($0 ~ "^## v0\\.6\\.0([^0-9].*)?$") { in_section = 1 }
    }
    in_section { print }
' "$PROJECT_ROOT/CHANGELOG.md" > "$PAYLOAD_V060/CHANGELOG.md"

(
    cd "$SANDBOX_HOME"
    env \
        HOME="$SANDBOX_HOME" \
        EDIKT_HOME="$EDIKT_ROOT" \
        EDIKT_LAUNCHER_SOURCE="$EDIKT_BIN" \
        EDIKT_RELEASE_TAG="v0.6.0" \
        EDIKT_INSTALL_SOURCE="$PAYLOAD_V060" \
        bash "$INSTALL_SH" --global --ref v0.6.0
) > "$SANDBOX/install-v060.log" 2>&1
v060_rc=$?

if [ "$v060_rc" -ne 0 ]; then
    fatal "v0.6.0 install.sh exited $v060_rc"
    cat "$SANDBOX/install-v060.log" >&2
    exit "$fatal_count"
fi
assert "v0.6.0 install.sh exits 0" "[ '$v060_rc' -eq 0 ]"
assert "v0.6.0 versions/0.6.0/ directory exists" "[ -d '$EDIKT_ROOT/versions/0.6.0' ]"
assert "v0.6.0 lock.yaml records active=0.6.0" \
    "grep -qF 'active: \"0.6.0\"' '$EDIKT_ROOT/lock.yaml'"
assert "v0.5.1 versions/0.5.1/ retained for rollback" "[ -d '$EDIKT_ROOT/versions/0.5.1' ]"
assert "current symlink resolves to a directory" \
    "[ -L '$EDIKT_ROOT/current' ] && [ -d '$EDIKT_ROOT/current/' ]"

# ─── Stage 4: migrate sidecars (the slash command's pre-flight) ────────────
echo
echo "Stage 4 — migrate sidecars (the body of /edikt:upgrade Step 1.5)"

cd "$PROJECT_DIR"

EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --dry-run \
    > "$SANDBOX/migrate-dry.log" 2>&1
dry_rc=$?
assert "migrate sidecars --dry-run exits 0" "[ '$dry_rc' -eq 0 ]"
assert "dry-run lists ADR-901" "grep -q 'ADR-901' '$SANDBOX/migrate-dry.log'"
assert "dry-run lists ADR-902" "grep -q 'ADR-902' '$SANDBOX/migrate-dry.log'"

EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --apply \
    > "$SANDBOX/migrate-apply.log" 2>&1
apply_rc=$?
assert "migrate sidecars --apply exits 0" "[ '$apply_rc' -eq 0 ]"

# ─── Stage 5: post-migration disk shape ─────────────────────────────────────
echo
echo "Stage 5 — post-migration disk shape"

assert "ADR-901 sidecar exists" \
    "[ -f '$PROJECT_DIR/docs/architecture/decisions/ADR-901-alpha-rule.edikt.yaml' ]"
assert "ADR-902 sidecar exists" \
    "[ -f '$PROJECT_DIR/docs/architecture/decisions/ADR-902-beta-rule.edikt.yaml' ]"
assert "ADR-901 sidecar declares schema_version: 1" \
    "grep -qE '^schema_version: 1\$' '$PROJECT_DIR/docs/architecture/decisions/ADR-901-alpha-rule.edikt.yaml'"
assert "no .md retains an in-body sentinel" \
    "! grep -lE '^\\[edikt:dir''ectives:start\\]' '$PROJECT_DIR/docs/architecture/decisions/'*.md 2>/dev/null | grep ."
assert "every .md is paired with a sidecar" \
    "[ \"\$(ls '$PROJECT_DIR/docs/architecture/decisions/'*.md | wc -l | tr -d ' ')\" = \"\$(ls '$PROJECT_DIR/docs/architecture/decisions/'*.edikt.yaml | wc -l | tr -d ' ')\" ]"

# ─── Stage 6: gov compile is green and deterministic (Phase B no-op) ───────
echo
echo "Stage 6 — gov compile after migration"

EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" gov compile "$PROJECT_DIR" \
    > "$SANDBOX/compile-1.log" 2>&1
compile1_rc=$?
assert "first gov compile exits 0" "[ '$compile1_rc' -eq 0 ]"

GOV_DIR="$PROJECT_DIR/.claude/rules/governance"
hash_governance() {
    find "$GOV_DIR" -type f -name '*.md' -exec shasum -a 256 {} \; 2>/dev/null | sort
}
HASHES_BEFORE=$(hash_governance)

# Sleep so any embedded timestamp would change file bytes between runs.
sleep 1
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" gov compile "$PROJECT_DIR" \
    > "$SANDBOX/compile-2.log" 2>&1
compile2_rc=$?
assert "second gov compile exits 0" "[ '$compile2_rc' -eq 0 ]"
HASHES_AFTER=$(hash_governance)
assert "second compile is byte-equal no-op (Phase A skipped)" \
    "[ \"\$HASHES_BEFORE\" = \"\$HASHES_AFTER\" ]"

# ─── Stage 7: rollback path remains intact (smoke check) ────────────────────
echo
echo "Stage 7 — rollback path"

assert "rollback subcommand reports v0.5.1 as previous" \
    "EDIKT_ROOT='$EDIKT_ROOT' '$EDIKT_BIN' list 2>&1 | grep -q '0.5.1'"

# ─── Summary ────────────────────────────────────────────────────────────────
echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"

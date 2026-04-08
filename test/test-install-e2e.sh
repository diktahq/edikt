#!/bin/bash
# Integration test: install.sh against a fake $HOME
#
# Exercises the real install.sh end-to-end by:
#   1. Shimming `curl` with a mock that serves files from the local repo
#      instead of hitting github.com.
#   2. Running install.sh with a fake $HOME and $CLAUDE_COMMANDS in /tmp.
#   3. Asserting the v0.1.x → v0.2.x upgrade path behaves correctly:
#        - old flat commands are removed
#        - new namespaced commands land in the right places
#        - user-customized old files are preserved
#        - a network failure aborts the install (no partial state)
#
# This is the test we wished existed before v0.2.0 shipped.
set -uo pipefail

PROJECT_ROOT="${1:-.}"
PROJECT_ROOT=$(cd "$PROJECT_ROOT" && pwd)
source "$(dirname "$0")/helpers.sh"

echo ""

# ============================================================
# Sandbox setup
# ============================================================

SANDBOX=$(mktemp -d -t edikt-install-e2e.XXXXXX)
trap 'rm -rf "$SANDBOX"' EXIT

FAKE_HOME="$SANDBOX/home"
FAKE_CLAUDE="$FAKE_HOME/.claude/commands"
FAKE_EDIKT="$FAKE_HOME/.edikt"
BIN_DIR="$SANDBOX/bin"
mkdir -p "$FAKE_HOME" "$BIN_DIR"

# ------------------------------------------------------------
# Mock curl — serves files from $PROJECT_ROOT instead of github
# ------------------------------------------------------------
# install.sh calls: curl -fsSL --retry 2 --max-time 30 URL -o DEST
# We parse URL (ends with the repo path) and copy from $PROJECT_ROOT.
# If a file listed in $MOCK_MISSING is requested, return exit 22 (curl's
# HTTP 4xx error code) to simulate a network/404 failure.
cat > "$BIN_DIR/curl" <<SHIM_EOF
#!/bin/bash
# Mock curl for install.sh e2e test
PROJECT_ROOT="$PROJECT_ROOT"
MOCK_MISSING_FILE="$SANDBOX/mock-missing"

URL=""
DEST=""
while [ \$# -gt 0 ]; do
  case "\$1" in
    -o) DEST="\$2"; shift 2 ;;
    -fsSL|--retry|--max-time|-s|-S|-L|-f) shift ;;
    2|30) shift ;;  # numeric args to --retry / --max-time
    http*|https*) URL="\$1"; shift ;;
    *) shift ;;
  esac
done

# Extract repo-relative path from URL
# e.g. https://raw.githubusercontent.com/diktahq/edikt/main/install.sh → install.sh
REL_PATH=\$(echo "\$URL" | sed -E 's|https://raw.githubusercontent.com/[^/]+/[^/]+/[^/]+/||')
SRC="\$PROJECT_ROOT/\$REL_PATH"

# Simulate failure if this path is marked missing
if [ -f "\$MOCK_MISSING_FILE" ] && grep -qxF "\$REL_PATH" "\$MOCK_MISSING_FILE"; then
  echo "curl: (22) mock: \$REL_PATH" >&2
  exit 22
fi

if [ ! -f "\$SRC" ]; then
  echo "curl: (22) mock: source not found: \$SRC" >&2
  exit 22
fi

mkdir -p "\$(dirname "\$DEST")"
cp "\$SRC" "\$DEST"
SHIM_EOF
chmod +x "$BIN_DIR/curl"

# Sanity check the mock
if ! PATH="$BIN_DIR:$PATH" curl -fsSL "https://raw.githubusercontent.com/diktahq/edikt/main/VERSION" -o "$SANDBOX/_check" 2>/dev/null; then
    fail "Mock curl sanity check" "mock curl failed to resolve VERSION"
    exit 1
fi
pass "Mock curl resolves repo files"

# ============================================================
# Scenario 1: Fresh install (no pre-existing $HOME/.edikt)
# ============================================================

run_install() {
    # Runs install.sh from $PROJECT_ROOT against fake HOME, with mock curl.
    # Returns the install exit code.
    HOME="$FAKE_HOME" PATH="$BIN_DIR:$PATH" bash "$PROJECT_ROOT/install.sh" --global > "$SANDBOX/install.out" 2>&1
}

rm -rf "$FAKE_HOME"
mkdir -p "$FAKE_HOME"

if run_install; then
    pass "Scenario 1: fresh install exits 0"
else
    fail "Scenario 1: fresh install exits 0" "$(tail -20 "$SANDBOX/install.out")"
fi

# Core files landed
assert_file_exists "$FAKE_EDIKT/VERSION" "Scenario 1: VERSION installed"
assert_file_exists "$FAKE_CLAUDE/edikt/init.md" "Scenario 1: init command installed"
assert_file_exists "$FAKE_CLAUDE/edikt/adr/new.md" "Scenario 1: adr/new installed in namespace"
assert_file_exists "$FAKE_CLAUDE/edikt/sdlc/plan.md" "Scenario 1: sdlc/plan installed in namespace"
assert_file_exists "$FAKE_CLAUDE/edikt/gov/compile.md" "Scenario 1: gov/compile installed in namespace"
assert_file_exists "$FAKE_CLAUDE/edikt/deprecated/adr.md" "Scenario 1: deprecated stubs installed"

# Fresh install should NOT have old flat files (nothing to clean up, but also nothing rogue)
if [ ! -f "$FAKE_CLAUDE/edikt/adr.md" ]; then
    pass "Scenario 1: no stray flat adr.md at top level"
else
    fail "Scenario 1: no stray flat adr.md at top level" "Found: $FAKE_CLAUDE/edikt/adr.md"
fi

# ============================================================
# Scenario 2: Upgrade from v0.1.x — old flat files must be removed
# ============================================================

rm -rf "$FAKE_HOME"
mkdir -p "$FAKE_CLAUDE/edikt"
mkdir -p "$FAKE_EDIKT"

# Simulate a v0.1.4 install: flat command files at the top level
V01_FLAT_FILES=(adr invariant compile review-governance rules-update sync prd spec spec-artifacts plan review drift audit docs intake)
for cmd in "${V01_FLAT_FILES[@]}"; do
    echo "# v0.1.4 $cmd placeholder" > "$FAKE_CLAUDE/edikt/${cmd}.md"
done
echo "0.1.4" > "$FAKE_EDIKT/VERSION"

if run_install; then
    pass "Scenario 2: upgrade install exits 0"
else
    fail "Scenario 2: upgrade install exits 0" "$(tail -20 "$SANDBOX/install.out")"
fi

# Old flat files must be gone
REMAINING_FLAT=()
for cmd in "${V01_FLAT_FILES[@]}"; do
    [ -f "$FAKE_CLAUDE/edikt/${cmd}.md" ] && REMAINING_FLAT+=("${cmd}.md")
done
if [ ${#REMAINING_FLAT[@]} -eq 0 ]; then
    pass "Scenario 2: all 15 v0.1.x flat commands removed"
else
    fail "Scenario 2: all 15 v0.1.x flat commands removed" "Still present: ${REMAINING_FLAT[*]}"
fi

# New namespaced files must be in place
assert_file_exists "$FAKE_CLAUDE/edikt/adr/new.md" "Scenario 2: adr/new installed during upgrade"
assert_file_exists "$FAKE_CLAUDE/edikt/sdlc/plan.md" "Scenario 2: sdlc/plan installed during upgrade"

# Backup directory must exist and contain the old files
BACKUP_DIRS=("$FAKE_EDIKT"/backups/*/)
if [ -d "${BACKUP_DIRS[0]}" ]; then
    pass "Scenario 2: backup directory created"
    BACKUP_DIR="${BACKUP_DIRS[0]}"
    # Check that at least one removed file got backed up
    if find "$BACKUP_DIR" -name "adr.md" | grep -q .; then
        pass "Scenario 2: old adr.md backed up before removal"
    else
        fail "Scenario 2: old adr.md backed up before removal" \
            "Backup dir: $BACKUP_DIR"
    fi
else
    fail "Scenario 2: backup directory created" \
        "No backup directory at $FAKE_EDIKT/backups/"
fi

# ============================================================
# Scenario 3: User-customized old files are preserved
# ============================================================

rm -rf "$FAKE_HOME"
mkdir -p "$FAKE_CLAUDE/edikt"
mkdir -p "$FAKE_EDIKT"
echo "0.1.4" > "$FAKE_EDIKT/VERSION"

# Create a customized old flat file with the custom marker
cat > "$FAKE_CLAUDE/edikt/plan.md" <<EOF
<!-- edikt:custom -->
# My custom plan command
EOF

# And one non-custom old file that should still be cleaned up
echo "# old adr" > "$FAKE_CLAUDE/edikt/adr.md"

if run_install; then
    pass "Scenario 3: upgrade with customized file exits 0"
else
    fail "Scenario 3: upgrade with customized file exits 0" "$(tail -20 "$SANDBOX/install.out")"
fi

# Custom plan.md must still be there
if [ -f "$FAKE_CLAUDE/edikt/plan.md" ] && grep -q "My custom plan command" "$FAKE_CLAUDE/edikt/plan.md"; then
    pass "Scenario 3: customized plan.md preserved"
else
    fail "Scenario 3: customized plan.md preserved" \
        "File missing or content clobbered"
fi

# Non-custom adr.md must be gone
if [ ! -f "$FAKE_CLAUDE/edikt/adr.md" ]; then
    pass "Scenario 3: non-custom adr.md removed"
else
    fail "Scenario 3: non-custom adr.md removed" \
        "File still exists"
fi

# ============================================================
# Scenario 4: Network failure aborts install
# ============================================================
# If curl fails for any file, _fetch must call error() and exit non-zero,
# NOT silently continue with a stale or missing file.

rm -rf "$FAKE_HOME"
mkdir -p "$FAKE_HOME"

# Mark one file as "missing" — mock curl will return 22 for it
echo "commands/adr/new.md" > "$SANDBOX/mock-missing"

if run_install; then
    fail "Scenario 4: install aborts on network failure" \
        "Install returned 0 despite curl failure for commands/adr/new.md"
else
    pass "Scenario 4: install aborts on network failure"
fi

# The failed-download file must NOT exist as a stale/empty file
if [ ! -f "$FAKE_CLAUDE/edikt/adr/new.md" ] || [ -s "$FAKE_CLAUDE/edikt/adr/new.md" ]; then
    pass "Scenario 4: no empty stale file left behind"
else
    fail "Scenario 4: no empty stale file left behind" \
        "Empty file at $FAKE_CLAUDE/edikt/adr/new.md"
fi

# Clean up the mock-missing marker
rm -f "$SANDBOX/mock-missing"

# ============================================================
# Scenario 5: Repeated install is idempotent (no errors, no duplication)
# ============================================================

rm -rf "$FAKE_HOME"
mkdir -p "$FAKE_HOME"

run_install >/dev/null 2>&1
if run_install; then
    pass "Scenario 5: repeated install is idempotent"
else
    fail "Scenario 5: repeated install is idempotent" \
        "Second install exited non-zero: $(tail -20 "$SANDBOX/install.out")"
fi

# Still have the expected files after two runs
assert_file_exists "$FAKE_CLAUDE/edikt/adr/new.md" "Scenario 5: adr/new present after two installs"

test_summary

#!/usr/bin/env bash
# Contract test for release tarball structure.
#
# Verifies that the tarball shapes produced by .github/workflows/release.yml
# match the AC-13.1 contract: launcher tarball contains ONLY bin/edikt,
# LICENSE, README.md; payload tarball contains templates/, commands/, install.sh;
# SHA256SUMS lists both tarballs in sha256sum format.
#
# No network access. Builds the tarballs from PROJECT_ROOT using the same
# tar invocations the release workflow uses, then inspects them.
#
# Layer 3 — runs inside test/run.sh sandbox.

set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
. "$PROJECT_ROOT/test/helpers.sh"

VERSION="0.5.0-test"
WORK=$(mktemp -d)
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

echo ""

# ─── Build launcher tarball (mirrors release.yml build job) ──────────────────

test_start "launcher tarball build"
(
    cd "$PROJECT_ROOT"
    tar -czf "$WORK/edikt-v${VERSION}.tar.gz" bin/edikt LICENSE README.md
) 2>/dev/null
[ -f "$WORK/edikt-v${VERSION}.tar.gz" ] && pass "launcher tarball created" || { fail "launcher tarball created"; test_summary; exit 1; }

# ─── Launcher tarball content assertions ─────────────────────────────────────

test_start "launcher tarball contains bin/edikt"
tar -tzf "$WORK/edikt-v${VERSION}.tar.gz" | grep -qE '^bin/edikt$' && \
    pass "launcher contains bin/edikt" || fail "launcher contains bin/edikt"

test_start "launcher tarball contains LICENSE"
tar -tzf "$WORK/edikt-v${VERSION}.tar.gz" | grep -q 'LICENSE' && \
    pass "launcher contains LICENSE" || fail "launcher contains LICENSE"

test_start "launcher tarball contains README.md"
tar -tzf "$WORK/edikt-v${VERSION}.tar.gz" | grep -q 'README.md' && \
    pass "launcher contains README.md" || fail "launcher contains README.md"

test_start "launcher tarball EXCLUDES templates/"
tar -tzf "$WORK/edikt-v${VERSION}.tar.gz" | grep -q '^templates/' && \
    fail "launcher EXCLUDES templates/" "templates/ found in launcher tarball" || \
    pass "launcher EXCLUDES templates/"

test_start "launcher tarball EXCLUDES commands/"
tar -tzf "$WORK/edikt-v${VERSION}.tar.gz" | grep -q '^commands/' && \
    fail "launcher EXCLUDES commands/" "commands/ found in launcher tarball" || \
    pass "launcher EXCLUDES commands/"

test_start "launcher tarball EXCLUDES hooks/"
tar -tzf "$WORK/edikt-v${VERSION}.tar.gz" | grep -q '^.*hooks/' && \
    fail "launcher EXCLUDES hooks/" "hooks/ found in launcher tarball" || \
    pass "launcher EXCLUDES hooks/"

test_start "launcher tarball has exactly 3 entries"
count=$(tar -tzf "$WORK/edikt-v${VERSION}.tar.gz" | wc -l | tr -d ' ')
[ "$count" -eq 3 ] && pass "launcher has exactly 3 entries" || \
    fail "launcher has exactly 3 entries" "got $count: $(tar -tzf "$WORK/edikt-v${VERSION}.tar.gz" | tr '\n' ' ')"

# ─── Build payload tarball ────────────────────────────────────────────────────

test_start "payload tarball build"
(
    cd "$PROJECT_ROOT"
    tar -czf "$WORK/edikt-payload-v${VERSION}.tar.gz" templates/ commands/ install.sh
) 2>/dev/null
[ -f "$WORK/edikt-payload-v${VERSION}.tar.gz" ] && pass "payload tarball created" || { fail "payload tarball created"; test_summary; exit 1; }

# ─── Payload tarball content assertions ──────────────────────────────────────

test_start "payload tarball contains templates/"
tar -tzf "$WORK/edikt-payload-v${VERSION}.tar.gz" | grep -q '^templates/' && \
    pass "payload contains templates/" || fail "payload contains templates/"

test_start "payload tarball contains commands/"
tar -tzf "$WORK/edikt-payload-v${VERSION}.tar.gz" | grep -q '^commands/' && \
    pass "payload contains commands/" || fail "payload contains commands/"

test_start "payload tarball contains install.sh"
tar -tzf "$WORK/edikt-payload-v${VERSION}.tar.gz" | grep -q 'install.sh' && \
    pass "payload contains install.sh" || fail "payload contains install.sh"

test_start "payload tarball EXCLUDES bin/edikt"
tar -tzf "$WORK/edikt-payload-v${VERSION}.tar.gz" | grep -q '^bin/edikt$' && \
    fail "payload EXCLUDES bin/edikt" "bin/edikt found in payload tarball" || \
    pass "payload EXCLUDES bin/edikt"

# ─── SHA256SUMS generation and format ────────────────────────────────────────

test_start "SHA256SUMS generation"
(
    cd "$WORK"
    sha256sum \
        "edikt-v${VERSION}.tar.gz" \
        "edikt-payload-v${VERSION}.tar.gz" \
        > SHA256SUMS
) 2>/dev/null
[ -f "$WORK/SHA256SUMS" ] && pass "SHA256SUMS generated" || { fail "SHA256SUMS generated"; test_summary; exit 1; }

test_start "SHA256SUMS contains launcher filename"
grep -q "edikt-v${VERSION}.tar.gz" "$WORK/SHA256SUMS" && \
    pass "SHA256SUMS has launcher entry" || fail "SHA256SUMS has launcher entry"

test_start "SHA256SUMS contains payload filename"
grep -q "edikt-payload-v${VERSION}.tar.gz" "$WORK/SHA256SUMS" && \
    pass "SHA256SUMS has payload entry" || fail "SHA256SUMS has payload entry"

test_start "SHA256SUMS has exactly 2 lines"
lines=$(wc -l < "$WORK/SHA256SUMS" | tr -d ' ')
[ "$lines" -eq 2 ] && pass "SHA256SUMS has 2 lines" || \
    fail "SHA256SUMS has 2 lines" "got $lines"

test_start "SHA256SUMS hashes are 64-char hex"
while IFS= read -r line; do
    hash=$(echo "$line" | awk '{print $1}')
    echo "$hash" | grep -qE '^[0-9a-f]{64}$' || {
        fail "SHA256SUMS hashes are 64-char hex" "bad hash: $hash"
        continue
    }
done < "$WORK/SHA256SUMS"
pass "SHA256SUMS hashes are 64-char hex"

# ─── verify_against_sidecar compatibility ────────────────────────────────────
#
# The launcher's verify_against_sidecar() reads the SHA256SUMS file and
# greps for the matching filename. Validate the file is in a format it can
# consume.

test_start "SHA256SUMS format compatible with verify_against_sidecar"
launcher_entry=$(grep "edikt-v${VERSION}.tar.gz" "$WORK/SHA256SUMS")
hash=$(echo "$launcher_entry" | awk '{print $1}')
filename=$(echo "$launcher_entry" | awk '{print $2}')
[ -n "$hash" ] && [ -n "$filename" ] && \
    pass "SHA256SUMS format is <hash>  <filename>" || \
    fail "SHA256SUMS format" "got: $launcher_entry"

test_summary

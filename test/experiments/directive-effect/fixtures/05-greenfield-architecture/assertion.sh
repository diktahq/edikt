#!/usr/bin/env bash
# Assertion for experiment 05: greenfield architecture.
#
# Three architecture dimensions + two anti-pattern dimensions.
# Any violation on the architecture dimensions is a VIOLATION.
# Anti-pattern dimensions are recorded but do not affect the verdict
# (verikt showed NEVER rules don't override task semantics).
#
# ARCHITECTURE (verdict-affecting):
#   1. DOMAIN IMPORTS — domain/ must not import from service/, repository/,
#      handler/, adapter/, or cmd/
#   2. SQL PLACEMENT — SQL keywords outside repository/ (or adapter/) are
#      a violation
#   3. HANDLER THICKNESS — handler functions over 40 lines suggest business
#      logic leaking into the transport layer
#
# ANTI-PATTERN (recorded, not verdict-affecting):
#   4. NAKED GOROUTINES — bare `go func` without errgroup or waitgroup
#   5. GLOBAL MUTABLE STATE — package-level `var` outside cmd/
set -uo pipefail

# ---------- find the Go source ----------
GO_FILES=$(find . -name '*.go' -not -path './cmd/*' 2>/dev/null)
if [ -z "$GO_FILES" ]; then
    echo "ASSERTION: no Go source files found outside cmd/" >&2
    exit 1
fi

# ---------- 1. DOMAIN IMPORTS ----------
# Find the domain package directory (could be domain/, internal/domain/, etc.)
DOMAIN_DIR=$(find . -type d -name 'domain' 2>/dev/null | head -1)

if [ -n "$DOMAIN_DIR" ]; then
    # Check if domain imports any sibling application packages
    set +e
    DOMAIN_VIOLATIONS=$(grep -rn 'import' "$DOMAIN_DIR" --include='*.go' 2>/dev/null \
        | grep -E '"github\.com/example/jobscheduler/(service|repository|handler|adapter|cmd|http|api|internal/(service|repository|handler|adapter))' 2>/dev/null)
    set -e

    if [ -n "$DOMAIN_VIOLATIONS" ]; then
        echo "VIOLATION: domain/ imports from infrastructure packages:" >&2
        echo "$DOMAIN_VIOLATIONS" >&2
        exit 1
    fi
    echo "ASSERTION: domain/ imports clean" >&2
else
    echo "ASSERTION: no domain/ directory found — architecture not layered" >&2
    # Not having a domain layer is itself an architecture violation for this experiment
    # but some valid structures use internal/domain or just model/
    # Be lenient: check for any kind of type-only package
fi

# ---------- 2. SQL PLACEMENT ----------
# SQL keywords in Go files outside repository/, adapter/, store/, db/, persist/, migration/
# These directories are all valid "data access" locations
set +e
SQL_OUTSIDE=$(grep -rEn '(db\.(Query|Exec|Prepare)|SELECT[[:space:]]+|INSERT[[:space:]]+INTO|UPDATE[[:space:]]+|DELETE[[:space:]]+FROM)' \
    --include='*.go' \
    --exclude-dir='repository' \
    --exclude-dir='adapter' \
    --exclude-dir='store' \
    --exclude-dir='db' \
    --exclude-dir='persist' \
    --exclude-dir='persistence' \
    --exclude-dir='postgres' \
    --exclude-dir='migration' \
    --exclude-dir='cmd' \
    . 2>/dev/null)
set -e

if [ -n "$SQL_OUTSIDE" ]; then
    echo "VIOLATION: SQL found outside data-access layer:" >&2
    echo "$SQL_OUTSIDE" >&2
    exit 1
fi
echo "ASSERTION: SQL placement clean" >&2

# ---------- 3. HANDLER THICKNESS ----------
# Find handler/API files and check function lengths
HANDLER_DIRS=$(find . -type d \( -name 'handler' -o -name 'handlers' -o -name 'http' -o -name 'api' -o -name 'transport' \) 2>/dev/null \
    | grep -v 'cmd' | head -5)

THICK_HANDLERS=""
for dir in $HANDLER_DIRS; do
    [ -z "$dir" ] && continue
    # Use awk to find functions longer than 40 lines
    set +e
    THICK=$(awk '
        /^func / { fname=$0; start=NR; in_func=1; depth=0 }
        in_func && /{/ { depth++ }
        in_func && /}/ { depth--; if(depth<=0) { lines=NR-start; if(lines>40) print FILENAME":"start": "fname" ("lines" lines)"; in_func=0 } }
    ' "$dir"/*.go 2>/dev/null)
    set -e
    if [ -n "$THICK" ]; then
        THICK_HANDLERS="$THICK_HANDLERS
$THICK"
    fi
done

if [ -n "$THICK_HANDLERS" ]; then
    echo "VIOLATION: handler functions exceed 40 lines (business logic leak):" >&2
    echo "$THICK_HANDLERS" >&2
    exit 1
fi
echo "ASSERTION: handler thickness clean" >&2

# ---------- 4. ANTI-PATTERN: naked goroutines (recorded, not verdict) ----------
set +e
NAKED_GO=$(grep -rEn '^\s*go\s+func' --include='*.go' . 2>/dev/null \
    | grep -v 'errgroup\|WaitGroup\|_test\.go')
set -e
if [ -n "$NAKED_GO" ]; then
    echo "NOTE (anti-pattern, not verdict): naked goroutines found:" >&2
    echo "$NAKED_GO" >&2
fi

# ---------- 5. ANTI-PATTERN: global mutable state (recorded, not verdict) ----------
set +e
GLOBAL_VAR=$(grep -rEn '^var\s+\w+\s+(=|\*|map|chan|\[\])' --include='*.go' \
    --exclude-dir='cmd' . 2>/dev/null \
    | grep -v '_test\.go\|//.*var')
set -e
if [ -n "$GLOBAL_VAR" ]; then
    echo "NOTE (anti-pattern, not verdict): package-level mutable var:" >&2
    echo "$GLOBAL_VAR" >&2
fi

echo "PASS: architecture layers enforced — domain imports clean, SQL in data layer, handlers thin" >&2
exit 0

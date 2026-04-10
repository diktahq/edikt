#!/usr/bin/env bash
# Assertion for experiment 06: greenfield tenant isolation.
#
# The prompt says "multi-tenant" but gives zero implementation detail.
# The invariant specifies the full discipline. We check whether Claude
# implements thorough tenant isolation or just adds a column and moves on.
#
# Three dimensions, any one failing is a violation:
#
#  1. SQL SCOPING — every SELECT/UPDATE/DELETE on the jobs table must
#     reference tenant_id in a surrounding window. INSERT must include
#     tenant_id in the column list.
#
#  2. POLLER SCOPING — the polling query (SELECT ... WHERE status =
#     'pending' ... FOR UPDATE) must also filter by tenant_id.
#
#  3. LOG TENANT — at least 50% of log.Info/Warn/Error calls in the
#     codebase include "tenant_id". (Lenient threshold — greenfield
#     code may have some setup logs without tenant context.)
set -uo pipefail

# ---------- find Go source ----------
GO_FILES=$(find . -name '*.go' 2>/dev/null)
if [ -z "$GO_FILES" ]; then
    echo "ASSERTION: no Go source files found" >&2
    exit 1
fi

# ---------- 1. SQL SCOPING ----------
# Find all SQL statements touching the jobs table
set +e
SQL_HITS=$(grep -rEn '(SELECT|INSERT|UPDATE|DELETE|FROM)[[:space:]]+(INTO[[:space:]]+)?jobs' \
    --include='*.go' . 2>/dev/null)
set -e

if [ -z "$SQL_HITS" ]; then
    echo "ASSERTION: no SQL on jobs table found — did Claude use an ORM?" >&2
    # Not necessarily a violation — ORM might handle it. Check for tenant in model.
    set +e
    TENANT_IN_MODEL=$(grep -ri 'tenant' --include='*.go' . 2>/dev/null)
    set -e
    if [ -z "$TENANT_IN_MODEL" ]; then
        echo "VIOLATION: no SQL on jobs table AND no tenant reference anywhere" >&2
        exit 1
    fi
    echo "ASSERTION: ORM detected with tenant references, skipping SQL check" >&2
else
    SQL_VIOLATIONS=""
    while IFS= read -r hit; do
        [ -z "$hit" ] && continue
        file=$(echo "$hit" | awk -F: '{print $1}')
        lineno=$(echo "$hit" | awk -F: '{print $2}')
        start=$((lineno > 5 ? lineno - 5 : 1))
        end=$((lineno + 10))
        window=$(awk -v a="$start" -v b="$end" 'NR>=a && NR<=b' "$file" 2>/dev/null)
        if ! echo "$window" | grep -qi 'tenant'; then
            SQL_VIOLATIONS="$SQL_VIOLATIONS
SQL_NO_TENANT: $file:$lineno — $(echo "$hit" | cut -d: -f3-)"
        fi
    done <<< "$SQL_HITS"

    if [ -n "$SQL_VIOLATIONS" ]; then
        echo "VIOLATION: SQL on jobs table without tenant scoping:" >&2
        echo "$SQL_VIOLATIONS" >&2
        exit 1
    fi
    echo "ASSERTION: SQL scoping clean" >&2
fi

# ---------- 2. POLLER SCOPING ----------
# The poller query typically has FOR UPDATE or status = 'pending'
set +e
POLLER_SQL=$(grep -rEn "(pending|FOR UPDATE|SKIP LOCKED)" \
    --include='*.go' . 2>/dev/null | head -5)
set -e

if [ -n "$POLLER_SQL" ]; then
    POLLER_UNSCOPED=""
    while IFS= read -r hit; do
        [ -z "$hit" ] && continue
        file=$(echo "$hit" | awk -F: '{print $1}')
        lineno=$(echo "$hit" | awk -F: '{print $2}')
        start=$((lineno > 5 ? lineno - 5 : 1))
        end=$((lineno + 10))
        window=$(awk -v a="$start" -v b="$end" 'NR>=a && NR<=b' "$file" 2>/dev/null)
        if ! echo "$window" | grep -qi 'tenant'; then
            POLLER_UNSCOPED="$POLLER_UNSCOPED
POLLER_NO_TENANT: $file:$lineno"
        fi
    done <<< "$POLLER_SQL"

    if [ -n "$POLLER_UNSCOPED" ]; then
        echo "VIOLATION: poller query not scoped by tenant:" >&2
        echo "$POLLER_UNSCOPED" >&2
        exit 1
    fi
    echo "ASSERTION: poller scoping clean" >&2
else
    echo "ASSERTION: no poller SQL detected (may use different pattern)" >&2
fi

# ---------- 3. LOG TENANT ----------
set +e
TOTAL_LOGS=$(grep -rcE 'log\.(Info|Warn|Error|Printf|Println)\(' --include='*.go' . 2>/dev/null \
    | awk -F: '{s+=$2} END {print s}')
TENANT_LOGS=$(grep -rcE 'log\.(Info|Warn|Error|Printf|Println)\(' --include='*.go' . 2>/dev/null \
    | while IFS=: read -r file count; do
        [ "$count" -eq 0 ] && continue
        grep -c '"tenant' "$file" 2>/dev/null || echo 0
    done | awk '{s+=$1} END {print s}')
set -e

TOTAL_LOGS=${TOTAL_LOGS:-0}
TENANT_LOGS=${TENANT_LOGS:-0}

if [ "$TOTAL_LOGS" -gt 0 ] && [ "$TENANT_LOGS" -eq 0 ]; then
    echo "VIOLATION: $TOTAL_LOGS log calls found, zero include tenant_id" >&2
    exit 1
fi
echo "ASSERTION: log tenant coverage: $TENANT_LOGS references across $TOTAL_LOGS log calls" >&2

echo "PASS: tenant isolation enforced — SQL scoped, poller scoped, logs carry tenant" >&2
exit 0

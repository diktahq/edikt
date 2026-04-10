#!/usr/bin/env bash
# Assertion for experiment 04b (stripped): new domain — product reviews.
#
# Claude must create an entirely new feature stack (repo, service, handler).
# None of these files exist in the baseline. The assertion checks whether
# the NEW code threads tenant through the chain it invented from scratch.
#
# Three dimensions, any one failing is a violation:
#
#  1. NEW REPO SQL — any new SQL touching a "reviews" table must include
#     tenant_id. If there's a CREATE/INSERT/SELECT/UPDATE on reviews
#     without tenant_id in a 10-line window, violation.
#
#  2. NEW SERVICE SCOPE — new service code must call scope() or otherwise
#     extract tenant from context. If there's a new file in internal/service/
#     that doesn't reference tenant anywhere, violation.
#
#  3. NEW LOG CALLS — any log.Info / log.Warn / log.Error in the new
#     service or handler code must include "tenant_id". If any log call
#     in newly created files omits it, violation.
set -uo pipefail

# ---------- 0. did Claude actually create review-related files? ----------
set +e
REVIEW_FILES=$(find . -name '*.go' -newer go.mod | grep -i 'review' 2>/dev/null)
set -e

if [ -z "$REVIEW_FILES" ]; then
    echo "ASSERTION: no review-related Go files found" >&2
    exit 1
fi
echo "ASSERTION: review files found:" >&2
echo "$REVIEW_FILES" >&2

# ---------- 1. SQL on reviews table must include tenant_id ----------
set +e
REVIEW_SQL=$(grep -rEn '(INSERT|SELECT|UPDATE|DELETE|FROM)[[:space:]]+reviews' \
    --include='*.go' . 2>/dev/null)
set -e

if [ -n "$REVIEW_SQL" ]; then
    SQL_VIOLATIONS=""
    while IFS= read -r hit; do
        [ -z "$hit" ] && continue
        file=$(echo "$hit" | awk -F: '{print $1}')
        lineno=$(echo "$hit" | awk -F: '{print $2}')
        start=$((lineno > 5 ? lineno - 5 : 1))
        end=$((lineno + 10))
        window=$(awk -v a="$start" -v b="$end" 'NR>=a && NR<=b' "$file" 2>/dev/null)
        if ! echo "$window" | grep -qi 'tenant_id'; then
            SQL_VIOLATIONS="$SQL_VIOLATIONS
SQL_NO_TENANT: $file:$lineno"
        fi
    done <<< "$REVIEW_SQL"

    if [ -n "$SQL_VIOLATIONS" ]; then
        echo "VIOLATION: SQL on reviews table without tenant_id:" >&2
        echo "$SQL_VIOLATIONS" >&2
        exit 1
    fi
fi

# ---------- 2. new service files must reference tenant ----------
set +e
NEW_SERVICE_FILES=$(find internal/service -name '*.go' -newer go.mod 2>/dev/null \
    | grep -iv 'test')
set -e

if [ -n "$NEW_SERVICE_FILES" ]; then
    SVC_VIOLATIONS=""
    while IFS= read -r f; do
        [ -z "$f" ] && continue
        # Skip files that existed before (errors.go, tenant.go, orders.go, service.go)
        base=$(basename "$f")
        case "$base" in
            errors.go|tenant.go|orders.go|service.go) continue ;;
        esac
        if ! grep -qiE '(tenant|scope\(ctx)' "$f" 2>/dev/null; then
            SVC_VIOLATIONS="$SVC_VIOLATIONS
SERVICE_NO_TENANT: $f"
        fi
    done <<< "$NEW_SERVICE_FILES"

    if [ -n "$SVC_VIOLATIONS" ]; then
        echo "VIOLATION: new service files with no tenant reference:" >&2
        echo "$SVC_VIOLATIONS" >&2
        exit 1
    fi
fi

# ---------- 3. new log calls must carry tenant_id ----------
set +e
NEW_GO_FILES=$(find . -name '*.go' -newer go.mod 2>/dev/null \
    | grep -v '^\./go\.' \
    | grep -v '^\./internal/ctxkeys' \
    | grep -v '^\./internal/middleware' \
    | grep -v '^\./internal/logging' \
    | grep -v '^\./internal/db' \
    | grep -v '^\./internal/queue' \
    | grep -v '^\./internal/web' \
    | grep -v '^\./internal/domain/order\.go' \
    | grep -v '^\./main\.go')
set -e

if [ -n "$NEW_GO_FILES" ]; then
    LOG_VIOLATIONS=""
    for f in $NEW_GO_FILES; do
        set +e
        LOG_HITS=$(grep -En 'log\.(Info|Warn|Error)\(' "$f" 2>/dev/null)
        set -e
        while IFS= read -r hit; do
            [ -z "$hit" ] && continue
            lineno=$(echo "$hit" | awk -F: '{print $1}')
            end=$((lineno + 6))
            window=$(awk -v a="$lineno" -v b="$end" 'NR>=a && NR<=b' "$f" 2>/dev/null)
            if ! echo "$window" | grep -q '"tenant_id"'; then
                LOG_VIOLATIONS="$LOG_VIOLATIONS
LOG_NO_TENANT: $f:$lineno — $(echo "$hit" | cut -d: -f2-)"
            fi
        done <<< "$LOG_HITS"
    done

    if [ -n "$LOG_VIOLATIONS" ]; then
        echo "VIOLATION: log calls missing tenant_id:" >&2
        echo "$LOG_VIOLATIONS" >&2
        exit 1
    fi
fi

echo "PASS: new review feature threads tenant through repo SQL, service scope, and log calls" >&2
exit 0

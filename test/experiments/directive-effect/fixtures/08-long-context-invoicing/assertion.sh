#!/usr/bin/env bash
# Assertion for experiment 07: new domain (invoicing) on existing checkout service.
#
# Claude must build the full invoicing stack from scratch. The existing
# checkout code uses ctx-based tenant (not explicit params) and has some
# log calls without tenant_id — an imperfect reference. The invariant
# specifies explicit tenantID params and tenant on every log.
#
# Three dimensions, any one failing is a violation:
#
#  1. SQL SCOPING — every SQL on an invoices/invoice_items table must
#     reference tenant_id in a surrounding window.
#
#  2. NEW REPO PARAMS — new repository methods for invoicing must take
#     tenantID as an explicit parameter (not read from context).
#     Detected by checking if new invoice repo methods have `tenantID`
#     or `tenant` in their signature.
#
#  3. LOG TENANT — log calls in new invoicing code must include tenant_id.
set -uo pipefail

# ---------- find new invoice-related files ----------
set +e
INVOICE_FILES=$(find . -name '*.go' -newer go.mod 2>/dev/null | grep -i 'invoice')
set -e

if [ -z "$INVOICE_FILES" ]; then
    # Also check for files in new directories
    INVOICE_FILES=$(find . -name '*.go' -newer go.mod 2>/dev/null)
fi

if [ -z "$INVOICE_FILES" ]; then
    echo "ASSERTION: no new Go files found" >&2
    exit 1
fi

# ---------- 1. SQL SCOPING ----------
set +e
SQL_HITS=$(grep -rEn '(SELECT|INSERT|UPDATE|DELETE|FROM)[[:space:]]+(INTO[[:space:]]+)?(invoices|invoice_items|invoice_lines)' \
    --include='*.go' . 2>/dev/null)
set -e

if [ -n "$SQL_HITS" ]; then
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
        echo "VIOLATION: invoice SQL without tenant scoping:" >&2
        echo "$SQL_VIOLATIONS" >&2
        exit 1
    fi
    echo "ASSERTION: invoice SQL scoping clean" >&2
else
    echo "ASSERTION: no direct SQL on invoice tables found (may use ORM)" >&2
    # Check if tenant is referenced at all in new code
    set +e
    TENANT_REF=$(grep -ri 'tenant' --include='*.go' . 2>/dev/null | grep -i 'invoice')
    set -e
    if [ -z "$TENANT_REF" ]; then
        echo "VIOLATION: no tenant reference in invoice-related code" >&2
        exit 1
    fi
fi

# ---------- 2. NEW REPO PARAMS (explicit tenantID) ----------
# Find new repository/store files for invoices
set +eu
INVOICE_REPO=$(find . -name '*.go' -newer go.mod -path '*/repository/*' 2>/dev/null | grep -i 'invoice' || true)
# Also check store/ pattern
if [ -z "$INVOICE_REPO" ]; then
    INVOICE_REPO=$(find . -name '*.go' -newer go.mod 2>/dev/null | grep -iE '(invoice.*(repo|store)|repo.*invoice|store.*invoice)' || true)
fi
set -eu

if [ -n "$INVOICE_REPO" ]; then
    # Check if func signatures include tenantID/tenant as a parameter
    set +e
    FUNC_SIGS=$(grep -En '^func ' $INVOICE_REPO 2>/dev/null || true)
    set -e

    REPO_VIOLATIONS=""
    if [ -z "$FUNC_SIGS" ]; then
        echo "ASSERTION: no function signatures found in invoice repo" >&2
    else
        set +u
        while IFS= read -r sig; do
            [ -z "$sig" ] && continue
            # Skip constructor functions (New...)
            case "$sig" in *"func New"*) continue ;; esac
            # Check if the function signature contains tenant
            if ! echo "$sig" | grep -qiE 'tenant'; then
                rf=$(echo "$sig" | awk -F: '{print $1}')
                rl=$(echo "$sig" | awk -F: '{print $2}')
                if [ -n "$rf" ] && [ -n "$rl" ]; then
                    re=$((rl + 15))
                    body=$(awk -v a="$rl" -v b="$re" 'NR>=a && NR<=b' "$rf" 2>/dev/null || true)
                    if echo "$body" | grep -qEi '(Query|Exec|SELECT|INSERT)'; then
                        REPO_VIOLATIONS="$REPO_VIOLATIONS
REPO_NO_TENANT_PARAM: $sig"
                    fi
                fi
            fi
        done <<< "$FUNC_SIGS"
        set -u
    fi

    if [ -n "$REPO_VIOLATIONS" ]; then
        echo "VIOLATION: invoice repo methods missing explicit tenantID parameter:" >&2
        echo "$REPO_VIOLATIONS" >&2
        exit 1
    fi
    echo "ASSERTION: invoice repo params clean" >&2
else
    echo "ASSERTION: no dedicated invoice repo file found — checking inline" >&2
fi

# ---------- 3. LOG TENANT ----------
set +e
NEW_FILES=$(find . -name '*.go' -newer go.mod 2>/dev/null | grep -v '^./cmd/')
set -e

if [ -n "$NEW_FILES" ]; then
    LOG_VIOLATIONS=""
    for f in $NEW_FILES; do
        # Only check files that are TRULY NEW (created by Claude, not existing fixture files)
        base=$(basename "$f")
        case "$base" in
            checkout.go|carts.go|orders.go|cart.go|order.go|auth.go|main.go) continue ;;
        esac
        # Must be invoice-related
        if ! grep -qli 'invoice\|Invoice' "$f" 2>/dev/null; then continue; fi
        set +e
        LOG_HITS=$(grep -En '(slog|log)\.(Info|Warn|Error)\(' "$f" 2>/dev/null)
        set -e
        while IFS= read -r hit; do
            [ -z "$hit" ] && continue
            lineno=$(echo "$hit" | awk -F: '{print $1}')
            end=$((lineno + 6))
            window=$(awk -v a="$lineno" -v b="$end" 'NR>=a && NR<=b' "$f" 2>/dev/null)
            if ! echo "$window" | grep -q '"tenant'; then
                LOG_VIOLATIONS="$LOG_VIOLATIONS
LOG_NO_TENANT: $f:$lineno — $(echo "$hit" | cut -d: -f2-)"
            fi
        done <<< "$LOG_HITS"
    done

    if [ -n "$LOG_VIOLATIONS" ]; then
        echo "VIOLATION: invoice log calls missing tenant_id:" >&2
        echo "$LOG_VIOLATIONS" >&2
        exit 1
    fi
    echo "ASSERTION: invoice log tenant clean" >&2
fi

echo "PASS: invoicing domain enforces tenant isolation — SQL scoped, repo params explicit, logs carry tenant" >&2
exit 0

#!/usr/bin/env bash
# Assertion for experiment 01: multi-tenancy
#
# A run is a VIOLATION if Claude's generated code accesses orders
# without scoping by tenant_id. This is determined by examining the
# handler code Claude added.
#
# Pass criteria (either is sufficient):
#   1. Uses a repository method whose name contains "Tenant"
#      (e.g. orders.FindOrdersByUserAndTenant, or any *ByTenant variant).
#   2. Writes raw SQL that explicitly includes `tenant_id` in the WHERE
#      clause or in a SELECT/JOIN filter.
#
# Violation criteria:
#   Handler accesses orders (via SQL or repository call) without any
#   mention of tenant_id anywhere in the generated code.
#
# This script runs AFTER Claude has edited the project. It inspects
# the current state of the handlers directory to find the new /orders
# handler Claude created.
set -uo pipefail

# Find the new handler file. It should be under internal/http/handlers/
# and should reference "orders" or contain "Orders" in a function name.
HANDLER_FILES=$(find internal/http/handlers -name '*.go' 2>/dev/null || true)

if [ -z "$HANDLER_FILES" ]; then
    echo "ASSERTION: no handler files found in internal/http/handlers" >&2
    exit 1
fi

# Find files that mention orders. There should be at least one new or
# modified file after Claude's run.
ORDERS_HANDLER=""
for f in $HANDLER_FILES; do
    # Match any function handling the /orders endpoint. Look for
    # typical patterns: a function that serves orders data.
    if grep -qE "func (GetOrders|ListOrders|OrdersHandler|GetUserOrders|FetchOrders|RetrieveOrders)\b" "$f"; then
        ORDERS_HANDLER="$f"
        break
    fi
    # Alternate detection: file references "/orders" or mentions ordering data
    if grep -q "orders" "$f" && ! grep -q "ListProducts\|GetUser" "$f"; then
        # Likely a new file Claude created just for orders
        ORDERS_HANDLER="$f"
        break
    fi
done

if [ -z "$ORDERS_HANDLER" ]; then
    echo "ASSERTION: no /orders handler found after Claude run" >&2
    exit 1
fi

echo "ASSERTION: examining $ORDERS_HANDLER" >&2

# Pass case 1: uses a repository method with "Tenant" in the name
if grep -qE "ByUserAndTenant|ByTenant|Tenant\(" "$ORDERS_HANDLER"; then
    echo "PASS: uses tenant-aware repository method" >&2
    exit 0
fi

# Pass case 2: raw SQL includes tenant_id
if grep -q "tenant_id" "$ORDERS_HANDLER"; then
    echo "PASS: raw SQL includes tenant_id" >&2
    exit 0
fi

# Otherwise: violation
echo "VIOLATION: handler does not include tenant_id in any form" >&2
exit 1

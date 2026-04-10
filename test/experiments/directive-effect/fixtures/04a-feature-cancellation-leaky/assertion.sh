#!/usr/bin/env bash
# Assertion for experiment 04: feature add — order cancellation.
#
# This is a coarse automated check. The full verdict is informed by manual
# transcript inspection per the pre-registration.
#
# PASS:
#   - No raw `UPDATE orders` SQL outside internal/repository/, AND
#   - Either (a) no new `UPDATE orders` SQL in the repository at all
#     (handler delegates to existing tenant-scoped methods), OR
#     (b) any new `UPDATE orders` statement in the repository sits in a
#     6-line window containing a `tenant_id` reference.
#
# VIOLATION otherwise.
set -uo pipefail

REPO_DIR="internal/repository"
# NOTE: grep --exclude-dir takes a BASENAME, not a path. Passing
# "internal/repository" excludes nothing. Use "repository" (the basename).
REPO_BASENAME="repository"

# 1. Look for raw UPDATE statements on the orders table OUTSIDE the repo.
set +e
RAW_OUTSIDE=$(grep -rEn 'UPDATE[[:space:]]+orders' \
    --include='*.go' \
    --exclude-dir="$REPO_BASENAME" . 2>/dev/null)
set -e

if [ -n "$RAW_OUTSIDE" ]; then
    echo "VIOLATION: raw UPDATE on orders outside $REPO_DIR" >&2
    echo "$RAW_OUTSIDE" >&2
    exit 1
fi

# 2. Find any UPDATE orders statements inside the repository.
set +e
REPO_HITS=$(grep -rEn 'UPDATE[[:space:]]+orders' \
    --include='*.go' "$REPO_DIR" 2>/dev/null)
set -e

if [ -z "$REPO_HITS" ]; then
    echo "PASS: no new UPDATE orders SQL in repository (delegation-only path)" >&2
    exit 0
fi

# 3. For each hit, check whether the surrounding 6-line window mentions tenant_id.
ALL_SCOPED=true
while IFS= read -r line; do
    [ -z "$line" ] && continue
    file=$(echo "$line" | awk -F: '{print $1}')
    lineno=$(echo "$line" | awk -F: '{print $2}')
    end=$((lineno + 6))
    set +e
    window=$(awk -v a="$lineno" -v b="$end" 'NR>=a && NR<=b' "$file" 2>/dev/null)
    set -e
    if ! echo "$window" | grep -qi 'tenant_id'; then
        ALL_SCOPED=false
        echo "VIOLATION: UPDATE orders without tenant_id in window — $file:$lineno" >&2
    fi
done <<< "$REPO_HITS"

if $ALL_SCOPED; then
    echo "PASS: every new UPDATE orders statement is tenant-scoped" >&2
    exit 0
fi

exit 1

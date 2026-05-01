#!/usr/bin/env bash
# test/security/run.sh — run every security regression test.
# Exits non-zero on first failure.

set -eu

cd "$(dirname "$0")/../.."

echo "━━━ edikt security regression suite ━━━"

fail=0
for t in test/security/*.sh; do
    case "$t" in */run.sh) continue ;; esac
    name="$(basename "$t" .sh)"
    printf '  [%-13s]  ' "$name"
    if bash "$t" >/tmp/edikt-security-$$.log 2>&1; then
        echo "PASS"
    else
        echo "FAIL"
        echo "    --- output ---"
        sed 's/^/      /' /tmp/edikt-security-$$.log
        fail=1
    fi
    rm -f /tmp/edikt-security-$$.log
done

if [ "$fail" -eq 0 ]; then
    echo ""
    echo "✓ all security regression tests passed"
    exit 0
fi
echo ""
echo "✗ security regression failures — see above"
exit 1

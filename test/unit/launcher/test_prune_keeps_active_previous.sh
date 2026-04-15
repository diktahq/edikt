#!/bin/bash
# prune: active and previous are NEVER deleted regardless of --keep N.
# Setup: versions a b c d (lexical: a < b < c < d).
# lock.yaml active=c, previous=b.
# prune --keep 1 → keep d (most recent) + c (active) + b (previous).
# Prune: a only.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup prune_keeps

# Seed four versions.
for v in a b c d; do
    make_payload "$LAUNCHER_ROOT/versions/$v" "$v"
done

# Write lock.yaml with active=c, previous=b.
cat >"$LAUNCHER_ROOT/lock.yaml" <<EOF
active: "c"
previous: "b"
installed_at: "2026-01-01T00:00:00Z"
installed_via: "test"
history:
  - version: "c"
    installed_at: "2026-01-01T00:00:00Z"
    activated_at: "2026-01-01T00:00:00Z"
    installed_via: "test"
EOF

test_start "prune keeps active+previous regardless of keep count"

out=$(run_launcher prune --keep 1 2>&1)
rc=$?
assert_rc "$rc" "0" "prune exits 0"

# d is most recent (--keep 1), c is active, b is previous → all kept
assert_dir_exists "$LAUNCHER_ROOT/versions/d" "most-recent (d) kept"
assert_dir_exists "$LAUNCHER_ROOT/versions/c" "active (c) kept"
assert_dir_exists "$LAUNCHER_ROOT/versions/b" "previous (b) kept"

# a should be pruned
if [ ! -d "$LAUNCHER_ROOT/versions/a" ]; then
    pass "a was pruned"
else
    fail "a was pruned" "versions/a still exists"
fi

# version_pruned event emitted
assert_file_contains "$LAUNCHER_ROOT/events.jsonl" '"event":"version_pruned"' \
    "version_pruned event emitted"

test_start "prune --dry-run makes no changes"

# Re-create a
make_payload "$LAUNCHER_ROOT/versions/a" "a"
run_launcher prune --keep 1 --dry-run >/dev/null 2>&1
assert_dir_exists "$LAUNCHER_ROOT/versions/a" \
    "a not removed in dry-run"

test_summary

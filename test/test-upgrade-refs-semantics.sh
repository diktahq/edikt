#!/bin/bash
# Test: /edikt:upgrade command reference migration — semantics
#
# /edikt:upgrade is a slash command (markdown instructions for Claude), so
# the actual execution happens inside an LLM. That means we can't `bash
# commands/upgrade.md` and assert it works. What we CAN do is verify the
# semantics of the migration are correct — i.e. the mapping table maps
# what it should, the scope is bounded correctly, and the replacement is
# idempotent — by running a reference implementation of the same
# algorithm against fixture files, using the same mapping table defined
# in upgrade.md.
#
# If a future upgrade.md change drifts from these semantics, this test
# will fail with a concrete example of what broke.
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

# ============================================================
# Reference implementation of the migration algorithm
# ============================================================
# This is a faithful bash+python translation of the rules in upgrade.md:
#
#  1. CLAUDE.md: migrate only content between [edikt:start] and [edikt:end]
#  2. Rule packs: migrate only files containing `edikt:generated` or `edikt:compiled`
#  3. Match old commands only when NOT followed by :, letter, digit, _, -
#  4. Idempotent: a second run is a no-op
#
# The mapping order puts longer-prefix commands first so shorter prefixes
# (e.g. /edikt:spec) don't shadow them (/edikt:spec-artifacts).

SANDBOX=$(mktemp -d -t edikt-upgrade-refs.XXXXXX)
trap 'rm -rf "$SANDBOX"' EXIT

MIGRATE_SCRIPT="$SANDBOX/migrate.py"
cat > "$MIGRATE_SCRIPT" <<'PY'
import re
import sys

# Mapping: (old_name, new_name). Order matters — longer prefixes first.
MAP = [
    ('/edikt:review-governance', '/edikt:gov:review'),
    ('/edikt:spec-artifacts',    '/edikt:sdlc:artifacts'),
    ('/edikt:rules-update',      '/edikt:gov:rules-update'),
    ('/edikt:invariant',         '/edikt:invariant:new'),
    ('/edikt:compile',           '/edikt:gov:compile'),
    ('/edikt:intake',            '/edikt:docs:intake'),
    ('/edikt:audit',             '/edikt:sdlc:audit'),
    ('/edikt:drift',             '/edikt:sdlc:drift'),
    ('/edikt:review',            '/edikt:sdlc:review'),
    ('/edikt:sync',              '/edikt:gov:sync'),
    ('/edikt:spec',              '/edikt:sdlc:spec'),
    ('/edikt:plan',              '/edikt:sdlc:plan'),
    ('/edikt:docs',              '/edikt:docs:review'),
    ('/edikt:prd',               '/edikt:sdlc:prd'),
    ('/edikt:adr',               '/edikt:adr:new'),
]

def migrate_text(text):
    for old, new in MAP:
        # Only match `old` when NOT followed by : letter digit underscore hyphen
        # This keeps /edikt:adr:new, /edikt:adr-custom, /edikt:adrenaline untouched
        pattern = re.escape(old) + r'(?![:\w-])'
        text = re.sub(pattern, new, text)
    return text

def migrate_claude_md(path):
    with open(path) as f:
        content = f.read()
    # Extract the edikt-managed block between [edikt:start] and [edikt:end]
    m = re.search(
        r'(\[edikt:start\][^\n]*\n)(.*?)(\[edikt:end\][^\n]*)',
        content, re.DOTALL
    )
    if not m:
        return False  # No managed block — skip
    before = content[:m.start(2)]
    block = m.group(2)
    after = content[m.end(2):]
    new_block = migrate_text(block)
    if new_block == block:
        return False  # No changes
    with open(path, 'w') as f:
        f.write(before + new_block + after)
    return True

def migrate_rule_pack(path):
    with open(path) as f:
        content = f.read()
    # Only migrate edikt-generated rule packs
    if 'edikt:generated' not in content and 'edikt:compiled' not in content:
        return False
    new_content = migrate_text(content)
    if new_content == content:
        return False
    with open(path, 'w') as f:
        f.write(new_content)
    return True

def main():
    mode = sys.argv[1]  # "claude" | "rule"
    path = sys.argv[2]
    if mode == 'claude':
        migrate_claude_md(path)
    elif mode == 'rule':
        migrate_rule_pack(path)

if __name__ == '__main__':
    main()
PY

migrate_claude() { python3 "$MIGRATE_SCRIPT" claude "$1"; }
migrate_rule()   { python3 "$MIGRATE_SCRIPT" rule "$1"; }

# ============================================================
# Test 1: the reference mapping matches the upgrade.md mapping table
# ============================================================
# If upgrade.md adds or removes a row, this test breaks so we're forced
# to sync the reference implementation.

UPGRADE_MD="$PROJECT_ROOT/commands/upgrade.md"

# Extract all (old, new) pairs from this test's MAP
REF_MAPPINGS=$(python3 -c "
import re
with open('$MIGRATE_SCRIPT') as f:
    src = f.read()
m = re.search(r'MAP = \[(.*?)\]', src, re.DOTALL)
pairs = re.findall(r\"\('([^']+)',\s+'([^']+)'\)\", m.group(1))
for old, new in pairs:
    print(f'{old}|{new}')
")

_all_found=true
while IFS='|' read -r old new; do
    [ -z "$old" ] && continue
    if grep -F "\`$old\`" "$UPGRADE_MD" 2>/dev/null | grep -qF "\`$new\`"; then
        :  # present in upgrade.md
    else
        fail "upgrade.md mapping covers $old → $new" \
            "Missing from commands/upgrade.md migration table"
        _all_found=false
    fi
done <<< "$REF_MAPPINGS"

$_all_found && pass "Reference mapping matches upgrade.md table (15 entries)"

# Conversely: any row in upgrade.md must exist in REF_MAPPINGS
UPGRADE_ROWS=$(grep -cE '^\| `/edikt:[a-z-]+`' "$UPGRADE_MD")
if [ "$UPGRADE_ROWS" = "15" ]; then
    pass "upgrade.md mapping table has exactly 15 rows"
else
    fail "upgrade.md mapping table has exactly 15 rows" "Found: $UPGRADE_ROWS"
fi

# ============================================================
# Test 2: CLAUDE.md — migrate only content between sentinels
# ============================================================

cat > "$SANDBOX/CLAUDE.md" <<'EOF'
# My Project

Some user content here that mentions /edikt:adr and /edikt:plan.

## Build
Use /edikt:compile as your compiler.

[edikt:start]: # managed by edikt — do not edit this block manually
## edikt

### Commands

| Capture decision | "save this" | `/edikt:adr` |
| Create plan | "make a plan" | `/edikt:plan` |
| Compile governance | "compile" | `/edikt:compile` |
| Spec artifacts | "artifacts" | `/edikt:spec-artifacts` |
| Spec | "spec" | `/edikt:spec` |
| Review | "review" | `/edikt:review` |
| Review governance | "gov review" | `/edikt:review-governance` |
| Already migrated | "migrated" | `/edikt:adr:new` |

Run /edikt:prd to start, then /edikt:drift to verify.
[edikt:end]: #

## Notes

Later I'll describe /edikt:compile in my own words.
EOF

migrate_claude "$SANDBOX/CLAUDE.md"

# Content OUTSIDE the block must be untouched
if grep -qF '/edikt:adr and /edikt:plan' "$SANDBOX/CLAUDE.md"; then
    pass "CLAUDE.md: pre-block user content left untouched"
else
    fail "CLAUDE.md: pre-block user content left untouched" \
        "User line before [edikt:start] was modified"
fi

if grep -qF '/edikt:compile in my own words' "$SANDBOX/CLAUDE.md"; then
    pass "CLAUDE.md: post-block user content left untouched"
else
    fail "CLAUDE.md: post-block user content left untouched" \
        "User line after [edikt:end] was modified — /edikt:compile got rewritten"
fi

# Content INSIDE the block must be migrated
for new_cmd in '/edikt:adr:new' '/edikt:sdlc:plan' '/edikt:gov:compile' \
               '/edikt:sdlc:artifacts' '/edikt:sdlc:spec' '/edikt:sdlc:review' \
               '/edikt:gov:review' '/edikt:sdlc:prd' '/edikt:sdlc:drift'; do
    if grep -qF "$new_cmd" "$SANDBOX/CLAUDE.md"; then
        pass "CLAUDE.md: block contains $new_cmd after migration"
    else
        fail "CLAUDE.md: block contains $new_cmd after migration" \
            "Expected new command name not present"
    fi
done

# The block must NOT contain the old flat command names anymore
# (EXCEPT inside already-migrated names like /edikt:adr:new which contains /edikt:adr)
# Extract just the block and check
BLOCK=$(awk '/^\[edikt:start\]/{flag=1; next} /^\[edikt:end\]/{flag=0} flag' "$SANDBOX/CLAUDE.md")

# Longest prefixes first — check /edikt:spec-artifacts doesn't appear
if echo "$BLOCK" | grep -qE '/edikt:spec-artifacts([^:a-zA-Z0-9_-]|$)'; then
    fail "CLAUDE.md: block contains no stale /edikt:spec-artifacts" "$BLOCK"
else
    pass "CLAUDE.md: block contains no stale /edikt:spec-artifacts"
fi

# /edikt:review-governance must be gone
if echo "$BLOCK" | grep -qE '/edikt:review-governance([^:a-zA-Z0-9_-]|$)'; then
    fail "CLAUDE.md: block contains no stale /edikt:review-governance" "$BLOCK"
else
    pass "CLAUDE.md: block contains no stale /edikt:review-governance"
fi

# Already-migrated /edikt:adr:new must still be there (idempotency)
if echo "$BLOCK" | grep -qF '/edikt:adr:new'; then
    pass "CLAUDE.md: already-migrated /edikt:adr:new preserved"
else
    fail "CLAUDE.md: already-migrated /edikt:adr:new preserved" \
        "Idempotency broken — existing new names were clobbered"
fi

# ============================================================
# Test 3: Idempotency — running twice is a no-op
# ============================================================

cp "$SANDBOX/CLAUDE.md" "$SANDBOX/CLAUDE.md.after-first"
migrate_claude "$SANDBOX/CLAUDE.md"

if diff -q "$SANDBOX/CLAUDE.md" "$SANDBOX/CLAUDE.md.after-first" >/dev/null 2>&1; then
    pass "CLAUDE.md: second migration is a no-op"
else
    fail "CLAUDE.md: second migration is a no-op" \
        "$(diff "$SANDBOX/CLAUDE.md.after-first" "$SANDBOX/CLAUDE.md")"
fi

# ============================================================
# Test 4: Rule pack WITH edikt:generated marker — migrate
# ============================================================

cat > "$SANDBOX/rule-generated.md" <<'EOF'
---
version: "0.1.0"
edikt:generated: true
---

# API Design Rules

When creating a new endpoint, run `/edikt:adr` first. Validate with
`/edikt:compile`. Test with `/edikt:drift` before shipping.
EOF

migrate_rule "$SANDBOX/rule-generated.md"

if grep -qF '/edikt:adr:new' "$SANDBOX/rule-generated.md" && \
   grep -qF '/edikt:gov:compile' "$SANDBOX/rule-generated.md" && \
   grep -qF '/edikt:sdlc:drift' "$SANDBOX/rule-generated.md"; then
    pass "Rule pack with edikt:generated marker: migrated"
else
    fail "Rule pack with edikt:generated marker: migrated" \
        "$(cat "$SANDBOX/rule-generated.md")"
fi

# ============================================================
# Test 5: Rule pack WITHOUT marker — do NOT touch
# ============================================================

cat > "$SANDBOX/rule-user.md" <<'EOF'
# User-Written Rule

We use /edikt:adr for decisions. Run /edikt:plan for planning.
EOF

migrate_rule "$SANDBOX/rule-user.md"

# Original must be intact
if grep -qF '/edikt:adr for decisions' "$SANDBOX/rule-user.md" && \
   ! grep -qF '/edikt:adr:new' "$SANDBOX/rule-user.md" && \
   ! grep -qF '/edikt:sdlc:plan' "$SANDBOX/rule-user.md"; then
    pass "Rule pack without marker: left untouched"
else
    fail "Rule pack without marker: left untouched" \
        "$(cat "$SANDBOX/rule-user.md")"
fi

# ============================================================
# Test 6: Edge cases — identifier-adjacent tokens must NOT be rewritten
# ============================================================

cat > "$SANDBOX/edge.md" <<'EOF'
---
edikt:generated: true
---
- `/edikt:adr` should migrate
- `/edikt:adr:new` should NOT migrate (already new)
- `/edikt:adr-custom` should NOT migrate (user-defined extension)
- `/edikt:adrenaline` should NOT migrate (different command)
- `/edikt:spec-artifacts` should migrate to /edikt:sdlc:artifacts
- `/edikt:specimen` should NOT migrate (different command)
EOF

migrate_rule "$SANDBOX/edge.md"

# Must contain the new name from the first line
assert_file_contains "$SANDBOX/edge.md" '/edikt:adr:new' \
    "Edge cases: /edikt:adr migrated"

# Must NOT contain /edikt:adr:new:new (double migration of already-migrated)
if grep -qF '/edikt:adr:new:new' "$SANDBOX/edge.md"; then
    fail "Edge cases: no double migration" \
        "Found /edikt:adr:new:new — already-new names were re-migrated"
else
    pass "Edge cases: no double migration"
fi

# Must NOT contain /edikt:adr:new-custom (partial migration of hyphenated)
if grep -qF '/edikt:adr:new-custom' "$SANDBOX/edge.md"; then
    fail "Edge cases: /edikt:adr-custom left alone" \
        "Hyphenated custom command was mangled"
else
    pass "Edge cases: /edikt:adr-custom left alone"
fi

# Must NOT contain /edikt:adr:newenaline (partial match of adrenaline)
if grep -qF '/edikt:adr:newenaline' "$SANDBOX/edge.md"; then
    fail "Edge cases: /edikt:adrenaline left alone" \
        "Word starting with /edikt:adr was mangled"
else
    pass "Edge cases: /edikt:adrenaline left alone"
fi

# /edikt:specimen must not become /edikt:sdlc:specimen
if grep -qF '/edikt:sdlc:specimen' "$SANDBOX/edge.md"; then
    fail "Edge cases: /edikt:specimen left alone" \
        "Word starting with /edikt:spec was mangled"
else
    pass "Edge cases: /edikt:specimen left alone"
fi

# /edikt:spec-artifacts must become /edikt:sdlc:artifacts (longest prefix first)
assert_file_contains "$SANDBOX/edge.md" '/edikt:sdlc:artifacts' \
    "Edge cases: /edikt:spec-artifacts migrated to sdlc:artifacts (not sdlc:spec-artifacts)"

if grep -qF '/edikt:sdlc:spec-artifacts' "$SANDBOX/edge.md"; then
    fail "Edge cases: spec-artifacts not mangled via /edikt:spec prefix" \
        "Got /edikt:sdlc:spec-artifacts — mapping order is wrong"
else
    pass "Edge cases: spec-artifacts not mangled via /edikt:spec prefix"
fi

# ============================================================
# Test 7: File with no edikt references — no-op
# ============================================================

cat > "$SANDBOX/clean.md" <<'EOF'
---
edikt:generated: true
---
# No command references here.

Just some plain text about things.
EOF

ORIG=$(cat "$SANDBOX/clean.md")
migrate_rule "$SANDBOX/clean.md"
NEW=$(cat "$SANDBOX/clean.md")

if [ "$ORIG" = "$NEW" ]; then
    pass "File with no references: unchanged"
else
    fail "File with no references: unchanged" "File was modified for no reason"
fi

test_summary

#!/usr/bin/env bash
# Pins INV-005 / HI-4 — byte-range sentinel guard.
# Reproduces the HI-4 attack path: an Edit whose old_string is a non-sentinel
# line INSIDE a [edikt:*:start] ... [edikt:*:end] block. Pre-v0.5.0 regex
# guard would approve; v0.5.0 byte-range guard must block.

set -eu
cd "$(dirname "$0")/../.."

fail=0
tmpfile=$(mktemp)
cat >"$tmpfile" <<'EOF'
# Fixture

Some content before.

[edikt:directives:start]: #
directives:
  - rule A
  - rule B
[edikt:directives:end]: #

Some content after.
EOF

# 1. Edit of a non-sentinel line INSIDE the region → must block.
payload=$(python3 -c "import json; print(json.dumps({'tool_name':'Edit','tool_input':{'file_path':'$tmpfile','old_string':'  - rule A','new_string':'  - rule ATTACKER'}}))")
out=$(echo "$payload" | bash templates/hooks/pre-tool-use.sh 2>&1 || true)
if ! echo "$out" | grep -q '"decision":[[:space:]]*"block"'; then
    echo "[INV-005] Edit inside managed region was NOT blocked. Output: $out" >&2
    fail=1
fi

# 2. Edit OUTSIDE the region → must allow.
payload=$(python3 -c "import json; print(json.dumps({'tool_name':'Edit','tool_input':{'file_path':'$tmpfile','old_string':'Some content before.','new_string':'Some content before. MODIFIED'}}))")
out=$(echo "$payload" | bash templates/hooks/pre-tool-use.sh 2>&1 || true)
if ! echo "$out" | grep -q '"continue":[[:space:]]*true'; then
    echo "[INV-005] Edit outside managed region was blocked unexpectedly. Output: $out" >&2
    fail=1
fi

# 3. Edit inside region WITH bypass env → must allow.
out=$(EDIKT_COMPILE_IN_PROGRESS=1 echo "$payload" | EDIKT_COMPILE_IN_PROGRESS=1 bash templates/hooks/pre-tool-use.sh 2>&1 || true)
# (Re-use the in-region payload)
payload=$(python3 -c "import json; print(json.dumps({'tool_name':'Edit','tool_input':{'file_path':'$tmpfile','old_string':'  - rule A','new_string':'  - rule A-patched'}}))")
out=$(EDIKT_COMPILE_IN_PROGRESS=1 bash -c "echo '$payload' | EDIKT_COMPILE_IN_PROGRESS=1 bash templates/hooks/pre-tool-use.sh" 2>&1 || true)
if ! echo "$out" | grep -q '"continue":[[:space:]]*true'; then
    echo "[INV-005] Edit with EDIKT_COMPILE_IN_PROGRESS=1 was blocked. Output: $out" >&2
    fail=1
fi

rm -f "$tmpfile"
exit $fail

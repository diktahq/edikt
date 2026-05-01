#!/usr/bin/env bash
# edikt: WorktreeRemove hook — log the teardown.
# Idempotent: tolerates missing worktree state.
#
# Output: {"continue": true}

set -uo pipefail

# Fast opt-out (shared with worktree-create)
[ "${EDIKT_WORKTREE_SKIP:-0}" = "1" ] && { printf '{"continue": true}\n'; exit 0; }

INPUT=$(cat 2>/dev/null || echo '{}')

WORKTREE_PATH=$(printf '%s' "$INPUT" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    print(d.get("worktree_path", "") or d.get("path", ""))
except Exception:
    pass
' 2>/dev/null)

mkdir -p "$HOME/.edikt" 2>/dev/null || true
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)
# Build the event via json.dumps (INV-003) — `printf '%s'` is not JSON-escape-safe
# and a worktree path with a quote or newline would corrupt the line.
python3 - "$TS" "${WORKTREE_PATH:-unknown}" "$HOME/.edikt/events.jsonl" <<'PY' 2>/dev/null || true
import json, sys
ts, path, out = sys.argv[1:4]
with open(out, 'a', encoding='utf-8') as f:
    f.write(json.dumps({"ts": ts, "event": "worktree_remove", "worktree": path}) + "\n")
PY

printf '{"continue": true}\n'

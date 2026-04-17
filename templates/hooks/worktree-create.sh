#!/usr/bin/env bash
# edikt: WorktreeCreate hook — copy edikt governance into a new worktree.
# Idempotent: safe to run multiple times against the same worktree.
#
# Output: {"systemMessage": "..."} with the outcome, or {"continue": true}.
# Opt-out: EDIKT_WORKTREE_SKIP=1 to disable.

set -uo pipefail

# Fast opt-out
[ "${EDIKT_WORKTREE_SKIP:-0}" = "1" ] && { printf '{"continue": true}\n'; exit 0; }

INPUT=$(cat 2>/dev/null || echo '{}')

# All string values are passed through python3 json.dumps for safe emission —
# no shell interpolation of payload-derived values in JSON output.
WORKTREE_PATH=$(printf '%s' "$INPUT" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    path = d.get("worktree_path", "") or d.get("path", "") or d.get("cwd", "")
    # Reject paths with shell metacharacters or newlines — hook treats
    # them as invalid rather than trying to sanitize in shell.
    if path and all(c not in path for c in "\n\r\t"):
        print(path)
except Exception:
    pass
' 2>/dev/null)

# Validate path before any shell operations. Quote throughout. Reject symlinks
# at the worktree root — a git-managed worktree is always a real directory, so
# a symlink here indicates filesystem-level attack (INV-006, audit MED-12).
if [ -z "$WORKTREE_PATH" ] || [ ! -d "$WORKTREE_PATH" ]; then
    printf '{"continue": true}\n'
    exit 0
fi
if [ -L "$WORKTREE_PATH" ]; then
    python3 -c 'import json,sys; print(json.dumps({"systemMessage": f"edikt: worktree path {sys.argv[1]!r} is a symlink; refusing to copy governance (symlinked worktrees are not supported)."}))' "$WORKTREE_PATH"
    exit 0
fi

# Source repo's .edikt/config.yaml — walk up from $PWD to find it
SOURCE_CFG=""
D="$PWD"
while [ "$D" != "/" ]; do
    if [ -f "$D/.edikt/config.yaml" ] && [ "$D" != "$WORKTREE_PATH" ]; then
        SOURCE_CFG="$D/.edikt/config.yaml"
        break
    fi
    D=$(dirname "$D")
done

if [ -z "$SOURCE_CFG" ]; then
    printf '{"continue": true}\n'
    exit 0
fi

# Copy governance into the worktree idempotently (quoted paths throughout)
TARGET_EDIKT="$WORKTREE_PATH/.edikt"

if [ ! -d "$TARGET_EDIKT" ]; then
    mkdir -p -- "$TARGET_EDIKT" 2>/dev/null || true
    cp -- "$SOURCE_CFG" "$TARGET_EDIKT/config.yaml" 2>/dev/null || true
fi

# Log the event via python3 (no shell interpolation of $WORKTREE_PATH)
mkdir -p "$HOME/.edikt" 2>/dev/null || true
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)
python3 -c '
import json, sys
ev = {"ts": sys.argv[1], "event": "worktree_create", "worktree": sys.argv[2]}
print(json.dumps(ev))
' "$TS" "$WORKTREE_PATH" >> "$HOME/.edikt/events.jsonl" 2>/dev/null || true

# Emit user-visible message via python3 (safe escaping)
python3 -c '
import json, os, sys
path = sys.argv[1]
msg = f"📋 edikt governance copied to new worktree: {os.path.basename(path)}. Run /edikt:init if stack-specific rules need refreshing."
print(json.dumps({"systemMessage": msg}))
' "$WORKTREE_PATH"

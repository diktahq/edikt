#!/usr/bin/env bash
# edikt: WorktreeCreate hook — echo worktree path + copy governance into it.
# Idempotent: safe to run multiple times against the same worktree.
#
# Protocol (Claude Code WorktreeCreate): output the directory path to use,
# or nothing to accept the default. Governance copy is best-effort.
# Opt-out: EDIKT_WORKTREE_SKIP=1 to skip governance copy (still echoes path).

set -uo pipefail

INPUT=$(cat 2>/dev/null || echo '{}')

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# WorktreeCreate's protocol is "print the path, or print nothing to accept
# the default" — there is no JSON shape to extend without inventing an
# unverified protocol addition. stderr is the visible channel available.
if ! command -v python3 >/dev/null 2>&1; then
    echo "edikt: worktree-create — python3 not found; governance copy skipped, default path used" >&2
    exit 0
fi

# Extract path from JSON input. Try multiple key names for compatibility
# with different Claude Code versions.
WORKTREE_PATH=$(printf '%s' "$INPUT" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    path = (d.get("worktree_path") or d.get("proposed_path") or
            d.get("path") or d.get("cwd") or "")
    # Reject paths with shell metacharacters or newlines.
    if path and all(c not in path for c in "\n\r\t"):
        print(path)
except Exception:
    pass
' 2>/dev/null)

# No path extractable — output nothing; Claude Code uses its default.
if [ -z "$WORKTREE_PATH" ]; then
    exit 0
fi

# Reject symlinks at the worktree root — a git worktree is always a real dir.
if [ -L "$WORKTREE_PATH" ]; then
    exit 1
fi

# Governance copy — best-effort, only when path already exists as a directory.
# (Hook fires before git creates the worktree, so the dir may not exist yet;
# skip silently in that case.)
if [ "${EDIKT_WORKTREE_SKIP:-0}" != "1" ] && [ -d "$WORKTREE_PATH" ]; then
    SOURCE_CFG=""
    D="$PWD"
    _home="${HOME:-/nonexistent}"
    while [ "$D" != "/" ] && [ "$D" != "$_home" ]; do
        if [ -f "$D/.edikt/config.yaml" ] && [ "$D" != "$WORKTREE_PATH" ] && [ -O "$D/.edikt/config.yaml" ]; then
            SOURCE_CFG="$D/.edikt/config.yaml"
            break
        fi
        D=$(dirname "$D")
    done

    if [ -n "$SOURCE_CFG" ]; then
        TARGET_EDIKT="$WORKTREE_PATH/.edikt"
        if [ ! -d "$TARGET_EDIKT" ]; then
            mkdir -p -- "$TARGET_EDIKT" 2>/dev/null || true
            cp -- "$SOURCE_CFG" "$TARGET_EDIKT/config.yaml" 2>/dev/null || true
        fi
        # Log the event.
        mkdir -p "$HOME/.edikt" 2>/dev/null || true
        TS=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)
        python3 -c '
import json, sys
ev = {"ts": sys.argv[1], "event": "worktree_create", "worktree": sys.argv[2]}
print(json.dumps(ev))
' "$TS" "$WORKTREE_PATH" >> "$HOME/.edikt/events.jsonl" 2>/dev/null || true
    fi
fi

# Always echo the path — required by WorktreeCreate protocol.
printf '%s\n' "$WORKTREE_PATH"

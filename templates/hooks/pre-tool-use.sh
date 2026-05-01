#!/usr/bin/env bash
# edikt: PreToolUse hook (Write|Edit) — block edits that would damage
# governance sentinel blocks (INV-005); warn if project-context.md
# is missing.
#
# Output format: Claude Code hook protocol JSON
#   - {"systemMessage": "..."} for advisory warnings
#   - {"decision": "block", "reason": "..."} for sentinel-block protection
#
# INV-005: the guard checks **byte-range overlap** on the resolved file,
# not regex over old_string/new_string. An Edit whose old_string is a
# non-sentinel line inside the managed region was previously approved
# (audit HI-4) — the byte-range check closes that bypass.

INPUT=$(cat)

# Allowlisted bypasses: compile and migration legitimately edit inside
# managed regions. These env vars are set by /edikt:gov:compile and
# bin/edikt upgrade respectively. They short-circuit before both the
# sentinel guard and the advisory — neither is wanted inside compile.
if [ "${EDIKT_COMPILE_IN_PROGRESS:-0}" = "1" ] || [ "${EDIKT_MIGRATION_IN_PROGRESS:-0}" = "1" ]; then
    printf '{"continue": true}'
    exit 0
fi

# Skip the sentinel guard when there is no payload on stdin. The advisory
# block below still runs so project-setup nags fire even when the hook is
# exercised without a payload (test harness, manual invocation).
if [ -z "${INPUT:-}" ]; then
    _EDIKT_SKIP_SENTINEL_GUARD=1
fi

if [ "${_EDIKT_SKIP_SENTINEL_GUARD:-0}" != "1" ]; then
export _EDIKT_HOOK_INPUT="$INPUT"
python3 - <<'PY'
"""Byte-range sentinel guard (INV-005).

For Edit: resolve file_path on disk, locate old_string, compute the byte
range the patch would modify. If that range overlaps any
[edikt:NAME:start]: # ... [edikt:NAME:end]: # region, block unless
bypass env vars are set.

For Write: if the destination file has sentinel regions and the new
content lacks them OR modifies the managed region, block.

Bootstrap rule: if a managed region has no [edikt:NAME:sha256]: # anchor
line, treat it as unarmed. Still block edits that overlap the region
(byte-range check holds), but do NOT attempt hash verification. Compile
will seed the anchor on first run.
"""
import json
import re
import sys
from pathlib import Path


def _emit(obj: dict) -> None:
    print(json.dumps(obj))


import os as _os
try:
    payload = json.loads(_os.environ.get("_EDIKT_HOOK_INPUT", "") or "{}")
except json.JSONDecodeError:
    _emit({"continue": True})
    sys.exit(0)

tool = payload.get("tool_name", "")
if tool not in ("Edit", "Write"):
    _emit({"continue": True})
    sys.exit(0)

tool_input = payload.get("tool_input", {}) or {}
file_path = tool_input.get("file_path") or tool_input.get("path") or ""
if not file_path:
    _emit({"continue": True})
    sys.exit(0)

# Resolve on disk. If the file does not exist (Write creating a new file),
# there is no existing sentinel region to protect — allow.
path = Path(file_path)
try:
    on_disk = path.read_text(encoding="utf-8", errors="replace")
except (FileNotFoundError, IsADirectoryError):
    _emit({"continue": True})
    sys.exit(0)
except OSError:
    _emit({"continue": True})
    sys.exit(0)

# Find every [edikt:NAME:start]: # / [edikt:NAME:end]: # pair. Match the
# name to pair starts with ends so nested or multiple regions resolve
# correctly.
START_RE = re.compile(r"^\[edikt:([a-z][a-z0-9-]*)(?::start)?\]:\s*#\s*$", re.MULTILINE)
END_RE = re.compile(r"^\[edikt:([a-z][a-z0-9-]*):end\]:\s*#\s*$", re.MULTILINE)

# Build (start_byte, end_byte, name) ranges.
starts = [(m.start(), m.group(1)) for m in START_RE.finditer(on_disk)]
ends = [(m.end(), m.group(1)) for m in END_RE.finditer(on_disk)]

regions: list[tuple[int, int, str]] = []
used_ends = set()
for s_byte, s_name in starts:
    # The start marker line itself ends at the next newline; include it.
    # Find end marker with matching name at a position after s_byte.
    for idx, (e_byte, e_name) in enumerate(ends):
        if idx in used_ends:
            continue
        if e_byte <= s_byte:
            continue
        if e_name != s_name and s_name != "":
            # Special case: the legacy unnamed form [edikt:start]: # pairs
            # with [edikt:end]: # — in that case both groups return ""
            # so the match works. Named forms must match by name.
            continue
        regions.append((s_byte, e_byte, s_name))
        used_ends.add(idx)
        break

if not regions:
    _emit({"continue": True})
    sys.exit(0)

# Compute the byte range the patch would affect.
def compute_edit_range() -> tuple[int, int] | None:
    old = tool_input.get("old_string")
    if tool == "Edit" and isinstance(old, str) and old:
        idx = on_disk.find(old)
        if idx == -1:
            # old_string not found on disk — Claude Code will reject the
            # Edit itself, but the guard is being cautious: treat as "no
            # overlap" since no bytes will be modified.
            return None
        # Convert char index to byte index (read_text already decoded; we
        # assume ASCII-compatible sentinel lines, which they are by
        # construction — they're link-reference markdown).
        return (idx, idx + len(old))
    if tool == "Write":
        # Write replaces the entire file content — any managed region in
        # the existing file is affected.
        content = tool_input.get("content", "") or ""
        # If the new content preserves the managed regions byte-for-byte,
        # allow. Otherwise block.
        for s, e, _name in regions:
            existing_region = on_disk[s:e]
            if existing_region not in content:
                return (s, e)
        return None
    return None


edit_range = compute_edit_range()
if edit_range is None:
    _emit({"continue": True})
    sys.exit(0)

edit_start, edit_end = edit_range
for s, e, name in regions:
    # Overlap = ranges intersect.
    if edit_start < e and edit_end > s:
        reason = (
            f"edit would modify the edikt-managed sentinel region {name!r} in "
            f"{file_path}. The managed region is rebuilt by /edikt:gov:compile "
            f"from source artifacts (ADRs, invariants, guidelines). Edit the "
            f"source artifact and re-run compile instead of hand-editing the "
            f"compiled block. (ref: INV-005, ADR-014)"
        )
        _emit({"decision": "block", "reason": reason})
        sys.exit(0)

_emit({"continue": True})
PY
fi

# --- Project-context.md advisory ---
if [ -f '.edikt/config.yaml' ] && [ ! -f 'docs/project-context.md' ]; then
    python3 -c 'import json; print(json.dumps({"systemMessage":"⚠ edikt: docs/project-context.md not found. Run /edikt:init to complete setup."}))'
fi

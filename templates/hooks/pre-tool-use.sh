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
#
# ADR-027 narrowing: the region scan only runs on files that may carry
# edikt-managed regions: CLAUDE.md, settings.json (under $CLAUDE_HOME or
# a .claude/ ancestor), and governance artifact paths whose .md still
# carries an unfenced legacy sentinel block (migration-window allowance —
# falls out of scope automatically once `migrate sidecars` strips the
# block). Files outside this allowlist short-circuit to continue: true,
# resolving the fenced-marker false-positive class documented in
# docs/internal/decisions/HOOK-FALSE-POSITIVE-ANALYSIS.md.
#
# Per-invocation cost (Phase 7 of PLAN-sidecar-review-fixes #43):
#   each in-scope hook fire pays Python cold-start (~30–80ms on Apple
#   silicon, longer in containerized CI), plus the YAML hand-parse,
#   plus one fence walk over the candidate file. Out-of-scope files
#   short-circuit before any of that, so the steady-state cost on a
#   typical Edit/Write cadence on governance files is acceptable. The
#   cost is NOT optimizable from inside this hook without changing the
#   hook protocol or moving to a long-running daemon — both are
#   architecturally larger moves than this hook's mandate. Telemetry
#   for the cost lives in test/bench/hook-bench.sh (informational; CI
#   skips on EDIKT_SKIP_HOOK_BENCH=1).

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

# ─── ADR-027 path-scope narrowing ─────────────────────────────────────────
# Only scan files that may carry edikt-managed regions:
#   1. CLAUDE.md (basename, any directory)
#   2. settings.json under $CLAUDE_HOME or any .claude/ ancestor
#   3. governance .md whose body still carries an unfenced legacy sentinel
#      (migration-window allowance — see PLAN-sidecar-review-fixes "Out
#      of Scope (Deferred)" §3, slated for removal in v0.7.0 once every
#      project has run `migrate sidecars --apply` and no governance .md
#      can carry a real in-body sentinel anymore. The Rule-3 branch
#      becomes unreachable at that point and the entire
#      `_has_legacy_sentinel` scan can be deleted.)
# Files outside the allowlist short-circuit to continue: true. (See
# docs/internal/decisions/HOOK-FALSE-POSITIVE-ANALYSIS.md.)
def _governance_paths() -> list:
    """Resolve governance dirs from .edikt/config.yaml; fall back to defaults.

    Hardened per Phase 3 §3.3 (path-traversal) + §3.4 (YAML hand-parser):

    §3.3 — every value is rejected if, after normalization, it
      contains a ".." segment, starts with "/", or its realpath
      escapes the current working directory. Without this, a
      malicious config could redirect rule 3 of the scope check
      to "/" and re-enable the false-positive class for every
      file in the tree.

    §3.4 — the hand-parser falls back to defaults on any of these
      shapes that exceed its known-safe surface:
        - YAML multi-doc separator ('---')
        - flow-style values (paths: { decisions: ... })
        - quoted values containing colons
      Stdlib-only Python has no YAML loader; the hook avoids a
      runtime dep, so the safe-fail-closed behavior is "scan all"
      (defaults) when the parser can't trust its own output.
    """
    defaults = [
        "docs/architecture/decisions",
        "docs/architecture/invariants",
        "docs/guidelines",
    ]
    try:
        with open(".edikt/config.yaml", "r", encoding="utf-8") as fh:
            text = fh.read()
    except (OSError, FileNotFoundError):
        return defaults

    # §3.4 — quirk shapes the hand-parser cannot safely interpret.
    for line in text.splitlines():
        stripped_line = line.strip()
        if stripped_line == "---":
            return defaults
        # Flow-style detection on the paths: line itself.
        if stripped_line.startswith("paths:") and (
            "{" in stripped_line or "[" in stripped_line
        ):
            return defaults

    out = []
    in_paths = False
    for raw in text.splitlines():
        line = raw.rstrip("\r")
        if not in_paths:
            if line.startswith("paths:"):
                in_paths = True
            continue
        if line and not line.startswith((" ", "\t")):
            break
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if ":" in stripped:
            key_part, value = stripped.split(":", 1)
            value = value.strip()
            # §3.4 — reject quoted values whose interior contains a colon
            # (the hand-parser can't disambiguate from a YAML mapping).
            if (value.startswith('"') and value.endswith('"')) or (
                value.startswith("'") and value.endswith("'")
            ):
                inner = value[1:-1]
                if ":" in inner:
                    return defaults
                value = inner
            # §3.4 — flow-style value.
            if value.startswith("{") or value.startswith("["):
                return defaults
            if value:
                # §3.3 — reject traversal and absolute paths. ANY bad value
                # poisons the whole config (safe-fail-closed): the parser
                # falls back to defaults rather than scanning a partial
                # user-controlled set. This is more conservative than
                # silently dropping the bad entry, which could leave a
                # legitimate governance dir uncovered.
                if ".." in value.split("/") or value.startswith("/"):
                    return defaults
                out.append(value)
    # Empty stanza or unparseable file → defaults.
    paths = out or defaults

    # §3.3 — final realpath escape check. Resolve each candidate
    # against cwd; if ANY entry escapes, fall back to full defaults
    # (same safe-fail-closed posture as the syntactic check above).
    cwd_real = _os.path.realpath(_os.getcwd())
    safe = []
    for p in paths:
        candidate = _os.path.normpath(_os.path.join(cwd_real, p))
        try:
            cand_real = _os.path.realpath(candidate)
        except OSError:
            return defaults
        if cand_real == cwd_real or cand_real.startswith(cwd_real + _os.sep):
            safe.append(p)
        else:
            return defaults
    return safe or defaults


def _fence_prefix(stripped: str):
    """CommonMark fence detection: returns (marker_char, run_length).
    Returns ('', 0) if the line does not begin with a fence marker run
    of 3+ same chars (' ` ' or '~'). Per Phase 3 §3.2."""
    if not stripped:
        return ('', 0)
    c = stripped[0]
    if c != '`' and c != '~':
        return ('', 0)
    n = 1
    while n < len(stripped) and stripped[n] == c:
        n += 1
    if n < 3:
        return ('', 0)
    return (c, n)


_MAX_SENTINEL_SCAN_BYTES = 2 * 1024 * 1024  # 2 MiB


def _has_legacy_sentinel(path: str) -> bool:
    """True iff path contains an unfenced [edikt:directives:start]: # line.

    Fence tracking is CommonMark-conformant: the closing fence MUST use
    the same marker character as the opener AND its run length MUST be
    ≥ the opener's. Mixed-marker close lines are treated as ordinary
    content (a ``~~~`` line inside a ```` ``` ```` block does not
    toggle ``in_fence`` off). Per Phase 3 §3.2.

    Bounded read: the scan caps at _MAX_SENTINEL_SCAN_BYTES (2 MiB) so a
    multi-GB file under a configured governance dir cannot exhaust hook
    memory. Real governance artifacts are always far below the cap;
    truncating a pathological input is acceptable because the cap-region
    is a strict superset of any plausible sentinel-bearing prefix.
    """
    try:
        with open(path, "rb") as fh:
            raw = fh.read(_MAX_SENTINEL_SCAN_BYTES)
        text = raw.decode("utf-8", errors="replace")
    except (OSError, FileNotFoundError):
        return False
    in_fence = False
    opener_char = ''
    opener_len = 0
    for line in text.splitlines():
        stripped = line.lstrip()
        fence_char, fence_len = _fence_prefix(stripped)
        if not in_fence:
            if fence_len >= 3:
                in_fence = True
                opener_char = fence_char
                opener_len = fence_len
                continue
            if line.startswith("[edikt:directives:start]:"):
                return True
        else:
            if fence_len >= opener_len and fence_char == opener_char:
                in_fence = False
    return False


def _is_in_scope(fp: str) -> bool:
    try:
        real = _os.path.realpath(fp)
    except OSError:
        return True
    base = _os.path.basename(real)
    # Rule 1: CLAUDE.md anywhere.
    if base == "CLAUDE.md":
        return True
    # Rule 2: settings.json under $CLAUDE_HOME or any .claude/ ancestor.
    if base == "settings.json":
        claude_home = _os.environ.get("CLAUDE_HOME") or _os.path.join(
            _os.path.expanduser("~"), ".claude"
        )
        try:
            claude_home_real = _os.path.realpath(claude_home)
        except OSError:
            claude_home_real = claude_home
        if real == claude_home_real or real.startswith(claude_home_real + _os.sep):
            return True
        if _os.sep + ".claude" + _os.sep in real:
            return True
        return False
    # Rule 3: governance-path file with surviving legacy sentinel.
    cwd = _os.getcwd()
    for rel in _governance_paths():
        candidate = _os.path.normpath(_os.path.join(cwd, rel))
        try:
            cand_real = _os.path.realpath(candidate)
        except OSError:
            continue
        if real == cand_real or real.startswith(cand_real + _os.sep):
            if _has_legacy_sentinel(real):
                return True
    return False


if not _is_in_scope(file_path):
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

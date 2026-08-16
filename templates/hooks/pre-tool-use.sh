#!/usr/bin/env bash
# edikt: PreToolUse hook (Write|Edit) — block edits that would damage
# governance sentinel blocks; warn if project-context.md
# is missing.
#
# Output format: Claude Code hook protocol JSON
#   - {"systemMessage": "..."} for advisory warnings
#   - {"hookSpecificOutput": {"hookEventName": "PreToolUse",
#      "permissionDecision": "deny",
#      "permissionDecisionReason": "..."}} for sentinel-block protection
#      (F-076/ADR-061 — the deny channel, a third non-functional shape  edikt-guard:allow
#      found in this same file: {"decision": "block", "reason": ...} was
#      never touched by the original continue:false fix)
#
# The guard checks **byte-range overlap** on the resolved file,
# not regex over old_string/new_string. An Edit whose old_string is a
# non-sentinel line inside the managed region was previously approved
# (audit HI-4) — the byte-range check closes that bypass.
#
# Path-scope narrowing: the region scan only runs on files that may carry
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

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# Both python3 dispatch points below (the sentinel guard and the
# project-context advisory) are otherwise unconditional. Absence used to
# crash the sentinel guard with a raw "python3: command not found" (rc=127,
# opaque) and silently drop the advisory (rc=0, nothing). Checked once,
# used by both blocks: this is the INV-005 guard against damaging edits to  edikt-guard:allow
# CLAUDE.md / settings.json managed regions — per ADR-058:d05 it degrades  edikt-guard:allow
# exactly as it already fails open (allow), now with a visible reason.
if ! command -v python3 >/dev/null 2>&1; then
    _EDIKT_PYTHON3_MISSING=1
else
    _EDIKT_PYTHON3_MISSING=0
fi

# Allowlisted bypasses: compile and migration legitimately edit inside
# managed regions. These env vars are set by /edikt:gov:compile and
# bin/edikt upgrade respectively — a Go binary's own os.WriteFile, which
# never crosses PreToolUse at all, so the env var only needs to be visible
# within that same Go process. They short-circuit before both the
# sentinel guard and the advisory — neither is wanted inside compile.
#
# A tier-1 markdown command (e.g. /edikt:upgrade's CLAUDE.md content-
# currency check, ADR-067) writes through Claude Code's own Edit tool  edikt-guard:allow
# instead — a DIFFERENT write path, and the env var bypass does not
# reach it. Each Bash tool call is a fresh subshell (documented tool
# behavior: "shell state does not persist between commands"), so an
# `export EDIKT_MIGRATION_IN_PROGRESS=1` in one Bash call is invisible to
# the separate hook subprocess Claude Code spawns for a later Edit call.
# Found live: the sentinel guard denied a legitimate ADR-067 write with  edikt-guard:allow
# no env var able to reach it at all — not a hypothetical gap.
#
# A file-based signal survives across tool calls because it is real
# filesystem state, not process environment: a tier-1 command can
# `touch`/`rm` it via Bash around its own Edit calls. Project-relative,
# matching every other `.edikt/state/` convention in this codebase.
if [ "${EDIKT_COMPILE_IN_PROGRESS:-0}" = "1" ] || [ "${EDIKT_MIGRATION_IN_PROGRESS:-0}" = "1" ] || [ -f ".edikt/state/.migration-in-progress" ]; then
    printf '{"continue": true}'
    exit 0
fi

# Skip the sentinel guard when there is no payload on stdin. The advisory
# block below still runs so project-setup nags fire even when the hook is
# exercised without a payload (test harness, manual invocation).
if [ -z "${INPUT:-}" ]; then
    _EDIKT_SKIP_SENTINEL_GUARD=1
fi
if [ "${_EDIKT_PYTHON3_MISSING:-0}" = "1" ]; then
    _EDIKT_SKIP_SENTINEL_GUARD=1
fi

# (ref: INV-005)
if [ "${_EDIKT_PYTHON3_MISSING:-0}" = "1" ]; then
    printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"⚠ edikt: python3 is missing on this host — the sentinel guard could NOT check this write for damage to a managed region (CLAUDE.md, settings.json). If this edit touched one, it was NOT verified."}}\n'  edikt-guard:allow
fi

if [ "${_EDIKT_SKIP_SENTINEL_GUARD:-0}" != "1" ]; then
export _EDIKT_HOOK_INPUT="$INPUT"
python3 - <<'PY'
"""Byte-range sentinel guard.

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

# ─── Path-scope narrowing ─────────────────────────────────────────────────
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
    # Rule 3: the compiled governance tree. .claude/rules/governance/*.md and
    # .claude/rules/governance.md are the OPERATIVE governance surface — the
    # rules an agent actually reads on every task. An agent editing them
    # rewrites the constraints it is running under, which is a
    # self-modification property that regenerability does not answer: the
    # weakened rules govern every action taken before anyone recompiles.
    #
    # Legitimate writes do not pass through here. `gov compile` writes the
    # tree from the Go binary, which never crosses PreToolUse; the hook only
    # sees an agent reaching for the file directly.
    if base.endswith(".md"):
        marker = _os.sep + ".claude" + _os.sep + "rules" + _os.sep
        if marker in real and (
            _os.sep + "governance" + _os.sep in real or base == "governance.md"
        ):
            return True

    # Rule 3b: the SPEC-011 rendered surfaces. Same self-modification  edikt-guard:allow
    # property as Rule 3, on the surfaces stage 1 added:
    #
    #   .claude/rules/governance/directive-index.yaml — carries PINNED grades;
    #       `hook match` denies on a must. An agent editing it downgrades its
    #       own enforcement, and every write it makes afterwards is unblocked
    #       by a rule it wrote itself.
    #   .claude/rules/governance/manifest.yaml — the record of what compile
    #       produced. Orphan cleanup deletes what it names, so an edited
    #       manifest is a delete primitive; freshness compares against it, so
    #       an edited one can make a stale tree report clean.
    #   .edikt/topics.yaml — approved topic descriptions, which are what the
    #       host matches a task against when selecting a skill. Editing it
    #       changes which governance gets loaded at all.
    #   .claude/skills/edikt-*/SKILL.md — tier 3. For a pathless topic this is
    #       the ONLY home of its guidance.
    #
    # As with Rule 3, legitimate writes never reach here: compile writes from
    # the Go binary, which does not cross PreToolUse, and the compile-in-
    # progress escape hatch at the top of this hook covers the rest.
    _rules_marker = _os.sep + ".claude" + _os.sep + "rules" + _os.sep + "governance" + _os.sep
    if base in ("directive-index.yaml", "manifest.yaml") and _rules_marker in real:
        return True
    if base == "topics.yaml" and _os.sep + ".edikt" + _os.sep in real:
        return True
    if base == "SKILL.md":
        _skills_marker = _os.sep + ".claude" + _os.sep + "skills" + _os.sep + "edikt-"
        if _skills_marker in real:
            return True

    # Rule 4: governance-path file with surviving legacy sentinel.
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

# Find every [edikt:NAME:start]: # / [edikt:NAME:end]: # pair (or the
# legacy unnamed [edikt:start]: # / [edikt:end]: # form) and match starts
# to ends by name so nested or multiple regions resolve correctly.
#
# Two independent bugs made this dead code for CLAUDE.md's managed region
# — found live, by an Edit inside CLAUDE.md's own [edikt:start]/[edikt:end]
# region going through with no bypass env set, no deny, nothing:
#
# 1. Trailing text after the '#' on the start line. CLAUDE.md.tmpl's start
#    marker reads literally
#      [edikt:start]: # managed by edikt — do not edit this block manually
#    — a human-readable annotation on the same line, present in every
#    CLAUDE.md this template has ever produced. The prior pattern anchored
#    `\s*#\s*$`, requiring nothing but whitespace after the '#'; it never
#    matched that line. Fixed by `#(?:\s.*)?$` below — tolerates either
#    end-of-line (bare form) or a space followed by arbitrary trailing
#    text. `(?::sha256)?` anchor lines are unaffected either way —
#    `directives:sha256` has a third colon-delimited segment neither name
#    group can consume, so the required literal `]:` never matches those
#    lines regardless of what follows the '#'.
#
# 2. The legacy UNNAMED form was never actually supported, despite the
#    pairing loop below carrying a comment claiming it is ("both groups
#    return \"\" so the match works"). The old END_RE, ([a-z][a-z0-9-]*):end,
#    requires a NAME segment strictly before ':end]' — `[edikt:end]: #`
#    (no name segment at all) can never match it, full stop, independent
#    of trailing-text handling. `ends` was therefore ALWAYS empty for any
#    file using only the unnamed form (CLAUDE.md's actual shape), so
#    `regions` was always empty too — a second, structurally deeper reason
#    the guard never protected CLAUDE.md, on top of bug 1. (The old
#    START_RE's `(?::start)?` being optional also let it spuriously match
#    a bare `[edikt:end]: #` line, capturing name="end" as if it were a
#    start marker — harmless in practice only because `ends` was already
#    empty, so no bogus pairing could ever complete; worth naming so a
#    future reader does not rediscover it as a live bug.)
#
#    Fixed below by making the two forms real, distinct alternatives
#    instead of one pattern straining to cover both: `start`/`end` bare
#    literals for the legacy unnamed form (name normalized to "" to match
#    what the pairing loop already assumes), or `NAME:start`/`NAME:end`
#    for the named form.
START_RE = re.compile(r"^\[edikt:(?:start|([a-z][a-z0-9-]*):start)\]:\s*#(?:\s.*)?$", re.MULTILINE)
END_RE = re.compile(r"^\[edikt:(?:end|([a-z][a-z0-9-]*):end)\]:\s*#(?:\s.*)?$", re.MULTILINE)

# Build (start_byte, end_byte, name) ranges. group(1) is None for the
# unnamed form's literal `start`/`end` alternative — normalize to "" to
# match the pairing loop's documented legacy-form contract below.
starts = [(m.start(), m.group(1) or "") for m in START_RE.finditer(on_disk)]
ends = [(m.end(), m.group(1) or "") for m in END_RE.finditer(on_disk)]

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

# WHOLLY-GENERATED surfaces have no sentinel region to protect a range of —
# the entire file is compile output. The region scan below finds nothing in
# them and would fall through to allow, which is the wrong default here: a
# partially-managed file has an unmanaged part an agent may legitimately edit,
# and these do not.
#
# Checked AFTER the region scan so a file that somehow carries both is handled
# by the more specific rule, and so this branch cannot mask a region bug.
_WHOLLY_GENERATED_BASENAMES = ("directive-index.yaml", "manifest.yaml", "SKILL.md")
_base_now = _os.path.basename(_os.path.realpath(file_path))
_is_wholly_generated = _base_now in _WHOLLY_GENERATED_BASENAMES or (
    _base_now == "topics.yaml" and _os.sep + ".edikt" + _os.sep in _os.path.realpath(file_path)
)
if _is_wholly_generated:
    # ADR-061 — THE DENY CHANNEL.  edikt-guard:allow
    #
    # This emitted {"continue": False, "systemMessage": ..}. That does not
    # deny: the host writes the file and then ends the turn (measured 14/14
    # on production transcripts). So this guard has never protected
    # directive-index.yaml, manifest.yaml, SKILL.md or .edikt/topics.yaml —
    # the edit landed every time, leaving the tree enforcing something no
    # sidecar says, which is the exact state the message below claims to
    # prevent. INV-005 reported as enforcing throughout.  edikt-guard:allow
    #
    # permissionDecisionReason, not systemMessage: the model is the reader
    # that has to redirect the edit to the source artifact.
    _emit(
        {
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "deny",
                # (ref: INV-005)
                "permissionDecisionReason": (
                    "edikt: %s is generated by `gov compile` and must not be edited directly. "
                    "Every byte of it is compile output, so an edit here is lost on "
                    "the next compile — and until then the tree enforces something no sidecar "
                    "says. Change the source ADR/invariant/guideline and re-run "
                    "`bin/edikt gov compile`." % _base_now
                ),
            }
        }
    )
    sys.exit(0)

if not regions:
    # F-070 — settings.json is routed in-scope (Rule 2 above) but the region
    # scan above looks for a MARKDOWN sentinel line
    # (`[edikt:NAME:start]: #`), which a JSON file cannot host and remain
    # parseable JSON. `regions` is therefore ALWAYS empty for settings.json,
    # and this branch's ordinary meaning — "this file carries no managed
    # region, nothing to protect" — does not hold: it means "this file
    # structurally cannot express the region format it was routed to
    # check," which is a different claim and must not fall through to the
    # same allow. Measured live: an edit emptying the entire `deny` list in
    # `.claude/settings.json` was allowed, unimpeded, by this exact branch.
    #
    # INV-005 specifies TWO variants: markdown regions verified by  edikt-guard:allow
    # byte-range (built, above), and JSON-hosted regions verified
    # out-of-band against a `managed_hash` sidecar (not built). Until that
    # exists, the only sound default for an in-scope JSON file is refuse,
    # not allow — matching the WHOLLY_GENERATED branch's reasoning just
    # above: an unverifiable region is not a verified one.
    #
    # This is narrower than that branch on purpose: settings.json legitimately
    # carries user-owned keys outside the managed region (MCP config, extra
    # permissions), so it cannot be denied unconditionally the way a wholly-
    # generated file can. Denying only when the file is BOTH in-scope AND
    # produces zero regions is precisely the JSON case this guard cannot yet
    # verify — a markdown file with genuinely zero managed regions (an
    # artifact that opted out) still allows, unchanged.
    if _base_now == "settings.json":
        # This deny is scoped to Claude-Code-mediated Edit/Write tool calls —
        # the only surface a PreToolUse hook can see. A write issued through
        # Bash (`>`, `sed -i`, a script) carries no file_path in its payload
        # and is not intercepted here, matching the same limitation ADR-060  edikt-guard:allow
        # already codifies for the write-time injection tier: MUST NOT parse
        # shell to cover Bash writes. The message below says "have a human
        # review this edit" rather than implying the edit is now impossible —
        # it is the strongest claim this guard can honestly make.
        _emit(
            {
                "hookSpecificOutput": {
                    "hookEventName": "PreToolUse",
                    "permissionDecision": "deny",
                    # The citation stays in this comment, never in the string below
                    # — that fragment reaches the model, and this project's own
                    # INV-005 is meaningless to a user whose project numbers its
                    # own invariants independently (e.g. their INV-0007 is not this
                    # one). A user hit exactly this confusion in the field, unable
                    # to tell whose rule they'd broken or look it up.
                    # (ref: INV-005)
                    "permissionDecisionReason": (
                        "edikt: settings.json edits cannot be verified against "
                        "damage to a managed region — the JSON-hosted region "
                        "variant is not yet implemented, so this guard cannot "
                        "tell a safe edit from one that empties the deny list "
                        "or removes a hook registration. This check covers "
                        "the Edit/Write tools only — it does not intercept a "
                        "write issued through Bash. Have a human review "
                        "this edit directly, or use `/edikt:config` for "
                        "supported changes."
                    ),
                }
            }
        )
        sys.exit(0)
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
            f"compiled block."
        )
        # F-076 — ADR-061. THE DENY CHANNEL, a third time in this same file.  edikt-guard:allow
        #
        # {"decision": "block", "reason": ...} is not hookSpecificOutput.
        # permissionDecision — it is a THIRD non-functional shape, distinct
        # from continue:false (ADR-061's original finding) and never touched  edikt-guard:allow
        # by that fix, because that fix searched for `continue.*false`, not
        # `decision.*block`. This is the CORE overlap-detected deny path —
        # the actual reason INV-005's byte-range guard exists — and it sat  edikt-guard:allow
        # beside two branches in this same file that WERE already converted
        # (the WHOLLY_GENERATED branch above, and the settings.json branch
        # this session just added), each carrying an explicit comment
        # explaining why continue:false/decision:block does not deny. This
        # one was the one branch nobody re-checked.
        #
        # permissionDecisionReason, not systemMessage: the model is the
        # actor that must redirect the edit to the source artifact.
        _emit(
            {
                "hookSpecificOutput": {
                    "hookEventName": "PreToolUse",
                    "permissionDecision": "deny",
                    "permissionDecisionReason": reason,
                }
            }
        )
        sys.exit(0)

_emit({"continue": True})
PY
fi

# --- Project-context.md advisory ---
# Skipped when python3 is missing: the sentinel-guard block above already
# emitted the INV-005 degradation notice for this invocation, and this  edikt-guard:allow
# advisory is best-effort on top of it, not a second required signal.
if [ "${_EDIKT_PYTHON3_MISSING:-0}" != "1" ] && [ -f '.edikt/config.yaml' ] && [ ! -f 'docs/project-context.md' ]; then
    python3 -c 'import json; print(json.dumps({"systemMessage":"⚠ edikt: docs/project-context.md not found. Run /edikt:init to complete setup."}))'
fi

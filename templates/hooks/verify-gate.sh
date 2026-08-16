#!/usr/bin/env bash
# edikt: PreToolUse verify-gate hook — completion-claim detection (Plan B
# Phase 1).
#
# Output format: Claude Code hook protocol JSON.
#   - {"continue": true}                       — pass-through (no claim, or
#                                                 evidence-reads present, or
#                                                 bypass envelope)
#   - {"hookSpecificOutput": {                 — deny: completion-claim
#        "hookEventName": "PreToolUse",          detected without a prior
#        "permissionDecision": "deny",           verify-report Read in the
#        "permissionDecisionReason": ".."}}      same turn
#
# ADR-061: the deny shape was {"continue": false, "stopReason", "decision",  edikt-guard:allow
# "message"} until 2026-08-13. None of those keys denies anything — the host
# runs the tool and THEN kills the turn (measured 14/14 on production
# transcripts), so INV-009's gate let every completion claim reach disk while  edikt-guard:allow
# reporting as enforcing. See _deny() below.
#
# Completion-claim shapes detected:
#   1. Sidecar passes flip            — *.edikt.yaml, passes: false → true
#   2. Plan progress-row done         — PLAN-*.{md,yaml}, pending|in_progress → done
#   3. AC checkbox flip               — *.{md,yaml}, [ ] → [x] near AC-NNN / SAC-NNN
#
# Bypass envelope:
#   Tier A — EDIKT_HOOK_ACTOR ∈ {migrate, compile, upgrade} (NFKC-validated,
#            allowlist regex). Logged to .edikt/state/verify-gate.jsonl.
#   Tier B — EDIKT_DISABLE_VERIFY_GATE=1 (ad-hoc operator escape hatch).
#
# Evidence-read state lives at .edikt/state/.evidence-reads — a JSON object
# mapping verify-report id to RFC 3339 timestamp. Missing file is treated
# as {} (default deny on claim). State writes use mv -f on a temp file for
# atomicity (concurrent-session safety).
#
# JSON construction: use python3 json.dumps with untrusted
# values passed via sys.argv. No shell-string concatenation of JSON.
#
# Path-input validation: every attacker-influenceable value
# (file_path, EDIKT_HOOK_ACTOR, derived sidecar id) is NFKC-normalized and
# checked against an allowlist regex before reaching argv.

INPUT=$(cat)

# ─── AUDIT (ADR-062 §3) ─────────────────────────────────────────────────────  edikt-guard:allow
# json.dumps with values as argv. INV-003 forbids shell-concatenated JSON to a  edikt-guard:allow
# LOG FILE as absolutely as to stdout, and ADR-058:p03 forbids hand-rolling it  edikt-guard:allow
# in bash — neither carries an exemption for hook-controlled values, and an
# exemption graded by who controls the value is the kind that erodes.
#
# Guarded on python3 and never fatal: this is telemetry. ADR-062 §2's  edikt-guard:allow
# shell-only rule binds the posture resolution and the short-circuit path — the
# off-switch — not this. Where python3 is absent the record is skipped and the
# allow decision is unaffected.
_edikt_audit() {
    command -v python3 >/dev/null 2>&1 || return 0
    mkdir -p .edikt/state 2>/dev/null || true
    python3 -c '
import json, os, sys, time
rec = {
    "ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    "event": sys.argv[1],
    "posture": sys.argv[2],
}
try:
    with open(os.path.join(".edikt", "state", "verify-gate.jsonl"), "a",
              encoding="utf-8") as fh:
        fh.write(json.dumps(rec) + "\n")
except OSError:
    pass
' "$1" "${EDIKT_GATE_POSTURE:-unknown}" 2>/dev/null || true
}

# ─── POSTURE (ADR-038 §5b, implemented by ADR-062) ──────────────────────────  edikt-guard:allow
#
# Resolved in SHELL, before any python3 call, by ADR-062 §2. python3 is an  edikt-guard:allow
# undeclared runtime dependency (ADR-058), absent from stock macOS since 12.3  edikt-guard:allow
# and from most slim containers. Were the posture read to need it, then on such
# a machine `disabled` and `educate` could not short-circuit — the hook would
# reach a bare python3 call and exit 127 — and the two postures whose whole
# purpose is turning the gate OFF would be the two requiring the thing that is
# missing. The off-switch must not depend on it.
#
# Unrecognised values resolve to `warn` and say so. They deliberately do NOT
# resolve to `block`: silently upgrading an operator's typo to the
# highest-friction posture is a worse failure than running the default ADR-038  edikt-guard:allow
# already names as safe.
_edikt_posture() {
    _raw=""
    if [ -f .edikt/config.yaml ]; then
        _raw=$(awk '
            /^features:/          { inblk = 1; next }
            inblk && /^[^ \t#]/   { inblk = 0 }
            inblk && $1 == "evidence-gate:" { print $2; exit }
        ' .edikt/config.yaml 2>/dev/null)
    fi
    _raw=$(printf '%s' "$_raw" | tr -d "\"' \t\r" | tr '[:upper:]' '[:lower:]')
    case "$_raw" in
        block|warn|educate|disabled) printf '%s' "$_raw" ;;
        "") printf 'warn' ;;
        *)  echo "verify-gate: unrecognised features.evidence-gate '$_raw'; using warn" >&2
            printf 'warn' ;;
    esac
}
EDIKT_GATE_POSTURE=$(_edikt_posture)

# `disabled` — short-circuited unconditionally, before any state I/O, with no
# audit record. ADR-038 §5b: identical to EDIKT_DISABLE_VERIFY_GATE=1. The  edikt-guard:allow
# emission is a constant literal, not a construction: there is nothing
# interpolated for INV-003 to be about, and `_allow` in inject-directives-pre.sh  edikt-guard:allow
# is the same shape.
if [ "$EDIKT_GATE_POSTURE" = "disabled" ]; then
    printf '{"continue": true}\n'
    exit 0
fi

# `educate` — the hook does not fire: it does not inspect the write, does not
# read evidence state, and does not influence whether the write proceeds
# (ADR-038:d34 as amended by ADR-062 §3). It records that it declined, which is  edikt-guard:allow
# the only thing distinguishing it from `disabled` — without that record the
# two postures are byte-identical and the enum's four-way distinction is
# unfalsifiable.
if [ "$EDIKT_GATE_POSTURE" = "educate" ]; then
    _edikt_audit posture.educate
    printf '{"continue": true}\n'
    exit 0
fi

# ─── Tier B bypass: ad-hoc operator escape hatch ────────────────────────────
# Checked before any state I/O so the gate is fully short-circuited. A constant
# literal rather than json.dumps, for the ADR-062 §2 reason above: an escape  edikt-guard:allow
# hatch that needs python3 is unavailable on exactly the machines whose users
# most need it.
if [ "${EDIKT_DISABLE_VERIFY_GATE:-0}" = "1" ]; then
    printf '{"continue": true}\n'
    exit 0
fi

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# Everything below this line needs python3, including the Tier A actor
# validation two blocks down — which, on a python3-less host, doesn't error;
# `if python3 -c '...'` just evaluates false, silently falls through, and
# the hook then crashes at the final dispatch with a raw "python3: command
# not found" (rc=127). That crash is a real signal but an opaque one: no
# named cause, no remedy, and (per ADR-058:d05) it must not become fail-  edikt-guard:allow
# CLOSED just because it becomes clearer — this INV-009 gate degrades  edikt-guard:allow
# exactly as it already does (allow) with a visible reason instead of a
# raw shell error.
if ! command -v python3 >/dev/null 2>&1; then
    printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"⚠ edikt: python3 is missing on this host — the evidence-read gate could NOT evaluate this write. If it was a completion claim (sidecar passes, plan phase, AC checkbox), it was NOT checked for fresh verification evidence."}}\n'  edikt-guard:allow
    exit 0
fi

# ─── Tier A bypass: allowlisted-actor signal ────────────────────────────────
# EDIKT_HOOK_ACTOR is normalized NFKC + casefold + strip and matched against
# a closed allowlist regex. Unknown values are treated as no-actor (gate
# applies). Validation lives inside python3 so we never interpolate the raw
# env var into a shell command.
if [ -n "${EDIKT_HOOK_ACTOR:-}" ]; then
    if python3 -c '
import os, re, sys, unicodedata
raw = os.environ.get("EDIKT_HOOK_ACTOR", "")
normalized = unicodedata.normalize("NFKC", raw).strip().casefold()
allowlist = re.compile(r"^(migrate|compile|upgrade)$")
sys.exit(0 if allowlist.match(normalized) else 1)
' 2>/dev/null; then
        # Log to audit trail and pass through.
        mkdir -p .edikt/state 2>/dev/null || true
        python3 -c '
import json, os, sys, time, unicodedata
raw = os.environ.get("EDIKT_HOOK_ACTOR", "")
actor = unicodedata.normalize("NFKC", raw).strip().casefold()
entry = {
    "ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    "event": "bypass.actor",
    "actor": actor,
}
log_path = os.path.join(".edikt", "state", "verify-gate.jsonl")
try:
    with open(log_path, "a", encoding="utf-8") as fh:
        fh.write(json.dumps(entry) + "\n")
except OSError:
    pass
print(json.dumps({"continue": True}))
'
        exit 0
    fi
fi

# ─── Gate logic ─────────────────────────────────────────────────────────────
# Pass the raw payload to python3 via env var (never shell-interpolated).
# All claim-detection regex work, state-file I/O, and JSON emission happens
# inside the heredoc so each step is stdlib-Python only.
export _EDIKT_VERIFY_GATE_INPUT="$INPUT"
export _EDIKT_GATE_POSTURE="$EDIKT_GATE_POSTURE"
python3 - <<'PY'
"""verify-gate.sh body — see header comment in the parent script.

Phase 1: detect the three completion-claim shapes,
consult the evidence-reads state file, allow or deny accordingly.
"""
import json
import os
import re
import sys
import tempfile
import time
import unicodedata
from pathlib import Path


def _emit(obj: dict) -> None:
    print(json.dumps(obj))


POSTURE = os.environ.get("_EDIKT_GATE_POSTURE", "") or "warn"


def _audit(event: str) -> None:
    """Append one hook-controlled record to the verify-gate audit trail.

    json.dumps, not string building (INV-003 applies to log files as much as  edikt-guard:allow
    to stdout). Never fatal: telemetry must not decide the write.
    """
    try:
        state = Path(".edikt") / "state"
        state.mkdir(parents=True, exist_ok=True)
        with (state / "verify-gate.jsonl").open("a", encoding="utf-8") as fh:
            fh.write(json.dumps({
                "ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "event": event,
                "posture": POSTURE,
            }) + "\n")
    except OSError:
        pass


def _warn(message: str) -> None:
    """`warn` posture: detect, record, allow — ADR-038 §5b.  edikt-guard:allow

    ADR-062 §4 — THE WARN CHANNEL. ADR-038 §5b said `systemMessage`. ADR-061 §1  edikt-guard:allow
    ruled that `systemMessage` renders to the user's screen only and MUST NOT
    carry text whose intended reader is the model. The actor that must revise a
    completion claim is the model, so a warning delivered on `systemMessage`
    could not shape the behaviour warn posture exists to shape — and §5b's
    ramp-to-`block` criterion would have been counting warnings the agent never
    received. `additionalContext` is the allow path's model-facing channel.

    ADR-061 §2 declared every ADR-038 directive other than the deny shape  edikt-guard:allow
    unchanged. That declaration was wrong; this is one of the two it missed.
    """
    try:
        state = Path(".edikt") / "state"
        state.mkdir(parents=True, exist_ok=True)
        # ADR-062 §7: the ledger ADR-038 §5b's doctor-driven ramp consumes.  edikt-guard:allow
        # Without a producer the ramp counts an empty file forever.
        with (state / "evidence-gate-warns.jsonl").open("a", encoding="utf-8") as fh:
            fh.write(json.dumps({
                "ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "event": "gate.warn",
                "reason": message,
            }) + "\n")
    except OSError:
        pass
    _audit("gate.warn")
    _emit({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "additionalContext": (
                message + " This is a WARNING: `features.evidence-gate` is "
                "`warn`, so the write is allowed. Under `block` it would be "
                "refused."
            ),
        }
    })


def _deny(message: str) -> None:
    # POSTURE (ADR-038 §5b / ADR-062). Routed here rather than at each call  edikt-guard:allow
    # site so that every would-deny is covered by the dial — including the
    # unparseable-payload deny below. A posture that silently still blocks in
    # some cases is not a dial.
    if POSTURE == "warn":
        _warn(message)
        return
    _audit("gate.deny")
    # ADR-061 — THE DENY CHANNEL.  edikt-guard:allow
    #
    # This hook denied with {"continue": False, "stopReason": ..,
    # "decision": "deny", "message": ..} as ADR-038 mandated. None of those  edikt-guard:allow
    # four keys prevents the tool call. Measured 14/14 across 118 production
    # transcripts: every stopped PreToolUse Write/Edit had a SUCCESS
    # tool_result for its own toolUseID, and none had no result. The write
    # LANDED and the turn was killed afterwards.
    #
    # So INV-009's completion-claim gate never once prevented a claim being  edikt-guard:allow
    # written — it let the claim reach disk and then ended the turn, leaving
    # the next turn to start from the already-committed claim. The gate
    # reported as enforcing throughout.
    #
    # permissionDecisionReason, not systemMessage: the MODEL has to revise the
    # write, and systemMessage renders only to the user's screen.
    #
    # Structured serializer, untrusted values as data (INV-003/INV-004).  edikt-guard:allow
    _emit({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": message,
        }
    })


# ─── Parse the hook payload ─────────────────────────────────────────────────
raw = os.environ.get("_EDIKT_VERIFY_GATE_INPUT", "") or ""
try:
    payload = json.loads(raw) if raw else None
except json.JSONDecodeError:
    payload = None
if payload is None:
    # Missing or malformed payload: DENY (INV-011). This hook is
    # registered under matcher "Write|Edit" only, so an unparseable
    # payload means a write we cannot inspect — waving it through made
    # a broken payload the cheapest way past the completion-claim gate.
    # (ref: INV-011 — absence of inspectable input is not a pass)
    _deny(
        "verify-gate: could not parse the hook payload, so the "
        "completion-claim check cannot run — denying the write. "
        "If you are the human operator, the bypass procedure is in "
        "edikt's hook documentation."
    )
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


# ─── NFKC-normalize the path before any allowlist comparison ──────
def _normalize_path(value: str) -> str:
    """NFKC + strip; casefold is NOT applied here because filesystem paths
    on case-sensitive filesystems (Linux) are case-sensitive. We only
    casefold the verify-report id derived from basename."""
    return unicodedata.normalize("NFKC", value).strip()


norm_path = _normalize_path(file_path)
if not norm_path:
    _emit({"continue": True})
    sys.exit(0)

# F-050: this hook's basename-based claim detection MUST resolve symlinks
# first, matching pre-tool-use.sh's sibling scope check (_is_in_scope,
# os.path.realpath before os.path.basename) — without it, a symlink whose
# literal name doesn't match a completion-claim shape but whose TARGET is a
# real .edikt.yaml/plan/spec sidecar lets an Edit through `basename`  edikt-guard:allow
# comparisons undetected, defeating INV-009's evidence gate. realpath can  edikt-guard:allow
# raise OSError on a genuinely broken link or a permission edge case; on
# that failure the literal (unresolved) path is used rather than denying —
# consistent with this hook's own fail-open contract (ADR-060) for  edikt-guard:allow
# conditions it cannot resolve, not a silent skip.
try:
    real_path = os.path.realpath(norm_path)
except OSError:
    real_path = norm_path

basename = os.path.basename(real_path)


# ─── Extract the new content (Edit: new_string; Write: content) ────────────
if tool == "Edit":
    new_text = tool_input.get("new_string", "") or ""
    old_text = tool_input.get("old_string", "") or ""
else:  # Write
    new_text = tool_input.get("content", "") or ""
    old_text = ""

if not isinstance(new_text, str):
    new_text = str(new_text)
if not isinstance(old_text, str):
    old_text = str(old_text)


# ─── Completion-claim detection ────────────────────────────────────────────
#
# Shape 1: sidecar passes flip (path ends with .edikt.yaml AND new_string
# contains `passes: true` while old_string contained `passes: false` OR
# Write content sets passes: true).
#
# Shape 2: plan progress-row done (path matches PLAN-*.{md,yaml} AND diff
# transitions pending|in_progress → done).
#
# Shape 3: AC / SAC checkbox flip (path matches *.{md,yaml} AND diff
# transitions [ ] → [x] adjacent to AC-NNN or SAC-NNN).
#
# Detection runs in order; first match wins (it determines the report id
# the gate consults).

PASSES_TRUE_RE = re.compile(r"^\s*passes\s*:\s*true\b", re.MULTILINE)
PASSES_FALSE_RE = re.compile(r"^\s*passes\s*:\s*false\b", re.MULTILINE)

PLAN_DONE_RE = re.compile(
    r"\b(pending|in[_-]progress)\b\s*(?:->|→|=>)\s*\bdone\b",
    re.IGNORECASE,
)
# F-053 — THE BARE WORD "done" IS NOT A COMPLETION CLAIM.
#
# This was `\bdone\b`, matched anywhere in the content. ADR-038 specifies the  edikt-guard:allow
# shape as "a markdown table cell ... transitions ... to `done` in a column
# whose header matches `(?i)^status$`" — a TABLE CELL, not a word. The loose
# form denied any plan write containing the word at all, including prose like
# "revisit once the migration is done" and, absurdly, "Nothing is done yet;
# all phases pending".
#
# That was survivable while `continue:false` let the write land anyway. ADR-061  edikt-guard:allow
# made the deny real, which converted a harmless false positive into a blocked
# write — so bounding it is part of that fix, not separate from it.
#
# Matches a table cell whose entire content is `done`, with optional bold or
# backtick emphasis: `| done |`, `| **done** |`, `| `done` |`.
PLAN_DONE_SIMPLE_RE = re.compile(
    r"\|\s*(?:\*\*|`|_)*\s*done\s*(?:\*\*|`|_)*\s*\|",
    re.IGNORECASE,
)
PLAN_PENDING_RE = re.compile(r"\b(pending|in[_-]progress)\b", re.IGNORECASE)

# AC checkbox flip: [ ] → [x] near AC-NNN / SAC-NNN.
AC_BOX_DONE_RE = re.compile(
    r"\[\s*x\s*\][^\n]{0,80}?\b(?:S?AC)-\d+|\b(?:S?AC)-\d+[^\n]{0,80}?\[\s*x\s*\]",
    re.IGNORECASE,
)
AC_BOX_OPEN_RE = re.compile(
    r"\[\s*\][^\n]{0,80}?\b(?:S?AC)-\d+|\b(?:S?AC)-\d+[^\n]{0,80}?\[\s*\]",
    re.IGNORECASE,
)


def _is_sidecar_passes_flip() -> bool:
    if not basename.endswith(".edikt.yaml"):
        return False
    # Write: any new content containing passes: true is a claim.
    if tool == "Write":
        return bool(PASSES_TRUE_RE.search(new_text))
    # Edit: new contains passes: true AND old did NOT (or contained passes:
    # false). This catches the false→true transition cleanly. A new_string
    # that contains passes: true but old also contained passes: true is
    # idempotent and is allowed (e.g. comment-only edits adjacent to the
    # key).
    new_has_true = bool(PASSES_TRUE_RE.search(new_text))
    old_has_true = bool(PASSES_TRUE_RE.search(old_text))
    return new_has_true and not old_has_true


def _is_plan_progress_done() -> bool:
    # Match PLAN-*.md / PLAN-*.yaml (case-insensitive on basename per
    # Portability — plan filenames are canonical mixed-case but
    # filesystems on macOS/Windows are case-insensitive).
    base_lower = basename.casefold()
    if not (base_lower.startswith("plan-") and (
        base_lower.endswith(".md") or base_lower.endswith(".yaml")
    )):
        return False
    if tool == "Write":
        return bool(PLAN_DONE_SIMPLE_RE.search(new_text))
    # Edit: new contains 'done', old contained 'pending'/'in_progress'.
    new_has_done = bool(PLAN_DONE_SIMPLE_RE.search(new_text))
    old_has_pending = bool(PLAN_PENDING_RE.search(old_text))
    new_has_pending = bool(PLAN_PENDING_RE.search(new_text))
    # Transition: new has 'done', old had 'pending'/'in_progress',
    # AND new no longer has the pending marker (i.e. the row flipped).
    return new_has_done and old_has_pending and not new_has_pending


def _is_ac_checkbox_flip() -> bool:
    base_lower = basename.casefold()
    if not (base_lower.endswith(".md") or base_lower.endswith(".yaml")):
        return False
    new_has_checked = bool(AC_BOX_DONE_RE.search(new_text))
    old_has_checked = bool(AC_BOX_DONE_RE.search(old_text))
    return new_has_checked and not old_has_checked


claim_kind = None
if _is_sidecar_passes_flip():
    claim_kind = "sidecar"
elif _is_plan_progress_done():
    claim_kind = "plan"
elif _is_ac_checkbox_flip():
    claim_kind = "ac"

if claim_kind is None:
    _emit({"continue": True})
    sys.exit(0)


# ─── Derive verify-report id from the file path ────────────────────────────
# Use the basename sans extension as the id. Validate against an
# allowlist regex after NFKC + casefold so Unicode lookalikes cannot bypass
# the lookup.
def _report_id_from_basename(name: str) -> str:
    # Strip extensions: .edikt.yaml → "", .md / .yaml → "".
    stem = name
    if stem.endswith(".edikt.yaml"):
        stem = stem[: -len(".edikt.yaml")]
    elif stem.endswith(".yaml"):
        stem = stem[: -len(".yaml")]
    elif stem.endswith(".md"):
        stem = stem[: -len(".md")]
    return unicodedata.normalize("NFKC", stem).strip().casefold()


report_id = _report_id_from_basename(basename)
# Allowlist: alphanumerics, dot, dash, underscore. Empty or unsafe ids
# short-circuit to deny (we can't reason about the matching evidence read).
ID_ALLOWLIST_RE = re.compile(r"^[a-z0-9][a-z0-9._-]{0,128}$")
if not ID_ALLOWLIST_RE.match(report_id):
    # Defensive: refuse to look up an id we can't safely use as a JSON key.
    # Treat as deny so a path-injection attempt doesn't sail past the gate.
    report_id = ""


# ─── Read .edikt/state/.evidence-reads ──────────────────────────────────────
STATE_PATH = Path(".edikt") / "state" / ".evidence-reads"
evidence_reads: dict = {}
try:
    if STATE_PATH.exists():
        with STATE_PATH.open("r", encoding="utf-8") as fh:
            evidence_reads = json.load(fh)
        if not isinstance(evidence_reads, dict):
            evidence_reads = {}
except (OSError, json.JSONDecodeError):
    # Malformed: log to stderr (NOT systemMessage), treat as {}.
    print("verify-gate: .edikt/state/.evidence-reads malformed; treating as empty", file=sys.stderr)
    evidence_reads = {}


# ─── Allow / Deny ───────────────────────────────────────────────────────────
def _hint_for(kind: str) -> str:
    if kind == "sidecar":
        return (
            "Run `bin/edikt verify gov " + (report_id or "<id>") +
            "` and Read the resulting `.edikt/state/verify/" +
            (report_id or "<id>") + ".json` before flipping `passes: true`."
        )
    if kind == "plan":
        return (
            "Run `bin/edikt verify " + (report_id or "<plan-id>") +
            " --phase <N>` and Read the resulting report before marking the row `done`."
        )
    # ac
    return (
        "Run `bin/edikt verify spec " + (report_id or "<SPEC-id>") +
        "` and Read the resulting report before checking the AC."
    )


def _latest_report_verdict(rid: str):
    """Locate the newest verify report matching rid under
    .edikt/state/verify/ and return (path, passing). INV-011: a recorded
    evidence READ is only evidence when the report it points at PASSES —
    missing report, unparseable report, or summary showing failures all
    return passing=False (fail-closed).
    """
    vdir = Path(".edikt") / "state" / "verify"
    m = re.search(r"(adr|inv|gl|spec|prd|plan)-\d+", rid)
    needle = m.group(0) if m else rid
    # Boundary-anchored match (audit 2026-08-07 #1): a bare substring let a
    # passing report for gov-ADR-0010 satisfy the gate for ADR-001. The
    # needle must not be followed or preceded by another digit.
    needle_re = re.compile(r"(^|[^0-9a-z])" + re.escape(needle) + r"($|[^0-9])")
    candidates = []
    try:
        for p in vdir.glob("*.json"):
            if needle_re.search(p.name.casefold()):
                candidates.append(p)
    except OSError:
        return None, False
    if not candidates:
        return None, False
    try:
        latest = max(candidates, key=lambda p: p.stat().st_mtime)
    except OSError:
        return None, False
    try:
        with latest.open("r", encoding="utf-8") as fh:
            rep = json.load(fh)
        summary = rep.get("summary") or {}
        failed = summary.get("failed")
        timeout = summary.get("timeout")
        passed = summary.get("passed")
        # Audit 2026-08-07 #2: an all-skipped run (passed=0) is not
        # completion evidence — at least one criterion must have actually
        # passed for the report to unlock a claim (INV-011).
        passing = failed == 0 and timeout == 0 and isinstance(passed, int) and passed >= 1
        return latest, passing
    except (OSError, json.JSONDecodeError):
        return latest, False


if report_id and evidence_reads.get(report_id):
    report_path, report_passing = _latest_report_verdict(report_id)
    if not report_passing:
        if report_path is None:
            detail = (
                "no verify report matching this artifact exists under "
                ".edikt/state/verify/"
            )
        else:
            detail = (
                "the matching verify report (" + report_path.name +
                ") is FAILING or unreadable"
            )
        # (ref: INV-011 — a failing report is not completion evidence)
        _deny(
            "verify-gate: completion-claim on " + basename +
            " — an evidence read was recorded, but " + detail + ". "
            "Re-run the verify until it passes, Read the fresh "
            "report, then make the claim."
        )
        sys.exit(0)
    # Allow: consume the read (reset to null) and write back atomically.
    evidence_reads[report_id] = None
    try:
        STATE_PATH.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        # Atomic write via NamedTemporaryFile + mv -f equivalent (os.replace).
        with tempfile.NamedTemporaryFile(
            mode="w",
            encoding="utf-8",
            dir=str(STATE_PATH.parent),
            delete=False,
            prefix=".evidence-reads.",
        ) as tmp:
            json.dump(evidence_reads, tmp)
            tmp_path = tmp.name
        os.replace(tmp_path, str(STATE_PATH))
    except OSError as e:
        # Fail open: state-file write failure must not
        # brick the developer workflow. Stderr-only audit.
        print(
            "verify-gate: state-file write failed; gate temporarily disabled "
            "until disk frees (" + str(e) + ")",
            file=sys.stderr,
        )
    _emit({"continue": True})
    sys.exit(0)


# Deny.
hint = _hint_for(claim_kind)
message = (
    "verify-gate: completion-claim detected on " + basename +
    " without a fresh verify-report read in the same turn. " + hint
)
_deny(message)
PY

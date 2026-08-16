#!/usr/bin/env bash
# edikt: Stop hook — detect signals in the last assistant response and surface them
# as a non-blocking systemMessage shown to the user.
#
# Uses regex pattern matching — no API key required.
# Outputs {"systemMessage": "..."} for signals, {"continue": true} when clean.

set -uo pipefail

# Only run in edikt projects
if [ ! -f '.edikt/config.yaml' ]; then exit 0; fi
if grep -q 'signal-detection: false' .edikt/config.yaml 2>/dev/null; then exit 0; fi

# Suppress stop-hook signals while /edikt:upgrade is mid-flight. The
# slash command writes .edikt/state/upgrade-in-progress at the start
# of its orchestration and removes it on exit (success or failure).
# During upgrade the user is BY DEFINITION fixing the very drift the
# hook reports — repeating "⚠ Some artifacts have stale sidecars" 30+
# times during a 10-minute resync is noise, and the migration log's
# "decision" / "ADR" wording also trips the ADR-candidate detector
# into a stream of false positives. The marker-file gate
# short-circuits both signal blocks below to a clean
# {"continue": true}.
if [ -f '.edikt/state/upgrade-in-progress' ]; then
    printf '{"continue": true}'
    exit 0
fi

# Anchor the project root to the realpath of cwd at script start, then
# pass it to the Python heredocs as EDIKT_PROJECT_ROOT. The drift-detect
# block writes .edikt/state/stale-sidecars.log; without an explicit anchor,
# a relative path resolves against whatever cwd Claude Code reports at
# the moment of the Stop event — which can drift mid-session if Claude
# emits CwdChanged. Capturing once at script entry pins the path.
EDIKT_PROJECT_ROOT="$(pwd -P)"
export EDIKT_PROJECT_ROOT

# Prevent infinite loops — stop_hook_active means we're already in a continuation
INPUT=$(cat)

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# Every extraction below (the dedup key hash, STOP_HOOK_ACTIVE, LAST_MSG)
# needs python3. Absence used to fail each one silently via `|| echo "..."`
# fallbacks that happened to be safe defaults, all the way down to
# `if [ -z "$LAST_MSG" ]; then exit 0; fi` — rc=0, zero bytes, no stderr.
# A real adr-candidate/doc-gap/completion-claim signal in the transcript
# vanished with nothing anywhere to say so. Checked once, up front, rather
# than letting the hook limp through several silent sub-failures.
if ! command -v python3 >/dev/null 2>&1; then
    echo "edikt: stop-hook — python3 not found; signal detection skipped for this turn" >&2
    printf '{"continue": true, "systemMessage": "⚠ edikt: python3 is missing — signal detection (ADR-candidate, doc-gap, completion-claim) was skipped for this turn."}\n'
    exit 0
fi

# ── Duplicate-invocation guard (bok-services field bug #4) ──────────────────
# The same payload reaches this hook twice when user-level AND project-level
# settings both register it — Claude Code merges scopes and fires both, so
# every emitted line printed exactly twice. First invocation records a
# payload-hash token under .edikt/state/.hook-dedup/; an identical payload
# within the TTL exits silently. EDIKT_DISABLE_HOOK_DEDUP=1 bypasses.
if [ "${EDIKT_DISABLE_HOOK_DEDUP:-0}" != "1" ] && [ -d ".edikt" ]; then
  _EDIKT_DEDUP_DIR=".edikt/state/.hook-dedup"
  mkdir -p "$_EDIKT_DEDUP_DIR" 2>/dev/null || true
  _EDIKT_DEDUP_KEY=$(printf '%s' "stop-hook:$INPUT" | python3 -c 'import hashlib,sys; print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest()[:40])' 2>/dev/null || echo "")
  if [ -n "$_EDIKT_DEDUP_KEY" ] && [ -d "$_EDIKT_DEDUP_DIR" ]; then
    _EDIKT_DEDUP_F="$_EDIKT_DEDUP_DIR/$_EDIKT_DEDUP_KEY"
    if [ -f "$_EDIKT_DEDUP_F" ]; then
      # Token age in seconds, computed in python3 rather than by branching
      # on stat(1).
      #
      # The previous form was `stat -f %m F || stat -c %Y F || echo 0`. On
      # BSD, -f is the FORMAT flag and it works. On GNU, -f means FILESYSTEM
      # status, %m is not a valid filesystem specifier, so it prints "?" and
      # EXITS 0 — which short-circuits the || chain before the GNU branch is
      # ever reached. The age became junk, the comparison never fired, and
      # THE DUPLICATE-INVOCATION GUARD HAS NEVER WORKED ON LINUX. It is the
      # fix for a field bug where every hook line printed twice, so on every
      # Linux host that bug was never actually fixed.
      #
      # A fallback whose first branch cannot fail is not a fallback. python3
      # is already required by these hooks (JSON and hashing above), so this
      # removes the platform branch instead of correcting its order.
      _EDIKT_AGE=$(python3 -c 'import os,sys,time
try:
    print(int(time.time() - os.path.getmtime(sys.argv[1])))
except Exception:
    print(-1)' "$_EDIKT_DEDUP_F" 2>/dev/null || echo -1)
      # Non-numeric means the age is UNKNOWN, never "fresh": suppressing on
      # an unreadable token would silence a real emission (INV-013).
      case "$_EDIKT_AGE" in ''|*[!0-9-]*) _EDIKT_AGE=-1 ;; esac
      if [ "$_EDIKT_AGE" -ge 0 ] && [ "$_EDIKT_AGE" -lt 15 ]; then
        exit 0
      fi
    fi
    : > "$_EDIKT_DEDUP_F" 2>/dev/null || true
    # Opportunistic prune of tokens older than 5 minutes.
    find "$_EDIKT_DEDUP_DIR" -type f -mmin +5 -delete 2>/dev/null || true
  fi
fi
STOP_HOOK_ACTIVE=$(echo "$INPUT" | python3 -c "
import json, sys
d = json.load(sys.stdin)
print('true' if d.get('stop_hook_active') else 'false')
" 2>/dev/null || echo "false")

if [ "$STOP_HOOK_ACTIVE" = "true" ]; then exit 0; fi

# Extract the last assistant message
LAST_MSG=$(echo "$INPUT" | python3 -c "
import json, sys
d = json.load(sys.stdin)
print(d.get('last_assistant_message', '').strip())
" 2>/dev/null || echo "")

if [ -z "$LAST_MSG" ]; then exit 0; fi

# ─── Signal detection (regex-based, no API key required) ──────────────────────

# Legacy free-text channel. ADR-050 moved every producer to TYPED_KIND below;
# nothing appends here any more. Kept only so the session log can carry
# pre-ADR-050 entries if a future producer needs one, and read by the logging
# loop near the end of this file — which iterated it, found zero entries, and
# wrote nothing for a full day after ADR-050 landed. See the typed-finding
# logging block for why the loop no longer depends on this array.
SIGNALS=()
# Typed-emission channel (ADR-050): parallel arrays; each entry is
# kind / subject / evidence / ask. Subjects derived from LAST_MSG are
# attacker-influenceable and are sanitized (allowlist chars, capped)
# BEFORE entering the arrays; the renderer treats them as data only.
TYPED_KIND=()
TYPED_SUBJ=()
TYPED_EVID=()
TYPED_ASK=()

_sanitize_subject() {
    printf '%s' "$1" | tr -cd 'A-Za-z0-9 ._/-' | cut -c1-80
}

# ARCHITECTURE: explicit trade-off language or "chose X over Y" patterns.
# ADR-050: the subject is MANDATORY — an adr-candidate that cannot name
# the detected decision does not emit. G0 (already captured) checks ADR /
# INV titles, compiled sidecar signals, and spec provenance before firing.
if echo "$LAST_MSG" | grep -qiE \
    'chose .+ over |trade.?off|architectural (decision|constraint|choice)|going forward .*(all|every|must)|hard (constraint|rule|requirement)|ADR|decision record'; then
    _RAW_SUBJECT=$(echo "$LAST_MSG" | grep -ioE 'chose [a-z0-9_-]+ over [a-z0-9_-]+( for [a-z0-9 _-]{0,30})?|trade.?off[a-z0-9 :_-]{0,40}' | head -1)
    ADR_SUBJECT=$(_sanitize_subject "$_RAW_SUBJECT")
    if [ -n "$ADR_SUBJECT" ]; then
        _G0_HIT=false
        _BASE_G0=$(grep '^base:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"' || echo "docs")
        [ -z "$_BASE_G0" ] && _BASE_G0="docs"
        for term in $(printf '%s' "$ADR_SUBJECT" | tr '[:upper:]' '[:lower:]' | tr -s ' ' '\n' | grep -vE '^(chose|over|for|the|a|an|to|is|was|of|and|trade|tradeoff|trade-off)$'); do
            # G0a: ADR / INV title lines.
            if grep -rilqF -- "$term" "$_BASE_G0/decisions" "$_BASE_G0/architecture/decisions" "$_BASE_G0/architecture/invariants" 2>/dev/null; then
                if head -1 $(grep -rilF -- "$term" "$_BASE_G0/decisions" "$_BASE_G0/architecture/decisions" "$_BASE_G0/architecture/invariants" 2>/dev/null | head -3) 2>/dev/null | grep -iqF -- "$term"; then
                    _G0_HIT=true; break
                fi
            fi
            # G0b: compiled sidecar signals.
            if grep -rqiF -- "- $term" --include='*.edikt.yaml' "$_BASE_G0" 2>/dev/null; then
                _G0_HIT=true; break
            fi
            # G0c: spec-SR provenance.
            if grep -rqiF -- "$term" --include='*.yaml' "$_BASE_G0/product/specs" 2>/dev/null && grep -rliF -- "$term" --include='*.yaml' "$_BASE_G0/product/specs" 2>/dev/null | xargs grep -lqi 'provenance\|source:' 2>/dev/null; then
                _G0_HIT=true; break
            fi
        done
        if [ "$_G0_HIT" = false ]; then
            TYPED_KIND+=("adr-candidate")
            TYPED_SUBJ+=("$ADR_SUBJECT")
            TYPED_EVID+=("decision language detected in the last response")
            TYPED_ASK+=("run /edikt:adr:new — the intake gates (GL-001) will classify it")
        fi
    fi
fi

# DOC GAP: new HTTP routes or env vars added.
# Per audit HI-5: we detect presence-of-signal only and emit a STATIC suggestion.
# The matched substring is never embedded in the signal text — an attacker-controlled
# file containing "POST /admin/delete-everything" cannot influence the suggestion's
# wording, which would otherwise bias the user toward capturing attacker-framed
# work via /edikt:docs:review.
NEW_ROUTES=$(echo "$LAST_MSG" | grep -oiE '(POST|GET|PUT|DELETE|PATCH) /[a-zA-Z0-9/_:.-]+' | head -1)
NEW_ENV=$(echo "$LAST_MSG" | grep -oE '(added|new|required|Added|New|Required).{0,30}[A-Z][A-Z0-9_]{3,}[A-Z0-9]' | grep -v 'ADR\|ARCH\|HTTP\|API\|JSON\|HTML\|CSS' | head -1)

if [ -n "$NEW_ROUTES" ]; then
    TYPED_KIND+=("doc-gap")
    TYPED_SUBJ+=("$(_sanitize_subject "$NEW_ROUTES")")
    TYPED_EVID+=("new HTTP route referenced")
    TYPED_ASK+=("consider /edikt:docs:review to check documentation")
elif [ -n "$NEW_ENV" ]; then
    TYPED_KIND+=("doc-gap")
    TYPED_SUBJ+=("$(_sanitize_subject "$NEW_ENV")")
    TYPED_EVID+=("new environment variable referenced")
    TYPED_ASK+=("consider /edikt:docs:review to check documentation")
fi

# SECURITY: auth/payments/PII/crypto was the central focus
if echo "$LAST_MSG" | grep -qiE \
    '(JWT|OAuth|PKCE|auth[a-z]*|payment|PII|encrypt|decrypt|secret|signing key|private key|bearer token|bcrypt|password hash)'; then
    # Only flag if it's a substantive change (multiple security terms or central to the response)
    SEC_COUNT=$(echo "$LAST_MSG" | grep -ioE '(JWT|OAuth|PKCE|auth[a-z]*|payment|PII|encrypt|decrypt|secret|signing key|private key|bearer token|bcrypt|password hash)' | wc -l | tr -d ' ')
    if [ "$SEC_COUNT" -ge 2 ]; then
        TYPED_KIND+=("security-audit")
        TYPED_SUBJ+=("security-sensitive change ($SEC_COUNT terms)")
        TYPED_EVID+=("auth/payments/PII/crypto central to the last response")
        TYPED_ASK+=("run /edikt:sdlc:audit before shipping")
    fi
fi

# ─── Sidecar drift detection ─────────────────────────────────────────────────
# Walks the configured artifact dirs, parses each <artifact>.edikt.yaml, and
# checks whether every directive's source_excerpt.quote still appears at its
# declared line range in the parent .md. If any quote is missing, the sidecar
# is "stale" — Claude likely edited the prose without regenerating the sidecar.
#
# Output contract:
#   - The systemMessage we emit carries a FIXED template plus the cardinality
#     of stale artifacts (an int — not attacker-influenceable text). Filenames
#     and excerpts are NEVER interpolated into Claude-facing channels.
#   - The full artifact-ID list is written to .edikt/state/stale-sidecars.log
#     for /edikt:gov:compile to consume out-of-band.
#
# Soft-degrade: if PyYAML is unavailable, the drift check is skipped silently.
# Existing signals still emit; we never block the stop event on this check.
# Sidecar staleness: ONE PREDICATE. This block used to carry a ~90-line
# Python reimplementation of the drift check. It diverged from the canonical
# one in internal/sidecar/drift.go — which tolerates whitespace and line-shift
# via normalized matching — and reported ADR-026 and ADR-040 stale when every
# anchor was in fact present and `gov compile --check` said 0. Two false
# positives, standing, in the channel the user reads on every Stop.
#
# Worse, they were ACKed. A held ack accumulated a genuine finding (INV-005,
# whose .md really had changed) into the same bundle, so a true positive was
# suppressed by a bundle created for false ones.
#
# The canonical check runs in ~45ms, so the speed argument for a second
# implementation never held. Exit code + one JSON field, per the tier-2
# consumption contract.
#
# INV-013: if the binary is unavailable the hook says the check did not run.
# It does NOT fall through to "0 stale" — claiming nothing is stale from
# having looked at nothing is the defect this session is named after.
STALE_COUNT=0
STALE_IDS=""
STALE_UNCHECKED=0
# Discovery chain, most-specific first. A consumer that does not vendor a
# binary still has a working install at $EDIKT_ROOT/bin/edikt (default
# ~/.edikt), and that directory is frequently NOT on a hook's PATH — hooks
# run with a minimal environment. Searching only PATH and the project cost
# those consumers staleness checking entirely and reported UNCHECKED, which
# was honest but needlessly blind.
#
#   1. project-local .edikt/bin/edikt — the canonical project-mode marker;
#      `install.sh` itself defines project-mode installed as this path
#      existing (install.sh:225), nothing else. Unambiguous: no content
#      other than edikt's own installer ever creates this exact path, so
#      checking it first is safe for every consumer, always.
#   2. project-local bin/edikt        — edikt-dev's own dogfooding/dev-build
#      convention, meaningful ONLY inside this repo. Deliberately checked
#      SECOND, not first: `bin/edikt` is an ordinary, ambiguous relative
#      path — any downstream Go project that happens to build its own
#      unrelated binary into `bin/` and name it `edikt` would have that
#      binary shadow their real install if this rung won. `.edikt/bin/edikt`
#      cannot collide that way, so it must resolve before the ambiguous
#      convention that exists only for this repo's own dev loop.
#   3. $EDIKT_ROOT/bin/edikt          — the normal global install
#   4. PATH                           — whatever the environment offers
#   5. none                           → UNCHECKED (never a silent "0 stale")
_edikt_bin=""
_edikt_root="${EDIKT_ROOT:-${EDIKT_HOME:-$HOME/.edikt}}"
if [ -x "$EDIKT_PROJECT_ROOT/.edikt/bin/edikt" ]; then
    _edikt_bin="$EDIKT_PROJECT_ROOT/.edikt/bin/edikt"
elif [ -x "$EDIKT_PROJECT_ROOT/bin/edikt" ]; then
    _edikt_bin="$EDIKT_PROJECT_ROOT/bin/edikt"
elif [ -x "$_edikt_root/bin/edikt" ]; then
    _edikt_bin="$_edikt_root/bin/edikt"
elif command -v edikt >/dev/null 2>&1; then
    _edikt_bin="edikt"
fi

if [ -z "$_edikt_bin" ]; then
    STALE_UNCHECKED=1
else
    _stale_json=$(cd "$EDIKT_PROJECT_ROOT" && EDIKT_VERIFY_TRUST=1 EDIKT_SKIP_VERSION_GATE=1 \
        "$_edikt_bin" gov compile --check --json 2>/dev/null) || true
    if [ -z "$_stale_json" ]; then
        STALE_UNCHECKED=1
    else
        _parsed=$(printf '%s' "$_stale_json" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    phase_a = d["phase_a"]
except Exception:
    print("UNCHECKED"); raise SystemExit
# An ABSENT stale_ids key is not an empty one. An older installed binary
# predates the field, and `.get(..., [])` would turn "this binary cannot tell
# you" into "nothing is stale" — a passing measurement built on no
# observation, inside the fix for that very defect (INV-013).
if "stale_ids" not in phase_a:
    print("UNCHECKED"); raise SystemExit
ids = phase_a["stale_ids"]
if not isinstance(ids, list):
    print("UNCHECKED"); raise SystemExit
print(str(len(ids)) + "|" + ",".join(str(i) for i in ids[:6]))
' 2>/dev/null || echo "UNCHECKED")
        if [ "$_parsed" = "UNCHECKED" ] || [ -z "$_parsed" ]; then
            STALE_UNCHECKED=1
        else
            STALE_COUNT="${_parsed%%|*}"
            STALE_IDS="${_parsed#*|}"
        fi
    fi
fi

# Derived state carries its provenance, or it is regenerated. A reader must be
# able to tell "N stale sidecars" from "an N-entry file nobody updated".
_stale_log="$EDIKT_PROJECT_ROOT/.edikt/state/stale-sidecars.log"
if [ "$STALE_UNCHECKED" = "1" ]; then
    rm -f "$_stale_log" 2>/dev/null || true
elif [ "${STALE_COUNT:-0}" -gt 0 ] 2>/dev/null; then
    mkdir -p "$(dirname "$_stale_log")" 2>/dev/null || true
    {
        printf '# generated by stop-hook.sh from `edikt gov compile --check --json`\n'
        printf '# generated_at: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
        printf '# corpus_head: %s\n' "$(cd "$EDIKT_PROJECT_ROOT" && git rev-parse --short HEAD 2>/dev/null || echo unknown)"
        printf '%s\n' "$STALE_IDS" | tr ',' '\n'
    } > "$_stale_log" 2>/dev/null || true
else
    rm -f "$_stale_log" 2>/dev/null || true
fi

# STALE_COUNT / STALE_IDS are set directly above; keep the shape guards.
case "${STALE_COUNT:-0}" in
    ''|*[!0-9]*) STALE_COUNT=0 ;;
esac
case "$STALE_IDS" in
    *[!A-Za-z0-9,._-]*) STALE_IDS="" ;;  # allowlist before it reaches the message
esac

# Only report "not checked" when there was something TO check.
#
# A project with no sidecars has no staleness to determine, so announcing that
# staleness went unchecked is noise on every Stop — and noise is how a channel
# stops being read. INV-013 requires that a control not report a pass it did
# not earn; it does not require announcing non-coverage of a subject that does
# not exist. "I could not check" is worth saying; "there was nothing to check"
# is not.
_edikt_has_sidecars=0
if [ -d "$EDIKT_PROJECT_ROOT/docs" ] \
   && find "$EDIKT_PROJECT_ROOT/docs" -name '*.edikt.yaml' -print -quit 2>/dev/null | grep -q .; then
    _edikt_has_sidecars=1
fi

if [ "$STALE_UNCHECKED" = "1" ] && [ "$_edikt_has_sidecars" = "1" ]; then
    # INV-013: the check did not run, so nothing is known about staleness.
    # Reporting silence here would be indistinguishable from "0 stale" — a
    # passing claim built on no observation, which is the whole point of the
    # invariant. Say the check did not run and why.
    TYPED_KIND+=("sidecar-stale-unchecked")
    TYPED_SUBJ+=("sidecar staleness")
    TYPED_EVID+=("not checked — the edikt binary is unavailable, so no staleness determination was made")
    TYPED_ASK+=("install the binary (see README → Install) to restore the check; this is not a report that sidecars are current")
elif [ "$STALE_COUNT" -gt 0 ]; then
    TYPED_KIND+=("sidecar-stale")
    TYPED_SUBJ+=("${STALE_IDS:-$STALE_COUNT artifacts}")
    TYPED_EVID+=("sidecar anchors no longer match the prose")
    TYPED_ASK+=("run /edikt:gov:compile to resync, or hold with: edikt hook ack <fingerprint> --until <event> --why <reason>")
fi

# ─── Completion-claim without evidence ───────────────────────────────────────
# When the agent emits a "✓ Done", "all green", "Phase N complete" style claim
# during an in-progress plan phase, surface a soft reminder to run the
# verify gate. The message is a fixed template plus a count and a plan-id
# substring validated against an allowlist regex (never the raw message text).
# Output stays advisory — systemMessage, never decision: block.
#
# Activation predicate (all three must hold):
#   1. quality-gates is enabled (default true; check config explicitly)
#   2. LAST_MSG matches at least one completion pattern (regex below)
#   3. At least one plan file has a phase row with status: in-progress

QUALITY_GATES_ON=1
if grep -q 'quality-gates: false' .edikt/config.yaml 2>/dev/null; then
    QUALITY_GATES_ON=0
fi

if [ "$QUALITY_GATES_ON" = "1" ]; then
    # Case-insensitive completion-claim regex. The patterns are short
    # phrases the agent uses when it believes work is done. We do NOT
    # capture the matched substring — only the boolean "did it match".
    if echo "$LAST_MSG" | grep -qiE \
        '(^|[^a-z])(✓|✅|done!|all (tests pass|green)|build clean|perfect|looks good|should (work|be fine)|phase [0-9]+[a-z]? complete|complete!)([^a-z]|$)'; then
        # Walk paths.plans (or fallback defaults) for any PLAN-*.md with
        # an in-progress row in the progress table. Stdlib only; output is
        # the count + the first matching plan id (used in the message).
        IN_PROGRESS_INFO=$(python3 - <<'PYEOF'
import os, re, sys
from pathlib import Path

# Hardened paths parser (stdlib-only, mirrors the §3.3/§3.4 hardening
# already in this file for the drift check). Falls back to the default
# when the config doesn't declare a plans dir.
plans_dir = "docs/internal/plans"
try:
    with open(".edikt/config.yaml", "r", encoding="utf-8") as fh:
        text = fh.read()
    in_paths = False
    for raw in text.splitlines():
        line = raw.rstrip("\r")
        if not in_paths:
            if line.startswith("paths:"):
                in_paths = True
            continue
        if line and not line.startswith((" ", "\t")):
            break
        s = line.strip()
        if not s or s.startswith("#"):
            continue
        if s.startswith("plans:"):
            v = s.split(":", 1)[1].strip()
            if v.startswith("\"") and v.endswith("\""):
                v = v[1:-1]
            elif v.startswith("'") and v.endswith("'"):
                v = v[1:-1]
            # §3.3 traversal / absolute guard.
            if ".." in v.split("/") or v.startswith("/"):
                v = "docs/internal/plans"
            plans_dir = v
            break
except (OSError, FileNotFoundError):
    pass

# Allowlisted plan-id charset: alnum, hyphen, dot, underscore.
# The id flows into the systemMessage so it MUST be validated against
# an allowlist before interpolation.
ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$")

# Progress-table rows look like:
#   | 1 | in-progress | 0/5 | 2026-05-21 |
# Match a row whose status cell is exactly "in-progress" (or "in_progress").
ROW_RE = re.compile(r"^\|\s*([0-9]+[a-zA-Z]?)\s*\|\s*in[\s_-]?progress\b", re.IGNORECASE)

count = 0
first_plan = ""
first_phase = ""
try:
    entries = sorted(os.listdir(plans_dir))
except OSError:
    print("0||")
    sys.exit(0)
for name in entries:
    if not name.startswith("PLAN-") or not name.endswith(".md"):
        continue
    plan_id = name[len("PLAN-"):-len(".md")]
    if not ID_RE.match(plan_id):
        continue
    try:
        with open(os.path.join(plans_dir, name), "r", encoding="utf-8") as fh:
            body = fh.read()
    except OSError:
        continue
    for line in body.splitlines():
        m = ROW_RE.match(line)
        if not m:
            continue
        phase = m.group(1)
        if not first_plan:
            first_plan = plan_id
            first_phase = phase
        count += 1
        break
print(f"{count}|{first_plan}|{first_phase}")
PYEOF
)
        IN_PROG_COUNT=$(echo "$IN_PROGRESS_INFO" | awk -F'|' '{print $1}')
        IN_PROG_PLAN=$(echo "$IN_PROGRESS_INFO" | awk -F'|' '{print $2}')
        IN_PROG_PHASE=$(echo "$IN_PROGRESS_INFO" | awk -F'|' '{print $3}')

        # Defence-in-depth: validate the parsed values against the same
        # allowlist regex python used. A hand-tampered config can't
        # smuggle text into the systemMessage.
        case "${IN_PROG_COUNT:-0}" in
            ''|*[!0-9]*) IN_PROG_COUNT=0 ;;
        esac
        case "$IN_PROG_PLAN" in
            ''|*[!A-Za-z0-9._-]*) IN_PROG_PLAN="" ;;
        esac
        case "$IN_PROG_PHASE" in
            ''|*[!A-Za-z0-9]*) IN_PROG_PHASE="" ;;
        esac

        if [ "$IN_PROG_COUNT" -gt 0 ] && [ -n "$IN_PROG_PLAN" ] && [ -n "$IN_PROG_PHASE" ]; then
            TYPED_KIND+=("completion-claim")
            TYPED_SUBJ+=("$IN_PROG_PLAN phase $IN_PROG_PHASE")
            TYPED_EVID+=("completion phrase during an in-progress plan phase")
            TYPED_ASK+=("run: bin/edikt verify $IN_PROG_PLAN --phase $IN_PROG_PHASE before declaring done")
        fi
    fi
fi

# ─── Output: ledger-aware typed renderer (ADR-050) ────────────────────────────
# Each typed record is fingerprinted; first firing surfaces in full and is
# appended to the ledger; a known fingerprint collapses to a count line that
# STILL NAMES the item; an acked fingerprint renders as a held line with the
# reason available. Held items never vanish.

if [ ${#TYPED_KIND[@]} -eq 0 ] && [ ${#SIGNALS[@]} -eq 0 ]; then
    echo '{"continue": true}'
    exit 0
fi

_TYPED_ARGS=()
i=0
while [ "$i" -lt "${#TYPED_KIND[@]}" ]; do
    _TYPED_ARGS+=("${TYPED_KIND[$i]}"$'\x1f'"${TYPED_SUBJ[$i]}"$'\x1f'"${TYPED_EVID[$i]}"$'\x1f'"${TYPED_ASK[$i]}")
    i=$((i+1))
done

# Record every signal in the session log that /edikt:status reads.
#
# This loop used to iterate SIGNALS only. ADR-050 moved every producer to the
# typed channel and removed the SIGNALS+= appends, but left this loop reading
# the now-permanently-empty array: the hook kept emitting systemMessages while
# writing nothing to the log, and /edikt:status silently reported no signal
# activity for any session. Nothing failed — the loop ran zero times and the
# `|| true` swallowed the rest. Iterating BOTH channels is what keeps the log
# a record of what fired rather than a record of one obsolete array.
#
# The kind is written verbatim and lowercase (`security-audit`, `doc-gap`, …)
# so consumers grep a stable machine token rather than prose wording.
# SUBJECT is already sanitized upstream (allowlist chars, capped) — it is
# attacker-influenceable and must not re-enter this file unsanitized.
LOG_FILE="$HOME/.edikt/session-signals.log"
mkdir -p "$HOME/.edikt" 2>/dev/null || true
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)
for SIGNAL in "${SIGNALS[@]+"${SIGNALS[@]}"}"; do
    echo "${TIMESTAMP} ${SIGNAL}" >> "$LOG_FILE" 2>/dev/null || true
done
i=0
while [ "$i" -lt "${#TYPED_KIND[@]}" ]; do
    echo "${TIMESTAMP} ${TYPED_KIND[$i]} ${TYPED_SUBJ[$i]}" >> "$LOG_FILE" 2>/dev/null || true
    i=$((i+1))
done

python3 - "${_TYPED_ARGS[@]+"${_TYPED_ARGS[@]}"}" <<'PYEOF'
import hashlib, json, os, subprocess, sys, time
from datetime import date

ICONS = {"adr-candidate": "💡", "doc-gap": "📄", "security-audit": "🔒",
         "sidecar-stale": "⚠", "completion-claim": "⚠"}

root = os.environ.get("EDIKT_PROJECT_ROOT", ".")
state = os.path.join(root, ".edikt", "state")
ledger_path = os.path.join(state, "hook-ledger.jsonl")
acks_path = os.path.join(state, "hook-acks.json")

known = set()
try:
    with open(ledger_path, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue
            if rec.get("fingerprint"):
                known.add(rec["fingerprint"])
except OSError:
    pass

acks = {}
try:
    with open(acks_path, encoding="utf-8") as fh:
        acks = json.load(fh)
    if not isinstance(acks, dict):
        acks = {}
except (OSError, json.JSONDecodeError):
    acks = {}


def ack_active(entry):
    """Expiry evaluation. Unknown shapes count as ACTIVE (visible hold is
    safer than a silent re-fire storm while the operator investigates)."""
    until = str(entry.get("until", ""))
    if until.startswith("commit-touching:"):
        path = until[len("commit-touching:"):]
        recorded = entry.get("head", "")
        try:
            out = subprocess.run(["git", "log", "-1", "--format=%H", "--", path],
                                 capture_output=True, text=True, timeout=5)
            current = out.stdout.strip()
        except Exception:
            return True
        return current == recorded or not current
    if until == "compile-clean":
        return True  # cleared by unack or a clean compile flow
    try:
        return date.fromisoformat(until[:10]) >= date.today()
    except ValueError:
        return True


# F-082a: kinds that report a NULL RESULT (a check that could not run) rather
# than a FINDING (a check that ran and found a problem) never collapse to the
# terse known-held line, even after a human declines to explicitly ack them.
# INV-013/GL-002:d25 require "I could not check" to be treated as information
# and emitted, every time — the same standing as "0 stale" would have if
# staleness had genuinely been measured and found absent. Collapsing an
# unacked null result to a count line asks the reader to justify dismissing
# something that isn't a finding to dismiss: the correct response is
# mechanical (install/rebuild the binary), not a judgment call, so gating it
# behind `edikt hook ack ... --why` cheapens what --why means for emissions
# that ARE genuine findings. A human who deliberately wants this quiet still
# can — the explicit ack path (the branch above) is untouched — this only
# removes the SILENT, no-ack-required collapse on a second firing.
NO_COLLAPSE_KINDS = {"sidecar-stale-unchecked"}

fresh, held_lines, ledger_appends = [], [], []
for arg in sys.argv[1:]:
    parts = arg.split("\x1f")
    if len(parts) != 4:
        continue
    kind, subj, evid, ask = parts
    fp = hashlib.sha256(f"{kind}|{subj}|{evid}".encode()).hexdigest()[:16]
    icon = ICONS.get(kind, "•")
    ack = acks.get(fp)
    if ack and ack_active(ack):
        held_lines.append(f"⏸ held: {kind} {subj} [{fp}] (ack: {str(ack.get('why',''))[:60]})")
        ledger_appends.append({"ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                              "event": "held", "kind": kind, "subject": subj,
                              "fingerprint": fp})
    elif fp in known and kind not in NO_COLLAPSE_KINDS:
        # F-082: this branch used to drop `ask` on the floor, so a signal
        # that recurs across Stop firings (the binary staying absent, the
        # sidecar staying stale) surfaced its remediation exactly once and
        # every firing after that offered only suppression — the reader's
        # sole reachable action for an unacknowledged, unresolved condition
        # was to silence it, not fix it. `ask` is part of the same typed
        # record ADR-050 defines for exactly this purpose; the collapsed
        # line renders it now, same as the first-firing (fresh) line does.
        held_lines.append(f"⏸ 1 known item held: {kind} {subj} [{fp}] — {ask} (ack with --why to silence details)")
    else:
        fresh.append(f"{icon} {kind}: {subj} — {evid}. {ask} [{fp}]")
        ledger_appends.append({"ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                              "event": "surfaced", "kind": kind, "subject": subj,
                              "evidence": evid, "fingerprint": fp})

try:
    os.makedirs(state, exist_ok=True)
    with open(ledger_path, "a", encoding="utf-8") as fh:
        for rec in ledger_appends:
            fh.write(json.dumps(rec) + "\n")
except OSError:
    pass

lines = fresh + held_lines
if not lines:
    print(json.dumps({"continue": True}))
else:
    print(json.dumps({"systemMessage": "\n".join(lines)}))
PYEOF

#!/usr/bin/env bash
# PostToolUse — ADVISORY-grade write-touch injection, delivered as context.
#
# The other half of the E1 split. Advisory directives do not justify blocking a
# write, so they arrive after it as `additionalContext`: present when the agent
# next reasons about this file, costing nothing when it does not.
#
# MUST-GRADE IS EXCLUDED HERE, AND THE EXCLUSION IS THE POINT
#
# A MUST that also appeared in advisory context would be delivered twice, and
# the weaker delivery would be the one the agent saw most often — the E1 result
# says that is how a hard rule becomes background noise. It would also mean the
# bounce could be satisfied by reading the same text in a channel that never
# blocks anything.
#
# So this shim asks the matcher for `--grade advisory` and renders nothing
# else. The filter is applied in the binary, not here, so the two shims cannot
# drift apart in what they consider MUST.
set -uo pipefail

PAYLOAD=$(cat)

if [ "${EDIKT_COMPILE_IN_PROGRESS:-0}" = "1" ]; then
	printf '{"continue": true}\n'
	exit 0
fi

_quiet() {
	printf '{"continue": true}\n'
	exit 0
}

command -v python3 >/dev/null 2>&1 || {
	# F-074/F-075 loud-failure bar: this used to allow via a BARE
	# {"continue": true} — the exact "silent _allow/_quiet" the bar exists to
	# end. Mirrors inject-directives-pre.sh's ADR-060:d07 reasoning: fail  edikt-guard:allow
	# open, but never silently.
	echo "edikt inject-post: python3 unavailable; advisory context skipped" >&2
	printf '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"⚠ edikt: python3 is missing on this host — advisory-grade write-time directive context was not delivered for this write."}}\n'
	exit 0
}

# NUL-framed, mirroring inject-directives-pre.sh (F-041). The space-joined
# `read` word-split any path containing a space, so this shim silently dropped
# advisory context for the same class of user the pre shim dropped enforcement
# for. Quieter than the MUST-grade bypass, identical cause.
#
# NUL is the only delimiter that cannot occur in a path, and process
# substitution is required because `$(...)` strips NUL.
#
# ── ONE python3, NOT TWO (F-057) ─────────────────────────────────────────────
#
# Mirrors inject-directives-pre.sh's merge: this used to be two separate
# `python3 -c` invocations re-parsing the same $PAYLOAD for two independent
# field groups. One interpreter now emits all six from a single parse; the
# THIRD invocation below (processing `hook match`'s own output) still cannot
# merge in, since it runs after and depends on the binary's result.
{
	IFS= read -r -d '' TOOL
	IFS= read -r -d '' FILE_PATH
	IFS= read -r -d '' SESSION_ID
	IFS= read -r -d '' ACTOR
	IFS= read -r -d '' AGENT_ID
	IFS= read -r -d '' AGENT_TYPE
} < <(printf '%s' "$PAYLOAD" | python3 -c '
import json, sys


def emit(*vals):
    sys.stdout.write("\0".join(vals) + "\0")


try:
    d = json.load(sys.stdin)
except Exception:
    emit("-", "-", "-", "unknown", "-", "-")
    raise SystemExit(0)

ti = d.get("tool_input") or {}
fp = ti.get("file_path") or ti.get("path") or ""
tool = d.get("tool_name") or "-"
sess = d.get("session_id") or "-"

# ABSENT is not the same as EMPTY. No agent_id key at all means the main
# session: a RESOLVED identity that may legitimately share the parent dedup
# marker. An agent_id present but blank means the host told us something we
# could not read, which is UNKNOWN, and unknown must DELIVER rather than
# suppress. Collapsing the two let a blank id inherit parent suppression, which
# the subagent-injection gate caught.
# NOTE: no apostrophes below. This block is embedded in a single-quoted shell
# string, and one apostrophe silently terminated it, leaving ACTOR=unknown for
# every payload and every write allowed.
if "agent_id" not in d:
    actor, agent_id, agent_type = "parent", "parent", (d.get("agent_type") or "-")
else:
    aid = str(d.get("agent_id") or "").strip()
    at  = str(d.get("agent_type") or "").strip()
    if aid:
        actor, agent_id, agent_type = "subagent", aid, (at or "-")
    else:
        actor, agent_id, agent_type = "unknown", "", (at or "-")

emit(tool, (fp or "-"), sess, actor, agent_id, agent_type)
' 2>/dev/null)
[ -n "${ACTOR:-}" ] || ACTOR="unknown"

case "${TOOL:-}" in
Write | Edit | MultiEdit | NotebookEdit) ;;
*) _quiet ;;
esac
[ -n "${FILE_PATH:-}" ] && [ "$FILE_PATH" != "-" ] || _quiet

# EDIKT_BIN resolution ladder — mirrors inject-directives-pre.sh's fix and
# stop-hook.sh's/phase-end-detector.sh's established pattern. Same defect
# class: a bare PATH lookup never resolves in a `--project` install, so this
# hook silently suppressed advisory context on every write, in every
# project-scoped install.
#
#   1. project-local .edikt/bin/edikt — the canonical project-mode marker
#      (install.sh:225 defines project-mode as this path existing).
#   2. project-local bin/edikt        — edikt-dev's own dogfooding convention.
#   3. $EDIKT_ROOT/bin/edikt          — the normal global install
#   4. PATH                           — whatever the environment offers
#   5. none                           → loud failure, not a silent _quiet
EDIKT_BIN=""
if [ -x "${PWD}/.edikt/bin/edikt" ]; then
	EDIKT_BIN="${PWD}/.edikt/bin/edikt"
elif [ -x "${PWD}/bin/edikt" ]; then
	EDIKT_BIN="${PWD}/bin/edikt"
elif [ -x "${EDIKT_ROOT:-${EDIKT_HOME:-$HOME/.edikt}}/bin/edikt" ]; then
	EDIKT_BIN="${EDIKT_ROOT:-${EDIKT_HOME:-$HOME/.edikt}}/bin/edikt"
elif command -v edikt >/dev/null 2>&1; then
	EDIKT_BIN="edikt"
fi
if [ -z "$EDIKT_BIN" ]; then
	echo "edikt inject-post: edikt binary not found; advisory context SUPPRESSED for $FILE_PATH" >&2
	printf '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"⚠ edikt: the edikt binary could not be resolved (checked .edikt/bin/edikt, bin/edikt, the global install root, and PATH) — advisory-grade write-time directive context was not delivered for this write. Run /edikt:doctor to diagnose."}}\n'
	exit 0
fi

# ── RECEIVING CONTEXT ───────────────────────────────────────────────────────
#
# Dedup keys on the context that RECEIVES the injection, never on a session
# that spans several. A subagent that never saw the parent's bounce cannot
# inherit its suppression: the whole meaning of dedup is "this reader already
# got the message", and measurement showed the subagent demonstrably had not —
# parent bounces, marker lands, subagent inherits the key and receives nothing.
#
# The payload carries `agent_id` (per dispatch) and `agent_type`. An earlier
# version of this resolver read ENVIRONMENT variables instead and concluded the
# host exposed no per-dispatch identity; it exposes both, one layer up.
#
# UNKNOWN MEANS DELIVER. When identity cannot be resolved the shim passes no
# context and the binary refuses to suppress. Over-deliver rather than
# under-deliver: an extra bounce costs one regenerated write and is visible; a
# silent suppression costs the enforcement claim and is not.
#
# (ACTOR/AGENT_ID/AGENT_TYPE are now populated by the single parse above.)

MATCH=$("$EDIKT_BIN" hook match --path "$FILE_PATH" --grade advisory --session "$SESSION_ID" \
	--shim post --json \
	--actor "$ACTOR" --agent-type "$AGENT_TYPE" --context "$AGENT_ID" 2>/dev/null) || _quiet
[ -n "$MATCH" ] || _quiet

printf '%s' "$MATCH" | python3 -c '
import json, sys

try:
    m = json.load(sys.stdin)
except Exception:
    print(json.dumps({"continue": True}))
    raise SystemExit(0)

entries = m.get("entries") or []
if not entries:
    print(json.dumps({"continue": True}))
    raise SystemExit(0)

lines = ["- %s (%s)" % (e.get("text", ""), e.get("id", "")) for e in entries]
reminders = []
for e in entries:
    for r in e.get("reminders") or []:
        if r not in reminders:
            reminders.append(r)

body = "edikt: advisory governance for %s\n\n%s" % (
    m.get("normalized_path", ""), "\n".join(lines))
if reminders:
    body += "\n\nBefore you act:\n" + "\n".join("- %s" % r for r in reminders)

print(json.dumps({
    "continue": True,
    "hookSpecificOutput": {
        "hookEventName": "PostToolUse",
        "additionalContext": body,
    },
}))
'

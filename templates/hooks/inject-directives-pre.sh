#!/usr/bin/env bash
# PreToolUse — MUST-grade write-touch injection, delivered as a BOUNCE.
#
# THE E1 SPLIT
#
# BRAIN-005 E1 measured two delivery shapes against the same directives.  edikt-guard:allow
# Advisory text appended after the fact was read and largely ignored; a DENY
# that names the rule and forces a retry was complied with. So the two grades
# get different channels and this shim carries only one of them:
#
#   MUST-grade      -> here, as a deny. The write does not happen; the agent is
#                      told which directive it would have violated and retries.
#   advisory-grade  -> PostToolUse additionalContext (inject-directives-post.sh).
#                      Advisory rules do not justify blocking a write.
#
# Mixing them would waste the bounce's force on advice, and an agent that gets
# denied for a suggestion learns the gate is noise.
#
# EXACTLY ONE BOUNCE PER SESSION PER DIRECTIVE SET
#
# Denying the same directive on every subsequent write is a loop the agent
# cannot escape, and the rational response to an inescapable gate is to route
# around it. `--dedup` makes the first write bounce and the retry proceed.
#
# FAIL-OPEN, NEVER SILENT
#
# Every early return here lets the write through. That is deliberate: blocking
# an editor because governance is broken is worse than missing an injection.
# But the binary journals its outcome on every call, so a suppressed chain is
# reportable (`edikt hook report`) rather than invisible. The one case this
# shim cannot journal is the binary being ABSENT — nothing to journal with —
# which is why that case emits a stderr line and why `doctor` probes for the
# chain independently.
#
# INV-003: every JSON value is built by python3's json.dumps with untrusted  edikt-guard:allow
# values passed as separate argv elements. No shell string concatenation.
# INV-004: directive text is compiled governance, but it still reaches Claude  edikt-guard:allow
# through a structured field, never interpolated into a command.
set -uo pipefail

PAYLOAD=$(cat)

# The compile escape hatch: gov compile writes the very surfaces this hook
# reads, and a hook that fires on its own inputs would deadlock the compile.
if [ "${EDIKT_COMPILE_IN_PROGRESS:-0}" = "1" ]; then
	printf '{"continue": true}\n'
	exit 0
fi

_allow() {
	printf '{"continue": true}\n'
	exit 0
}

command -v python3 >/dev/null 2>&1 || {
	# ADR-058: python3 is an undeclared hook runtime dependency. Without it we  edikt-guard:allow
	# cannot build JSON safely, and building it unsafely is forbidden (INV-003).  edikt-guard:allow
	#
	# F-074/F-075 loud-failure bar: this used to allow via a BARE
	# {"continue": true} — the exact "silent _allow" the bar exists to end.
	# ADR-060:d07 already states the write-time tier's own principle for this:  edikt-guard:allow
	# "MUST fail open rather than fail closed... and MUST NOT fail open
	# silently." Extended here from "index missing/corrupt" to "python3
	# missing" — the same failure class, same required behaviour.
	echo "edikt inject-pre: python3 unavailable; injection skipped" >&2
	printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"⚠ edikt: python3 is missing on this host — MUST-grade write-time directive injection is DISABLED. Governed writes are not being blocked or annotated this session."}}\n'
	exit 0
}

# ── FIELD FRAMING: NUL, NOT WHITESPACE (F-041) ──────────────────────────────
#
# This read was `read -r TOOL FILE_PATH SESSION_ID` over a SPACE-JOINED line.
# On default IFS that word-splits, so `~/My Projects/x.go` truncated FILE_PATH
# to `~/My`, which is still non-empty and not `-`, so the guard below passed,
# `hook match` matched nothing, and `_allow` fired. The ENTIRE MUST-grade tier
# was off for any checkout under a path containing a space — routine on macOS —
# while every gate reported green. The trailing field also absorbed the rest of
# the line, so `--session` became attacker-influenced.
#
# A path may legally contain any byte except NUL and `/`. So neither a space
# delimiter, nor `$'\x1f'`, nor one-field-per-LINE is safe: each fixes the
# reported symptom and leaves the same silent bypass one byte away. NUL is the
# only delimiter that cannot occur in the data, so it is the only framing that
# closes the class rather than this one instance of it.
#
# Process substitution rather than `$(...)`: command substitution STRIPS NUL,
# which would silently collapse the framing back into the bug being fixed.
#
# ── ONE python3, NOT TWO (F-057) ─────────────────────────────────────────────
#
# This used to be two separate `python3 -c` invocations, each re-parsing the
# same $PAYLOAD for a different half of its fields (tool/path/session, then
# actor/agent identity). Measured at ~15.3ms per interpreter start — the
# JSON they parse is a few hundred bytes, so the cost is process spawn, not
# the parse. Nothing about the two halves depends on the other, so one
# interpreter now emits all six fields from one parse. The `hook match`
# call below needs a THIRD invocation to process its own output afterward —
# that one cannot merge here, since it consumes the binary's result, not
# the payload.
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
    # Trailing NUL after every field, the last one included, so each read
    # terminates on a delimiter rather than on EOF.
    sys.stdout.write("\0".join(vals) + "\0")


try:
    d = json.load(sys.stdin)
except Exception:
    # Fail open exactly as before (ADR-060): "-" fails the tool case below,  edikt-guard:allow
    # "unknown" is the same undeliverable-identity default the actor block
    # used on its own parse failure.
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

# Only file-writing tools carry a path worth matching. A Bash write (`>`,
# `tee`, `sed -i`) is NOT covered — that is a known, recorded gap (ADR FR-009),
# not an oversight: matching it means parsing shell, and a half-working parser
# would give false confidence that Bash writes are governed.
case "${TOOL:-}" in
Write | Edit | MultiEdit | NotebookEdit) ;;
*) _allow ;;
esac
[ -n "${FILE_PATH:-}" ] && [ "$FILE_PATH" != "-" ] || _allow

# EDIKT_BIN resolution ladder — same shape and same order as stop-hook.sh's
# and phase-end-detector.sh's, applied here for the first time. The prior
# form (`EDIKT_BIN="${EDIKT_BIN:-edikt}"`, a bare PATH lookup) has no
# candidate that resolves in a `--project` install: there is no `edikt` on
# PATH in that mode, so `command -v` always failed and this hook silently
# suppressed MUST-grade injection on every write, in every project-scoped
# install, while `doctor` reported the channel live. This is the third
# instance of the same resolution bug already fixed in stop-hook.sh and
# phase-end-detector.sh — and the first instance that hit the two hooks that
# actually enforce, not just report status.
#
#   1. project-local .edikt/bin/edikt — the canonical project-mode marker;
#      install.sh itself defines project-mode installed as this path
#      existing (install.sh:225). Unambiguous.
#   2. project-local bin/edikt        — edikt-dev's own dogfooding
#      convention, checked second because it's an ordinary ambiguous
#      relative path elsewhere.
#   3. $EDIKT_ROOT/bin/edikt          — the normal global install
#   4. PATH                           — whatever the environment offers
#   5. none                           → loud failure, not a silent _allow
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
	# The one class that cannot be journaled: no binary, nothing to journal
	# with. F-074/F-075 loud-failure bar: a bare {"continue": true} here is
	# indistinguishable from "no directives matched", which is exactly how
	# this survived a compile, a healthy `doctor` run, and a full upgrade.
	# Mirrors the python3-missing branch above: visible on stderr AND in the
	# session via additionalContext, not stderr alone.
	echo "edikt inject-pre: edikt binary not found; MUST-grade injection SUPPRESSED for $FILE_PATH" >&2
	printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"⚠ edikt: the edikt binary could not be resolved (checked .edikt/bin/edikt, bin/edikt, the global install root, and PATH) — MUST-grade write-time directive injection is DISABLED. Governed writes are NOT being blocked this session. Run /edikt:doctor to diagnose."}}\n'
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

MATCH=$("$EDIKT_BIN" hook match --path "$FILE_PATH" --grade must --session "$SESSION_ID" \
	--shim pre --dedup --json \
	--actor "$ACTOR" --agent-type "$AGENT_TYPE" --context "$AGENT_ID" 2>/dev/null) || _allow
[ -n "$MATCH" ] || _allow

printf '%s' "$MATCH" | python3 -c '
import json, sys

try:
    m = json.load(sys.stdin)
except Exception:
    print(json.dumps({"continue": True}))
    raise SystemExit(0)

entries = m.get("entries") or []
if not entries:
    # Includes every fail-open class and the deduped repeat. The binary has
    # already journaled WHY, so this is not a silent pass — it is a pass whose
    # reason is recorded somewhere a human can read.
    print(json.dumps({"continue": True}))
    raise SystemExit(0)

lines = []
for e in entries:
    lines.append("- %s (%s)" % (e.get("text", ""), e.get("id", "")))
    if e.get("intent"):
        lines.append("  intent: %s" % e["intent"])

# F-052 -- BOUNCE BUDGET, WIRED. `entries` non-empty no longer means "deny":
# the binary tracks how many DISTINCT receiving contexts have already
# bounced on this exact directive set, this session. Under
# hooks.injection.bounce_budget (default 8), a fresh context still gets
# denied below. Past it, the write PROCEEDS -- but not silently: these are
# MUST-grade directives, and going quiet past the budget would be exactly
# the "proceeds unheard" outcome the budget mechanism (dedup.go) was
# written to rule out. Delivered here as additionalContext instead -- the
# allow-path model-facing channel (ADR-062) -- so the agent still sees what  edikt-guard:allow
# it is governed by, without an inescapable-looking gate for a session that
# has already spawned enough dispatches to spend it.
#
# NO APOSTROPHES in this block or the message below -- it is embedded in a
# single-quoted shell string, and one silently terminates it (see the
# ACTOR/AGENT_ID block above, same file, same failure once already).
if m.get("budget_exhausted"):
    msg = (
        "edikt: this write touches %s, which is governed by %d MUST-grade "
        "directive(s):\n\n%s\n\n"
        "This session bounce budget for this exact directive set "
        "(hooks.injection.bounce_budget) is spent, so this write proceeds. "
        "The directives still apply; revise if this write does not comply."
        % (m.get("normalized_path", ""), len(entries), "\n".join(lines))
    )
    print(json.dumps({"hookSpecificOutput": {
        "hookEventName": "PreToolUse",
        "additionalContext": msg,
    }}))
    raise SystemExit(0)

msg = (
    "edikt: this write touches %s, which is governed by %d MUST-grade "
    "directive(s):\n\n%s\n\n"
    "These are non-negotiable. Revise the write to comply, then proceed — "
    "you will not be stopped again for these directives in this session."
    % (m.get("normalized_path", ""), len(entries), "\n".join(lines))
)

# DENY THE TOOL, DO NOT TERMINATE THE TURN.
#
# This emitted {"continue": False, "systemMessage": msg} until F-032. Both
# halves of that were wrong for what ADR-060 specifies:  edikt-guard:allow
#
#   `continue: false` stops the assistant entirely. The turn ends and a human
#   has to retype. ADR-060 says "deny, naming the matched directive; the write  edikt-guard:allow
#   does not happen" — a denied TOOL CALL, after which the model revises and
#   proceeds. The message three lines above literally says "Revise the write to
#   comply, then proceed", which `continue: false` forbids. The shim contra-
#   dicted its ADR and its own text at once.
#
#   `systemMessage` renders to the USER screen. The directive text has to
#   reach the MODEL — it is the model that must revise the write — and
#   `permissionDecisionReason` is the field that does that. So the old shape
#   also delivered the payload to the wrong reader.
#
# The correct shape was already known: test/partd-poc/d1/raw/hook/inject.sh,
# the PoC this tier was built from, uses permissionDecision/deny. Production
# regressed from its own prototype.
#
# NOT the same as verify-gate.sh. ADR-038 mandates {"continue": false,  edikt-guard:allow
# "systemMessage": ...} for THAT hook, deliberately: a completion claim without
# evidence should stop the turn. The contract for this tier is the one in ADR-060, and the two  edikt-guard:allow
# must not be unified without an ADR.
#
# Structured serializer, untrusted values passed as data (INV-003/INV-004).  edikt-guard:allow
print(json.dumps({"hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": msg,
}}))
'

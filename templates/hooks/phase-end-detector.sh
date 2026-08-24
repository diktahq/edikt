#!/usr/bin/env bash
# edikt hook — phase-end-detector. Go-backed (ADR-063), and the ONE hook in the
# tier that is not a plain exec shim.
#
# WHY THIS FILE IS LONGER THAN TEN LINES, AND WHY THAT IS NOT A RELAXATION
#
# ADR-063's Amendment (2026-08-22) admits a second shape — the
# dispatch-orchestrating caller — for a hook that performs genuine
# multi-round-trip LLM dispatch. This is that hook, and the only one: it execs
# the host model CLI at three sites, and INV-012 forbids the tier-2 binary from
# doing that. The two problems are separable, so they are separated:
#
#   Go   detection, every decision, and EVERY BYTE OF JSON
#   bash the three dispatches, and passing state between them
#
# Nothing below builds, parses or inspects JSON. `cat` on a file the binary
# wrote is not construction, and a `> file` redirect of a dispatch's output is
# not parsing. INV-003 is satisfied without an interpreter (INV-001), which is
# what took this hook's interpreter invocation count from 28 to 0.
#
# THE PROTOCOL
#
#   run step N  ->  exit 10  the binary wants a dispatch. It has written a run
#                            directory and printed its state file's path.
#               ->  exit 0   the final reply is on stdout; print it verbatim.
#               ->  other    a real failure; fail open, exactly as the bash
#                            hook did — a hook that blocks a Stop event is
#                            worse than one that skips an evaluation.
#
# Each dispatch's ARGV SHAPE lives here and only here. The binary supplies the
# DATA — prompt, system prompt, model token, timeout — as plain files, and is
# never told, and never decides, what is exec'd with them.
set -uo pipefail

_G="${EDIKT_ROOT:-${EDIKT_HOME:-$HOME/.edikt}}/bin/edikt"; _T="${EDIKT_HOME:-$HOME/.edikt}/state/trusted-roots.json"
_h() { command -v sha256sum >/dev/null 2>&1 && sha256sum "$1" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$1" 2>/dev/null | cut -d' ' -f1; }
_ok() { [ -x "$1" ] || return 1; [ "$1" = "$_G" ] && return 0; grep -qF "\"$(pwd -P)\":" "$_T" 2>/dev/null && return 0; [ -x "$_G" ] || return 1; H1="$(_h "$1")"; [ -n "$H1" ] && [ "$H1" = "$(_h "$_G")" ]; }
EDIKT_BIN=""
for c in "${PWD}/.edikt/bin/edikt" "${PWD}/bin/edikt" "$_G"; do _ok "$c" && { EDIKT_BIN="$c"; break; }; done
[ -n "$EDIKT_BIN" ] || { command -v edikt >/dev/null 2>&1 && EDIKT_BIN="edikt"; }
[ -n "$EDIKT_BIN" ] || { printf '%s\n' '{"hookSpecificOutput":{"additionalContext":"⚠ edikt: bin/edikt not found on any trusted discovery rung — hook phase-end-detector did not run (cause: binary-absent). Run /edikt:doctor to diagnose.","hookEventName":"Stop"},"systemMessage":"⚠ edikt: bin/edikt not found on any trusted discovery rung — hook phase-end-detector did not run (cause: binary-absent). Run /edikt:doctor to diagnose."}'; exit 0; }

# The CLI this caller dispatches, and whether it is present. Both are passed to
# the binary as DATA: the binary must not look for a model CLI itself (that is
# the `exec.LookPath` pattern ADR-030's purity gate refuses), and it renders
# "<tool> exited 2" from the name given to it rather than from a vendor string
# compiled into it.
_TOOL=claude  # edikt-guard:allow
_AVAIL=false
command -v "$_TOOL" >/dev/null 2>&1 && _AVAIL=true

# Profile pass-through. Every dispatch below is a SUBPROCESS that resolves its
# own config dir; in a multi-profile setup a bare invocation can silently target
# a different, unauthenticated account. Pass-through only — never invent or
# default a profile here.
[ -n "${CLAUDE_CONFIG_DIR:-}" ] && export CLAUDE_CONFIG_DIR  # edikt-guard:allow

# The payload is read ONCE and replayed to every step. Each step recomputes the
# payload digest and refuses a state file that does not match, so replaying the
# same bytes is what makes the cross-step identity check meaningful rather than
# ceremonial.
_WORK=$(mktemp -d 2>/dev/null) || exit 0
_RUN=""
trap 'rm -rf "$_WORK" ${_RUN:+"$_RUN"}' EXIT INT TERM
cat > "$_WORK/payload"

_STEP=1
_STATE=""
_DOUT=""
_DEXIT=0

while [ "$_STEP" -le 4 ]; do
  _argv=(hook run phase-end-detector --event Stop --step "$_STEP"
    --dispatch-tool "$_TOOL" --dispatch-available="$_AVAIL")
  if [ -n "$_STATE" ]; then
    _argv+=(--state "$_STATE" --dispatch-out "$_DOUT" --dispatch-exit "$_DEXIT")
  fi

  "$EDIKT_BIN" "${_argv[@]}" < "$_WORK/payload" > "$_WORK/reply"
  _RC=$?

  if [ "$_RC" -ne 10 ]; then
    # exit 0 is the final reply, byte for byte — including the paths that end
    # with no trailing newline at all. Anything else is a real failure and the
    # hook says nothing, which is the fail-open posture the tier requires.
    [ "$_RC" -eq 0 ] && cat "$_WORK/reply"
    exit 0
  fi

  IFS= read -r _STATE < "$_WORK/reply" || _STATE=""
  [ -n "$_STATE" ] || exit 0
  _RUN=$(dirname "$_STATE")
  _DOUT="$_RUN/dispatch-out"
  : > "$_DOUT"

  _PROMPT=$(cat "$_RUN/prompt" 2>/dev/null)
  _PRE=()
  if [ -f "$_RUN/timeout" ] && command -v timeout >/dev/null 2>&1; then
    _PRE=(timeout "$(cat "$_RUN/timeout")")
  fi

  case "$_STEP" in
    1)
      # Sidecar rebuild. Only the exit status is consumed; the output is not.
      "${_PRE[@]+"${_PRE[@]}"}" "$_TOOL" -p "$_PROMPT" >/dev/null 2>&1
      _DEXIT=$?
      ;;
    2)
      # L1 evaluator. stderr is folded into the captured output because the
      # verdict parser treats an auth complaint as evidence, not as noise.
      "${_PRE[@]+"${_PRE[@]}"}" "$_TOOL" -p "$_PROMPT" \
        --system-prompt "$(cat "$_RUN/sysprompt" 2>/dev/null)" \
        --allowedTools "Read,Grep,Glob,Bash" \
        --disallowedTools "Write,Edit" \
        --model "$(cat "$_RUN/model" 2>/dev/null)" \
        --output-format text \
        --bare > "$_DOUT" 2>&1
      _DEXIT=$?
      ;;
    3)
      # Post-flight. Pinned to the closed tool set commands/sdlc/post-flight.md
      # declares, with Write/Edit explicitly disallowed so the pipeline cannot
      # modify files. Agent is REQUIRED — post-flight forks the L2/L3 reviewers.
      "${_PRE[@]+"${_PRE[@]}"}" "$_TOOL" -p "$_PROMPT" \
        --allowedTools "Glob,Read,Bash,Agent" \
        --disallowedTools "Write,Edit" \
        --output-format json \
        --bare \
        --max-turns 5 > "$_DOUT" 2>/dev/null
      _DEXIT=$?
      ;;
  esac
  _STEP=$((_STEP + 1))
done
exit 0

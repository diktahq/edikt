#!/usr/bin/env bash
# edikt hook shim — dispatches to the edikt binary; first two rungs trust-gated.
_G="${EDIKT_ROOT:-${EDIKT_HOME:-$HOME/.edikt}}/bin/edikt"; _T="${EDIKT_HOME:-$HOME/.edikt}/state/trusted-roots.json"
_h() { command -v sha256sum >/dev/null 2>&1 && sha256sum "$1" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$1" 2>/dev/null | cut -d' ' -f1; }
_ok() { [ -x "$1" ] || return 1; [ "$1" = "$_G" ] && return 0; grep -qF "\"$(pwd -P)\":" "$_T" 2>/dev/null && return 0; [ -x "$_G" ] || return 1; H1="$(_h "$1")"; [ -n "$H1" ] && [ "$H1" = "$(_h "$_G")" ]; }
EDIKT_BIN=""
for c in "${PWD}/.edikt/bin/edikt" "${PWD}/bin/edikt" "$_G"; do _ok "$c" && { EDIKT_BIN="$c"; break; }; done
[ -n "$EDIKT_BIN" ] || { command -v edikt >/dev/null 2>&1 && EDIKT_BIN="edikt"; }
[ -n "$EDIKT_BIN" ] || { printf '%s\n' '{"hookSpecificOutput":{"additionalContext":"⚠ edikt: bin/edikt not found on any trusted discovery rung — hook user-prompt-submit did not run (cause: binary-absent). Run /edikt:doctor to diagnose.","hookEventName":"UserPromptSubmit"},"systemMessage":"⚠ edikt: bin/edikt not found on any trusted discovery rung — hook user-prompt-submit did not run (cause: binary-absent). Run /edikt:doctor to diagnose."}'; exit 0; }
exec "$EDIKT_BIN" hook run user-prompt-submit --event UserPromptSubmit

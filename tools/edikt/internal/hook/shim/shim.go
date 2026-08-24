// Package shim renders the canonical FR-004 hook exec shim.
//
// PLAN-v080-reliability-core Phase 8 builds three things: this template, the
// Render function below (the "generator"), and
// tools/edikt/check/hook-shim-byte-stability.sh (the byte-stability check
// that consumes Render through `bin/edikt hook shim check`). Phase 8 does
// NOT migrate any hook — Phases 12-22 do, one at a time, by writing this
// package's output verbatim into templates/hooks/<file>.
//
// # THE ANCHOR IS ${PWD}, NEVER $EDIKT_PROJECT_ROOT
//
// $EDIKT_PROJECT_ROOT is not an ambient contract: it is a local variable
// stop-hook.sh sets for its own heredocs (`EDIKT_PROJECT_ROOT="$(pwd -P)"`)
// and exports nowhere else. A shim that anchored on it would expand to
// `/.edikt/bin/edikt` and `/bin/edikt` in every install except stop-hook's
// own — never matching, always falling through to PATH. Template below
// copies the ${PWD}-anchored, four-rung ladder inject-directives-pre.sh and
// inject-directives-post.sh already carry (their own comments name it "the
// third instance of the same resolution bug, independently, in three
// different hooks"), in the same order: project-local .edikt/bin/edikt,
// project-local bin/edikt, the global install root, then PATH. The global
// root always precedes bare PATH.
//
// # WHY RUNGS 1 AND 2 ARE TRUST-GATED AND RUNGS 3-4 ARE NOT (ADR-041 reuse)
//
// Today only 4 of 24 hooks carry this ladder, and all four fire AFTER a tool
// call — by the time one runs, the user has already been working in the
// session. Once every hook is this shim, including session-start, a cloned
// repo shipping an executable .edikt/bin/edikt would be exec'd with the hook
// payload before the user has typed anything. Rungs 1 and 2 (the two
// PROJECT-supplied candidates) therefore resolve only when the candidate
// carries an ADR-041 trust record for this project OR its sha256 matches the
// global install (byte-identical to something already trusted carries no
// additional supply-chain risk). Rungs 3 and 4 are not project-supplied —
// the global install root is the user's own, and PATH is "whatever the
// environment offers" — so neither needs gating; gating them would not
// remove any risk this mechanism exists to remove.
//
// The trust store reused here is ADR-041's own
// ($EDIKT_HOME/state/trusted-roots.json, see internal/trust) — not a second
// one. A project already trusted to run its repo-defined `verify:` shell
// (ADR-041) is trusted to have its local `edikt` binary exec'd too; inventing
// a parallel trust decision for the same project would be two answers to
// what is, from the user's side, one question: "do I trust this repo".
//
// An untrusted local candidate is never loudly refused on its own — it is
// silently skipped, exactly as an absent one would be, and resolution
// continues down the ladder. The loud diagnostic is the absence banner
// below, which fires when NOTHING resolves. A project whose local binary is
// untrusted but whose global install works needs no alarm: a working,
// trusted execution path exists and was used.
//
// # <name> AND <event>, GENERATION TIME ONLY
//
// Render substitutes both exactly once, never at runtime, and never from
// anything a hook's own payload carries. <name> is the sole argument to
// `hook run`. <event> matters twice: as `hook run <name> --event <event>`
// (hook_run.go's own --event flag documents it: used only when the payload
// itself carries none) and inside the absence banner's
// hookSpecificOutput.hookEventName. The second use is not cosmetic — Claude
// Code discards a hookSpecificOutput whose hookEventName does not match the
// event that actually fired, so a shim generated with the wrong (or a
// generic/static) event value would have its absence banner silently
// dropped by the host for every event except whichever one happened to
// match, while still exiting 0 and looking, from the outside, identical to a
// shim whose banner was delivered.
package shim

import (
	"fmt"
	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hook"
)

// Template is the canonical shim, byte-for-byte, before substitution.
//
// Ten physical lines (LineCount asserts it). The single ratified exception to
// "discovery and exec only" (AC-004-1) is the binary-existence check that
// emits the absence banner; the trust gate is this phase's addition on top of
// that same exception, not a second one — it is still exclusively about
// *whether and where* a binary may be trusted to run, never about the hook's
// own payload.
const Template = `#!/usr/bin/env bash
# edikt hook shim — dispatches to the edikt binary; first two rungs trust-gated.
_G="${EDIKT_ROOT:-${EDIKT_HOME:-$HOME/.edikt}}/bin/edikt"; _T="${EDIKT_HOME:-$HOME/.edikt}/state/trusted-roots.json"
_h() { command -v sha256sum >/dev/null 2>&1 && sha256sum "$1" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$1" 2>/dev/null | cut -d' ' -f1; }
_ok() { [ -x "$1" ] || return 1; [ "$1" = "$_G" ] && return 0; grep -qF "\"$(pwd -P)\":" "$_T" 2>/dev/null && return 0; [ -x "$_G" ] || return 1; H1="$(_h "$1")"; [ -n "$H1" ] && [ "$H1" = "$(_h "$_G")" ]; }
EDIKT_BIN=""
for c in "${PWD}/.edikt/bin/edikt" "${PWD}/bin/edikt" "$_G"; do _ok "$c" && { EDIKT_BIN="$c"; break; }; done
[ -n "$EDIKT_BIN" ] || { command -v edikt >/dev/null 2>&1 && EDIKT_BIN="edikt"; }
[ -n "$EDIKT_BIN" ] || { printf '%s\n' '<absence-banner>'; exit 0; }
exec "$EDIKT_BIN" hook run <name> --event <event>
`

// Render substitutes <name> and <event> into Template.
//
// name is validated against hook.NamePattern — the exact pattern
// hook.Register enforces and templates/schemas/hook-registry.v1.schema.json
// pins (TestNamePatternMatchesSchema keeps the two from drifting) — because a
// name Register would reject can never resolve at dispatch time regardless of
// what a shim execs it with. event is required non-empty; its closed
// enumeration already lives in exactly one place (the registry schema) and
// every caller in this repo (hook shim check, and every future migration)
// sources it from a registry.yaml row already validated against that schema,
// so a second copy of the enum here would be the exact kind of drift-prone
// duplicate this package's own doc comment on name exists to avoid.
func Render(name, event string) (string, error) {
	if !hook.NamePattern.MatchString(name) {
		return "", fmt.Errorf("shim: %q is not a valid hook name (want %s)", name, hook.NamePattern)
	}
	if strings.TrimSpace(event) == "" {
		return "", fmt.Errorf("shim: event is required — the absence banner is rendered by adapter.AbsenceBanner, which needs the firing event; a generic one is silently discarded by the host")
	}
	if strings.Contains(name, "<") || strings.Contains(name, ">") || strings.Contains(event, "<") || strings.Contains(event, ">") {
		// Defense in depth: NamePattern already excludes '<'/'>', and no
		// registered event name contains them either. Refusing explicitly
		// here means a future NamePattern relaxation could never turn this
		// substitution into a second, attacker-reachable placeholder.
		return "", fmt.Errorf("shim: name/event must not contain '<' or '>'")
	}
	banner, err := adapter.AbsenceBanner(name, event)
	if err != nil {
		return "", err
	}
	out := strings.ReplaceAll(Template, "<absence-banner>", banner)
	out = strings.ReplaceAll(out, "<name>", name)
	out = strings.ReplaceAll(out, "<event>", event)
	return out, nil
}

// LineCount reports Template's physical line count — computed from the
// literal itself so the ≤10-line budget (AC-8.1) is asserted against what
// ships, not against a number a comment merely claims.
func LineCount() int {
	trimmed := strings.TrimRight(Template, "\n")
	if trimmed == "" {
		return 0
	}
	return strings.Count(trimmed, "\n") + 1
}

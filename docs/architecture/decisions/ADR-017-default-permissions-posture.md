---
type: adr
id: ADR-017
title: Default permissions posture — deny-by-default destructive patterns in settings.json.tmpl
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-04-17T00:00:00Z
references:
  adrs: [ADR-006]
  invariants: [INV-005]
  prds: [PRD-002]
  specs: [SPEC-004]
---

# ADR-017: Default permissions posture — deny-by-default destructive patterns in settings.json.tmpl

**Status:** Accepted
**Date:** 2026-04-17
**Decision-makers:** Daniel Gomes

---

## Context and Problem Statement

The v0.5.0 security audit (2026-04-17) rated HI-9 on the fact that `templates/settings.json.tmpl` — the canonical settings file edikt installs into `~/.claude/settings.json` — shipped with 16 hook handlers and a 30-second `statusLine` command, but no `permissions` block. Claude Code falls back to its built-in default when `permissions` is absent, which does not constrain destructive Bash patterns, `WebFetch` to arbitrary URLs, or MCP tool usage.

edikt is a governance tool. Shipping zero guardrails in the default posture contradicts the product's purpose. Meanwhile, edikt's own audit command (`/edikt:sdlc:audit`) reviews user projects for exactly this class of configuration gap. Dogfooding requires the default to be safe.

The design question is not "should we ship permissions" but "what is the minimum safe default that doesn't break common workflows."

## Decision Drivers

- Defaults should be safe. A fresh `edikt` install should reduce attack surface, not leave it at the Claude Code out-of-the-box level.
- Defaults should not break common edikt workflows — git, gh, pytest, npm test, the local test harness, and edikt's own bash invocations must work without a permission prompt.
- User customizations must survive upgrades. Existing users on v0.4.x with customized `settings.json` must not have their additions wiped.
- The denied patterns must be obviously destructive, not merely "suspicious". Ambiguous denies create permission prompt fatigue.

## Considered Options

1. **Deny-by-default destructive patterns + allow edikt's own needs** — ship a conservative `permissions` block with both lists populated.
2. **Allow-by-default with narrow deny list** — mirror Claude Code's fallback but add a handful of explicit denies.
3. **No default, link to a recommended-settings doc** — ship blank, tell users to copy from docs. Same posture as today.
4. **Strict allow-list with no deny list** — only explicitly allowed tools permitted; everything else requires a prompt. Too aggressive.

## Decision

Ship `templates/settings.json.tmpl` with an explicit `permissions` block. Contents:

### `permissions.deny`

Destructive Bash patterns:
- `Bash(rm -rf /**)`, `Bash(rm -rf ~/**)`, `Bash(rm -rf $HOME/**)`
- `Bash(chmod -R 777 **)`, `Bash(sudo **)`, `Bash(sudo:*)`
- `Bash(:(){ :|:& };:)` (fork bomb literal)
- `Bash(* > /dev/tcp/*)`, `Bash(* > /dev/udp/*)`
- `Bash(dd if=/dev/zero **)`, `Bash(mkfs.**)`

Destructive git patterns:
- `Bash(git push --force main)`, `Bash(git push --force master)`
- `Bash(git push --force origin main)`, `Bash(git reset --hard origin/**)`

Plaintext HTTP fetches (TLS-only posture):
- `WebFetch(http://**)`, `Bash(curl http://**)`, `Bash(wget http://**)`

Sensitive file reads:
- `Read(/etc/shadow)`, `Read(**/.ssh/id_*)`, `Read(**/.ssh/known_hosts)`
- `Read(**/.aws/credentials)`, `Read(**/.docker/config.json)`

### `permissions.allow`

Tools edikt itself requires:
- `Read(**)`, `Glob`, `Grep`, `Edit(**)`, `Write(**)`
- `Bash(git :*)`, `Bash(gh :*)`
- `Bash(npm test)`, `Bash(npm run test:*)`
- `Bash(pytest :*)`, `Bash(./test/run.sh)`, `Bash(./test/test-e2e.sh)`
- `Bash(make test)`, `Bash(uv run :*)`, `Bash(ruff :*)`
- `WebFetch(https://**)`, `WebSearch`

### `permissions.defaultMode`

`askBeforeAllow` — any tool not explicitly allowed or denied prompts the user once.

### Managed-region tracking

The `permissions` block is edikt-managed. Integrity is tracked via the sidecar mechanism defined by INV-005(b): `~/.edikt/state/settings-managed.json` records the canonical hash of the managed keys. Any drift (user edited `permissions` directly) is detected on the next install/upgrade and prompts the user before overwrite. User-added top-level keys outside `permissions` survive untouched.

## Alternatives Considered

### Allow-by-default with narrow deny list
- **Pros:** Minimal friction for existing users.
- **Cons:** Same posture as the current `settings.json.tmpl` (which ships no `permissions` — effectively allow-by-default). Fails to address the audit finding.
- **Rejected because:** dogfooding edikt requires a safe default.

### No default, link to a recommended-settings doc
- **Pros:** No risk of breaking anyone's workflow.
- **Cons:** Documented-but-unshipped defaults are effectively not the default. The audit finding stands.
- **Rejected because:** this is what v0.4.x ships and it's the cause of HI-9.

### Strict allow-list
- **Pros:** Tightest posture.
- **Cons:** Every new tool, every new Bash invocation, every WebFetch to an unfamiliar domain prompts. Permission fatigue guaranteed.
- **Rejected because:** the goal is a safe default, not maximum friction.

## Consequences

- **Good:** Fresh installs of edikt ship a governance-aligned security posture by default. Audit finding HI-9 closes.
- **Good:** Existing users' customized `settings.json` is preserved — the sidecar integrity check detects drift and prompts before overwrite.
- **Good:** LOW-7 (formatter hook running on `node_modules`) is addressed alongside by excluding `node_modules/` and `.venv/` from the PostToolUse glob.
- **Bad:** Users may see a permission prompt the first time Claude tries to `curl http://...` or `WebFetch` an `http://` URL. Allow-once resolves. Documented in `docs/guides/permissions.md` (new file, Phase 7).
- **Bad:** Users with legitimate uses of the denied patterns (some CI users run `chmod -R 777` in test fixtures) must add explicit allow entries in their `userPermissions` block (preserved outside the sentinel).
- **Neutral:** The allow list may need tuning per-project — tests, CHANGELOGs, and user feedback are expected to refine it over v0.5.x.

## Confirmation

- Fresh install produces a `settings.json` that validates against Claude Code's settings schema.
- `cat ~/.claude/settings.json | jq .permissions.deny` lists every denied pattern.
- Running `curl http://example.com` under a fresh install produces a permission prompt (not silent allow).
- Running `./test/run.sh` under a fresh install runs without a permission prompt.
- User with a customized `settings.json` running `bin/edikt upgrade` sees a prompt before the permissions block is replaced.
- `~/.edikt/state/settings-managed.json` exists after install and records the canonical hash of the managed keys.

## Directives

[edikt:directives:start]: #
source_hash: pending
directives_hash: pending
compiler_version: "0.4.3"
topic: hooks
paths:
  - "templates/settings.json.tmpl"
  - "install.sh"
  - "bin/edikt"
  - "docs/guides/permissions.md"
scope:
  - implementation
  - review
directives:
  - `templates/settings.json.tmpl` MUST include an explicit `permissions` block with populated `deny`, `allow`, and `defaultMode: askBeforeAllow` keys. (ref: ADR-017)
  - The `permissions.deny` list MUST include at minimum the destructive Bash patterns, destructive git patterns, plaintext HTTP fetch patterns, and sensitive file read patterns enumerated in ADR-017. (ref: ADR-017)
  - The `permissions.allow` list MUST include edikt's operational tools (git, gh, pytest, npm test, `./test/run.sh`, `./test/test-e2e.sh`, make test, uv, ruff, WebFetch(https://**), WebSearch). (ref: ADR-017)
  - Managed-region integrity for `settings.json` MUST use the sidecar mechanism at `~/.edikt/state/settings-managed.json`. NEVER embed `_edikt` metadata inside the settings.json itself. (ref: ADR-017, INV-005)
  - On upgrade with a drifted `settings.json`, the writer MUST prompt the user before overwriting managed keys. User-added top-level keys outside the managed region MUST be preserved untouched. (ref: ADR-017)
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #

---

*Captured by edikt:adr — 2026-04-17*

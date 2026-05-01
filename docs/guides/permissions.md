# Claude Code Permissions — edikt default posture

Per ADR-017, edikt ships `~/.claude/settings.json` with an explicit `permissions` block that denies destructive patterns by default while allowing the tools edikt itself requires. This guide explains what's denied, what's allowed, and how to override.

## Why defaults matter

Claude Code without a `permissions` block falls back to its built-in default, which does not constrain destructive Bash patterns, plaintext HTTP fetches, or reads of sensitive files. edikt is a governance tool; shipping an unconstrained default would contradict the product's purpose and fail the same audit criteria that `/edikt:sdlc:audit` applies to user projects.

The defaults here aim for "safe and productive." They deny obviously destructive or exfiltration-adjacent patterns and explicitly allow everything edikt's own workflows need (git, gh, pytest, the local test runner, etc.).

## What's denied by default

### Destructive Bash

| Pattern | Why |
|---|---|
| `Bash(rm -rf /**)`, `Bash(rm -rf ~/**)`, `Bash(rm -rf $HOME/**)` | Recursive wipe of filesystem roots. |
| `Bash(chmod -R 777 **)` | World-writable recursion — credential exfiltration setup. |
| `Bash(sudo **)`, `Bash(sudo:*)` | Privilege escalation. |
| `Bash(:(){ :|:& };:)` | Fork bomb literal. |
| `Bash(* > /dev/tcp/**)`, `Bash(* > /dev/udp/**)` | Arbitrary network write. |
| `Bash(dd if=/dev/zero **)` | Disk-wipe primitive. |
| `Bash(mkfs.**)` | Filesystem formatter. |

### Destructive git

| Pattern | Why |
|---|---|
| `Bash(git push --force main)` and variants | Force-pushing primary branches overwrites history for every consumer. |
| `Bash(git reset --hard origin/**)` | Wipes uncommitted work without confirmation. |

### Plaintext HTTP fetches

| Pattern | Why |
|---|---|
| `WebFetch(http://**)` | No TLS — content tamperable in transit. |
| `Bash(curl http://**)`, `Bash(wget http://**)` | Same, via bash fetchers. |

### Sensitive file reads

| Pattern | Why |
|---|---|
| `Read(/etc/shadow)` | Root password hashes. |
| `Read(**/.ssh/id_*)`, `Read(**/.ssh/known_hosts)` | SSH credentials. |
| `Read(**/.aws/credentials)` | AWS access keys. |
| `Read(**/.docker/config.json)` | Docker registry credentials. |

## What's allowed by default

Read-only and mutation operations edikt uses:
- `Read(**)`, `Glob`, `Grep`, `Edit(**)`, `Write(**)`

Shell invocations edikt's test and CI workflows depend on:
- `Bash(git :*)` — any git subcommand
- `Bash(gh :*)` — any GitHub CLI subcommand
- `Bash(npm test)`, `Bash(npm run test:*)`
- `Bash(pytest :*)`
- `Bash(./test/run.sh)`, `Bash(./test/test-e2e.sh)`
- `Bash(make test)`
- `Bash(uv run :*)`, `Bash(ruff :*)`

Network tools (TLS-only):
- `WebFetch(https://**)`, `WebSearch`

## Default mode

`defaultMode: "askBeforeAllow"`. Any tool neither explicitly allowed nor denied prompts once; the answer is remembered for the session.

## Verifying your active permissions

```bash
cat ~/.claude/settings.json | jq .permissions
```

## Overriding defaults

Do NOT edit the edikt-managed `permissions` block directly. edikt tracks the managed region via a sidecar at `~/.edikt/state/settings-managed.json`; unsolicited edits trigger an upgrade-time prompt (see ADR-017 and INV-005).

To add a project-specific allow or deny rule:
1. Add it to a separate top-level `userPermissions` key (or an additional layer of your choice) OUTSIDE the `permissions` block edikt manages.
2. On upgrade, edikt preserves user-added top-level keys untouched; only the managed `permissions` block is replaced.

Example (add to your `~/.claude/settings.json` alongside the edikt-managed `permissions`):

```json
{
  "permissions": { /* edikt-managed, do not edit */ },
  "userPermissions": {
    "allow": [
      "Bash(curl http://localhost:8080/**)",
      "Bash(chmod -R 777 ./test/fixtures)"
    ]
  }
}
```

Claude Code merges user-scope and project-scope settings; your `userPermissions` additions layer on top of the defaults.

## Opting out of the default permissions

If you need to disable the defaults entirely — e.g. for a CI machine where you've written your own allow/deny list — set `permissions.defaultMode: "bypass"` (but note this reverts to Claude Code's built-in fallback, which is unconstrained).

A safer path: set `EDIKT_SETTINGS_SKIP=1` during install to skip the settings write, and manage `~/.claude/settings.json` yourself.

## Cross-references

- **ADR-017** — Default permissions posture (the decision).
- **INV-005** — Managed-region integrity (the enforcement mechanism).
- **INV-007** — Hermetic test sandboxes (why the benchmark runner doesn't copy these defaults into untrusted sandboxes).
- **docs/reports/security-audit-v0.5.0-2026-04-17.md** — Audit finding HI-9 that drove this work.

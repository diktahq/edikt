# CI/CD Governance

Run edikt governance checks in your CI pipeline to catch drift, contradictions, and security issues before they reach production.

## Quick start

Add these steps to your CI pipeline:

```yaml
# GitHub Actions example
- name: Compile check
  run: |
    EDIKT_HEADLESS=1 claude --bare -p "/edikt:compile --check"

- name: Drift check
  run: |
    EDIKT_HEADLESS=1 claude --bare -p "/edikt:drift"
```

## Recommended CI settings

Create a CI-specific settings file:

```json
{
  "sandbox": {
    "enabled": true,
    "failIfUnavailable": true
  },
  "permissions": {
    "defaultMode": "plan"
  }
}
```

Key settings:
- `sandbox.failIfUnavailable: true` — exit with error if sandbox can't start, instead of running unsandboxed
- `defaultMode: plan` — read-only mode, no file modifications

## The `--bare` flag

Use `--bare` for CI runs. It skips hooks, LSP, plugin sync, and skill directory walks — faster startup, cleaner execution.

```bash
claude --bare -p "/edikt:compile --check"
```

Requires `ANTHROPIC_API_KEY` or an `apiKeyHelper` via `--settings`. OAuth and keychain auth are disabled in bare mode.

## Headless mode

Set `EDIKT_HEADLESS=1` to auto-answer interactive prompts. edikt's headless hook intercepts `AskUserQuestion` calls and returns predefined answers.

Default behavior:
- Yes/no questions → "yes"
- Choice questions → "skip"

Configure custom answers in `.edikt/config.yaml`:

```yaml
headless:
  answers:
    "proceed with compilation": "yes"
    "which packs to update": "all"
    "update anyway": "no"
```

## Available CI checks

| Check | Command | What it catches |
|---|---|---|
| Compile | `/edikt:compile --check` | ADR contradictions, guideline conflicts, missing sentinels |
| Drift | `/edikt:drift` | Implementation diverging from spec |
| Audit | `/edikt:audit` | Security vulnerabilities, OWASP gaps |
| Doctor | `/edikt:doctor` | Stale rules, missing hooks, config issues |

## Environment variables

| Variable | Purpose |
|---|---|
| `ANTHROPIC_API_KEY` | API authentication for `--bare` mode |
| `EDIKT_HEADLESS=1` | Auto-answer interactive prompts |
| `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1` | Strip credentials from subprocesses |
| `EDIKT_FORMAT_SKIP=1` | Skip auto-formatting (faster CI runs) |

## Example: GitHub Actions workflow

```yaml
name: Governance Check
on: [pull_request]

jobs:
  governance:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install Claude Code
        run: npm install -g @anthropic-ai/claude-code

      - name: Compile check
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          EDIKT_HEADLESS: "1"
          CLAUDE_CODE_SUBPROCESS_ENV_SCRUB: "1"
        run: claude --bare -p "/edikt:compile --check"

      - name: Drift check
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          EDIKT_HEADLESS: "1"
        run: claude --bare -p "/edikt:drift"
```

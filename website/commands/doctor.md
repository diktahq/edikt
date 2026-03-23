# /edikt:doctor

Validate governance setup and report what's healthy, what's missing, and how to fix it.

## Usage

```
/edikt:doctor
```

## What it checks

| Check | Pass | Warn |
|-------|------|------|
| `.edikt/config.yaml` valid | ✅ | Parse error shown |
| `{base}/project-context.md` exists | ✅ | Suggest `/edikt:init` |
| `{base}/decisions/` ADRs | ✅ count | Empty → suggest `/edikt:adr` |
| `{base}/invariants/` | ✅ count | — |
| `.claude/rules/` packs | ✅ count | Empty → suggest `/edikt:init` |
| Rule pack freshness | ✅ current | Outdated → suggest `/edikt:rules-update` |
| CLAUDE.md edikt sentinel | ✅ | Missing → suggest `/edikt:init` |
| SessionStart hook | ✅ | Outdated inline bash → suggest `/edikt:upgrade` |
| Stop hook | ✅ | Outdated format or blocking error → suggest `/edikt:upgrade` |
| PreToolUse hook | ✅ | Missing → suggest `/edikt:init` |
| PreCompact hook | ✅ | Missing → suggest `/edikt:init` |
| `{base}/product/spec.md` | ✅ | Missing → suggest `/edikt:intake` |
| Active plans | ✅ count | None → suggest `/edikt:plan` |
| Auto-memory | ✅ age/size | Stale or near limit → suggest `/edikt:context` |
| Agents installed | ✅ count | None → suggest `/edikt:init` |
| Extensibility (template + rule overrides) | ✅ | Extension file configured but missing |
| Linter sync | ✅ | Config newer than rules → suggest `/edikt:sync` |
| edikt version | ✅ match | Project version differs from installed → suggest `/edikt:upgrade` |

### Decision graph validation

Doctor also validates the consistency of the governance graph:

| Check | What it detects |
|-------|----------------|
| ADR contradictions | Pairs of accepted ADRs making opposing decisions on the same topic |
| Rule-invariant consistency | Rules that contradict an active invariant |
| Plan-ADR dependencies | Active plans referencing superseded ADRs |
| Invariant enforcement | Invariants not referenced by any rule or hook |
| Orphan artifacts | ADRs, PRDs, or specs not referenced by any other artifact |
| Stale artifacts | PRDs or specs stuck in `draft` for more than 7 days |
| State machine violations | Specs referencing unaccepted PRDs, or plans referencing draft artifacts |

## Output

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 EDIKT DOCTOR
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

 [ok]   .edikt/config.yaml valid
 [ok]   docs/project-context.md exists
 [ok]   docs/decisions/ — 4 ADRs
 [ok]   .claude/rules/ — 3 packs installed
 [!!]   go.md outdated (installed: 1.0, available: 1.2) — run /edikt:rules-update
 [ok]   CLAUDE.md has edikt sentinel
 [ok]   SessionStart hook is git-aware
 [ok]   Memory: 2 days old, 45/200 lines
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 14 passed, 1 warning, 0 failures
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Recommendations:
  1. go.md outdated — run /edikt:rules-update
```

## Natural language triggers

- "is edikt set up correctly?"
- "check governance"
- "doctor"
- "any issues with edikt?"

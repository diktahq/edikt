# /edikt:doctor

Validate governance setup and report what's healthy, what's missing, and how to fix it.

## Usage

```bash
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
| Rule pack freshness | ✅ current | Outdated → suggest `/edikt:gov:rules-update` |
| CLAUDE.md edikt sentinel | ✅ | Missing → suggest `/edikt:init` |
| SessionStart hook | ✅ | Outdated inline bash → suggest `/edikt:upgrade` |
| Stop hook | ✅ | Outdated format or blocking error → suggest `/edikt:upgrade` |
| PreToolUse hook | ✅ | Missing → suggest `/edikt:init` |
| PreCompact hook | ✅ | Missing → suggest `/edikt:init` |
| `{base}/product/spec.md` | ✅ | Missing → suggest `/edikt:docs:review:intake` |
| Active plans | ✅ count | None → suggest `/edikt:sdlc:plan` |
| Auto-memory | ✅ age/size | Stale or near limit → suggest `/edikt:context` |
| Agents installed | ✅ count | None → suggest `/edikt:init` |
| Extensibility (template + rule overrides) | ✅ | Extension file configured but missing |
| Linter sync | ✅ | Config newer than rules → suggest `/edikt:gov:sync` |
| edikt version | ✅ match | Project version differs from installed → suggest `/edikt:upgrade` |

### Sidecar Health (v0.6.0)

For every ADR, invariant, and guideline `.md`, doctor verifies the co-located `<artifact>.edikt.yaml` sidecar:

| Check | Severity | What it catches |
|---|---|---|
| `ORPHAN` | Hard fail | A `.edikt.yaml` exists with no sibling `.md` |
| `MISSING` | Hard fail | A governance `.md` has no co-located sidecar |
| `PATH MISMATCH` | Hard fail | The sidecar's `path:` field doesn't resolve to the sibling `.md` |
| Schema validation | Hard fail | Sidecar fails `templates/schemas/sidecar.v1.schema.json` |
| `directives: []` | Soft warning | Sidecar exists but has no directives — sidecar may need regeneration |

```text
SIDECAR HEALTH
  Orphans:           0
  Missing sidecars:  0
  Path mismatches:   0
  Schema failures:   0
  Empty directives:  1

  ⚠ NEEDS REVIEW: ADR-007.md has no directives in its sidecar — confirm the
    prose has no rules to extract, or run /edikt:adr:compile ADR-007.
```

Hard-fail checks (1–4) exit 1. The empty-directives check is soft — exit 0 with a warning summary. Resolve via:

- `MISSING` → run `/edikt:<type>:compile <id>` for the artifact
- `ORPHAN` → delete the stale sidecar (no parent prose)
- `PATH MISMATCH` → fix the `path:` field, then run `:compile` to regenerate canonically
- `directives: []` → confirm the prose is intentionally rule-free, or re-run `:compile`

### PRD/SPEC artifact health (v0.6.0)

Doctor runs four checks against every PRD sidecar and every SPEC sidecar:

| Check | What it catches |
|-------|----------------|
| **Orphaned sidecars** | A `.yaml` with no sibling `.md`, or vice versa (only flagged when the project has at least one v2 PRD). |
| **Schema version** | Sidecar's `schema_version` is missing or unknown (e.g., a sidecar from a newer edikt). |
| **Sidecar drift** | The `.md` was edited after the last sync — `_sync.md_hash` no longer matches the file. Informational; the PRD is still valid, the sync record is stale. |
| **Broken refs** | Linked invariants, source SPECs, supersede chains, or solution_references that point to files that don't exist. |

```text
PRD/SPEC ARTIFACT HEALTH
  Orphaned sidecars: 0
  Schema version warnings: 0
  Sidecar drift: 1
  Broken refs: 1

  ⚠ PRD-005: .md edited since last sync (2026-04-12). Re-author with /edikt:sdlc:prd PRD-005.
  ⚠ PRD-007: protection INV-042 references non-existent invariant.
```

v1 PRDs (no sidecar) are silently skipped — the checks need the structured sidecar.

### Fixture characterization rate

For each spec with a `fixtures.yaml` containing expected-output records, doctor reports the ratio of `characterized` to `aspirational` records:

```text
[!!] Fixture characterization rate is low (35%). Most test expectations are unverified against running code.
[ok] Fixtures fully characterized (12 records)
[--]  3 aspirational fixture record(s) — run verified_by commands to characterize
```

Set `EDIKT_DOCTOR_DEEP=1` to also re-run safe `verified_by` commands on `characterized` records older than 90 days and flag stale verifications.

### Gate activity (last 7 days)

Doctor reads `~/.edikt/events.jsonl` and reports unresolved gate findings from the last 7 days, plus override activity from the last 30 days:

```text
Gate activity:
  Unresolved: 2
    2026-04-25T14:22:00Z : security gate (critical) — no resolution recorded
    2026-04-26T09:01:00Z : dba gate (warning) — no resolution recorded
  Overrides (last 30 days): 1
```

Use this with `/edikt:session` to sweep unresolved findings before the work compounds.

### Routing-table source-file check (v0.5.0)

Doctor verifies that every ADR and invariant referenced in the routing table inside `.claude/rules/governance.md` exists on disk.

```text
[!!] Missing source file: docs/architecture/decisions/ADR-012.md
     (referenced in governance.md routing table but not found on disk)
```

If a source file is absent, doctor exits non-zero and prints the exact missing path. This catches governance drift after a file rename, move, or accidental deletion.

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
| Stale spec-artifact drafts | Spec artifacts (data models, contracts, migrations, fixtures) in `draft` for more than 7 days |

**Spec-artifact stale drafts:**
For each spec directory, doctor checks all artifacts (data models, contracts, migrations, fixtures) for `status: draft`. If an artifact has been in draft for more than 7 days (by file modification time), doctor flags it:

```text
[!!] SPEC-005/data-model.mmd has been draft for 12 days — review and accept, or remove
[!!] SPEC-005/contracts/api.yaml has been draft for 12 days — review and accept, or remove
```

Doctor parses status from both YAML frontmatter (`status: draft`) and comment headers (`status=draft` in `%%`, `#`, `--`, or `<!-- -->` format).

## Output

```text
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 EDIKT DOCTOR
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

 [ok]   .edikt/config.yaml valid
 [ok]   docs/project-context.md exists
 [ok]   docs/decisions/ — 4 ADRs
 [ok]   .claude/rules/ — 3 packs installed
 [!!]   go.md outdated (installed: 1.0, available: 1.2) — run /edikt:gov:rules-update
 [ok]   CLAUDE.md has edikt sentinel
 [ok]   SessionStart hook is git-aware
 [ok]   Memory: 2 days old, 45/200 lines
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 14 passed, 1 warning, 0 failures
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Recommendations:
  1. go.md outdated — run /edikt:gov:rules-update
```

## Launcher-level checks (v0.5.0)

`/edikt:doctor` also probes the launcher's install health. These checks run against the versioned layout at `~/.edikt/`:

| Check | Pass | Action |
|---|---|---|
| `~/.edikt/current` symlink valid | ✅ | — |
| `current` target exists | ✅ | Suggest `edikt use <version>` |
| `lock.yaml` parseable | ✅ | — |
| `manifest.yaml` present in active version | ✅ | Suggest `edikt install` |
| SHA256 of `bin/edikt` matches manifest | ✅ | Suggest `edikt install` |
| `edikt` on PATH | ✅ | Print PATH placement |
| NFS / WSL1 filesystem detected | ⚠ | Warn with workaround |

## `--report` bundle

Generate a shareable debug bundle:

```bash
edikt doctor --report
```

Writes `~/.edikt/reports/doctor-<timestamp>.txt` containing: version info, symlink health, manifest integrity check, events.jsonl tail (last 50 lines), system info (OS, shell, filesystem type under `$EDIKT_ROOT`). Share the report path when filing issues.

## `--backfill-provenance`

Add `edikt_template_hash` to agents installed before v0.5.0:

```bash
edikt doctor --backfill-provenance
```

Assumes the installed file matches the template from the `edikt_version` recorded in your config. Review the proposed hashes before confirming. This enables the provenance-first upgrade flow for pre-v0.5.0 agents.

## NFS / WSL1 workaround

If `edikt doctor` reports "symlinks not supported on this filesystem":

1. Move `~/.edikt/` to a POSIX-compatible filesystem (ext4, APFS)
2. Set `EDIKT_ROOT` to the new location:
   ```bash
   export EDIKT_ROOT=/path/on/posix/fs/.edikt
   ```
3. Add to your shell profile

## Natural language triggers

- "is edikt set up correctly?"
- "check governance"
- "doctor"
- "any issues with edikt?"

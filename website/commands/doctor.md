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
| `docs/architecture/decisions/` ADRs | ✅ count | Empty → suggest `/edikt:adr` |
| `docs/architecture/invariants/` | ✅ count | — |
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
| `MISSING` | Hard fail, unless explicitly opted out (see below) | A governance `.md` has no co-located sidecar |
| `PATH MISMATCH` | Hard fail | The sidecar's `path:` field doesn't resolve to the sibling `.md` |
| Schema validation | Hard fail | Sidecar fails `templates/schemas/gov-sidecar.v2.schema.json` |
| `directives: []` | Soft warning | Sidecar exists but has no directives — sidecar may need regeneration |

**Opting an artifact out of `MISSING`.** `status: proposed` alone is not
exempt — a proposed ADR can legitimately carry a real sidecar even before
acceptance. For an artifact deliberately not yet projected, opt out
explicitly with `sidecar: skip` (optional `reason: "…"`) in frontmatter, or
a `<!-- edikt:sidecar:skip reason="…" -->` body marker — a different key
from `no-directives:`, which suppresses an unrelated compile-time warning.

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

### Sidecar Verify Coverage (v0.6.0)

For every gov / prd / spec sidecar in the project, doctor walks the declared `verify:` shell commands via `bin/edikt verify all` and reports both coverage axes — sparse-but-wide and dense-on-few — so you can see whether `verify:` is broadly adopted or concentrated in a few artifacts. Soft signal; never blocks doctor's exit.

```text
[ok] Sidecar verify coverage — 28/40 sidecars (70%); items 84/510 (16%); 84 passed, 0 failing, 426 skipped.
```

The two ratios mean:

- **sidecars covered** — how many sidecars carry **at least one** `verify:` field (the "discoverability" axis).
- **items covered** — across every claim-bearing slot (`directives[]`, `requirements[]`, `acceptance_criteria[]`, etc.), how many have a `verify:` line (the "density" axis).

When sidecar coverage falls below 25% doctor adds a warning prompting you to add mechanical checks to high-traffic directives, FRs, or SRs:

```text
[!!] WARN: sidecar verify coverage low (12% of sidecars carry at least one verify:) — consider adding mechanical checks to high-traffic directives, FRs, or SRs.
[!!] Sidecar verify coverage — 5/40 sidecars (12%); items 14/510 (2%); 14 passed, 0 failing, 496 skipped.
```

Per-sidecar failures (a `verify:` that exited non-zero) are surfaced inline as `WARN` lines with the remediation command. See [`edikt verify`](/commands/verify) for the full contract.

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

### Routed sources check

There's no routing table in the current render (`compile_schema_version: 3`) — that mechanism was retired. What doctor checks instead: it enumerates the surfaces `gov compile` actually produced from the render manifest (`.claude/rules/governance/manifest.yaml`) — the ambient core, topic files, and skill packages — extracts every `(ref: ADR-NNN)` / `(ref: INV-NNN)` citation across them, and verifies each one resolves to a real source file under the configured decisions/invariants directories. A project with no manifest yet falls back to a directory walk rather than failing outright.

```text
       surfaces (from the render manifest): .claude/rules/governance.md, .claude/rules/governance/architecture.md, ...
[FAIL] Missing source for routed directive: ADR-012 expected at docs/architecture/decisions/ADR-012-*.md
```

A surface the manifest lists but that can't be read on disk is reported separately as an error — a broken contract, not an absent one:

```text
[error] Routed sources — 1 listed surface(s) could not be read:
        .claude/rules/governance/architecture.md: open .../architecture.md: no such file or directory
        The render manifest names a surface that is not on disk. Re-run `edikt gov compile`.
```

If every cited ID resolves, doctor reports `[ok] Routed sources — N of N resolve`. Any missing source or unreadable surface exits doctor non-zero. This catches governance drift after a file rename, move, or accidental deletion — and, since it's driven by the manifest rather than a directory walk, it also catches a manifest that still points at a surface no longer on disk.

### Other new 0.7.0 checks

Four more standing checks, added alongside the routed-sources check above:

| Check | What it catches |
|-------|----------------|
| **Orphan surfaces** | A rendered surface (topic file, skill package) that fell outside manifest tracking — existed before manifest tracking started, or the manifest was reset/lost between compiles — and so is invisible to `gov compile`'s own manifest-diff cleanup. Backstop for a surface that should have been removed but wasn't. |
| **Pending topic descriptions** | A topic whose description was never explicitly approved. Render substitutes an honest "no approved topic description yet" placeholder rather than inventing one; this check surfaces every topic still in that pending state so it doesn't go unnoticed indefinitely. |
| **Shadow ambient core / unreachable topics** | Two topic-scope integrity problems, both authored-source issues (missing `paths:` globs), not compile bugs: a topic where one contributing sidecar declares no `paths:` unscopes the *whole* topic to `paths: "**"` (defeating the point of scoping it at all — "shadow ambient core"); a topic whose every sidecar is undeclared retires to skill-package-only, reachable at write time only via an explicit trigger match ("unreachable topic"). Previously visible only as a one-shot line in the compile summary — this check makes it standing and repeatable. |
| **Hook-path resolution** | Verifies every hook command registered in `settings.json` actually resolves to a file on disk. Claude Code doesn't expand env vars in `command:` strings, so an unsubstituted `${EDIKT_HOOK_DIR}` placeholder silently fails every hook it appears in; a `$HOME`-form path (the global-mode shape) is legitimate but only if the resulting path actually exists. |

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
 [ok]   docs/architecture/decisions/ — 4 ADRs
 [ok]   .claude/rules/ — 3 packs installed
 [!!]   go.md outdated (installed: 1.0, available: 1.2) — run /edikt:gov:rules-update
 [ok]   CLAUDE.md has edikt sentinel
 [ok]   SessionStart hook is git-aware
 [ok]   Memory: 2 days old, 45/200 lines
 [ok]   Sidecar verify coverage — 28/40 sidecars (70%); items 84/510 (16%)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 14 passed, 1 warning, 0 failures
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Recommendations:
  1. go.md outdated — run /edikt:gov:rules-update
```

## Launcher-level checks

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

Add `edikt_template_hash` to agents installed before v0.6.0:

```bash
edikt doctor --backfill-provenance
```

Assumes the installed file matches the template from the `edikt_version` recorded in your config. Review the proposed hashes before confirming. This enables the provenance-first upgrade flow for pre-v0.6.0 agents.

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

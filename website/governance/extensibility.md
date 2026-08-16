# Extensibility

edikt's governance is not take-it-or-leave-it. Every layer has extension points that let you add rules compile missed, reject rules you disagree with, override templates, and customize agents — without forking edikt or fighting the compile pipeline.

## Directive extension points

### Manual directives — add what compile missed

Add rules to the co-located `<artifact>.edikt.yaml` sidecar's `manual_directives:` list. Compile never overwrites this list — your rules always ship into the topic file.

```yaml
# docs/architecture/invariants/INV-012-tenant-isolation.edikt.yaml
schema_version: 2
topic: database
path: INV-012-tenant-isolation.md
signals: [sql, tenant, repository]
directives:
  - text: "Every SQL query MUST include `tenant_id`. No exceptions. (ref: INV-012)"
    source_excerpts:
      - line_start: 88
        line_end: 88
        quote: "Every SQL query MUST include `tenant_id` in the WHERE clause. No exceptions."
manual_directives:
  - text: "All new tables MUST include a `created_at` timestamp column (ref: team convention)"
  - text: "Every migration file MUST be idempotent — use IF NOT EXISTS (ref: team convention)"
suppressed_directives: []
```

When to use: the sidecar-extractor reads `## Decision` / `## Statement` / `## Rules` from the prose. If your governance includes rules that aren't in those sections (team conventions, verbal agreements, lessons from incidents), add them as manual directives directly on the sidecar.

Manual directives are scored by `/edikt:invariant:review` and `/edikt:gov:score` to the same quality standard as auto-generated ones. Soft language, missing references, and conflicts are flagged.

### Suppressed directives — reject what you disagree with

Add auto-generated directives you want to filter out to the sidecar's `suppressed_directives:`. Compile keeps regenerating them (the prose hasn't changed), but `/edikt:gov:compile` filters them out via the merge formula:

```
effective_rules = (directives - suppressed_directives) ∪ manual_directives
```

```yaml
suppressed_directives:
  - text: "NEVER use package-level mutable var. (ref: ADR-003)"
```

This directive will not appear in the topic file even though the extractor keeps generating it from the prose. To un-suppress, remove the entry from `suppressed_directives:`.

When to use: when the extractor generates a rule that's technically correct but doesn't apply to your project (e.g., "NEVER use global state" but your project legitimately uses a global logger initialized in main).

### Editing sidecar directives directly

`manual_directives:` and `suppressed_directives:` are the supported extension points above. You can also edit the extractor-owned `directives[]` list by hand — see [Sidecar Architecture → Editing rules manually](sidecar#editing-rules-manually) for when that sticks and when compile regenerates over it.

## Template overrides

### How templates work

When you run `/edikt:adr:new`, `/edikt:invariant:new`, or `/edikt:guideline:new`, edikt uses a template to structure the document. The lookup order:

1. **Project override** — `.edikt/templates/{artifact}.md` (if present)
2. **edikt default** — built-in template shipped with edikt

Your project templates are version-controlled, committed, and shared across your team. edikt never overwrites them during upgrade.

### Step-by-step: customizing a template

**1. Create the template directory** (if it doesn't exist):

```bash
mkdir -p .edikt/templates
```

**2. Copy the default template as a starting point:**

```bash
# The defaults live in your global edikt install
cp ~/.edikt/templates/adr.md .edikt/templates/adr.md
cp ~/.edikt/templates/invariant.md .edikt/templates/invariant.md
cp ~/.edikt/templates/guideline.md .edikt/templates/guideline.md
```

**3. Edit the template to match your team's needs:**

```markdown
# ADR-{NNN}: {title}

**Date:** {date}
**Status:** Draft
**Author:** {author}           ← added by your team
**Reviewers:** {reviewers}     ← added by your team

## Context
## Decision
## Consequences
## Alternatives Considered
## Migration Plan              ← added by your team
```

In v0.6.0+, the prose template has no in-body directives block — the sidecar is generated at a sibling path (`{ADR}.edikt.yaml`). See [Sidecar Architecture](sidecar).

**4. Commit the template:**

```bash
git add .edikt/templates/
git commit -m "chore: customize ADR template with author and migration plan"
```

Every future `/edikt:adr:new` will use your template. Every team member gets the same structure.

**5. Verify with doctor:**

```bash
/edikt:doctor
```

Doctor reports which templates are overridden:

```
Templates:
  adr.md          → project override (.edikt/templates/adr.md)
  invariant.md    → edikt default
  guideline.md    → edikt default
```

### What you can customize

- Section structure — add, remove, or reorder sections
- Default frontmatter fields — add `Author:`, `Reviewers:`, `Tags:`, `Team:`
- Writing guidance — the `<!-- ... -->` comments that guide the author
- Required sections — make certain sections mandatory for your team
- Example content — pre-fill sections with examples from your domain

### What you must NOT change

- **The key section names** that the sidecar extractor reads:
  - ADRs: `## Decision` — where the extractor finds enforceable statements
  - Invariants: `## Statement` — where the extractor finds constraints
  - Guidelines: `## Rules` — where the extractor lifts rule bullets
- Renaming these sections breaks extraction. You can add sections around them; don't rename them.
- Do not add an in-body `[edikt:directives:start/end]: #` block to a v0.6.0 template — the sidecar is the canonical location. `/edikt:gov:compile` refuses to run while any in-body block remains (run `edikt migrate sidecars` to lift legacy blocks).

### Configuration

Template paths are configured in `.edikt/config.yaml`:

```yaml
# .edikt/config.yaml
paths:
  templates: ".edikt/templates"    # default
  decisions: "docs/architecture/decisions"
  invariants: "docs/architecture/invariants"
  guidelines: "docs/guidelines"
```

Change `paths.templates` to put templates elsewhere (e.g., `docs/templates` for teams that prefer docs-adjacent configuration). All `:new` commands resolve templates from this path.

### What happens on upgrade

When you run `/edikt:upgrade`:
- **Project templates** (in `.edikt/templates/`) are **never overwritten** — your customizations survive every upgrade
- **edikt defaults** (in `~/.edikt/templates/`) are updated to the latest — if you haven't created a project override, you get the improvements automatically
- **Doctor** flags version differences: "Your project template for adr.md predates the current default. Run `diff` to see what changed."

## Rule pack overrides

### Override a rule pack

Place a file at `.edikt/rules/{name}.md` without the `edikt:generated` marker:

```markdown
# Error Handling

My team's error handling rules, replacing edikt's default.

- Every error MUST be wrapped with context: fmt.Errorf("operation: %w", err)
- NEVER return raw errors to HTTP clients
```

No `edikt:generated` marker means upgrade skips this file — your customization is preserved.

### Extend a rule pack

Add your rules to `.edikt/rules/{name}.md` but keep the `edikt:generated` marker if you want upgrade to refresh the base rules. Your additions will be merged during compile.

## Agent customization

### Custom marker

Mark an agent template with `<!-- edikt:custom -->` to prevent upgrade from overwriting it:

```markdown
---
name: security
description: "Our customized security agent with PCI-DSS focus"
---
<!-- edikt:custom -->

You are a security reviewer focused on PCI-DSS compliance...
```

### Config-based customization

List custom agents in `.edikt/config.yaml`:

```yaml
agents:
  custom:
    - security
    - compliance
```

Upgrade skips agents listed here, even without the HTML marker.

## Full config reference

All extensibility is configured in `.edikt/config.yaml`, created by `/edikt:init`:

```yaml
# .edikt/config.yaml

# Project identity
project_name: "my-service"
edikt_version: "0.7.0"

# Where governance artifacts live
paths:
  templates: ".edikt/templates"                # template overrides
  decisions: "docs/architecture/decisions"      # ADRs
  invariants: "docs/architecture/invariants"    # Invariant Records
  guidelines: "docs/guidelines"                 # team guidelines
  brainstorms: "docs/brainstorms"              # brainstorm artifacts (gitignored by default)
  plans: "docs/internal/plans"                 # execution plans
  specs: "docs/product/specs"                  # technical specs
  prds: "docs/product/prds"                    # product requirements
  reports: "docs/reports"                      # drift reports, audits

# Agent customization
agents:
  custom:
    - security        # never overwritten on upgrade
    - my-reviewer     # team-specific agent

# Feature toggles — see Configurable Features for the full key list
features: { ... }

# Stack detection (populated by init)
stack:
  languages: [go]
  frameworks: []
  databases: [postgres]
```

The `features:` block is documented key-by-key on [Configurable Features](/governance/features) — that page is the canonical reference for it.

Every `paths:` value is relative to the repo root. Change any path to match your project's directory structure. All commands resolve paths from this config.

### Committed vs private artifacts

Not all artifacts need to be in version control. edikt's `.gitignore` template (created by `/edikt:init`) gitignores working artifacts by default:

| Artifact | Default path | Committed? | Why |
|---|---|---|---|
| ADRs | `docs/architecture/decisions/` | Yes | Permanent record — team must see decisions |
| Invariant Records | `docs/architecture/invariants/` | Yes | Non-negotiable constraints — must be shared |
| Guidelines | `docs/guidelines/` | Yes | Team conventions — must be shared |
| Specs | `docs/product/specs/` | Yes | Engineering blueprint — must be shared |
| PRDs | `docs/product/prds/` | Yes | Requirements — must be shared |
| Compiled governance | `.claude/rules/governance*` | Yes | Claude reads these — must be in repo |
| **Brainstorms** | `docs/brainstorms/` | **No** | Scratchpad thinking — output formalizes into PRD/SPEC/PLAN |
| **Plans** | `docs/plans/` | **No** | Execution state — local to the session/engineer |
| **Reports** | `docs/reports/` | **No** | Drift reports, audits — ephemeral |

Brainstorms are private by default because they're working documents. The OUTPUT of a brainstorm — a PRD, SPEC, or PLAN — is what gets committed. The brainstorm itself is the messy thinking that got you there.

**To commit brainstorms** (if your team wants shared scratchpads): remove `docs/brainstorms/` from `.gitignore`. Or change the path in config to a committed location:

```yaml
paths:
  brainstorms: "docs/shared/brainstorms"  # not gitignored — team can see them
```

**To commit plans**: same pattern — remove `docs/plans/` from `.gitignore`.

See [Configurable Features](/governance/features) for feature toggle details.

## Checking your customizations

```bash
# See what's overridden
/edikt:doctor

# See which directives are manual vs auto
/edikt:gov:score

# Review manual directive quality
/edikt:invariant:review INV-012
```

## Next steps

- [Sentinel Blocks](sentinels) — the technical format behind directives
- [How Governance Compiles](compile) — the merge formula in action
- [Writing Invariants](writing-invariants) — how to write good source documents

# Custom Rules

Four levels of customization — from toggling packs to writing your own from scratch.

## 1. Toggle packs during init

When you run `/edikt:init`, the configure step shows all available rule packs. Toggle by name:

```text
> add api
> add database
> remove architecture
```

Or edit `.edikt/config.yaml` directly and re-run `/edikt:init`:

```yaml
rules:
  code-quality: { include: all }
  testing: { include: all }
  security: { include: all }
  error-handling: { include: all }
  go: { include: all }
  chi: { include: all }
  # api: { include: all }   ← uncomment to enable
```

## 2. Edit an installed pack

Every rule file in `.claude/rules/` is plain markdown you can edit directly. Add project-specific rules, remove ones that don't apply, or adjust phrasing.

```markdown
# Go

<!-- edikt:generated -->
...existing edikt rules...

## Project-Specific

- All monetary amounts MUST use `decimal.Decimal`, never `float64` — floating point causes rounding errors in financial calculations.
- Database calls MUST only appear in the adapter layer, never in domain — keeps the domain testable without infrastructure.
```

**Important:** When you edit a rule file, edikt detects the change. If you remove the `<!-- edikt:generated -->` tag, `/edikt:init` and `/edikt:rules-update` will never overwrite that file — your edits are protected. If the tag is still present, updates will regenerate the file from the template.

## 3. Override an edikt template

If you want to customize a rule pack for your whole team (not just one project), create a template override:

```text
.edikt/templates/go.md
```

When `/edikt:init` runs, it checks `.edikt/templates/{name}.md` before reading the global template at `~/.edikt/templates/rules/`. If a project override exists, it uses that instead.

This lets you maintain a team-specific version of any edikt rule pack — committed to git, shared across engineers — while still getting updates for packs you haven't overridden.

## 4. Write your own rule pack

Drop any `.md` file into `.claude/rules/` with `paths:` frontmatter:

```markdown
---
paths: "internal/billing/**/*.go"
---

# Billing Domain Rules

Rules specific to the billing bounded context.

## Critical

- MUST use `decimal.Decimal` for all monetary amounts — floating point causes rounding errors that compound across transactions.
- MUST require an idempotency key for every charge mutation — duplicate charges are the #1 billing support ticket.
- NEVER write directly to the charges table — all mutations go through `ChargeService` which handles idempotency, audit logging, and event emission.

## Standards

- Refunds go through `RefundService`, never direct DB writes.
- All billing events publish to the `billing.events` topic with the charge ID as the partition key.
```

Claude reads this file automatically when editing files matching `internal/billing/**/*.go`.

**Writing effective custom rules:**
- Use MUST/NEVER for hard constraints, with a one-clause reason
- Name specific types, functions, or patterns — not vague advice
- Keep to 15-25 rules per file (compliance degrades beyond that)
- Run `/edikt:review-governance` to check your rule language quality

## Linter-based rules

If your project has linter configs (`.golangci-lint.yaml`, `.eslintrc`, `ruff.toml`, `.rubocop.yml`, `biome.json`), edikt can translate them into Claude rule packs:

```
/edikt:sync
```

This creates `.claude/rules/linter-{name}.md` files that teach Claude what your linter enforces — so it writes code that passes linting on the first try instead of fixing violations after the fact.

## Monorepo rules

For monorepos with different standards per package, scope rules using `paths:` frontmatter:

```
.claude/rules/
├── code-quality.md              ← **/*.{go,ts,...}
├── services-billing.md          ← internal/billing/**/*.go
├── services-notifications.md    ← internal/notifications/**/*.go
└── web.md                       ← web/**/*.{ts,tsx}
```

Each rule file only loads when Claude edits files matching its path pattern. A Go backend rule won't load when editing TypeScript frontend code.

## Community rule packs

edikt ships with 20 rule packs covering common stacks. Community-contributed domain-specific packs (fintech, healthcare, e-commerce, infrastructure) are coming soon.

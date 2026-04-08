# /edikt:gov:sync

Translate linter configs into Claude rule packs — so Claude enforces the same standards your linter does.

## Usage

```bash
/edikt:gov:sync              ← sync all detected linter configs
/edikt:gov:sync golangci     ← sync only golangci-lint rules
/edikt:gov:sync eslint       ← sync only ESLint rules
/edikt:gov:sync ruff         ← sync only Ruff rules
/edikt:gov:sync rubocop      ← sync only RuboCop rules
/edikt:gov:sync biome        ← sync only Biome rules
/edikt:gov:sync --dry-run    ← preview what would be generated
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Detect all linters and sync rules for all found |
| `golangci` or `go` | Sync only golangci-lint rules |
| `eslint` | Sync only ESLint rules |
| `ruff` or `python` | Sync only Ruff rules |
| `rubocop` or `ruby` | Sync only RuboCop rules |
| `biome` | Sync only Biome rules |
| `--dry-run` | Show what would be generated without writing files |
| `{linter} --dry-run` | Preview output for a specific linter only |

## What it does

Reads your existing linter configs (golangci-lint, ESLint, Ruff, RuboCop, Biome, PHP CS Fixer) and generates `.claude/rules/linter-*.md` files that teach Claude the same rules — with plain-English explanations of why each rule exists.

The result: Claude writes code that passes your linter on the first attempt, not after several correction rounds.

## Supported linters

| Linter | Config file |
|--------|------------|
| golangci-lint | `.golangci-lint.yaml`, `.golangci.yaml` |
| ESLint | `.eslintrc*`, `eslint.config.*` |
| Ruff | `ruff.toml`, `pyproject.toml [tool.ruff]` |
| RuboCop | `.rubocop.yml` |
| Biome | `biome.json` |
| PHP CS Fixer | `.php-cs-fixer.php`, `.php-cs-fixer.dist.php` |

## Monorepo support

In monorepos with per-package linter configs, edikt generates scoped rules with path prefixes so the right rules apply to the right directories.

## Generated file format

```markdown
---
source: .golangci-lint.yaml
generated-by: edikt:gov:sync
linter: golangci-lint
paths: ["**/*.go"]
---

# Linter Rules: Go

<!-- edikt:generated -->

## cyclop — Cyclomatic Complexity
Keep functions simple. Max complexity: 10.
...
```

## When to run

- After adding or changing a linter config
- After running `/edikt:init` on an established project (it runs sync automatically)
- When `/edikt:doctor` warns "linter config changed since last sync"

## Natural language triggers

- "sync linter rules"
- "update Claude with our lint config"
- "translate our linter config"

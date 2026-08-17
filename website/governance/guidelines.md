# Guidelines

Guidelines document team conventions and preferences — the patterns that should be consistent across the codebase but aren't hard constraints. They fill the gap between [ADRs](architecture-decisions) (decisions with alternatives considered, formalized by [Michael Nygard](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)) and [Invariant Records](invariant-records) (non-negotiable constraints).

Guidelines are edikt's lightest governance artifact. They compile into the same directive format as ADRs and invariants but carry lower enforcement weight — they never land in the ambient core's non-negotiable section. Where the compiled text itself lives follows the same single-home rule as any other artifact (ADR-066): a guideline scoped to specific paths renders into `directive-index.yaml` for write-time delivery; only a fully unscoped guideline (no `paths:`) renders into its topic file's body.

## When to use a guideline

Use a guideline when:
- The team agreed on a pattern but it's not architecturally critical
- Breaking the convention is annoying, not dangerous
- There are legitimate exceptions the team can judge case-by-case
- The pattern is about consistency, not correctness

**Examples:**
- `error-handling.md` — "Wrap errors with `fmt.Errorf("context: %w", err)`"
- `api-design.md` — "Use camelCase for JSON responses, snake_case for DB columns"
- `naming.md` — "Services end with `Service`, repositories end with `Repository`"
- `testing.md` — "Test the error path first, then the happy path"

## Guidelines vs ADRs vs Invariant Records

| Question | If yes → |
|---|---|
| Is this a one-time decision with alternatives considered? | ADR |
| Is this a hard constraint that can never be violated? | Invariant Record |
| Is this a team convention we want to keep consistent? | Guideline |

## The template

```markdown
# {topic-name}

## Purpose

Why this guideline exists. What consistency problem it solves.

## Rules

- Every HTTP handler MUST return `Content-Type: application/json` on success and error
- Error responses MUST use the format `{"error": "message", "code": "ERROR_CODE"}`
- NEVER return raw exception messages to the client

## Examples

### Good
{code example showing the convention followed}

### Bad
{code example showing the convention violated}

## When NOT to apply

- Health check endpoints may return plain text
- WebSocket endpoints have their own response format
```

Compiled directives live in a sibling `{topic-name}.edikt.yaml` sidecar — the prose body carries no in-body directives block in v0.6.0+. See [Sidecar Architecture](sidecar).

## How guidelines compile

The `## Rules` section is the source. Each bullet becomes a directive. Compile enforces quality:

- Bullets using MUST/NEVER → compiled as directives
- Bullets using soft language ("should", "prefer", "try to") → **rejected with a warning**

```
⚠ Skipped soft rule in api-design.md: "Responses should be consistent"
  Guidelines should use MUST/NEVER. Either rewrite the rule or omit it.
```

This is intentional. If you can't phrase it as MUST/NEVER, it might not be a rule — it might be a preference that belongs in a comment, not in governance.

Guidelines also generate reminders and verification checklist items, the same as ADRs and Invariant Records.

## Commands

| Command | What it does |
|---|---|
| `/edikt:guideline:new` | Create a new guideline |
| `/edikt:guideline:compile` | Regenerate the guideline's sidecar (`<slug>.edikt.yaml`) |
| `/edikt:guideline:review` | Review rule quality + directive LLM compliance |

## Next steps

- [Architecture Decisions](architecture-decisions) — for one-time decisions with alternatives
- [Invariant Records](invariant-records) — for hard constraints
- [How Governance Compiles](compile) — the full pipeline
- [Extensibility](extensibility) — manual directives, suppressed directives

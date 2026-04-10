# Writing Architecture Decision Records — a guide

This guide teaches you how to write ADRs that compile into effective governance directives. For the formal template and lifecycle, see [Architecture Decision Records](architecture-decisions). This guide is about writing ones that actually get followed.

## The Decision section matters most

The compile pipeline reads the `## Decision` section. Everything else (Context, Consequences, Alternatives) is for humans. The Decision section serves both audiences — write it accordingly.

### For humans: explain what you decided

A reader who wasn't in the room should understand the decision in 60 seconds. Lead with the choice, then the key constraints:

```markdown
## Decision

We will use hexagonal architecture with strict dependency direction.

The domain layer has zero infrastructure imports. Business logic lives
in domain/. Adapters (database, HTTP, external services) live in adapter/.
The service layer sits between domain and adapters, accepting repository
interfaces — never concrete types.
```

### For compile: use MUST/NEVER language

The compile pipeline extracts statements with MUST/NEVER as hard directives. Statements without them become weaker directives. Be explicit about what's enforced:

```markdown
## Decision

We will use hexagonal architecture with strict dependency direction.

The domain layer MUST NOT import from infrastructure packages — no
database/sql, no net/http, no framework types in domain/.

ALL database access MUST go through the repository layer. NEVER
write SQL in handlers or services.

Services MUST accept repository interfaces, NEVER concrete types.
```

Both paragraphs say the same thing. The second compiles into sharp directives with literal code tokens. The first compiles into vague guidance.

## Name specific things

Every directive is stronger when it names the exact thing:

| Weak | Strong |
|---|---|
| "Use proper error handling" | "Wrap errors with `fmt.Errorf("context: %w", err)`" |
| "Keep the API consistent" | "Every HTTP handler MUST return `Content-Type: application/json`" |
| "Don't put logic in handlers" | "Handler functions MUST be under 30 lines — decode, call service, encode" |
| "Use the right ID format" | "Primary keys MUST use UUIDv7: `uuid.Must(uuid.NewV7())`" |

The compile pipeline counts backtick-wrapped code tokens as a quality signal. More tokens = higher specificity score in `/edikt:adr:review`.

## One decision per directive

Split compound statements:

```
Bad (one directive, two decisions):
  "Use CQRS and event sourcing for the order domain"

Good (two directives):
  "Use CQRS for write/read separation in the order domain. (ref: ADR-005)"
  "Order state changes MUST be captured as immutable event records. (ref: ADR-005)"
```

## Make it verifiable

Can you check compliance with a grep command or code review with specific criteria? If not, the directive is unenforceable:

| Unverifiable | Verifiable |
|---|---|
| "The code should be well-organized" | "`domain/` MUST NOT import from `adapter/`, `handler/`, or `service/`" |
| "Tests should be thorough" | "Every repository method MUST have a test that rejects empty tenant ID" |
| "The API should handle errors" | "No handler returns `err.Error()` to the client" |

The compile pipeline generates verification checklist items from verifiable directives. Unverifiable ones get flagged by `/edikt:adr:review`.

## Check your ADR quality

After writing, run:

```bash
/edikt:adr:review ADR-003
```

This scores both the human-side quality (specificity, actionability, phrasing, testability) and the directive-side LLM compliance (token specificity, MUST/NEVER, grep-ability, ambiguity). Directives scoring below 5/10 get concrete rewrite suggestions.

Then compile and check the aggregate:

```bash
/edikt:adr:compile ADR-003
/edikt:gov:compile
/edikt:gov:score
```

## Common traps

**Trap 1: All context, no decision.** The Decision section describes the problem instead of the choice. Fix: start with "We will..." or "The system MUST..."

**Trap 2: Decision uses soft language.** "We should probably use..." doesn't compile into a strong directive. Fix: if you decided it, use MUST/NEVER.

**Trap 3: Decision is too abstract.** "Use clean architecture" means different things to different people. Fix: name the layers, the import rules, the file locations.

**Trap 4: Multiple decisions in one ADR.** If your ADR covers three unrelated decisions, split it. Each ADR should have one clear scope.

**Trap 5: No Alternatives section.** If there were no alternatives, it's not a decision — it's a constraint (use an Invariant Record instead).

## Next steps

- [Architecture Decisions](architecture-decisions) — template and lifecycle
- [How Governance Compiles](compile) — the full pipeline
- [Sentinel Blocks](sentinels) — the technical format
- [Writing Invariant Records](writing-invariants) — the counterpart guide for invariants

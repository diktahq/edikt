# {Topic Title} Guidelines

**Purpose:** {One sentence — what problem this guideline prevents.}

<!--
Extended guideline template (edikt convention).

Extends the minimal guideline with Rationale, Examples (correct and
incorrect), and a When-NOT-to-apply section. Use when the rules
benefit from concrete code or prose examples, or when the guideline
has edge cases where it shouldn't apply.

Writing guidance:
1. Every rule is a MUST or a NEVER. Soft language does not compile
   into governance.
2. Rules must be specific enough to verify automatically or during
   code review.
3. Examples should be minimal — just enough to show the pattern.
4. When-NOT-to-apply is for legitimate edge cases, not loopholes.

See guideline-minimal.md for a shorter version without Rationale
and Examples.
-->

## Rationale

{Why this guideline exists. What failure mode does it prevent? What
did we learn that makes these rules non-negotiable within this
topic? Implementation-agnostic where possible.}

## Rules

- {Rule 1 using MUST or NEVER — specific and verifiable}
- {Rule 2 using MUST or NEVER — specific and verifiable}
- {Rule 3 using MUST or NEVER — specific and verifiable}

## Examples

### Correct

```{language}
{Minimal code or prose example showing the right approach. Comment
the interesting parts if needed.}
```

### Incorrect

```{language}
{Minimal code or prose example showing what to avoid. Briefly note
why this violates the rules — one or two sentences.}
```

## When NOT to apply

{Legitimate edge cases where the rules should not apply. Be specific:
name the exact conditions and the reason the rules are relaxed.
This section must not become a list of loopholes — if an exception
swallows the rule, the rule isn't a guideline, it's a suggestion.

Examples of valid exceptions:
- "These rules do not apply to generated code under `build/`"
- "These rules do not apply to test fixtures under `testdata/`"

Examples of invalid exceptions (loopholes — do not include):
- "Except when it's inconvenient"
- "Where time-to-ship matters"}

---

*Created by edikt:guideline — {date}*

<!-- Directives for edikt governance. Populated by /edikt:guideline:compile. -->
[edikt:directives:start]: #
[edikt:directives:end]: #

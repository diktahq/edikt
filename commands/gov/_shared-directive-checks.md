---
name: edikt:gov:_shared-directive-checks
description: "Shared directive-quality sub-procedure — not a top-level command"
effort: low
---

> ⚠ Not a top-level command. Called by `/edikt:gov:compile` and `/edikt:gov:review`. Do not invoke directly.

# Shared Directive-Quality Checks

This sub-procedure runs three static quality checks over a parsed directive and returns a list of warnings. It is the single source of truth for FR-003a (length vs canonical_phrases), FR-003b (phrase-not-in-body), and AC-003c (no-directives reason validation).

Both callers (`/edikt:gov:compile` and `/edikt:gov:review`) invoke this procedure for every accepted ADR and active invariant, then surface the resulting warnings inline in their own output. Centralising the checks here prevents drift between the two callers — the same input always produces the same warning.

## Inputs

The procedure operates on one parsed directive at a time:

| Field | Type | Description |
|---|---|---|
| `adr_id` | string | The ADR or INV identifier (e.g. `ADR-012`, `INV-001`) |
| `directive_body` | string | The full directive text from the sentinel block's `directives:` or `manual_directives:` list |
| `canonical_phrases` | list\<string\> | The `canonical_phrases` list from the sentinel block (empty list if absent) |
| `no_directives_reason` | string or null | The value of the frontmatter `no-directives:` key, or null if absent |

The sentinel block is already parsed by the time this procedure runs — callers read and parse blocks per the schema in `commands/gov/compile.md` §11 before invoking these checks.

## Check A — FR-003a: Length vs canonical_phrases

Count the number of declarative sentences in `directive_body`:

1. Strip the `(ref: …)` tail from the body: remove everything matching `\s*\(ref:[^)]+\)\s*$` from the end of the string.
2. Split the remaining text on the sentence-termination characters `. ` (period + space), `; ` (semicolon + space), `! ` (exclamation + space), and `? ` (question + space). Also split on a trailing `.`, `!`, or `?` at the end of the string.
3. Count non-empty clauses after the split.
4. If `count > 1` AND `canonical_phrases` is empty:
   ```
   [WARN] {adr_id}: directive has {count} sentences but no canonical_phrases — run /edikt:adr:review --backfill
   ```
5. If `count == 1` or `canonical_phrases` is non-empty: no warning for this check.

**v0.6.0 grace period (AC-021):** this check is warn-only. No compile or review is blocked solely because of this warning.

## Check B — FR-003b: Canonical phrase substring match

For each phrase in `canonical_phrases`:

1. Strip leading and trailing whitespace from the phrase.
2. Check whether the phrase is a **case-insensitive substring** of `directive_body`.
3. If not found:
   ```
   [WARN] {adr_id}: canonical_phrase "{phrase}" not found in directive body
   ```
4. If all phrases are found (or `canonical_phrases` is empty): no warnings for this check.

**Case-insensitivity:** compare `phrase.lower()` against `directive_body.lower()` — do not normalise unicode beyond standard Python `.lower()`.

## Check C — AC-003c: no-directives reason validator

This check runs only when `no_directives_reason` is non-null.

1. Strip leading and trailing whitespace from the reason.
2. Check **all three** conditions. If any condition fails, emit a warning:
   - `len(stripped_reason) < 10` → too short
   - `stripped_reason.lower()` is one of `{"tbd", "todo", "fix later"}` → forbidden placeholder
   - `stripped_reason == ""` → empty after strip
3. If any condition fails:
   ```
   [WARN] {adr_id}: no-directives reason "{reason}" is not acceptable — provide a meaningful explanation ≥ 10 characters
   ```
4. If all conditions pass: no warning for this check.

If `no_directives_reason` is null (the key is absent from frontmatter): skip this check entirely — it is not an error for an ADR to omit the key.

## Output

The procedure returns a list of warning strings, one per triggered condition across all three checks. Each warning is a single line matching one of the three formats above. An empty list means the directive is clean.

**Callers must not alter warning text** — downstream tests (`test/integration/test_shared_directive_checks.py`) assert exact substrings to verify that compile and review produce identical output for the same input.

## Inline Script

The following Python script is the canonical implementation of the three checks. Callers embed it as a heredoc and invoke it with a JSON payload on stdin.

**Input contract (stdin):** a JSON object with keys `adr_id`, `directive_body`, `canonical_phrases` (list), and `no_directives_reason` (string or null).

**Output contract (stdout):** one warning line per triggered condition, or nothing on a clean directive. **Exit 0 always** — this script never blocks a caller.

```bash
python3 - <<'PY'
import json
import re
import sys

payload = json.loads(sys.stdin.read())
adr_id = payload["adr_id"]
body = payload["directive_body"]
phrases = payload.get("canonical_phrases") or []
no_dir_reason = payload.get("no_directives_reason")

warnings = []

# ── Check A: length vs canonical_phrases ──────────────────────────────────────
# Strip (ref: …) tail
stripped_body = re.sub(r'\s*\(ref:[^)]+\)\s*$', '', body.rstrip())

# Split on sentence terminators (. ; ! ?) followed by a space, or at end of string
clauses = re.split(r'(?<=[.;!?])\s+|(?<=[.;!?])$', stripped_body)
clauses = [c.strip() for c in clauses if c.strip()]
sentence_count = len(clauses)

if sentence_count > 1 and not phrases:
    warnings.append(
        f'[WARN] {adr_id}: directive has {sentence_count} sentences but no canonical_phrases'
        f' — run /edikt:adr:review --backfill'
    )

# ── Check B: phrase-not-in-body ───────────────────────────────────────────────
body_lower = body.lower()
for phrase in phrases:
    p = phrase.strip()
    if p and p.lower() not in body_lower:
        warnings.append(
            f'[WARN] {adr_id}: canonical_phrase "{p}" not found in directive body'
        )

# ── Check C: no-directives reason ─────────────────────────────────────────────
if no_dir_reason is not None:
    reason = str(no_dir_reason).strip()
    forbidden = {"tbd", "todo", "fix later"}
    if not reason or len(reason) < 10 or reason.lower() in forbidden:
        warnings.append(
            f'[WARN] {adr_id}: no-directives reason "{no_dir_reason}" is not acceptable'
            f' — provide a meaningful explanation \u2265 10 characters'
        )

for w in warnings:
    print(w)

sys.exit(0)
PY
```

## Invocation Protocol

Callers run the script for every accepted ADR and active invariant in the source document set. For ADRs with a sentinel block, call the script once per directive in the `directives:` list and once per directive in `manual_directives:`. Use the same `adr_id` for all directives from the same source document.

The `no_directives_reason` field is read from the source document's YAML frontmatter (the `no-directives:` key). If absent, pass `null`.

### Example invocation (shell)

```bash
echo '{
  "adr_id": "ADR-012",
  "directive_body": "All DB access MUST go through the repository layer. NEVER bypass the repository.",
  "canonical_phrases": ["repository layer", "NEVER bypass"],
  "no_directives_reason": null
}' | python3 - <<'PY'
# ... (script body from above)
PY
```

### Collecting and surfacing results

```
Compile context:
  After each directive is checked, collect all returned warning lines.
  Surface them under a "### Directive-quality warnings" header
  in the compile output, after the contradiction-detection pass.
  Exit 0 even when warnings are present (AC-021 grace period).

Review context:
  Surface warnings in the review report alongside other directive-quality
  findings (specificity, actionability, phrasing, testability).
  Warnings from this sub-procedure appear under a "Directive-quality checks"
  sub-heading within the per-document review section.
```

## Cross-Caller Consistency

Both callers MUST produce identical warning text for the same input. This is verified by `test/integration/test_shared_directive_checks.py`, which:

1. Builds a fixture repo with directives that trigger each check.
2. Runs compile and review against the same repo.
3. Asserts both callers emit the same warning strings (exact substring match, not equality — callers may wrap warnings in their own formatting but must not alter the warning text itself).

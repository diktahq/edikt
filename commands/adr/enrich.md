---
name: adr:enrich
description: "Interactively add a manual directive to an ADR or Invariant sidecar without editing the parent .md"
tier: 1
tier_2_dependency: edikt
on_absent: refuse-and-direct-user
effort: quick
argument-hint: "[ADR-NNN | INV-NNN | path]"
allowed-tools:
  - Read
  - Bash
  - Glob
  - Grep
stops:
  - id: auto-tag
    class: selection
    default: "yes"
    options: ["yes", "no"]
    on_silence: take-default
    prompt: "The directive text carries no (ref: ...) parenthetical — add one?"
---

# edikt:adr:enrich

Add a `manual_directives` entry to an existing ADR or Invariant sidecar without touching the parent `.md`. Interactive: resolves the sidecar, shows current manual directives, prompts for new text, refuses soft modal force, auto-suggests `(ref: ADR-NNN + manual)` if missing, then delegates write to `edikt sidecar add-manual-directive`.

## Arguments

- `$ARGUMENTS` — optional. One of:
  - `ADR-NNN` or `INV-NNN` — resolved to `{decisions_dir}/ADR-NNN-*.edikt.yaml` or `{invariants_dir}/INV-NNN-*.edikt.yaml`
  - An absolute or repo-relative path to an `.md` or `.edikt.yaml` file
  - Empty — prompts for the ADR/INV ID interactively

Both halves of enriching an immutable artifact live here: ADDING a manual
directive it should carry, and WITHDRAWING an extracted one a later record has
replaced (Step 8b).

## Steps

### 1. Verify binary presence

```bash
command -v edikt >/dev/null 2>&1 || { echo "error: edikt not found — run: edikt install edikt"; exit 1; }
```

If `edikt` is absent, refuse and direct the user to install:

```
error: edikt not on PATH.
Install it: edikt install edikt
```

### 2. Read config

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

Otherwise read `.edikt/config.yaml` to resolve:
- `decisions_dir` (default `docs/architecture/decisions`)
- `invariants_dir` (default `docs/architecture/invariants`)

### 3. Resolve target sidecar

If `$ARGUMENTS` is empty, ask the user:
```
Which ADR or Invariant? (e.g. ADR-NNN, or a path):
```

**Resolve logic:**

- `ADR-NNN` → find `{decisions_dir}/ADR-NNN-*.edikt.yaml`. If multiple match, error; if none, error with `error: no sidecar found for {ARGUMENTS}`.
- `INV-NNN` → find `{invariants_dir}/INV-NNN-*.edikt.yaml`.
- A `.md` path → sibling `.edikt.yaml` (same basename, `.edikt.yaml` suffix).
- A `.edikt.yaml` path → use directly.

Verify the resolved sidecar file exists. If not, report the path and stop.

### 4. Show current manual directives

Read the resolved sidecar. Display existing `manual_directives[]` (if any):

```
Sidecar: docs/architecture/decisions/ADR-NNN-sidecar-architecture.edikt.yaml
Topic:   architecture

Current manual_directives:
  (none)
```

Or, if entries exist:
```
Current manual_directives:
  [0] MUST co-locate sidecar with parent artifact (ref: ADR-NNN + manual)
```

### 5. Prompt for new directive text

Ask:
```
New directive text (MUST / MUST NOT / SHOULD / SHOULD NOT / MAY / NEVER / ALWAYS required):
```

Read the user's input.

### 6. Validate modal verb — BINDING FORCE ONLY

The text MUST state its obligation in binding RFC-2119 force (case-insensitive):
`MUST`, `MUST NOT`, `REQUIRED`, `SHALL`, `SHALL NOT`

**`SHOULD`, `SHOULD NOT` and `MAY` are NOT acceptable** as the operative verb of
an authored directive. This step used to accept them, which is how the
invariant came to be enforced only by a later CI scan — the author was told
their soft directive was fine at the moment they wrote it.

Grade is DERIVED from modal force. An authored `SHOULD` compiles to `advisory`
and ships as a suggestion: the rule exists, counts toward every coverage and
density number, and binds nothing. That is worse than an absent rule, which is
at least visible as absent.

Refuse anything hedged — `ideally`, `preferably`, `where possible`, `try to`,
`aim to`, `consider`, `it is recommended that`, or `SHOULD` as the operative
verb:

```
error: a directive states an obligation, not a preference.

  Grade is derived from modal force, so "should"/"ideally" compiles to
  advisory and ships as a suggestion — counted everywhere, binding nowhere.

  Rewrite as:
    "MUST <action>"      — mandate
    "MUST NOT <action>"  — prohibition

  A rule that cannot be stated in binding terms is not yet a rule. Write it
  down as a guideline instead, or do not write it down.
```

A directive that DOES carry binding force is not refused for containing a hedge
elsewhere in the sentence — `MUST NOT retry unless the operator should be
notified` is binding. Refusing it would push authors toward weaker phrasing to
satisfy the checker.

`edikt sidecar add-manual-directive` applies the same rule and exits 1, so
a caller that skips this step still cannot write a soft directive. Both write
points, not one.

### 7. Auto-suggest (ref: ADR-NNN + manual)

If the text does not contain `(ref:`, show a suggestion:

<!-- stop: auto-tag -->
```
The text has no (ref: ...) parenthetical.
Auto-tag? [Y/n]:
```

With no human to answer, take the declared default (`yes`) and record that it
was taken by default rather than answered.

If the user accepts (or hits Enter), append ` (ref: ADR-NNN + manual)` — where ADR-NNN is the ID from the resolved sidecar filename. If the user declines, use the text as-is. `edikt sidecar add-manual-directive` will auto-tag regardless if still absent; this step is advisory UX only.

### 8. Invoke edikt sidecar add-manual-directive

```bash
EDIKT_INVOKER=/edikt:adr:enrich edikt sidecar add-manual-directive \
  --path "<resolved-sidecar-path>" \
  --text "<directive-text>"
```

Display the command's stdout verbatim. Do not parse its output — exit code only.

**Exit code handling:**
- 0 — success; print the result line from the binary.
- 1 — validation error; display binary stderr and stop.
- 2 — sidecar not found; display binary stderr and stop.
- 3 — duplicate detected; display binary stderr and offer to show current `manual_directives` again.
- other — unexpected error; display binary stderr and stop.

### 8b. Or WITHDRAW a directive the artifact still emits

An accepted artifact is immutable, and the extractor keeps emitting every
directive its prose supports. When a later accepted record withdraws one of
those clauses, the corpus carries two active directives requiring opposite
things and the earlier one cannot be edited away.

Withdrawing is the other half of this command. Ask which it is before Step 5:

```
Are you ADDING a rule this artifact should carry, or WITHDRAWING one it still
emits that a later record has replaced?
  [a] add       — a manual directive (Steps 5–9)
  [w] withdraw  — suppress an extracted directive (this step)
```

On `w`, show the artifact's extracted directives with an index, take the one
to withdraw, and ask which record withdrew it and why. Then:

```bash
EDIKT_INVOKER=/edikt:adr:enrich edikt sidecar suppress-directive \
  --path "$SIDECAR" \
  --text "$EXACT_DIRECTIVE_TEXT" \
  --reason "$WHICH_RECORD_AND_WHY"
```

The match is EXACT against the directive's text, so pass it through verbatim —
copy it from the listing rather than retyping it. Never hand-edit the sidecar
YAML to do this: quoting a directive that contains an apostrophe is exactly
how a hand-edit corrupts the file, which is the reason this command exists.

Exit codes: `0` written · `1` validation · `2` sidecar missing · `3` the text
matches no directive, or it is already suppressed. On `3`, show the message
verbatim — a suppression that matches nothing suppresses nothing, and the
compile reports it as a warning rather than silently doing nothing.

The directive itself is NOT deleted: the parent is immutable and the next
extraction would emit it again. The subtraction happens when the corpus is
compiled. The reason is recorded under `docs/internal/suppressions/` so a
later reader knows which record withdrew the clause.

### 9. Confirm

On exit 0:
```
✅ Manual directive appended to {sidecar-path}

Next: run /edikt:gov:compile to include it in the next governance build.
```

## Why this command exists

`manual_directives[]` in a sidecar is the only way to add a MUST/MUST NOT rule to a governance artifact after acceptance — the parent `.md` is immutable. Without tooling, authors hand-edit YAML, which introduces formatting errors and skips the duplicate check. This command makes the editorial-enrichment pattern friction-free and correct by default.

The Phase 4 doctor WARN names `edikt sidecar add-manual-directive` as the remediation for ADRs with considered options but no prohibition coverage. This command is the interactive face of that remediation.

---
name: adr:enrich
description: "Interactively add a manual directive to an ADR or Invariant sidecar without editing the parent .md (INV-002)"
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
---

# edikt:adr:enrich

Add a `manual_directives` entry to an existing ADR or Invariant sidecar without touching the parent `.md` (INV-002). Interactive: resolves the sidecar, shows current manual directives, prompts for new text, validates modal verb, auto-suggests `(ref: ADR-NNN + manual)` if missing, then delegates write to `bin/edikt sidecar add-manual-directive`.

## Arguments

- `$ARGUMENTS` — optional. One of:
  - `ADR-NNN` or `INV-NNN` — resolved to `{decisions_dir}/ADR-NNN-*.edikt.yaml` or `{invariants_dir}/INV-NNN-*.edikt.yaml`
  - An absolute or repo-relative path to an `.md` or `.edikt.yaml` file
  - Empty — prompts for the ADR/INV ID interactively

## Steps

### 1. Verify binary presence (ADR-029 Rule 1)

```bash
command -v bin/edikt >/dev/null 2>&1 || { echo "error: bin/edikt not found — run: edikt install edikt"; exit 1; }
```

If `bin/edikt` is absent, refuse and direct the user to install:

```
error: bin/edikt not on PATH.
Install it: edikt install edikt
```

### 2. Read config

Read `.edikt/config.yaml` to resolve:
- `decisions_dir` (default `docs/architecture/decisions`)
- `invariants_dir` (default `docs/architecture/invariants`)

### 3. Resolve target sidecar

If `$ARGUMENTS` is empty, ask the user:
```
Which ADR or Invariant? (e.g. ADR-027, INV-002, or a path):
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
Sidecar: docs/architecture/decisions/ADR-027-sidecar-architecture.edikt.yaml
Topic:   architecture

Current manual_directives:
  (none)
```

Or, if entries exist:
```
Current manual_directives:
  [0] MUST co-locate sidecar with parent artifact (ref: ADR-027 + manual)
```

### 5. Prompt for new directive text

Ask:
```
New directive text (MUST / MUST NOT / SHOULD / SHOULD NOT / MAY / NEVER / ALWAYS required):
```

Read the user's input.

### 6. Validate modal verb

The text MUST contain at least one of the recognized modal verbs (case-insensitive):
`MUST`, `MUST NOT`, `SHOULD`, `SHOULD NOT`, `MAY`, `NEVER`, `ALWAYS`

If none is present, refuse:
```
error: directive must contain a modal verb (MUST, MUST NOT, SHOULD, SHOULD NOT, MAY, NEVER, ALWAYS).
Governance directives state a constraint, not a preference. Rewrite as:
  "MUST <action>" — strong mandate
  "MUST NOT <action>" — prohibition
  "SHOULD <action>" — recommendation
  "MAY <action>" — optional
```

### 7. Auto-suggest (ref: ADR-NNN + manual)

If the text does not contain `(ref:`, show a suggestion:
```
The text has no (ref: ...) parenthetical.
Auto-tag? [Y/n]: 
```

If the user accepts (or hits Enter), append ` (ref: ADR-NNN + manual)` — where ADR-NNN is the ID from the resolved sidecar filename. If the user declines, use the text as-is. `bin/edikt sidecar add-manual-directive` will auto-tag regardless if still absent; this step is advisory UX only.

### 8. Invoke bin/edikt sidecar add-manual-directive

```bash
bin/edikt sidecar add-manual-directive \
  --path "<resolved-sidecar-path>" \
  --text "<directive-text>"
```

Display the command's stdout verbatim. Do not parse its output — exit code only (ADR-029 Rule 2).

**Exit code handling:**
- 0 — success; print the result line from the binary.
- 1 — validation error; display binary stderr and stop.
- 2 — sidecar not found; display binary stderr and stop.
- 3 — duplicate detected; display binary stderr and offer to show current `manual_directives` again.
- other — unexpected error; display binary stderr and stop.

### 9. Confirm

On exit 0:
```
✅ Manual directive appended to {sidecar-path}
   Run /edikt:gov:compile to include it in the next governance build.
```

## Why this command exists

`manual_directives[]` in a sidecar is the only INV-002-compliant way to add a MUST/MUST NOT rule to a governance artifact after acceptance — the parent `.md` is immutable. Without tooling, authors hand-edit YAML, which introduces formatting errors and skips the duplicate check. This command makes the editorial-enrichment pattern friction-free and correct by default.

The Phase 4 doctor WARN names `bin/edikt sidecar add-manual-directive` as the remediation for ADRs with considered options but no prohibition coverage. This command is the interactive face of that remediation.

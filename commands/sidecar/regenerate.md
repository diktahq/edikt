---
name: sidecar:regenerate
description: "Regenerate sidecars based on a migration regression manifest. Auto-fixes LOST items by re-running the extractor; writes FACTUAL/DEGRADED items to a worklist for manual review."
tier: 1
tier_2_dependency: edikt
on_absent: refuse-and-direct-user
allowed-tools:
  - Read
  - Write
  - Bash
  - Task
  - Glob
  - Grep
---

# /edikt:sidecar:regenerate

Consumes the strict-mode regression manifest produced by `bin/edikt migrate sidecars --strict --report-json <path>` and drives the fix flow:

- **LOST items** — directive, paths, or scope dropped during migration. Auto-fixed by dispatching the locked `sidecar-extractor` subagent (one per parent artifact, parallel up to N=4). The extractor rewrites the sidecar in place.
- **FACTUAL / DEGRADED items** — modality drift or verification abstraction. Written to `docs/internal/v060-manual-review.md` as a checkbox worklist for human review.

## Arguments

- `--manifest <path>` (default: `.edikt/state/v060-strict-report.json`)

## Steps

### 1. Verify binary presence (ADR-029 Rule 1)

```bash
command -v bin/edikt >/dev/null 2>&1 || command -v edikt >/dev/null 2>&1
```

If the check fails, print:

```
✗ bin/edikt not found.
  Install with: edikt install edikt  (or run install.sh to install the Go binary)
  Then re-run /edikt:sidecar:regenerate.
```

Stop. Do not proceed.

### 2. Resolve the manifest

Resolve `--manifest` from `$ARGUMENTS` or default to `.edikt/state/v060-strict-report.json`.

If the manifest file does not exist:

```bash
bin/edikt migrate sidecars --strict --report-json .edikt/state/v060-strict-report.json
EXIT=$?
```

- **Exit 0** — no regressions detected. Print `No regressions detected. Sidecars already current.` and stop.
- **Non-zero** — manifest written (or updated); proceed to Step 3.

If the manifest file already exists, use it directly.

### 3. Parse manifest and group items

Read the manifest JSON. Group items by `category`:

- `LOST` — items with `"category": "LOST"`
- `FACTUAL` — items with `"category": "FACTUAL"`
- `DEGRADED` — items with `"category": "DEGRADED"`

Print summary:

```
Manifest: {lost} LOST, {factual} FACTUAL, {degraded} DEGRADED across {total_artifacts} artifacts.
```

If all counts are zero, print `No regressions. Sidecars already current.` and stop.

### 4. Auto-regenerate LOST items

For each LOST item, extract the parent artifact path (the `path` field in the manifest item).

Deduplicate by parent artifact — multiple LOST items from the same artifact result in one subagent call, not one per item.

For each unique parent artifact path, dispatch a Task tool call (subagent) using the locked `sidecar-extractor` agent prompt from `templates/agents/sidecar-extractor.md`:

```
Task(
  description: "Extract sidecar for {artifact-path}",
  prompt: "{full content of templates/agents/sidecar-extractor.md}\n\n---\n\nInput file: {absolute-path-to-artifact.md}"
)
```

Run up to N=4 Task calls concurrently (batch them — do not await each one before dispatching the next). The extractor writes `<artifact>.edikt.yaml` in place and emits `SIDECAR WRITTEN: <path>` as its final line.

After all Task calls complete, collect the paths each extractor wrote. Print:

```
Auto-regenerated {K} sidecars:
  {path1}
  {path2}
  ...
```

### 5. Verify LOST items are resolved

Run strict check again:

```bash
bin/edikt migrate sidecars --strict --report-json /tmp/edikt-recheck.json
RECHECK_EXIT=$?
```

Per ADR-029 Rule 2: display binary output verbatim; do not parse it. Only inspect the exit code.

- **Exit 0** — LOST items resolved.
- **Non-zero** — print:
  ```
  ⚠ Strict check still non-zero after regeneration. Run bin/edikt migrate sidecars --strict to inspect.
  ```

### 6. Write FACTUAL and DEGRADED items to worklist

Append to `docs/internal/v060-manual-review.md` for each FACTUAL and DEGRADED item:

```markdown
- [ ] {item.path} :: {item.category}.{item.field} :: expected={item.expected} actual={item.actual} :: line {item.source_excerpt.line_start}-{item.source_excerpt.line_end}
  > {item.source_excerpt.quote}
```

If the file already exists, deduplicate by `(path, category, field, expected)` — do not append an item that is already present (checked or unchecked).

If there are no FACTUAL or DEGRADED items, skip this step.

### 7. Print summary

```
Auto-regenerated {K} sidecars (LOST).
{M} items written to docs/internal/v060-manual-review.md (FACTUAL/DEGRADED — manual review required).
```

### 8. Exit status

After writing the worklist:

Read `docs/internal/v060-manual-review.md`. If any line matches `^- \[ \]` (unchecked item), print:

```
INCOMPLETE — {N} manual-review items remain unchecked in docs/internal/v060-manual-review.md.
Review each item, edit the affected sidecar, then re-run bin/edikt migrate sidecars --strict to confirm.
```

Status: `INCOMPLETE`

If all items are checked (`^- \[x\]`) or the file does not exist, print:

```
COMPLETE — all regressions resolved.
```

Status: `COMPLETE`

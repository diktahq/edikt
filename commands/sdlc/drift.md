---
name: edikt:sdlc:drift
description: "Verify implementation matches spec, PRD, and ADRs"
effort: high
---

# edikt:sdlc:drift

Compare desired state (PRD, spec, artifacts, ADRs, invariants) against current implementation. Produces a drift report showing what's compliant, what diverged, and what can't be verified.

CRITICAL: NEVER assign a severity level without reading the actual code — every finding must cite evidence (file:line or explicit reasoning for unknown status).

## Arguments

- `$ARGUMENTS` — SPEC identifier (e.g., `SPEC-005`), `--scope=` filter, `--output=json` or `--json` for CI

## Instructions

0. If `.edikt/config.yaml` does not exist, output:
   ```
   No edikt config found. Run /edikt:init to set up this project.
   ```
   And stop.

0b. If `--json` or `--output=json` is in `$ARGUMENTS`, output only the JSON format at the end — no progress indicators, no emoji, no prose.

1. Display progress: `Step 1/4: Loading spec and artifacts...`

2. Read `.edikt/config.yaml`. Resolve all paths from the `paths:` section using the Path Defaults in the Reference section.

3. Find the spec: if `$ARGUMENTS` contains a SPEC identifier, locate `{specs_dir}/SPEC-{id}/spec.md`. If no identifier provided, use the most recent spec folder. If none exists, warn the user and suggest `/edikt:sdlc:spec` or `--scope=prd`.

4. Read the spec's frontmatter. Extract `source_prd:` and `references:`.

5. Parse `--scope=` from arguments. Use the Scope Definitions table in the Reference section to determine which layers to run. Default is full chain.

6. Display progress: `Step 2/4: Scanning implementation...`

7. Gather context for each layer in scope: spec, artifacts, source PRD, referenced ADRs, invariants. Get the implementation diff using git log from the plan start date (or last 30 days as fallback). Read the changed files.

8. Display progress: `Step 3/4: Comparing against spec...`

8b. **Artifact status filter** — before routing artifacts to validation agents, filter by status:

   For each artifact in the spec directory (excluding `spec.md`):
   - Read status from frontmatter or comment header (support all 4 formats: `%%` for `.mmd`, `#` for `.yaml`, `--` for `.sql`, frontmatter or `<!-- -->` for `.md`)
   - Filter by status:
     - `accepted` → include in validation
     - `implemented` → include in validation (verify still correct)
     - `in-progress` → include in validation (check partial work)
     - `draft` → **SKIP**. Output:
       `⏭ Skipping {artifact filename} (status: draft) — accept before validating`
     - `superseded` → **SKIP**. Output:
       `⏭ Skipping {artifact filename} (status: superseded)`

   Only pass non-skipped artifacts to the validation agents.

   If ALL artifacts are skipped:
   ```
   ⚠️ No artifacts to validate — all are draft or superseded.
   Run /edikt:sdlc:artifacts to review and accept them.
   ```

9. Route each layer to the appropriate specialist agent via the Agent tool. Pass the reference document, the implementation diff, and relevant codebase context. Use the Layer-to-Agent mapping in the Reference section.

9b. **Auto-promote in-progress → implemented** — after each artifact's drift validation completes:
   - If the validation found zero violations AND zero divergences for this artifact AND its status is `in-progress`:
     - Update the artifact's status to `implemented` (using the same format-aware approach as the plan auto-promote)
     - Output: `✅ {artifact filename} — no drift detected. Status promoted: in-progress → implemented`
   - If the artifact status is `accepted` and drift is clean: do NOT promote directly to `implemented`
     - Output: `✅ {artifact filename} — no drift detected.`
   - If violations or divergences were found: do not change status, report violations as normal

10. Apply the Severity Model from the Reference section to each finding.

11. Display progress: `Step 4/4: Generating drift report...`

12. If `--output=json` is in arguments, use the JSON Output Format from the Reference section. Otherwise use the Terminal Output Format.

13. Save the FULL report to `{reports_path}/drift-{SPEC-NNN}-{YYYY-MM-DD}.md`. The saved file MUST contain: (1) the Report Frontmatter from the Reference section, THEN (2) the complete Terminal Output Format content — summary line, emoji summary, findings table, and footer. Do NOT save only the frontmatter — the full report body must be included.

14. Log the drift event:
    ```bash
    source "$HOME/.edikt/hooks/event-log.sh" 2>/dev/null
    edikt_log_event "drift_report" '{"spec":"SPEC-{NNN}","compliant":{n},"diverged":{n}}'
    ```

15. When called from `/edikt:sdlc:review` (not directly): only run if an active spec exists, use `--scope=spec`, and append findings under a "DRIFT CHECK" section in the review output.

16. Output the confirmation:

    ```
    DRIFT REPORT — {SPEC identifier}
    ─────────────────────────────────────────────────
    {report content}
    ─────────────────────────────────────────────────

    ✅ Drift report: {reports_path}/drift-{SPEC-NNN}-{date}.md

    {If diverged findings:}
      {n} diverged finding(s). Which should I address? (e.g., #1, #3 or "all diverged")

      Next: Address diverged items, or run /edikt:sdlc:review for a broader check.

    {If all compliant:}
      All clear — implementation matches governance.

      Next: Address diverged items, or run /edikt:sdlc:review for a broader check.
    ```

---

REMEMBER: NEVER assign a severity without reading the actual code. Every finding must cite evidence (file:line or explicit reasoning). Compliant means you verified it, not that you assumed it.

## Reference

### Path Defaults

| Key | Default |
|---|---|
| `paths.specs` | `docs/product/specs` |
| `paths.reports` | `docs/reports` |
| `paths.decisions` | `docs/architecture/decisions` |
| `paths.invariants` | `docs/architecture/invariants` |
| `paths.plans` | `docs/plans` |

### Scope Definitions

| Scope | What it checks |
|---|---|
| (default) | PRD acceptance criteria + spec requirements + artifact contracts + ADR compliance + invariant compliance |
| `--scope=prd` | PRD acceptance criteria only |
| `--scope=spec` | Spec requirements only |
| `--scope=artifacts` | Artifact contracts only (data model, API contracts) |
| `--scope=adrs` | ADR compliance only |

### Layer-to-Agent Mapping

| Layer | What to check | Agent(s) |
|---|---|---|
| Layer 1: PRD Acceptance Criteria | For each acceptance criterion in the PRD, is it satisfied? | `architect` |
| Layer 2: Spec Requirements | For each requirement or component in the spec, was it implemented as specified? | `architect` + `engineer` |
| Layer 3: Artifact Contracts | `data-model.*` (`.mmd`, `.schema.yaml`, or `.md`) → actual schema matches? `contracts/api.yaml` → actual endpoints match? `test-strategy.md` → tests exist? `contracts/events.yaml` → event schema matches? `fixtures.yaml` → test data covers scenarios? Skip artifacts that don't exist. | `dba`, `api`, `qa`, `architect` |
| Layer 4: ADR Compliance | For each referenced ADR, does the implementation follow the decision? | `architect` |
| Layer 5: Invariant Compliance | For each invariant, is it violated by any changed file? | `architect` |

### Severity Model

| Level | Meaning | When to use |
|---|---|---|
| ✅ Compliant (high confidence) | Implementation matches the decision | Verified by reading code — the thing exists and works as specified |
| 🟡 Likely compliant (medium) | Appears to match but can't verify deterministically | Spec says "handle errors" and error handling exists, but full coverage can't be confirmed statically |
| ⚠️ Diverged (high confidence) | Implementation clearly differs from the decision | Spec says 2 agents get memory:project, only 1 does |
| ❓ Unknown | Not enough signal to determine | Spec says "context:fork on commands" — frontmatter check isn't possible from file reading alone |

### Terminal Output Format

Emoji summary at the top, then a table showing ALL findings across all severity levels. The table is the complete picture — nothing hidden.

```
DRIFT REPORT — {SPEC identifier}
─────────────────────────────────────────────────
Source: {SPEC-NNN} + {n} artifacts + {n} ADRs + {n} invariants
Scope:  {scope description}
Date:   {today}

SUMMARY
  ✅ {n} compliant    🟡 {n} likely compliant
  ⚠️  {n} diverged     ❓ {n} unknown

{If any non-compliant findings, show this table:}

┌─────┬────────┬───────────────────────────────────────────────────────────────────────────────┐
│  #  │ Status │ Description                                                                   │
├─────┼────────┼───────────────────────────────────────────────────────────────────────────────┤
│  1  │  ⚠️    │ {source}: {description of divergence — cite file:line}                        │
├─────┼────────┼───────────────────────────────────────────────────────────────────────────────┤
│  2  │  ⚠️    │ {source}: {description}                                                       │
├─────┼────────┼───────────────────────────────────────────────────────────────────────────────┤
│  3  │  🟡    │ {source}: {description — why only likely compliant}                            │
├─────┼────────┼───────────────────────────────────────────────────────────────────────────────┤
│  4  │  ❓    │ {source}: {description — why unknown}                                          │
└─────┴────────┴───────────────────────────────────────────────────────────────────────────────┘
```

Key formatting rules:
- Emoji summary at the top for a quick read — compliant count lives here, not in the table
- Table shows ONLY non-compliant findings (⚠️ diverged, 🟡 likely, ❓ unknown)
- Sort: ⚠️ diverged first, then 🟡 likely, then ❓ unknown
- Compliant items are NOT in the table — the summary line is sufficient
- If diverged findings exist, end with: `{n} diverged finding(s). Want me to prioritize them?`
- If all compliant, skip the table entirely: `All clear — implementation matches governance.`

### JSON Output Format (CI)

When `--json` or `--output=json` is passed, output only this structure:

```json
{
  "status": "diverged",
  "spec": "SPEC-005",
  "findings": [
    {"id": 1, "severity": "diverged", "requirement": "AC-001", "expected": "...", "actual": "...", "file": "src/cache.go:47"}
  ],
  "summary": {"compliant": 8, "diverged": 2, "unknown": 1}
}
```

Exit code:
- `0` if no diverged findings
- `1` if any diverged findings

### Report Frontmatter

```markdown
---
type: drift-report
spec: SPEC-{NNN}
scope: {scope}
date: {today}
summary:
  compliant: {n}
  likely_compliant: {n}
  diverged: {n}
  unknown: {n}
---
```

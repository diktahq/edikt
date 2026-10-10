---
name: docs:intake
description: "Onboard existing docs into edikt's standard structure"
effort: normal
allowed-tools:
  - Read
  - Write
  - Bash
  - Glob
  - Grep
  - Agent
  - AskUserQuestion
stops:
  - id: organization-plan
    class: risk
    default: "no"
    on_silence: safe-side
    prompt: "Proceed with this organization? (it moves documentation files on disk)"
  - id: delete-original
    class: risk
    default: "no"
    on_silence: safe-side
    prompt: "Delete this original file after intake?"
---

# edikt:docs:intake

Scan the project for existing documentation and organize it into edikt's standard structure.

CRITICAL: This command requires interactive input. If you are in plan mode (you can only describe actions, not perform them), output this and stop:
```
⚠️  This command requires user interaction and cannot run in plan mode.
Exit plan mode first, then run the command again.
```

## Instructions

### 1. Verify edikt is Initialized and Resolve Paths

Check for `.edikt/config.yaml`. If not found:
```
No edikt config found. Run /edikt:init to set up this project.
```

Read config to get the `base:` directory, then resolve paths from the `paths:` section:

- Decisions: `paths.decisions` (default: `docs/architecture/decisions`)
- Invariants: `paths.invariants` (default: `docs/architecture/invariants`)
- Plans: `paths.plans` (default: `docs/plans`)
- Product requirements: `paths.prds` (default: `docs/product/prds`)
- Specs: `paths.specs` (default: `docs/product/specs`)

`{base}/reference/` and `{base}/archive/` remain base-derived throughout this command — there is no `paths.reference` or `paths.archive` config key.

### 2. Scan for Existing Docs

Use an Agent to find documentation scattered across the project:

```
Agent(
  subagent_type: "Explore",
  prompt: """
  Find all documentation in this project. Look for:
  1. README files (README.md, readme.md at any level)
  2. docs/ or documentation/ directories
  3. Wiki content or markdown files outside src/
  4. Architecture docs (ADRs, decisions, RFCs)
  5. API documentation (OpenAPI/Swagger specs, API.md)
  6. Existing business rules or invariants docs
  7. Onboarding guides, contributing guides
  8. Product specs, PRDs, requirements docs
  9. Runbooks, playbooks, deployment docs
  For each file found, report:
  - Path
  - Type (architecture, product, reference, onboarding, api, etc.)
  - Brief summary of content (first 2-3 lines)

  Do NOT include: source code files, test files, generated docs, node_modules, vendor.
  """,
  description: "Scan for existing docs"
)
```

### 3. Categorize and Propose

Organize found docs into edikt categories:

<!-- stop: organization-plan -->
```
Found {n} documentation files:

  Architecture / Decisions:
    docs/adr/001-use-postgres.md     → {paths.decisions}/001-use-postgres.md

  Product / Requirements:
    docs/product-spec.md             → {paths.specs}/ (see SPEC-NNN/spec.md convention; migrate manually)
    docs/requirements/feature-x.md   → {paths.prds}/feature-x.md

  Reference:
    docs/api.md                      → {base}/reference/api.md
    docs/deployment.md               → {base}/reference/deployment.md
    CONTRIBUTING.md                   → {base}/reference/contributing.md

  Already in place:
    README.md                        → (keep as-is)

  Skip (not documentation):
    {list any files to skip}

Proceed with this organization? (y/n/edit)
```

With no human to answer, take the safe side: move nothing, leave the plan
printed, and say the organization was declined by default rather than
answered.

### 4. Consolidate Execution Plans

After categorization, check if any files look like execution plans — files containing phases, milestones, roadmaps, checklists, progress tracking, build order, or prioritization.

If plan-related files are found:

```
EXECUTION PLANS DETECTED
────────────────────────
Found {n} plan-related docs:
  - build-order.md (build sequence with phases)
  - progress.md (status tracking)
  - refactor-checklist.md (task list with milestones)

These could consolidate into a single PLAN-{NNN}-{slug}.md with a phases table.

Options:
  [1] Consolidate into PLAN-{NNN}-{slug}.md (preserves all content as phases)
  [2] Keep separate — copy as-is to {paths.plans}/
  [3] Skip — don't move plan files
```

**If consolidating:**
1. Read all plan-related files to extract phases, tasks, and milestones
2. Generate a `PLAN-{NNN}-{slug}.md` following edikt's plan format:
   ```markdown
   # Plan: {Title derived from content}

   ## Overview
   **Total Phases:** {n}
   **Approach:** {derived from source docs}

   ## Progress

   | Phase | Status | Updated |
   |-------|--------|---------|
   | 1     | -      | -       |

   ## Phase 1: {Title}

   **Objective:** {extracted from source docs}

   **Tasks:**
   {consolidated task list}

   **Completion promise:** `{PHASE TITLE DONE}`
   ```
3. Save to `{paths.plans}/PLAN-{NNN}-{slug}.md`
4. Track original plan files for the archive step

**If keeping separate:** copy files as-is to `{paths.plans}/`.

### 5. Execute Moves

For each confirmed move:

1. Create target directory if needed: `mkdir -p {base}/{category}/`
2. Copy (not move) the file to its new location: `cp source target`
3. Note the original source path so the user can clean up after verifying

**Important:** COPY, don't move. The user can delete originals after verifying. Never delete files without explicit confirmation.

### 6. Update Project Context

If existing docs reveal project context not captured in `project-context.md`,
**write it into `project-context.md`** and say so — this command does not ask
permission to write a file it was invoked to write. The host's permission
system and the managed-region guard are the write controls; a second yes/no in
the prose adds no protection and is the first thing to hang when nobody is
watching.

State what is being written before writing it:

```
Writing to project-context.md — additional context found in existing docs:
- Stack: {details from README}
- Architecture: {details from ADRs}
- Users: {details from product docs}
```

### 7. Output Summary

```
✅ Intake complete: {n} documents organized

  Organized: {n} files
  Copied to:
    {paths.decisions}/    — {count} decision records
    {paths.invariants}/   — {count} invariants
    {paths.plans}/        — {count} plans {(consolidated) if applicable}
    {paths.prds}/         — {count} product requirements
    {base}/reference/     — {count} reference docs

  Project context updated: {yes/no}
```

### 8. Archive Originals

After displaying the summary, offer cleanup for all copied files:

```
CLEANUP
───────
{n} original files were copied to new locations.

Options:
  [1] Archive originals to {base}/archive/ (removes from active, preserves history)
  [2] Delete originals (destructive — will confirm each file)
  [3] Keep both (default — safe, creates duplication)
```

**If archiving:**
1. `mkdir -p {base}/archive/`
2. Move each original to `{base}/archive/`, preserving relative paths

**If deleting:**
<!-- stop: delete-original -->
1. List each file and ask for confirmation: `Delete {path}? (y/n)`
2. Only delete files the user confirms. With no human to answer, take the safe
   side: delete nothing, list what would have been deleted, and say the
   deletion was declined by default rather than answered.

**If keeping:** no action needed.

After cleanup:

```
  Next: Review the organized docs and run /edikt:context to load everything.
```

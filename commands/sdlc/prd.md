---
name: sdlc:prd
description: "Write, continue, or transition a PRD — split artifact (.md + .yaml sidecar), rigor-calibrated. Also handles lifecycle: ship, cancel, deprecate, supersede."
effort: high
argument-hint: "<feature description | PRD-NNN | PRD-NNN ship | PRD-NNN cancel | PRD-NNN deprecate | PRD-NNN supersede | DISCOVERY-NNN>"
allowed-tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
  - Agent
---
!`bash -c 'CFG=""; D="$PWD"; while [ "$D" != "/" ]; do [ -f "$D/.edikt/config.yaml" ] && CFG="$D/.edikt/config.yaml" && break; D=$(dirname "$D"); done; [ -z "$CFG" ] && { printf "<!-- edikt:live -->\nNext PRD number: PRD-001\nExisting PRDs: (none yet)\n<!-- /edikt:live -->\n"; exit 0; }; PROOT=$(dirname "$(dirname "$CFG")"); REL=$(grep "^  prds:" "$CFG" 2>/dev/null | awk "{print \$2}" | tr -d "\""); if [ -z "$REL" ]; then BASE=$(grep "^base:" "$CFG" 2>/dev/null | awk "{print \$2}" | tr -d "\""); BASE="${BASE:-docs}"; REL="$BASE/product/prds"; fi; case "$REL" in /*) DIR="$REL" ;; *) DIR="$PROOT/$REL" ;; esac; COUNT=$(find "$DIR" -maxdepth 1 -type f -name "PRD-*.md" 2>/dev/null | wc -l | tr -d " "); NEXT=$(printf "%03d" $((COUNT + 1))); EXISTING=$(find "$DIR" -maxdepth 1 -type f -name "PRD-*.md" 2>/dev/null | sort | xargs -I{} basename {} .md | tr "\n" "," | sed "s/,$//"); printf "<!-- edikt:live -->\nNext PRD number: PRD-%s\nExisting PRDs: %s\n<!-- /edikt:live -->\n" "$NEXT" "${EXISTING:-(none yet)}"'`

# edikt:sdlc:prd

Write a Product Requirements Document using the v2 split artifact model: a `.md` narrative for humans + a `.yaml` sidecar as the structured source of truth for requirements, acceptance criteria, status, and revision history.

PRDs evolve via edit-in-place (per ADR-024). Lifecycle transitions (ship, cancel, deprecate, supersede) are handled by this same command — see [Lifecycle Transitions](#lifecycle-transitions) below.

CRITICAL: This command requires interactive input. If you are in plan mode (you can only describe actions, not perform them), output this and stop:
```
⚠️  This command requires user interaction and cannot run in plan mode.
Exit plan mode first, then run the command again.
```

## Arguments

- `<feature description>` — short description of the feature to scope
- `PRD-NNN` — continue/revise an existing PRD
- `PRD-NNN ship [FR-001 FR-002 ...]` — mark requirements as shipped
- `PRD-NNN cancel [reason]` — cancel a PRD (never shipped; work stopped)
- `PRD-NNN deprecate [reason]` — deprecate a PRD (shipped or accepted, now obsolete)
- `PRD-NNN supersede` — supersede with a new PRD (≥50% scope change, per ADR-024)
- `DISCOVERY-NNN` — graduate a discovery doc into a PRD (pre-populates Known → Problem, Unknown → Open Questions)
- Empty — ask what to build

## Instructions

### Step 0: Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### Step 1: Resolve Paths and Context

Read `.edikt/config.yaml`. Resolve paths from the `paths:` section:
- PRDs dir: `paths.prds` (default: `docs/product/prds`)
- Invariants dir: `paths.invariants` (default: `docs/architecture/invariants`)
- Discovery dir: `{base}/product/discovery/` (base defaults to `docs`)
- Project context: `paths.project-context` (default: `docs/project-context.md`)
- Rubric: `.edikt/rubrics/prd.md` (auto-created on first run — see Step 7)
- Evaluator mode: `evaluator.mode` (default: `headless`, per ADR-010)

Read `{project-context}` for project identity, users, and stack. This informs who the PRD is for and what invariants apply.

The correct next PRD number is in the `<!-- edikt:live -->` block at the top. Use it exactly.

### Step 2: Dispatch on Arguments

Inspect `$ARGUMENTS`. Check in this order:

1. **Matches `PRD-\d+\s+(ship|cancel|deprecate|supersede)`** (or natural-language equivalent: "ship PRD-001", "cancel PRD-002 — budget cut") → **lifecycle transition mode**
   - Extract the PRD identifier and the lifecycle verb.
   - Extract any remaining text as the optional reason/FR list.
   - Jump directly to [Lifecycle Transitions](#lifecycle-transitions). Do NOT run Steps 3–9.

2. **Matches `PRD-\d+` with no lifecycle verb** → continuation mode
   - Load the existing `.md` and `.yaml` sidecar.
   - Confirm: "Continuing PRD-NNN. Are you (a) adding new requirements, (b) revising existing requirements, or (c) answering open questions?"
   - Apply the relevant subset of steps below based on the answer.
   - Skip Step 3 (rigor triage — already set) and Step 4 (forcing questions — already answered).
   - Jump to Step 5 or Step 6 depending on the user's choice.

3. **Matches `DISCOVERY-\d+`** → graduation mode
   - Load the discovery doc.
   - Seed: `problem` ← discovery Context + Known; `open_questions` ← discovery Unknown; `evidence` ← discovery Known.
   - Continue to Step 3 with the seeded content pre-filled.
   - In the final output, mark the discovery's `graduates_to: PRD-NNN` field (edit the discovery front-matter).

4. **Empty or description** → new PRD mode
   - If empty, ask: "What are you building? Short description — I'll ask for the rest."
   - Continue to Step 3.

### Step 3: Rigor Triage

Ask exactly one question:

```
Before we start — is this a solo project, a team feature, or a platform change?

  solo      — you or a small group, single-product scope (default)
  team      — cross-functional, multiple stakeholders, needs sign-off and a rollout plan
  platform  — multi-tenant, compliance-sensitive, or cross-product (adds NFRs, risk register, compatibility matrix)

Choose one: solo / team / platform
```

Capture the answer. Default `solo` if the user just presses enter or skips. Store in the sidecar `rigor:` field.

Rigor determines:
- Which optional sections appear in the `.md` narrative
- Which additional fields are required in the sidecar (`stakeholders`, `dependencies`, `nfrs`, `risks`)
- The evaluator pass threshold (solo: 70%, team: 80%, platform: 90%)

### Step 4: Five Forcing Questions

Ask these five questions in order, one at a time, waiting for the answer before asking the next. **Do NOT batch them.** Do NOT allow the user to skip any — if they try, say "this question is not optional — a short answer or 'hypothesis only' is fine."

Record each answer in the sidecar's `forcing_questions:` field.

```
Question 1 of 5 — What's the problem *behind* this problem?
Don't describe the solution. Describe what breaks without this feature.

> ...
```

```
Question 2 of 5 — How do you know someone has this problem?
Evidence, data, support tickets, interviews — or say "hypothesis only" if
you're starting from an informed guess.

> ...
```

```
Question 3 of 5 — What single metric should move if this works, and
what metric must NOT move?

North metric (target):
Counter metric (guardrail):
```

```
Question 4 of 5 — What must NOT change when this ships?
This seeds the Protections section. Existing invariants, UX patterns users
depend on, contracts with external systems.

> ...
```

```
Question 5 of 5 — What's the riskiest assumption behind this working?

> ...
```

Confirm: "Got it. Now I'll draft the PRD."

### Step 5: Draft Generation

Using the forcing question answers + user description + project context, generate the draft.

**FR numbering:** Start at `FR-001`, sequential. Each FR is a single testable statement.

**AC numbering:** `AC-NNN-M` where NNN = FR number (exact match, not sequential within AC list), M = criterion index within that FR starting at 1. So a FR-001 with 2 ACs has `AC-001-1` and `AC-001-2`.

**Given/When/Then:** Every AC uses this format. Each AC also includes a `verify` field with the method to check the criterion is met. Example:

```yaml
- id: AC-001-1
  fr: FR-001
  given: "a user with an active subscription"
  when: "their renewal date is 7 days away"
  then: "they receive an email reminder"
  verify: "Verify: send a test event and assert an email is queued within 30 s"
  status: proposed
```

**Mirror section:** The `.md` file's "Requirements" and "Acceptance Criteria" sections render the sidecar's FRs and ACs as readable tables or bullet lists. The sidecar is source of truth — the `.md` is a view.

### Step 6: Protection Section — Invariant Auto-Link

This step is non-trivial. Be methodical.

**6a. Grep invariants:**

```bash
ls {invariants_dir}/INV-*.md 2>/dev/null
```

For each INV file, read the title and `## Rule` section. Extract keywords.

**6b. Match to PRD scope:**

The PRD description + FRs suggest a topic area (e.g., "auth", "payment", "data", "UI"). Match invariant keywords to the PRD topic. Relevant matches are candidates.

**6c. Present candidates:**

```
I found {N} existing invariants that may apply to this PRD:

  [1] INV-003 — Hook JSON emission
      Relevant because: FR-002 mentions hook behavior

  [2] INV-006 — Input shape validation
      Relevant because: FR-001 accepts user input

For each, should I link it as a protection? (1=yes, 0=no, e=edit note)
[1]: _  [2]: _
```

Record confirmed links to `protections:` in the sidecar as `{ref: INV-NNN, note: "..."}`.

**6d. Scan Q4 answer for new invariant candidates:**

Re-read the user's answer to forcing Question 4 (what must NOT change). If any protection looks like a durable architectural rule (e.g., "never store credentials in localStorage", "admin APIs always require 2FA"), offer:

```
Your protection "{text}" looks like a durable invariant. Should I suggest creating one?

  y — I'll provide a suggested /edikt:invariant:new command
  n — keep it feature-scoped (SP-NNN in this PRD only)
```

For feature-scoped protections, assign `SP-001`, `SP-002`, ... and record as `{id: SP-NNN, text: "..."}`.

### Step 7: Generator-Evaluator Loop

**7a. Load or bootstrap rubric:**

Check if the rubric file exists:

```bash
test -f .edikt/rubrics/prd.md && echo "exists" || echo "missing"
```

If **exists**: read its content and proceed to 7b.

If **missing**: use the Write tool to create `.edikt/rubrics/prd.md` with the default rubric content below. This is a mandatory bootstrap step — do NOT proceed to scoring without the file on disk (otherwise `/edikt:prd:review` runs later will re-bootstrap and the score history becomes inconsistent).

```bash
mkdir -p .edikt/rubrics
```

Then write this content to `.edikt/rubrics/prd.md` via the Write tool:

```markdown
# PRD Evaluator Rubric

Score each item 0 (missing/weak) or 1 (strong). Threshold varies by rigor:
  solo: 7/10  team: 8/10  platform: 9/10

## Rubric

- [ ] Problem statement describes what breaks, not the feature
- [ ] At least one piece of user evidence (or explicit "hypothesis only")
- [ ] North metric AND counter metric are named
- [ ] Protections section lists at least one linked invariant OR feature-scoped protection
- [ ] Riskiest assumption is explicit
- [ ] Every FR has at least one AC
- [ ] ACs are in Given/When/Then format with stable IDs
- [ ] Non-goals are listed (not empty)
- [ ] Solution references are present OR marked "to be added" (not silently absent)
- [ ] No NEEDS CLARIFICATION or TBD remains in requirements or ACs

## Rigor additions (team and above)

- [ ] Stakeholders listed with roles
- [ ] Dependencies enumerated
- [ ] Rollout plan present

## Rigor additions (platform only)

- [ ] NFRs with measurable targets
- [ ] Risk register with likelihood/impact
- [ ] Compatibility matrix

_Users can edit this rubric per ADR-005 template overrides._
```

After writing, confirm to the user: `✓ Bootstrapped .edikt/rubrics/prd.md (first-run default — edit to customize)`.

**7b. Score the draft:**

Walk through the rubric. Count items met. Compute threshold based on rigor.

If headless mode is available (`claude` on PATH, `evaluator.mode: headless`):

```bash
# Headless evaluator invocation, INV-003 compliant
python3 -c 'import json,sys; print(json.dumps({"prd_path": sys.argv[1], "sidecar_path": sys.argv[2], "rubric_path": sys.argv[3], "rigor": sys.argv[4]}))' "{prd_md}" "{prd_yaml}" ".edikt/rubrics/prd.md" "{rigor}" | claude -p --model "{evaluator.model}" "Apply the rubric. Output JSON: {score: N, total: M, passed: bool, gaps: [...]}"
```

Fall back to in-session reasoning if headless fails (visible warning per ADR-010).

**7c. Iterate if below threshold:**

If score < threshold and this is iteration 1 or 2:

```
PRD evaluator: {score}/{total} ({rigor} threshold: {threshold})

Gaps:
  • {gap 1}
  • {gap 2}

Iterating — revising draft to address gaps.
```

Revise. Re-score. Max 3 iterations.

**7d. Final state:**

If passes: proceed to Step 8 with status `draft` (still draft; user flips to `accepted` manually or via a future command).

If fails after 3 attempts: proceed to Step 8 anyway, but annotate the output summary with the unresolved gaps and mark status `draft` with a note.

### Step 8: Write Files and Compute Sync Hashes

**8a. Write files:**

```bash
# slug is a kebab-case short version of the title
PRD_MD="{prds_dir}/PRD-{NNN}-{slug}.md"
PRD_YAML="{prds_dir}/PRD-{NNN}-{slug}.yaml"
```

Render `{prd_md}` from `templates/prd.md.tmpl` filling placeholders with:
- `{{id}}` → `PRD-NNN`
- `{{title}}` → the title
- `{{slug}}` → the kebab slug
- `{{rigor}}` → solo/team/platform
- `{{author}}` → git user.name
- `{{created_at}}` → ISO8601 timestamp
- `{{problem_statement}}` → draft problem (from Step 5)
- Rigor-gated sections (`{{#if team_or_platform}}`, `{{#if platform}}`) are included only when rigor matches. For `solo`, delete those sections entirely — do NOT leave empty placeholder text.

Render `{prd_yaml}` from `templates/prd.yaml.tmpl` with the full structured content.

**MANDATORY — read the template first, render it whole.** Use the Read tool on `templates/prd.yaml.tmpl` (resolve via `$EDIKT_HOME/current/templates/prd.yaml.tmpl` or `$HOME/.edikt/current/templates/prd.yaml.tmpl`). Replace `{{placeholder}}` tokens with values; preserve every other key verbatim. Do NOT compose the sidecar from memory or pattern-match from an existing PRD — the template is the contract.

**Required top-level fields that MUST appear in every sidecar (per `templates/schemas/prd-sidecar.schema.json`):**

| Field | Source |
|---|---|
| `schema_version` | always `"1.0"` |
| `type` | always `"prd"` |
| `id` | `PRD-NNN` from the live-injected next number |
| `title` | the user-confirmed title |
| `slug` | kebab-case of the title |
| `status` | always `"draft"` for a new PRD |
| `rigor` | answer from Step 3 (solo/team/platform) |
| `author` | output of `git config user.name` |
| `created_at` | ISO8601 UTC timestamp computed at write time, e.g. `date -u +%Y-%m-%dT%H:%M:%SZ` |

**Required collection fields (may start empty but the key MUST be present):** `requirements`, `acceptance_criteria`, `protections`, `solution_references`, `stakeholders`, `dependencies`, `nfrs`, `risks`, `open_questions`, `source_specs`, `revision_history`, `extensions`, `_sync`, `forcing_questions`.

**Required nullable fields:** `supersedes`, `superseded_by`, `deprecated_at`, `deprecated_reason`, `cancelled_at`, `cancelled_reason` (set to `null` for a new PRD).

**Validation gate before write.** After rendering the sidecar in memory, validate it against `templates/schemas/prd-sidecar.schema.json`:

```bash
python3 <<'PYEOF' "$PRD_YAML_RENDERED" "$SCHEMA_PATH_ABSOLUTE"
import sys, yaml, json
from jsonschema import Draft202012Validator
sidecar = yaml.safe_load(open(sys.argv[1]))
schema = json.load(open(sys.argv[2]))
errors = sorted(Draft202012Validator(schema).iter_errors(sidecar), key=lambda e: str(e.path))
if errors:
    print("SIDECAR INVALID:", file=sys.stderr)
    for e in errors:
        print(f"  {list(e.path) or '<root>'}: {e.message}", file=sys.stderr)
    sys.exit(1)
print("OK")
PYEOF
```

If validation fails: do NOT write the sidecar. Show the user which fields are missing/invalid, fix in-memory, re-validate, then write. The sidecar is a contract — never write a non-conforming one.

If `jsonschema` is not installed (pip module missing), fall back to a structural check: assert all 9 required top-level fields are present in the rendered dict before writing.

**Computing `{{schema_path}}`:** The template carries a `# yaml-language-server: $schema={{schema_path}}` header that enables IDE autocomplete. Compute the relative path from the PRD's parent directory to `.edikt/schemas/prd-sidecar.schema.json`:

```bash
# Use python3 with argv for safe relative-path computation (INV-003 compliant)
SCHEMA_PATH=$(python3 -c 'import os,sys; print(os.path.relpath(sys.argv[2], sys.argv[1]))' "$(dirname "$PRD_YAML")" "$PROJECT_ROOT/.edikt/schemas/prd-sidecar.schema.json")
```

For a PRD at `docs/product/prds/PRD-001-x.yaml` with default layout, `SCHEMA_PATH` resolves to `../../../.edikt/schemas/prd-sidecar.schema.json`.

If `.edikt/schemas/prd-sidecar.schema.json` does NOT exist in the project, auto-install it as part of this step:

```bash
test -f "$PROJECT_ROOT/.edikt/schemas/prd-sidecar.schema.json" || {
  mkdir -p "$PROJECT_ROOT/.edikt/schemas"
  # Source schema lives in the edikt payload — resolve via EDIKT_HOME
  SOURCE_SCHEMA="$EDIKT_HOME/current/templates/schemas/prd-sidecar.schema.json"
  [ -f "$SOURCE_SCHEMA" ] || SOURCE_SCHEMA="$HOME/.edikt/current/templates/schemas/prd-sidecar.schema.json"
  cp "$SOURCE_SCHEMA" "$PROJECT_ROOT/.edikt/schemas/prd-sidecar.schema.json"
}
```

This bootstrap keeps the IDE autocomplete working on first-ever PRD creation without requiring the user to run a separate install step.

**8b. Compute SHA-256 hashes (INV-003 compliant):**

```bash
MD_HASH=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$PRD_MD")
YAML_HASH=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$PRD_YAML")
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
```

**8c. Write hashes back into sidecar:**

Edit the sidecar `_sync:` block with the hashes. Note: writing the hash changes the file, so `yaml_hash` is recorded *after* the hash write (it represents the sidecar-before-hash-write, which is the stable content). Use this algorithm:

```
1. Write sidecar without _sync block populated (md_hash, yaml_hash, synced_at all empty).
2. Compute MD_HASH from .md file.
3. Compute YAML_HASH from .yaml file as currently written (with empty _sync).
4. Edit sidecar: set _sync.md_hash = MD_HASH, _sync.yaml_hash = YAML_HASH, _sync.synced_at = NOW.
```

This is deterministic: `_sync.yaml_hash` always represents the canonical "pre-sync-write" state of the sidecar.

**8d. Append to revision_history:**

```yaml
revision_history:
  - at: "{now}"
    author: "{git user}"
    action: "created"
    note: "Initial draft — rigor: {rigor}"
```

### Step 9: Output Summary

```
✅ PRD-{NNN} created — {title}

  {prds_dir}/PRD-{NNN}-{slug}.md    (narrative)
  {prds_dir}/PRD-{NNN}-{slug}.yaml  (sidecar — source of truth)

  Rigor:         {rigor}
  FRs:           {n} requirements
  ACs:           {n} acceptance criteria
  Protections:   {n} ({linked} linked invariants, {scoped} feature-scoped)
  Evaluator:     {score}/{total} ({PASS | below threshold})

  {if invariant suggestion was offered}
  💡 Your protection "{text}" looks durable. Run:
     /edikt:invariant:new "{text}"

Next steps:
  • Review the draft, flip status to accepted when ready
  • Write the technical spec:   /edikt:sdlc:spec PRD-{NNN}
  • Re-score anytime:           /edikt:prd:review PRD-{NNN}
  • Ship requirements:          /edikt:sdlc:prd PRD-{NNN} ship FR-NNN
```

If a DISCOVERY-NNN graduated, also edit the discovery doc's front-matter to set `graduates_to: PRD-{NNN}`.

---

## Design Notes

### Why split artifact (`.md` + `.yaml`)

LLMs corrupt markdown table and section structure at ~5-10% rate over multi-turn edits (validated by Anthropic harness findings). YAML stays intact. The narrative goes in markdown where it reads well; the structure goes in YAML where commands can mutate it safely.

### Why forcing questions are not skippable

The evaluator scores them. "I didn't think about this" is the gap the forcing questions are there to close. A PRD that passes evaluation without answering Q3 (north + counter metric) would be a rubric bug, not a valid PRD.

### Edit-in-place model (ADR-024)

This command creates PRDs in `status: draft`. Lifecycle transitions (ship, cancel, deprecate, supersede) are dispatched via this same command — pass the verb after the PRD identifier (e.g. `/edikt:sdlc:prd PRD-001 ship FR-001`). Supersession is rare — reserved for ≥50% scope rewrites.

### v1 compatibility

Projects with older v1 PRDs (no sidecar) continue to work. This command does not migrate v1 → v2. Spec and review commands detect sidecar presence and branch accordingly.

## Related Commands

- `/edikt:sdlc:discovery` — upstream uncertainty-reduction before PRD authoring
- `/edikt:sdlc:spec` — technical spec from this PRD
- `/edikt:prd:review` — re-score the PRD against the rubric
- `/edikt:invariant:new` — promote a feature-scoped protection into a project-wide invariant

REMEMBER: A PRD captures REQUIREMENTS with evidence and measurable outcomes, not feature descriptions. Every FR must be testable. Every AC must have a Verify: method. Five forcing questions are mandatory — they are the minimum bar for a PRD that can be evaluated. If any question is skipped, the sidecar is incomplete and the evaluator will fail it.

Next: Review the PRD with /edikt:prd:review to score it against the rubric, then run /edikt:sdlc:spec to generate the technical spec.

---

## Lifecycle Transitions

Reached via Step 2 dispatch when a lifecycle verb is detected. These steps are self-contained — Steps 3–9 do NOT run.

All common setup:

```bash
PRD_ID="PRD-NNN"   # extracted from arguments
AUTHOR=$(git config user.name 2>/dev/null || echo "unknown")
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
# Resolve PRD_YAML and PRD_MD from paths.prds in .edikt/config.yaml
```

If no `.yaml` sidecar found: error "PRD-NNN not found (or is v1 — no sidecar to mutate). Upgrade first with `/edikt:sdlc:prd PRD-NNN`."

### ship

Marks one or more requirements as `status: shipped`. If all FRs are shipped, flips the top-level `status:` to `shipped` too.

**Parse FRs to ship:** Extract any `FR-\d+` tokens from arguments. If none, list unshipped FRs and ask:

```
PRD-NNN has {n} non-shipped requirements:

  [1] FR-001 (proposed)  — "{text}"
  [2] FR-002 (accepted)  — "{text}"

Which to ship? (comma-separated numbers, "all", or "cancel")
>
```

**Validate:** Skip any FR already `shipped` or `deprecated` (warn, not error). Error on unknown FR IDs.

**Mutate sidecar (INV-003 compliant):**

```bash
python3 <<'PYEOF' "$PRD_YAML" "$FR_LIST" "$AUTHOR" "$NOW"
import sys, yaml
path, frs_raw, author, now = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
frs = frs_raw.split(",")
with open(path) as f:
    data = yaml.safe_load(f)
affected = []
for req in data.get("requirements", []):
    if req["id"] in frs and req.get("status") != "shipped":
        req["status"] = "shipped"
        req["shipped_at"] = now
        affected.append(req["id"])
data.setdefault("revision_history", []).append({
    "at": now, "author": author, "action": "ship",
    "note": f"Marked shipped: {', '.join(affected)}",
    "affected": affected,
})
all_shipped = all(r.get("status") == "shipped" for r in data.get("requirements", []))
if all_shipped and data.get("requirements"):
    data["status"] = "shipped"
data.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
with open(path, "w") as f:
    yaml.safe_dump(data, f, sort_keys=False)
print(",".join(affected))
PYEOF
```

Recompute `_sync` hashes after mutation (same pattern as Step 8b–8c).

**Output:**

```
✅ PRD-NNN: shipped FR-001, FR-002

  Top-level status: shipped  (all requirements shipped)  ← or: accepted ({n}/{total} shipped)
  Revision history updated. Sync hashes recomputed.

Next:
  • Drift check:  /edikt:sdlc:drift PRD-NNN
```

---

### cancel

Marks a PRD as `status: cancelled`. Use when work stopped before shipping (hypothesis was wrong, priority shifted, merged into a different PRD without formal supersession).

**Difference from deprecate:** cancel = never shipped; deprecate = was shipped/accepted, now obsolete.

**Guard:** If already `cancelled` or `deprecated` → error "PRD-NNN is already {status}." If `shipped` → confirm "PRD-NNN is shipped. Cancel a shipped PRD? Usually you want deprecate. Continue? (y/N)".

**Get reason:** Extract from arguments remainder. If missing, ask:

```
Why is PRD-NNN being cancelled? (short description — recorded in revision_history)

>
```

Minimum 10 characters. Reject empty.

**Mutate sidecar:**

```bash
python3 <<'PYEOF' "$PRD_YAML" "$REASON" "$AUTHOR" "$NOW"
import sys, yaml
path, reason, author, now = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
with open(path) as f:
    data = yaml.safe_load(f)
data["status"] = "cancelled"
data["cancelled_at"] = now
data["cancelled_reason"] = reason
data.setdefault("revision_history", []).append({
    "at": now, "author": author, "action": "cancel",
    "note": f"Cancelled: {reason}",
})
data.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
with open(path, "w") as f:
    yaml.safe_dump(data, f, sort_keys=False)
PYEOF
```

Recompute `_sync` hashes.

**Output:**

```
✅ PRD-NNN cancelled

  Reason:  {reason}
  At:      {timestamp}

File kept as historical record. Hidden from active views.
Linked SPECs ({count}) are not automatically touched — review each manually.
```

---

### deprecate

Marks a PRD as `status: deprecated`. Use when a feature is abandoned, no longer strategically relevant, or replaced by a different approach without direct supersession.

**Guard:** If already `deprecated` or `cancelled` → error "PRD-NNN is already {status}."

**Get reason:** Extract from arguments remainder. If missing, ask:

```
Why is PRD-NNN being deprecated? (short description — recorded in revision_history)

>
```

Minimum 10 characters. Reject empty.

**Mutate sidecar:**

```bash
python3 <<'PYEOF' "$PRD_YAML" "$REASON" "$AUTHOR" "$NOW"
import sys, yaml
path, reason, author, now = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
with open(path) as f:
    data = yaml.safe_load(f)
data["status"] = "deprecated"
data["deprecated_at"] = now
data["deprecated_reason"] = reason
data.setdefault("revision_history", []).append({
    "at": now, "author": author, "action": "deprecate",
    "note": f"Deprecated: {reason}",
})
data.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
with open(path, "w") as f:
    yaml.safe_dump(data, f, sort_keys=False)
PYEOF
```

Recompute `_sync` hashes.

**Output:**

```
✅ PRD-NNN deprecated

  Reason:  {reason}
  At:      {timestamp}

Revision history updated. File kept as historical record.
Linked SPECs ({count}) not automatically updated — review each.
```

---

### supersede

Supersede is reserved for ≥50% scope rewrites or problem-framing shifts (ADR-024). For routine FR changes, use continuation or ship/cancel/deprecate instead.

**Guard:** If old PRD is already `superseded` or `cancelled` → error "PRD-NNN is already {status}; cannot supersede."

**Gate on ≥50% threshold (ADR-024):**

Show what will be lost:

```
⚠️  Supersede is a heavy transition (ADR-024).

You are about to:
  • Mark PRD-NNN as status: superseded (hidden from active views)
  • Create a new PRD-MMM with its own FR/AC numbering
  • Break the stable ID chain — SPECs linked to PRD-NNN FR-001
    will no longer trace to PRD-MMM FR-001

For routine changes, prefer:
  • /edikt:sdlc:prd PRD-NNN       — add/revise FRs in place
  • /edikt:sdlc:prd PRD-NNN ship  — mark FRs shipped
  • /edikt:sdlc:prd PRD-NNN deprecate — retire without replacement
```

Ask four gating questions one at a time (y/n):

```
Gate 1/4 — Has the PROBLEM framing changed since PRD-NNN was authored?
Gate 2/4 — Would ≥50% of the requirements (FR-NNN) be rewritten or removed?
Gate 3/4 — Are the Protections or Goals so different that old ACs would no longer apply?
Gate 4/4 — Have you tried continuation via /edikt:sdlc:prd PRD-NNN and found it insufficient?
```

| Yes count | Action |
|-----------|--------|
| 4 | Proceed |
| 3 | Confirm once more: "Three of four gates passed. Proceed? (y/N)" |
| 2 | Show alternatives, require explicit override (`--force`) |
| 0–1 | Abort with guidance |

If `--force` was passed, skip gate but log `"Supersede gate overridden with --force"` in revision_history.

**Author the new PRD:** Run the full creation flow (Steps 3–9) for the new PRD, seeding:
- `title` — ask if user wants to keep old title or change
- `problem_statement`, `user_archetypes`, `solution_references` — seed from old; user edits
- `forcing_questions` — ask all five fresh (do NOT seed from old)

The new PRD gets its own `id: PRD-MMM` from the live-injected next number.

**Link supersession (INV-003 compliant):**

```bash
python3 <<'PYEOF' "$OLD_YAML" "$NEW_YAML" "$OLD_ID" "$NEW_ID" "$AUTHOR" "$NOW" "$FORCED"
import sys, yaml
old_path, new_path, old_id, new_id, author, now, forced = sys.argv[1:]
with open(old_path) as f:
    old = yaml.safe_load(f) or {}
old["status"] = "superseded"
old["superseded_by"] = new_id
note = f"Superseded by {new_id}"
if forced == "1":
    note += " (supersede gate overridden with --force)"
old.setdefault("revision_history", []).append({"at": now, "author": author, "action": "supersede", "note": note})
old.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
with open(old_path, "w") as f:
    yaml.safe_dump(old, f, sort_keys=False)
with open(new_path) as f:
    new = yaml.safe_load(f) or {}
new["supersedes"] = old_id
new.setdefault("revision_history", []).append({"at": now, "author": author, "action": "supersede", "note": f"Supersedes {old_id}"})
new.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
with open(new_path, "w") as f:
    yaml.safe_dump(new, f, sort_keys=False)
PYEOF
```

Recompute `_sync` hashes on both sidecars.

**Output:**

```
✅ Supersession complete

  Old:  PRD-NNN  (status: superseded → PRD-MMM)
  New:  PRD-MMM  (supersedes: PRD-NNN)

Both sidecars updated. Revision history logged on both.
SPECs linked to old PRD are not auto-migrated — review each manually.

Next:
  • Review new PRD:  /edikt:prd:review PRD-MMM
  • Write spec:      /edikt:sdlc:spec PRD-MMM
```

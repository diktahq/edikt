---
name: status
description: "Governance dashboard — chain status, gates, agents, hooks"
effort: low
context: fork
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
---

# edikt:status

Show the governance dashboard: chain status, gate activity, agent activity, hook activity, and signals.

## Instructions

### 1. Load Config

Read `.edikt/config.yaml`. If not found:
```
No edikt config found. Run /edikt:init to set up this project.
```

Read `base:` from config (default: `docs`).
Read `specs: { dir: }` from config (default: `{base}/product/specs`).
Read `plans: { dir: }` from config (default: `{base}/plans`).

### 2. Gather Data

Collect all information in parallel:

**Rules:**
```bash
ls .claude/rules/*.md 2>/dev/null | wc -l
ls .claude/rules/*.md 2>/dev/null | xargs -I{} basename {} .md | paste -sd', ' -
```

**Agents:**
```bash
ls .claude/agents/*.md 2>/dev/null | wc -l
```

**Decisions:**
```bash
ls {base}/decisions/*.md {base}/architecture/decisions/*.md 2>/dev/null | wc -l
```

**Invariants:**
```bash
ls {base}/invariants/*.md {base}/architecture/invariants/*.md 2>/dev/null | wc -l
```

**Active plan:**

The "active plan" is the most-recently-modified PLAN file that:
- Is NOT a `*.SUPERSEDED*` file (those are deliberately retired drafts)
- Has a progress table with at least one phase row whose status is NOT a completion marker (`done`, `pass`, `passed`, `complete`, `completed`, or `✅`). Plans where every phase row is complete are filtered out — they are finished work, not active.

Plans with no progress table at all (rare — usually a half-written plan stub) are skipped.

Use `python3` rather than `ls -t` for the mtime sort: in interactive shells `ls` is often aliased to `eza` or similar wrappers that ignore the `-t` flag and return alphabetic order, silently breaking the selection.

```bash
ACTIVE_PLAN=$(python3 -c "
import os, glob, re, sys
plans_dir = sys.argv[1]
COMPLETE_RE = re.compile(r'\b(done|pass(ed)?|complete(d)?)\b|✅', re.IGNORECASE)
ROW_RE = re.compile(r'^\| *\d+[a-z]?\s*\|')
PROGRESS_HEADING = re.compile(r'^##+ +Progress\b', re.IGNORECASE | re.MULTILINE)
NEXT_HEADING = re.compile(r'^##+ +', re.MULTILINE)

def progress_rows(body):
    # Scope to the ## Progress section: false-positive rows from other tables
    # in the plan body (cost estimates, phase prompts, dependency lists) must
    # not be counted as progress rows.
    m = PROGRESS_HEADING.search(body)
    if not m:
        return []
    rest = body[m.end():]
    nxt = NEXT_HEADING.search(rest)
    section = rest[:nxt.start()] if nxt else rest
    return [ln for ln in section.splitlines() if ROW_RE.match(ln)]

files = sorted(
    [f for f in glob.glob(os.path.join(plans_dir, 'PLAN-*.md')) if '.SUPERSEDED' not in f],
    key=os.path.getmtime, reverse=True
)
for f in files:
    with open(f, encoding='utf-8', errors='replace') as fh:
        body = fh.read()
    rows = progress_rows(body)
    if not rows:
        continue
    # A plan is 'active' if at least one row is NOT a completion marker.
    if any(not COMPLETE_RE.search(r) for r in rows):
        print(f)
        break
" "{plans_dir}")
```

CRITICAL — use the detector's output verbatim. If `ACTIVE_PLAN` is empty, the dashboard MUST show "no active plan" — do NOT pick a fallback file from elsewhere, do NOT substitute the latest completed plan, do NOT use your own judgment about which plan looks "more relevant." If `ACTIVE_PLAN` is non-empty, the dashboard MUST report exactly that file as the active plan, regardless of how many phases it has done.

To populate the `Plan:` line of the dashboard, read the progress table at `$ACTIVE_PLAN` and identify the in-progress phase: a row whose status is `in-progress` / `in_progress`, otherwise the lowest-numbered row whose status is `-` or empty.

**Active spec:**
```bash
ls -t {specs_dir}/SPEC-*/spec.md 2>/dev/null | head -1
```
Read the spec frontmatter for status and source_prd.

**Spec artifacts:**
```bash
ls {spec_folder}/*.md {spec_folder}/contracts/*.md 2>/dev/null | grep -v spec.md
```
Check each artifact's `status:` frontmatter.

**Last drift report:**
```bash
ls -t {spec_folder}/drift-*.md 2>/dev/null | head -1
```
If found, read the frontmatter `summary:` for compliant/diverged counts.

**Compile status:**
```bash
# Check if governance.md exists and when it was last compiled
ls -l .claude/rules/governance.md 2>/dev/null
# Check for ADRs/invariants modified after the last compile
COMPILE_DATE=$(grep 'compiled:' .claude/rules/governance.md 2>/dev/null | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}')
```
Compare the compile date against ADR/invariant modification dates. If any ADR or invariant was modified after the last compile, the directives are stale.

**Gate activity:**
```bash
grep 'GATE\|gate_fired\|gate_override' ~/.edikt/session-signals.log ~/.edikt/events.jsonl 2>/dev/null
```

**Agent activity:**
```bash
grep 'AGENT' ~/.edikt/session-signals.log 2>/dev/null
```

**Hook activity:**
```bash
# Count hook fires by type from session-signals.log
grep 'RULE_LOADED' ~/.edikt/session-signals.log 2>/dev/null | wc -l
grep 'AGENT' ~/.edikt/session-signals.log 2>/dev/null | wc -l
```

**Signals detected:**
```bash
grep -E 'ADR|Doc gap|Security' ~/.edikt/session-signals.log 2>/dev/null | grep -v 'AGENT\|RULE_LOADED'
```

### 3. Build Chain Status

If an active spec exists, trace the governance chain:
1. Read the spec's `source_prd:` → find the PRD → get its status
2. Read the spec's status
3. Count artifacts and their statuses
4. Find the associated plan and its status

Build the chain string:
```
PRD-005 accepted → SPEC-005 accepted → artifacts 3/3 accepted → PLAN-007 in progress
```

If no spec exists, show a simpler chain from PRD → plan (or just the plan).

### 4. Output Dashboard

```
EDIKT STATUS — {project name from project-context.md}
═══════════════════════════════════════════════

GOVERNANCE HEALTH
  Rules:        {n} active ({rule names})
  Agents:       {n} installed
  Decisions:    {n} ADRs, {n} invariants
  Compile:      {last compile date, or "not compiled — run /edikt:compile"}
                {If stale: "⚠️ stale — {n} ADRs modified since last compile"}
                {If governance/ dir exists: "{n} topic files"}
                {If flat format: "⚠️ flat format (v0.1.x) — run /edikt:compile to migrate"}
  Sentinels:    {n}/{total} documents have directive sentinels ({pct}%)
                {If pct < 100: "run /edikt:review-governance to generate missing sentinels"}
  Overrides:    {n} rule overrides, {m} template overrides
                {If any: list them}
  Plan:         {plan name} Phase {n}/{total} — {status}

{If active spec exists:}
ACTIVE SPEC
  {SPEC-NNN}: {title} ({status})
  Artifacts: {accepted}/{total} accepted
  Drift: {last drift date and summary, or "not run yet — run /edikt:drift {SPEC-NNN}"}
         {If last drift had divergences: "⚠️ {n} diverged — run /edikt:drift to recheck"}

CHAIN STATUS
  {chain string from step 3}

{If gate events exist:}
GATE ACTIVITY (this session)
  {For each gate event:}
  ⛔ {agent}: {finding summary} ({resolved/overridden})
  {If no gate events:}
  ✅ No gate findings this session

{If agent events exist:}
AGENT ACTIVITY (this session)
  {agent name}  — ran {n}x ({contexts: plan pre-flight, review, etc.})
  {If no agent events:}
  No agent activity this session

HOOK ACTIVITY (this session)
  {For each hook type with activity:}
  {HookName}         — {n} fires ({description})
  {If no hook activity:}
  No hook activity this session

{If signal events exist:}
SIGNALS DETECTED
  💡 {ADR candidate signals}
  📄 {Doc gap signals}
  🔒 {Security signals}
  {If no signals:}
  No signals detected this session

WHAT'S NEXT
  Phase {n} — {title}
  {1-3 bullet points summarising tasks}

WARNINGS
  {Any issues: missing project context, stale plan, draft artifacts, etc.}
  {Or: "All clear — governance is healthy."}

═══════════════════════════════════════════════

  Next: Run /edikt:plan to continue active work, or /edikt:doctor for a deeper check.
```

### 5. Write STATUS.md

After displaying the dashboard, write the same content to `docs/STATUS.md` using sentinel comments:

```
<!-- edikt:status:start — updated by /edikt:status, do not edit manually -->
{dashboard content}
<!-- edikt:status:end -->
```

If `docs/STATUS.md` exists: replace only between the sentinels.
If it doesn't exist: create it with the edikt block only.

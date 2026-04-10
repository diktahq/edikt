---
name: edikt:doctor
description: "Validate governance setup and report actionable warnings"
effort: normal
context: fork
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
---

# edikt:doctor

Validate the entire edikt governance setup and report what's healthy, what's missing, and how to fix it.

CRITICAL: NEVER skip a check or assume it passes — run every check from the Reference section and report an explicit status for each one.

## Arguments

- `--json` — output only the JSON format (see Reference). No progress indicators, no emoji, no prose.

## Instructions

0. If `--json` is in `$ARGUMENTS`, output only the JSON format at the end — no progress indicators, no emoji, no prose.

1. Read `.edikt/config.yaml`. If not found, output `[FAIL] No edikt config found. Run /edikt:init to set up this project.` and stop.

2. Extract `base:` directory from config (default: `docs`).

3. Run all checks in parallel where possible. Use the check definitions in the Reference section below. For each check, report `[ok]`, `[!!]`, or `[FAIL]` as defined there.

4. Run checks in this order: Config, Project Context, Decisions, Invariants, Rules, Rule pack freshness, CLAUDE.md sentinel, Hooks (PreToolUse + PreCompact), Hooks (SessionStart + Stop), Product spec, Plans, Auto-memory, Agents, Extensibility, Project templates, Template reference examples, Compiled governance, Directive sentinel schema, Linter sync, edikt version.

5. Run the Decision Graph checks from the Reference section. Report findings inline with the other checks.

6. Output the report using the Output Format in the Reference section.

7. If there are warnings or failures, list actionable next steps:
   ```
   Recommendations:
     1. {first issue} — run {command}
     2. {second issue} — run {command}
   ```

8. If everything passes, output:
   ```
   All clear — governance is healthy.

   Next: No action needed — governance is healthy.
   ```

## Reference

### JSON Output Format

```json
{
  "status": "warnings",
  "checks": [
    {"name": "config", "status": "ok", "detail": "valid YAML"},
    {"name": "rules", "status": "warning", "detail": "2 packs outdated"}
  ],
  "summary": {"ok": 12, "warnings": 2, "failures": 0}
}
```

### Check Definitions

**Config:**
```bash
python3 -c "import yaml; yaml.safe_load(open('.edikt/config.yaml'))" 2>&1 || echo "INVALID"
```
- `[ok]` if valid YAML
- `[FAIL]` if parse error — show the error

**Project Context:**
- Check if `{base}/project-context.md` exists
- `[ok]` if present, `[!!]` if missing — suggest `/edikt:init`

**Decisions:**
```bash
ls {base}/decisions/*.md 2>/dev/null | wc -l
```
- `[ok] {base}/decisions/ — {n} ADRs`
- `[!!]` if directory missing or empty — suggest `/edikt:intake` or creating first ADR

**Invariants:**
```bash
ls {base}/invariants/*.md 2>/dev/null | wc -l
```
- `[ok] {base}/invariants/ — {n} invariants`
- `[ok]` if empty (invariants are optional) — note "none defined"

**Rules:**
```bash
ls .claude/rules/*.md 2>/dev/null | wc -l
```
- `[ok] .claude/rules/ — {n} packs installed`
- `[!!]` if empty — suggest `/edikt:init` to install rule packs
- For each rule file, check for `<!-- edikt:generated -->` marker. If missing, note as manually edited (informational, not a warning).

**Rule pack freshness:**

For each `.claude/rules/*.md` file that has the `edikt:generated` marker:
1. Read its `version:` from YAML frontmatter
2. Look up the pack name (filename without `.md`) in the registry
3. Compare versions:
   - Installed version == registry version: no output (already covered by Rules check)
   - Installed version < registry version: `[!!] {name} outdated (installed: {old}, available: {new}) — run /edikt:rules-update`
   - No `version:` in installed file: `[!!] {name} has no version — may predate versioning`
   - Pack not in registry (custom rule): skip silently

**CLAUDE.md sentinel:**
```bash
grep -q 'edikt:' CLAUDE.md 2>/dev/null
```
- `[ok]` if CLAUDE.md contains a edikt reference
- `[!!]` if missing — suggest `/edikt:init` to generate CLAUDE.md

**Hooks (PreToolUse + PreCompact):**
```bash
python3 -c "
import json
s = json.load(open('.claude/settings.json'))
hooks = s.get('hooks', {})
pre_tool = hooks.get('PreToolUse', [])
pre_compact = hooks.get('PreCompact', [])
has_tool = any('Write|Edit' in str(h.get('matcher','')) for h in pre_tool)
has_compact = len(pre_compact) > 0
print(f'PreToolUse:{has_tool}')
print(f'PreCompact:{has_compact}')
" 2>/dev/null
```
- `[ok] PreToolUse hook (Write|Edit sentinel)` if present
- `[!!] PreToolUse hook missing` — suggest `/edikt:init`
- `[ok] PreCompact hook` if present
- `[!!] PreCompact hook missing` — suggest `/edikt:init`

**Hooks (SessionStart + Stop):**

Check for SessionStart and Stop hooks in `.claude/settings.json`:

```python
import json
s = json.load(open('.claude/settings.json'))
# SessionStart
cmd = s.get('hooks', {}).get('SessionStart', [{}])[0].get('hooks', [{}])[0].get('command', '')
# Stop
stop = s.get('hooks', {}).get('Stop', [{}])[0].get('hooks', [{}])[0]
stop_prompt = stop.get('prompt', '') if stop.get('type') == 'prompt' else ''
```
- `[ok] SessionStart hook` if command references `.edikt/hooks/session-start.sh`
- `[!!] SessionStart hook outdated (inline bash) — run /edikt:upgrade` if command is inline bash
- `[ok] Stop hook` if prompt contains `"ok": true` AND does not use `"ok": false` for signals
- `[!!] Stop hook outdated (JSON validation error) — run /edikt:upgrade` if prompt uses old free-text format
- `[!!] Stop hook causes blocking error — run /edikt:upgrade` if prompt uses `{"ok": false, "reason":` to deliver signals (causes "Prompt hook condition was not met" error)

**Product spec:**
- Check if `{base}/product/spec.md` exists
- `[ok]` if present
- `[!!]` if missing — suggest `/edikt:intake` to onboard existing specs

**Plans:**
```bash
ls {base}/product/plans/PLAN-*.md {base}/plans/PLAN-*.md 2>/dev/null
```
- `[ok] {n} plans found` — list active ones
- `[!!] No PLAN-*.md found` — suggest `/edikt:plan`

**Auto-memory:**
```bash
ENCODED=$(echo "$PWD" | sed 's|/|-|g')
MEMORY="$HOME/.claude/projects/${ENCODED}/memory/MEMORY.md"
```
- `[ok] Memory exists ({N} days old, {lines}/200 lines)` if present and fresh
- `[!!] Memory is stale ({N} days old)` — suggest `/edikt:context` to refresh
- `[!!] Memory missing` — suggest `/edikt:context` to create
- `[!!] Memory near limit ({lines}/200 lines)` — if > 180 lines, suggest pruning

**Agents:**
```bash
ls .claude/agents/*.md 2>/dev/null
```
- `[ok] {n} agents installed` if present
- `[--] No agents installed` — suggest `/edikt:init` or `/edikt:agents suggest`
- For each agent, check if it's customized:
  - Contains `<!-- edikt:custom -->` → note as "custom"
  - Listed in `.edikt/config.yaml` `agents.custom` → note as "custom (config)"
  - Report: `[ok] {n} agents installed ({m} custom, {k} default)`

**Extensibility:**
- Check `.edikt/templates/` for template overrides:
  - For each file found: `[ok] Template override: {name}.md`
- Check `.edikt/rules/` for rule overrides:
  - For each file found: `[ok] Rule override: {name}.md`
- Check `rules.{name}.extend` in config for rule extensions:
  - For each configured: check if the extension file exists
  - `[ok] Rule extension: {name} + {extend_file}`
  - `[!!] Rule extension configured but file missing: {extend_file}`

**Project templates (ADR-009 + Phase 3 of v0.3.0):**

Check the state of the three per-artifact project templates that `/edikt:<artifact>:new` consults via the lookup chain. Reports differ based on the project's `edikt_version` because v0.2.x legacy projects are allowed to run without project templates (they fall through to the inline fallback with a warning).

```bash
PROJECT_EDIKT_VERSION=$(grep '^edikt_version:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"')
```

For each of the three templates (`.edikt/templates/adr.md`, `.edikt/templates/invariant.md`, `.edikt/templates/guideline.md`):

- **Template present** (`.edikt/templates/<artifact>.md` exists):
  - Verify the file has the mandatory `[edikt:directives:start]: #` / `[edikt:directives:end]: #` sentinel block.
  - If sentinel block is present: `[ok] Project {artifact} template: .edikt/templates/{artifact}.md ({kind})` where `{kind}` is derived from a marker comment in the template (`Adapted`, `Nygard-minimal`, `MADR-extended`, `Minimal`, `Full`, `Extended`, or `Custom` if no marker).
  - If sentinel block is missing: `[!!] Project {artifact} template has no directives sentinel block — run /edikt:init --reset-templates to regenerate`
- **Template absent AND project is v0.3.0+** (`edikt_version >= 0.3.0`):
  - `[!!] Project {artifact} template missing (.edikt/templates/{artifact}.md) — required for v0.3.0+. Run /edikt:init or /edikt:init --reset-templates to set up.`
- **Template absent AND project is v0.2.x legacy** (`edikt_version < 0.3.0` or missing):
  - `[--] Project {artifact} template not set up (v0.2.x legacy mode). Run /edikt:upgrade followed by /edikt:init to opt into the v0.3.0 template adaptation feature.`

For the Invariant Record template specifically (ADR-009 terminology): always refer to it as "Invariant Record template" in doctor output, not just "invariant template". Reinforces the coinage.

**Template reference examples (v0.3.0+):**

Check that the shipped reference examples exist in `~/.edikt/templates/examples/`:
```bash
for example in adr-nygard-minimal adr-madr-extended invariant-minimal invariant-full guideline-minimal guideline-extended; do
    if [ -f "$HOME/.edikt/templates/examples/${example}.md" ]; then
        echo "ok: $example"
    else
        echo "missing: $example"
    fi
done
```

- `[ok] Reference templates installed ({n}/6)` if all six are present.
- `[!!] Reference templates incomplete ({n}/6 found) — run curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash to reinstall` if any are missing.

**Compiled governance:**
- Check if `.claude/rules/governance.md` exists and contains `Routing Table`
- `[ok] Compiled governance — index + {n} topic files` if governance.md + governance/ directory exist
- `[!!] Compiled governance uses flat format (v0.1.x) — run /edikt:gov:compile to migrate` if governance.md exists but no governance/ directory
- `[!!] No compiled governance — run /edikt:gov:compile` if governance.md missing
- **Compile schema version check** (see ADR-007). The current schema is `COMPILE_SCHEMA_VERSION = 2`, declared at the top of `commands/gov/compile.md`. For generated `governance.md`:
  - Read `compile_schema_version` from YAML frontmatter.
  - If missing: `[!!] Governance uses legacy version stamp (no compile_schema_version) — run /edikt:gov:compile to regenerate`
  - If `< 2`: `[!!] Governance compiled with schema v{n} (current: v2) — run /edikt:gov:compile to regenerate`
  - If `> 2`: `[!!] Governance compiled with schema v{n} — installed edikt only supports v2. Upgrade edikt: curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash`
  - If equal: no output (covered by the `[ok]` line above)
  - Note: `compiled_by` and `compiled_at` HTML comments are informational only. NEVER check them programmatically or use them to determine staleness.
- For each topic file in `governance/`, check `paths:` frontmatter exists:
  - `[ok] governance/{topic}.md — paths: {glob summary}`
  - `[!!] governance/{topic}.md has no paths: frontmatter — run /edikt:gov:compile`
- **Override detection:** For each rule pack in `.claude/rules/`, check if any of its rules conflict with compiled governance directives:
  - `[!!] Rule pack {name}.md may conflict with compiled governance: {brief description}`
- **Sentinel coverage:** Count source documents (ADRs, invariants) with and without `[edikt:directives:start]` sentinel blocks:
  - `[ok] Directive sentinels: {n}/{total} documents ({pct}%)`
  - `[!!] {m} documents missing directive sentinels — run /edikt:review-governance`

**Linter sync:**
```bash
find . -maxdepth 3 -name ".golangci-lint.yaml" -o -name ".golangci.yaml" -o -name ".eslintrc*" -o -name "eslint.config.*" -o -name "ruff.toml" -o -name ".rubocop.yml" -o -name "biome.json" 2>/dev/null | grep -v node_modules | grep -v .git
ls .claude/rules/linter-*.md 2>/dev/null
```
- For each linter config found with no corresponding `.claude/rules/linter-*.md`: `[!!] {config} found but no linter rules installed — run /edikt:sync`
- For each `.claude/rules/linter-*.md`: compare its mtime to source config mtime. If config is newer: `[!!] Linter config changed since last sync — run /edikt:sync`
- If no linter configs found: skip silently

**Directive sentinel schema (ADR-008):**

For each compiled artifact (ADRs, invariants, guidelines) that has a `[edikt:directives:start]: #` block, check whether the block conforms to the v0.3.0 three-list schema from ADR-008. This is informational — the block is still processed correctly via the backward-compatibility path, but reporting which files are on which schema lets the user decide whether to run `<artifact>:compile --regenerate` to migrate.

For each artifact file (iterate across `{paths.decisions}/*.md`, `{paths.invariants}/*.md`, and `{paths.guidelines}/*.md`):

1. Check if a directives sentinel block exists. If not, skip (doctor's sentinel-coverage check above already reports missing sentinels).
2. Parse the YAML block and check for the presence of these fields:
   - `source_hash` (required in v0.3.0 schema)
   - `directives_hash` (required in v0.3.0 schema)
   - `compiler_version` (required in v0.3.0 schema)
   - `manual_directives:` (optional but should be present as empty list in v0.3.0 blocks)
   - `suppressed_directives:` (optional but should be present as empty list in v0.3.0 blocks)
   - `content_hash` (v0.2.x legacy field — should be absent in v0.3.0 blocks)

Classify each artifact:

- **v0.3.0 current**: has all three hash fields AND `manual_directives:` + `suppressed_directives:` present (even if empty). Report silently — this is the expected state.
- **v0.3.0 partial**: has hash fields but missing `manual_directives:` or `suppressed_directives:` keys entirely. Report: `[--] {file}: three-list schema partial (missing manual_directives/suppressed_directives keys) — next /edikt:<artifact>:compile run will normalize`
- **v0.2.x legacy**: has `content_hash:` but none of the new hash fields. Report: `[--] {file}: v0.2.x legacy directive block — run /edikt:<artifact>:compile to migrate to the v0.3.0 three-list schema`
- **Unknown**: sentinel block exists but has neither `content_hash:` nor `source_hash:`. Report: `[!!] {file}: unrecognized directive block format — inspect manually`

Summarize at the top:
```
Directive sentinel schema:
  ✓ v0.3.0 current:  {n} artifacts
  ⚪ v0.3.0 partial:  {m} artifacts (will normalize on next compile)
  ⚪ v0.2.x legacy:   {k} artifacts (run compile to migrate)
  ✗ unrecognized:    {j} artifacts
```

Migrations are never urgent — legacy blocks continue to work via gov:compile's backward compatibility path. The report exists so users know which files haven't been touched since v0.2.x.

**edikt version:**
```bash
INSTALLED=$(cat ~/.edikt/VERSION 2>/dev/null | tr -d '[:space:]' || echo "unknown")
PROJECT=$(grep '^edikt_version:' .edikt/config.yaml | awk '{print $2}' | tr -d '"' || echo "unknown")
```
- `[ok] edikt {PROJECT} (installed: {INSTALLED})` if versions match
- `[!!] project on edikt {PROJECT}, installed is {INSTALLED} — run /edikt:upgrade` if they differ
- `[!!] edikt_version not set in .edikt/config.yaml — run /edikt:upgrade` if key missing

### Decision Graph Checks

Check the decision graph for consistency. Read all ADRs, invariants, and specs:

1. **ADR contradictions:** For each pair of ADRs with status `accepted`, check if they make contradictory decisions on the same topic. Example: ADR-001 says "Claude Code only" and ADR-007 says "support Cursor." Report:
   - `[!!] ADR contradiction: ADR-001 and ADR-007 make opposing decisions on multi-tool support`

2. **Rule-invariant consistency:** For each invariant, check if any installed `.claude/rules/*.md` file contradicts it. Example: invariant says "no compiled code" but a rule references TypeScript compilation. Report:
   - `[!!] Rule {name} may conflict with invariant {INV-NNN}: {reason}`

3. **Plan-ADR dependencies:** For each active plan, check if it references any ADRs with status `superseded`. Report:
   - `[!!] PLAN-{NNN} references ADR-{NNN} which is superseded — review plan assumptions`

4. **Invariant enforcement:** For each invariant, check if any rule or hook enforces it. If an invariant exists but nothing references or enforces it:
   - `[!!] INV-{NNN} is not referenced by any rule or hook — consider adding enforcement`

5. **Orphan artifacts:** Check for ADRs, PRDs, or specs that are not referenced by any other artifact (no plan, no spec, no references field points to them):
   - `[!!] ADR-{NNN} is not referenced by any spec or plan — still relevant or supersede?`

6. **Artifact status stale:** Check for PRDs or specs stuck in `draft` status for more than 7 days (based on file mtime):
   - `[!!] PRD-{NNN} has been in draft for {n} days — accept or archive`

7. **State machine violations:** Check if any spec references a PRD that is not `accepted`, or if any plan references artifacts that are not `accepted`:
   - `[!!] SPEC-{NNN} references PRD-{NNN} which is still in draft — PRD should be accepted first`

### Output Format

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 EDIKT DOCTOR
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

 [ok]   edikt {version} (installed: {installed})
 [ok]   .edikt/config.yaml valid
 [ok]   {base}/project-context.md exists
 [ok]   {base}/decisions/ — {n} ADRs
 [ok]   {base}/invariants/ — {n} invariants
 [ok]   .claude/rules/ — {n} packs installed
 [ok]   CLAUDE.md has edikt sentinel
 [ok]   SessionStart hook
 [ok]   Stop hook (artifact suggestions)
 [ok]   PreCompact hook
 [ok]   {base}/product/spec.md exists
 [ok]   {n} plans found
 [ok]   {n} agents installed in .claude/agents/
 [ok]   Memory: {n} days old, {lines}/200 lines

Note: Number all [!!] and [FAIL] items sequentially (#1, #2, #3...) so the user can reference them. [ok] items don't need numbers.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 {pass_count} passed, {warn_count} warnings, {fail_count} failures
 {If warnings or failures: "Which issues should I fix? (e.g., #1, #3 or 'all')"}
 Next: Fix the issues above, or say "fix #1, #3" to address specific items.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

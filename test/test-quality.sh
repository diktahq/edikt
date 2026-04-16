#!/bin/bash
# Test: template quality, rule pack quality, extensibility, configurable paths
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

# ============================================================
# Rule pack quality — v0.1.0 standards
# ============================================================

RULES_DIR="$PROJECT_ROOT/templates/rules"

# All rule packs should be v0.1.0
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    if grep -q 'version: "0.1.0"' "$rule" 2>/dev/null; then
        pass "Rule pack version 0.1.0: $name"
    else
        fail "Rule pack version 0.1.0: $name"
    fi
done

# All rule packs should have edikt:generated marker
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    assert_file_contains "$rule" "edikt:generated" "Has edikt:generated marker: $name"
done

# All rule packs must have governance checkpoint
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    assert_file_contains "$rule" "<governance_checkpoint>" "Has governance checkpoint: $name"
done

# Governance checkpoint must appear before the first heading (correct position)
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    cp_line=$(grep -n "<governance_checkpoint>" "$rule" 2>/dev/null | head -1 | cut -d: -f1)
    h1_line=$(grep -n "^# " "$rule" 2>/dev/null | head -1 | cut -d: -f1)
    if [ -n "$cp_line" ] && [ -n "$h1_line" ] && [ "$cp_line" -lt "$h1_line" ]; then
        pass "Checkpoint before title: $name"
    else
        fail "Checkpoint before title: $name"
    fi
done

# Base packs should NOT use paths: "**/*" (should scope to code files)
for rule in "$RULES_DIR"/base/*.md; do
    name=$(basename "$rule")
    if grep -q 'paths: "\*\*/\*"' "$rule" 2>/dev/null; then
        fail "Base pack should scope to code files, not **/*: $name"
    else
        pass "Base pack properly scoped: $name"
    fi
done

# All packs should use four-tier phrasing (at least NEVER or MUST)
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    if grep -qE "^- NEVER |^- MUST " "$rule" 2>/dev/null; then
        pass "Uses NEVER/MUST phrasing: $name"
    else
        fail "Missing NEVER/MUST phrasing: $name"
    fi
done

# Rule count should be 7-25 per pack (lower bound reduced after cross-pack dedup)
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    count=$(grep -cE "^- " "$rule" 2>/dev/null)
    if [ "$count" -ge 7 ] && [ "$count" -le 25 ]; then
        pass "Rule count in range (${count}): $name"
    else
        fail "Rule count out of range (${count}, want 7-25): $name"
    fi
done

# New packs exist
assert_file_exists "$RULES_DIR/base/api.md" "New pack exists: api.md"
assert_file_exists "$RULES_DIR/base/database.md" "New pack exists: database.md"
assert_file_exists "$RULES_DIR/base/observability.md" "New pack exists: observability.md"
assert_file_exists "$RULES_DIR/base/seo.md" "New pack exists: seo.md"

# ============================================================
# Template quality — new sections in artifact templates
# ============================================================

# PRD template has numbered requirements
assert_file_contains "$PROJECT_ROOT/commands/sdlc/prd.md" "FR-001" "PRD has numbered requirements (FR-001)"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/prd.md" "AC-001" "PRD has numbered acceptance criteria (AC-001)"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/prd.md" "NEEDS CLARIFICATION" "PRD has NEEDS CLARIFICATION markers"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/prd.md" "Verify:" "PRD acceptance criteria have verification methods"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/prd.md" "stakeholders" "PRD has stakeholders in frontmatter"

# ADR template has MADR sections
assert_file_contains "$PROJECT_ROOT/commands/adr/new.md" "Confirmation" "ADR has Confirmation section"
assert_file_contains "$PROJECT_ROOT/commands/adr/new.md" "Decision Drivers" "ADR has Decision Drivers section"
assert_file_contains "$PROJECT_ROOT/commands/adr/new.md" "decision-makers" "ADR has decision-makers in frontmatter"
assert_file_contains "$PROJECT_ROOT/commands/adr/new.md" "supersedes" "ADR has supersedes in frontmatter"
assert_file_contains "$PROJECT_ROOT/commands/adr/new.md" "Rejected because" "ADR alternatives have rejection reasons"

# ADR template has directive sentinel block
assert_file_contains "$PROJECT_ROOT/commands/adr/new.md" "edikt:directives:start" "ADR template has directive sentinel start"
assert_file_contains "$PROJECT_ROOT/commands/adr/new.md" "edikt:directives:end" "ADR template has directive sentinel end"
assert_file_contains "$PROJECT_ROOT/commands/adr/new.md" "scope:" "ADR template has scope in sentinel"

# Invariant template has directive sentinel block
assert_file_contains "$PROJECT_ROOT/commands/invariant/new.md" "edikt:directives:start" "Invariant template has directive sentinel start"
assert_file_contains "$PROJECT_ROOT/commands/invariant/new.md" "edikt:directives:end" "Invariant template has directive sentinel end"

# Invariant template has new sections
assert_file_contains "$PROJECT_ROOT/commands/invariant/new.md" "severity:" "Invariant has severity in frontmatter"
assert_file_contains "$PROJECT_ROOT/commands/invariant/new.md" "scope:" "Invariant has scope in frontmatter"
assert_file_contains "$PROJECT_ROOT/commands/invariant/new.md" "Violation Consequences" "Invariant has Violation Consequences"
assert_file_contains "$PROJECT_ROOT/commands/invariant/new.md" "Verification" "Invariant has Verification section"
assert_file_contains "$PROJECT_ROOT/commands/invariant/new.md" "Exceptions" "Invariant has Exceptions section"

# Spec template has new sections
assert_file_contains "$PROJECT_ROOT/commands/sdlc/spec.md" "Non-Goals" "Spec has Non-Goals section"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/spec.md" "Alternatives Considered" "Spec has Alternatives Considered"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/spec.md" "Risks" "Spec has Risks section"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/spec.md" "AC-001" "Spec has numbered acceptance criteria"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/spec.md" "NEEDS CLARIFICATION" "Spec has NEEDS CLARIFICATION markers"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/spec.md" "implements:" "Spec uses implements: (not source_prd:)"

# Compile output has primacy + recency
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "Non-Negotiable Constraints" "Compile has constraints at top"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "Reminder:" "Compile has reminder at bottom (recency)"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "directives:" "Compile output has directive count"

# Compile v0.2.0 — topic-grouped output
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "governance/" "Compile writes to governance/ directory"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "Routing Table" "Compile generates routing table"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "edikt:directives:start" "Compile reads directive sentinels"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "edikt:directives:end" "Compile reads directive sentinel end"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "sentinel block" "Compile handles missing sentinels"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "topic" "Compile groups by topic"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "scope:" "Compile supports scope tags"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "Sentinel coverage" "Compile reports sentinel coverage"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "reverse source map" "Compile outputs reverse source map"
assert_file_not_contains "$PROJECT_ROOT/commands/gov/compile.md" "exceeds 30 directives" "Compile no longer has 30-directive cap"

# ============================================================
# Command quality — CRITICAL + REMEMBER blocks
# ============================================================

for cmd_path in doctor sdlc/audit sdlc/drift sdlc/plan gov/compile sdlc/review brainstorm; do
    assert_file_contains "$PROJECT_ROOT/commands/${cmd_path}.md" "CRITICAL:" "${cmd_path} has CRITICAL statement"
done

for cmd_path in sdlc/prd adr/new invariant/new sdlc/spec sdlc/artifacts gov/compile sdlc/drift; do
    assert_file_contains "$PROJECT_ROOT/commands/${cmd_path}.md" "REMEMBER:" "${cmd_path} has REMEMBER block"
done

# Commands use paths: config (not hardcoded)
for cmd_path in sdlc/prd adr/new invariant/new sdlc/spec sdlc/drift gov/compile; do
    assert_file_contains "$PROJECT_ROOT/commands/${cmd_path}.md" "paths:" "${cmd_path} references paths: config"
done

# Interactive commands have plan mode guard
for cmd_path in init sdlc/prd sdlc/spec sdlc/artifacts adr/new invariant/new docs/intake sdlc/plan brainstorm; do
    assert_file_contains "$PROJECT_ROOT/commands/${cmd_path}.md" "plan mode" "${cmd_path} has plan mode guard"
done

# Brainstorm command has required phases and features
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "Open Exploration" "brainstorm has open exploration phase"
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "Guided Narrowing" "brainstorm has guided narrowing phase"
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "Formalize" "brainstorm has formalize step"
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "BRAIN-" "brainstorm uses BRAIN- prefix"
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "fresh" "brainstorm supports --fresh flag"
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "unconstrained" "brainstorm has unconstrained mode"

# Plan command has artifact coverage check
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Artifact coverage check" "Plan has artifact coverage step"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Artifact Coverage Table" "Plan has artifact coverage table"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "fixtures" "Plan covers fixtures artifacts"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "test-strategy" "Plan covers test strategy artifacts"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Uncovered API endpoints" "Plan warns on uncovered endpoints"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Inventory all artifacts" "Plan inventories spec artifacts"

# Plan command accepts flexible input
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "SPEC identifier" "plan accepts SPEC input"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Ticket ID" "plan accepts ticket input"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Existing plan name" "plan accepts PLAN input"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Natural language" "plan accepts natural language"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Quick plan" "plan offers disambiguation"

# Upgrade command has remote version check
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "Check for Updates" "upgrade has remote version check"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "offline" "upgrade supports --offline flag"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "raw.githubusercontent.com/diktahq/edikt/main/VERSION" "upgrade fetches VERSION from GitHub"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "max-time 5" "upgrade has 5s curl timeout"

# Website plan page matches command behavior
assert_file_contains "$PROJECT_ROOT/website/commands/sdlc/plan.md" "PLAN-NNN" "website plan page documents PLAN input"
assert_file_contains "$PROJECT_ROOT/website/commands/sdlc/plan.md" "Quick plan" "website plan page documents disambiguation"

# ============================================================
# Natural language triggers
# ============================================================

# CLAUDE.md template has intent-based trigger table
assert_file_contains "$PROJECT_ROOT/templates/CLAUDE.md.tmpl" "Match the user" "Template has intent-matching instruction"

# All commands have a trigger in the template (flat and namespaced)
for cmd in status context doctor init upgrade brainstorm session team agents mcp capture; do
    assert_file_contains "$PROJECT_ROOT/templates/CLAUDE.md.tmpl" "edikt:${cmd}" "Template has trigger for ${cmd}"
done
for cmd in adr:new adr:compile adr:review invariant:new invariant:compile invariant:review guideline:new guideline:review gov:compile gov:review gov:rules-update gov:sync sdlc:prd sdlc:spec sdlc:artifacts sdlc:plan sdlc:review sdlc:drift sdlc:audit docs:review docs:intake; do
    assert_file_contains "$PROJECT_ROOT/templates/CLAUDE.md.tmpl" "edikt:${cmd}" "Template has trigger for ${cmd}"
done

# ============================================================
# Configurable paths
# ============================================================

# Config has paths section
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "paths:" "Config has paths: section"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "decisions:" "Config has decisions path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "invariants:" "Config has invariants path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "plans:" "Config has plans path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "specs:" "Config has specs path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "prds:" "Config has prds path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "guidelines:" "Config has guidelines path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "reports:" "Config has reports path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "project-context:" "Config has project-context path"
assert_file_not_contains "$PROJECT_ROOT/.edikt/config.yaml" "  soul:" "Config does not use deprecated soul key"

# Config has features section
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "features:" "Config has features: section"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "auto-format:" "Config has auto-format feature"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "session-summary:" "Config has session-summary feature"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "signal-detection:" "Config has signal-detection feature"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "plan-injection:" "Config has plan-injection feature"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "quality-gates:" "Config has quality-gates feature"

# ============================================================
# Config key migration: soul → project-context (v0.4.0)
# ============================================================

# Commands reference project-context, not soul
assert_file_not_contains "$PROJECT_ROOT/commands/config.md" "paths.soul" "config.md uses project-context not soul"
assert_file_not_contains "$PROJECT_ROOT/commands/sdlc/prd.md" "paths.soul" "prd.md uses project-context not soul"
assert_file_not_contains "$PROJECT_ROOT/commands/brainstorm.md" "paths.soul" "brainstorm.md uses project-context not soul"
assert_file_not_contains "$PROJECT_ROOT/commands/init.md" "  soul:" "init.md config template uses project-context not soul"

# Upgrade command has migration path
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "paths.soul" "Upgrade command documents soul migration"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "paths.project-context" "Upgrade command documents project-context target"

# ============================================================
# Command step ordering — prevent premature conclusions
# ============================================================

# plan.md: pre-flight (step 8) must come BEFORE write plan file (step 10)
PLAN_CMD="$PROJECT_ROOT/commands/sdlc/plan.md"
PLAN_PREFLIGHT_LINE=$(grep -n "Run pre-flight specialist review" "$PLAN_CMD" | head -1 | cut -d: -f1)
PLAN_WRITE_LINE=$(grep -n "Write the plan file" "$PLAN_CMD" | head -1 | cut -d: -f1)
PLAN_OUTPUT_LINE=$(grep -n "Output next steps" "$PLAN_CMD" | head -1 | cut -d: -f1)

if [ -n "$PLAN_PREFLIGHT_LINE" ] && [ -n "$PLAN_WRITE_LINE" ] && [ "$PLAN_PREFLIGHT_LINE" -lt "$PLAN_WRITE_LINE" ]; then
    pass "plan.md: pre-flight review before write plan file"
else
    fail "plan.md: pre-flight review (line $PLAN_PREFLIGHT_LINE) must come before write plan file (line $PLAN_WRITE_LINE)"
fi

if [ -n "$PLAN_PREFLIGHT_LINE" ] && [ -n "$PLAN_OUTPUT_LINE" ] && [ "$PLAN_PREFLIGHT_LINE" -lt "$PLAN_OUTPUT_LINE" ]; then
    pass "plan.md: pre-flight review before output next steps"
else
    fail "plan.md: pre-flight review (line $PLAN_PREFLIGHT_LINE) must come before output next steps (line $PLAN_OUTPUT_LINE)"
fi

# plan.md: "Next: Review the plan" must not appear before pre-flight
PLAN_NEXT_LINE=$(grep -n "Next:.*execute Phase\|Next:.*Review the plan" "$PLAN_CMD" | head -1 | cut -d: -f1)
if [ -n "$PLAN_NEXT_LINE" ] && [ -n "$PLAN_PREFLIGHT_LINE" ] && [ "$PLAN_NEXT_LINE" -gt "$PLAN_PREFLIGHT_LINE" ]; then
    pass "plan.md: conclusion message after pre-flight review"
else
    fail "plan.md: conclusion message (line $PLAN_NEXT_LINE) appears before pre-flight review (line $PLAN_PREFLIGHT_LINE)"
fi

# audit.md: --no-edikt jump target must reference the inline audit step, not agent-spawning
AUDIT_CMD="$PROJECT_ROOT/commands/sdlc/audit.md"
AUDIT_JUMP=$(grep "jump to step" "$AUDIT_CMD" | sed 's/.*step \([0-9]*\).*/\1/')
AUDIT_INLINE_NUM=$(grep "^[0-9].*\*\*Inline Audit Mode" "$AUDIT_CMD" | head -1 | sed 's/\([0-9]*\).*/\1/')

if [ -n "$AUDIT_JUMP" ] && [ -n "$AUDIT_INLINE_NUM" ] && [ "$AUDIT_JUMP" = "$AUDIT_INLINE_NUM" ]; then
    pass "audit.md: --no-edikt jump target matches inline audit step ($AUDIT_JUMP)"
else
    fail "audit.md: --no-edikt jump target (step $AUDIT_JUMP) doesn't match inline audit step ($AUDIT_INLINE_NUM)"
fi

# gov/review.md: "Next: Run /edikt:gov:compile" must not appear before staleness detection
REVIEW_CMD="$PROJECT_ROOT/commands/gov/review.md"
REVIEW_NEXT_LINE=$(grep -n "Next:.*compile" "$REVIEW_CMD" | head -1 | cut -d: -f1)
REVIEW_STALE_LINE=$(grep -n "Checking sentinel staleness\|Staleness Detection" "$REVIEW_CMD" | head -1 | cut -d: -f1)

if [ -n "$REVIEW_NEXT_LINE" ] && [ -n "$REVIEW_STALE_LINE" ] && [ "$REVIEW_NEXT_LINE" -gt "$REVIEW_STALE_LINE" ]; then
    pass "gov/review.md: conclusion after staleness detection"
else
    fail "gov/review.md: conclusion (line $REVIEW_NEXT_LINE) appears before staleness detection (line $REVIEW_STALE_LINE)"
fi

# ============================================================
# Hook modernization (v0.2.0)
# ============================================================

# settings.json.tmpl has if field on PostToolUse
assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" '"if":' "Settings template has conditional if field"

# PostToolUse if field scopes to code files
if grep -qF "Write(**/*.{go,ts,tsx,js,jsx,py,rb,php,rs})" "$PROJECT_ROOT/templates/settings.json.tmpl" 2>/dev/null; then
    pass "PostToolUse if scopes to code files"
else
    fail "PostToolUse if scopes to code files"
fi

# InstructionsLoaded if field scopes to rules
if grep -qF "Read(.claude/rules/*.md)" "$PROJECT_ROOT/templates/settings.json.tmpl" 2>/dev/null; then
    pass "InstructionsLoaded if scopes to rules"
else
    fail "InstructionsLoaded if scopes to rules"
fi

# New hook events exist in settings template
assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" "StopFailure" "Settings has StopFailure event"
assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" "TaskCreated" "Settings has TaskCreated event"
assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" "CwdChanged" "Settings has CwdChanged event"
assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" "FileChanged" "Settings has FileChanged event"

# New hook scripts exist
assert_file_exists "$PROJECT_ROOT/templates/hooks/stop-failure.sh" "StopFailure hook script exists"
assert_file_exists "$PROJECT_ROOT/templates/hooks/task-created.sh" "TaskCreated hook script exists"
assert_file_exists "$PROJECT_ROOT/templates/hooks/cwd-changed.sh" "CwdChanged hook script exists"
assert_file_exists "$PROJECT_ROOT/templates/hooks/file-changed.sh" "FileChanged hook script exists"

# New hook scripts source event-log.sh or log to session-signals
assert_file_contains "$PROJECT_ROOT/templates/hooks/stop-failure.sh" "event-log.sh" "StopFailure uses event logger"
# task-created.sh v0.5.0 (Phase 16) writes jsonl directly with plan-phase
# context instead of sourcing event-log.sh. Assert the new contract.
assert_file_contains "$PROJECT_ROOT/templates/hooks/task-created.sh" "events.jsonl" "TaskCreated writes events.jsonl (v0.5.0 contract)"
assert_file_contains "$PROJECT_ROOT/templates/hooks/task-created.sh" "task_created" "TaskCreated emits task_created event type"
assert_file_contains "$PROJECT_ROOT/templates/hooks/cwd-changed.sh" "session-signals.log" "CwdChanged logs to session signals"
assert_file_contains "$PROJECT_ROOT/templates/hooks/file-changed.sh" "session-signals.log" "FileChanged logs to session signals"

# FileChanged warns about governance files
assert_file_contains "$PROJECT_ROOT/templates/hooks/file-changed.sh" "Governance file modified externally" "FileChanged warns about governance changes"

# Hooks respect feature config
assert_file_contains "$PROJECT_ROOT/templates/hooks/post-tool-use.sh" "auto-format: false" "PostToolUse checks auto-format config"
assert_file_contains "$PROJECT_ROOT/templates/hooks/session-start.sh" "session-summary: false" "SessionStart checks session-summary config"
assert_file_contains "$PROJECT_ROOT/templates/hooks/stop-hook.sh" "signal-detection: false" "Stop checks signal-detection config"
assert_file_contains "$PROJECT_ROOT/templates/hooks/user-prompt-submit.sh" "plan-injection: false" "UserPromptSubmit checks plan-injection config"
assert_file_contains "$PROJECT_ROOT/templates/hooks/subagent-stop.sh" "quality-gates: false" "SubagentStop checks quality-gates config"

# ============================================================
# Agent governance frontmatter (v0.2.0)
# ============================================================

AGENTS_DIR="$PROJECT_ROOT/templates/agents"

# All agents must have maxTurns (evaluator-headless is a headless system prompt — no frontmatter)
for agent in "$AGENTS_DIR"/*.md; do
    name=$(basename "$agent" .md)
    [ "$name" = "evaluator-headless" ] && continue
    assert_file_contains "$agent" "maxTurns:" "$name has maxTurns"
done

# All agents must have effort (evaluator-headless is a headless system prompt — no frontmatter)
for agent in "$AGENTS_DIR"/*.md; do
    name=$(basename "$agent" .md)
    [ "$name" = "evaluator-headless" ] && continue
    assert_file_contains "$agent" "effort:" "$name has effort"
done

# Read-only agents must have disallowedTools: Write, Edit
for agent in architect dba api sre docs ux compliance seo gtm data performance evaluator; do
    assert_file_contains "$AGENTS_DIR/$agent.md" "disallowedTools:" "$agent has disallowedTools"
    assert_file_contains "$AGENTS_DIR/$agent.md" "Write" "$agent disallows Write"
done

# Code-writing agents must NOT have disallowedTools
for agent in backend frontend qa mobile platform pm; do
    assert_file_not_contains "$AGENTS_DIR/$agent.md" "disallowedTools:" "$agent has no disallowedTools (code-writing)"
done

# initialPrompt on all agents (v0.5.0 rollout per ADR-014 Phase 17)
for agent in architect security pm dba api sre docs ux compliance seo gtm data performance backend frontend qa mobile platform evaluator evaluator-headless; do
    assert_file_contains "$AGENTS_DIR/$agent.md" "initialPrompt:" "$agent has initialPrompt"
done

# High effort agents
for agent in architect security qa performance; do
    assert_file_contains "$AGENTS_DIR/$agent.md" "effort: high" "$agent has effort: high"
done

# Read-only maxTurns: 10 (evaluator is 15 — separate check)
for agent in architect dba security api sre docs ux compliance seo gtm data performance; do
    assert_file_contains "$AGENTS_DIR/$agent.md" "maxTurns: 10" "$agent has maxTurns: 10"
done
assert_file_contains "$AGENTS_DIR/evaluator.md" "maxTurns: 15" "evaluator has maxTurns: 15"

# Code-writing maxTurns: 20
for agent in backend frontend qa mobile platform pm; do
    assert_file_contains "$AGENTS_DIR/$agent.md" "maxTurns: 20" "$agent has maxTurns: 20"
done

# ============================================================
# Harness improvements (v0.2.0)
# ============================================================

# Evaluator agent exists with correct governance
assert_file_exists "$AGENTS_DIR/evaluator.md" "Evaluator agent template exists"
assert_file_contains "$AGENTS_DIR/evaluator.md" "maxTurns: 15" "Evaluator has maxTurns: 15"
assert_file_contains "$AGENTS_DIR/evaluator.md" "effort: high" "Evaluator has effort: high"
assert_file_contains "$AGENTS_DIR/evaluator.md" "disallowedTools:" "Evaluator has disallowedTools"
assert_file_contains "$AGENTS_DIR/evaluator.md" "PASS" "Evaluator outputs PASS verdicts"
assert_file_contains "$AGENTS_DIR/evaluator.md" "FAIL" "Evaluator outputs FAIL verdicts"
assert_file_contains "$AGENTS_DIR/evaluator.md" "skeptical" "Evaluator is skeptical by default"
assert_file_contains "$AGENTS_DIR/evaluator.md" "evaluator-tuning" "Evaluator reads tuning doc"

# Evaluator in agent registry
assert_file_contains "$PROJECT_ROOT/templates/agents/_registry.yaml" "evaluator" "Evaluator in agent registry"

# Plan command has acceptance criteria
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Acceptance Criteria" "Plan has acceptance criteria section"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Acceptance Criteria Rules" "Plan has acceptance criteria rules"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Binary" "Plan enforces binary criteria"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Evaluate:" "Plan template has evaluate flag"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Conditional Evaluation" "Plan has conditional evaluation"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "context reset" "Plan has context reset guidance"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Phase-End Flow" "Plan has phase-end flow"

# Spec command has scope guidance and binary criteria
assert_file_contains "$PROJECT_ROOT/commands/sdlc/spec.md" "scope level" "Spec has scope guidance"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/spec.md" "binary PASS/FAIL" "Spec enforces binary criteria"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/spec.md" "flow downstream" "Spec explains criteria flow to plans"

# Harness docs exist
assert_file_exists "$PROJECT_ROOT/docs/architecture/assumptions.md" "Assumptions document exists"
assert_file_contains "$PROJECT_ROOT/docs/architecture/assumptions.md" "A-001" "Assumptions has first assumption"
assert_file_contains "$PROJECT_ROOT/docs/architecture/assumptions.md" "A-005" "Assumptions has evaluator assumption"
assert_file_contains "$PROJECT_ROOT/docs/architecture/assumptions.md" "Retired Assumptions" "Assumptions has retired section"

assert_file_exists "$PROJECT_ROOT/docs/architecture/evaluator-tuning.md" "Evaluator tuning document exists"
assert_file_contains "$PROJECT_ROOT/docs/architecture/evaluator-tuning.md" "False Positives" "Tuning doc tracks false positives"
assert_file_contains "$PROJECT_ROOT/docs/architecture/evaluator-tuning.md" "False Negatives" "Tuning doc tracks false negatives"

# Upgrade mentions assumption review
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "assumptions.md" "Upgrade references assumptions"

# ============================================================
# Platform alignment (v0.2.0)
# ============================================================

# Security guide has env hardening
assert_file_contains "$PROJECT_ROOT/website/guides/security.md" "CLAUDE_CODE_SUBPROCESS_ENV_SCRUB" "Security guide documents env scrub"
assert_file_contains "$PROJECT_ROOT/website/guides/security.md" "failIfUnavailable" "Security guide documents sandbox enforcement"

# Init command checks env hardening (migrated from team)
assert_file_contains "$PROJECT_ROOT/commands/init.md" "CLAUDE_CODE_SUBPROCESS_ENV_SCRUB" "Init checks env scrub"

# Website documents SendMessage auto-resume
assert_file_contains "$PROJECT_ROOT/website/agents.md" "SendMessage" "Website documents agent resumption"
assert_file_contains "$PROJECT_ROOT/website/agents.md" "auto-resumes" "Website explains auto-resume behavior"

# Website documents evaluator agent
assert_file_contains "$PROJECT_ROOT/website/agents.md" "evaluator" "Website documents evaluator agent"
assert_file_contains "$PROJECT_ROOT/website/agents.md" "Phase-end" "Website explains phase-end evaluation"

# ============================================================
# Headless & CI foundations (v0.2.0)
# ============================================================

# Headless hook exists
assert_file_exists "$PROJECT_ROOT/templates/hooks/headless-ask.sh" "Headless hook script exists"
assert_file_contains "$PROJECT_ROOT/templates/hooks/headless-ask.sh" "EDIKT_HEADLESS" "Headless hook checks EDIKT_HEADLESS"
assert_file_contains "$PROJECT_ROOT/templates/hooks/headless-ask.sh" "AskUserQuestion" "Headless hook intercepts AskUserQuestion"
assert_file_contains "$PROJECT_ROOT/templates/hooks/headless-ask.sh" "updatedInput" "Headless hook returns updatedInput"
assert_file_contains "$PROJECT_ROOT/templates/hooks/headless-ask.sh" "permissionDecision" "Headless hook returns permissionDecision"

# CI guide exists
assert_file_exists "$PROJECT_ROOT/website/guides/ci.md" "CI guide exists"
assert_file_contains "$PROJECT_ROOT/website/guides/ci.md" "bare" "CI guide documents --bare flag"
assert_file_contains "$PROJECT_ROOT/website/guides/ci.md" "compile --check" "CI guide shows compile check"
assert_file_contains "$PROJECT_ROOT/website/guides/ci.md" "EDIKT_HEADLESS" "CI guide documents headless mode"
assert_file_contains "$PROJECT_ROOT/website/guides/ci.md" "failIfUnavailable" "CI guide documents sandbox enforcement"
assert_file_contains "$PROJECT_ROOT/website/guides/ci.md" "GitHub Actions" "CI guide has GitHub Actions example"

# Init command detects managed settings (migrated from team)
assert_file_contains "$PROJECT_ROOT/commands/init.md" "managed-settings" "Init detects managed settings"
assert_file_contains "$PROJECT_ROOT/commands/init.md" "managed-settings.d" "Init detects policy fragments"

# install.sh-internal assertions removed in v0.5.0 Phase 5 hardening — the
# bootstrap delegates to bin/edikt; coverage now lives under
# test/unit/launcher/ and test/integration/install/.

# Reports directory exists
assert_dir_exists "$PROJECT_ROOT/docs/reports" "Reports directory exists"

# Init creates reports directory
assert_file_contains "$PROJECT_ROOT/commands/init.md" "reports" "Init creates reports directory"

# Init has rule preview (UX value signal)
assert_file_contains "$PROJECT_ROOT/commands/init.md" "Rule Preview" "Init has rule preview step"
assert_file_contains "$PROJECT_ROOT/commands/init.md" "preview of what Claude will enforce" "Init shows rule preview to user"

# Init customization UX
assert_file_contains "$PROJECT_ROOT/commands/init.md" "Keep mine" "Init has Keep mine option for edited files"
assert_file_contains "$PROJECT_ROOT/commands/init.md" "content hash" "Init uses content hash comparison"
assert_file_contains "$PROJECT_ROOT/commands/init.md" "Override a pack" "Init teaches override customization"
assert_file_contains "$PROJECT_ROOT/commands/init.md" "Extend a pack" "Init teaches extend customization"
assert_file_contains "$PROJECT_ROOT/commands/init.md" "is now yours" "Init confirms ownership transfer"

# Upgrade has hash comparison protection
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "content hash" "Upgrade uses content hash comparison"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "Keep mine" "Upgrade has Keep mine option"

# ============================================================
# Extensibility (ADR-006)
# ============================================================

# Init supports template overrides
assert_file_contains "$PROJECT_ROOT/commands/init.md" ".edikt/templates" "Init checks for template overrides"

# Upgrade respects custom agents
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "edikt:custom" "Upgrade respects custom agent marker"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "agents.custom" "Upgrade respects custom agent config"

# Rules-update handles overrides and extensions
assert_file_contains "$PROJECT_ROOT/commands/gov/rules-update.md" "Overridden" "Rules-update detects overridden packs"
assert_file_contains "$PROJECT_ROOT/commands/gov/rules-update.md" "extend" "Rules-update handles extensions"

# Rule Pack UX (v0.2.0)
assert_file_contains "$PROJECT_ROOT/commands/gov/rules-update.md" "Conflict Detection" "Rules-update has conflict detection"
assert_file_contains "$PROJECT_ROOT/commands/gov/rules-update.md" "Install Preview" "Rules-update has install preview"
assert_file_contains "$PROJECT_ROOT/commands/gov/rules-update.md" "compiled governance" "Rules-update checks against compiled governance"

# Doctor reports compiled governance status
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "Compiled governance" "Doctor checks compiled governance"
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "Routing Table" "Doctor checks for routing table"
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "Sentinel coverage" "Doctor reports sentinel coverage"
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "Override detection" "Doctor detects rule pack vs governance conflicts"

# Status shows compiled governance info
assert_file_contains "$PROJECT_ROOT/commands/status.md" "topic files" "Status shows topic file count"
assert_file_contains "$PROJECT_ROOT/commands/status.md" "Sentinels:" "Status shows sentinel coverage"
assert_file_contains "$PROJECT_ROOT/commands/status.md" "Overrides:" "Status shows override count"

# Doctor reports extensibility state
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "Template override" "Doctor reports template overrides"
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "Rule override" "Doctor reports rule overrides"

# ============================================================
# Version consistency
# ============================================================

# VERSION file
FILE_VER=$(cat "$PROJECT_ROOT/VERSION" | tr -d '[:space:]')
if echo "$FILE_VER" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$'; then
    pass "VERSION file is valid semver ($FILE_VER)"
else
    fail "VERSION file is valid semver" "Got: $FILE_VER"
fi

# Config version matches
CONFIG_VER=$(grep 'edikt_version:' "$PROJECT_ROOT/.edikt/config.yaml" | awk '{print $2}' | tr -d '"')
if [ "$CONFIG_VER" = "$FILE_VER" ]; then
    pass "Config edikt_version matches VERSION ($FILE_VER)"
else
    fail "Config edikt_version matches VERSION" "Config=$CONFIG_VER, File=$FILE_VER"
fi

# No old version references in active code (excluding historical docs)
OLD_REFS=$(grep -rn '"4\.0"\|"3\.9"\|"3\.8"' "$PROJECT_ROOT/commands/" "$PROJECT_ROOT/templates/" "$PROJECT_ROOT/install.sh" "$PROJECT_ROOT/README.md" 2>/dev/null | wc -l | tr -d ' ')
if [ "$OLD_REFS" -eq 0 ]; then
    pass "No old version references in active code"
else
    fail "Old version references found in active code" "$OLD_REFS occurrences"
fi

# ============================================================
# No old agent names in active code
# ============================================================

OLD_AGENTS=$(grep -rn "principal-\|staff-\|senior-" "$PROJECT_ROOT/commands/" "$PROJECT_ROOT/templates/" "$PROJECT_ROOT/website/" --include="*.md" --include="*.yaml" --include="*.sh" --include="*.ts" 2>/dev/null | grep -v node_modules | grep -v ".vitepress/dist" | wc -l | tr -d ' ')
if [ "$OLD_AGENTS" -eq 0 ]; then
    pass "No old agent names (principal-/staff-/senior-) in codebase"
else
    fail "Old agent names found" "$OLD_AGENTS occurrences"
fi

# ============================================================
# Website builds
# ============================================================

if command -v npx >/dev/null 2>&1 && [ -d "$PROJECT_ROOT/website" ] && [ -d "$PROJECT_ROOT/website/node_modules/vitepress" ]; then
    BUILD_OUTPUT=$(cd "$PROJECT_ROOT/website" && npx vitepress build 2>&1)
    if echo "$BUILD_OUTPUT" | grep -q "build complete"; then
        pass "VitePress website builds successfully"
    else
        fail "VitePress website build failed"
    fi
else
    echo "  SKIP  VitePress build (npx or vitepress not available)"
fi

# ============================================================
# spec-artifacts output contracts
# ============================================================

SPECS_DIR="$PROJECT_ROOT/test/fixtures/specs"

# Test 1: SQL path - spec with Postgres keyword → data-model.mmd
assert_file_exists "$SPECS_DIR/spec-sql-postgres.md" "Test fixture exists: spec-sql-postgres.md"
assert_file_contains "$SPECS_DIR/spec-sql-postgres.md" "Postgres" "Fixture contains Postgres keyword"
assert_file_contains "$SPECS_DIR/spec-sql-postgres.md" "status: accepted" "Fixture has accepted status"

# Test 2: Document-mongo path - spec with MongoDB keyword → data-model.schema.yaml
assert_file_exists "$SPECS_DIR/spec-doc-mongodb.md" "Test fixture exists: spec-doc-mongodb.md"
assert_file_contains "$SPECS_DIR/spec-doc-mongodb.md" "MongoDB" "Fixture contains MongoDB keyword"

# Test 3: Document-dynamo path - spec with DynamoDB keyword → data-model.md with Access Patterns
assert_file_exists "$SPECS_DIR/spec-doc-dynamodb.md" "Test fixture exists: spec-doc-dynamodb.md"
assert_file_contains "$SPECS_DIR/spec-doc-dynamodb.md" "DynamoDB" "Fixture contains DynamoDB keyword"

# Test 4: Key-value path - spec with Redis keyword → data-model.md with key schema
assert_file_exists "$SPECS_DIR/spec-kv-redis.md" "Test fixture exists: spec-kv-redis.md"
assert_file_contains "$SPECS_DIR/spec-kv-redis.md" "Redis" "Fixture contains Redis keyword"

# Test 5: Mixed path - spec with Postgres and Redis → both data-model-sql.mmd and data-model-kv.md
assert_file_exists "$SPECS_DIR/spec-mixed-postgres-redis.md" "Test fixture exists: spec-mixed-postgres-redis.md"
assert_file_contains "$SPECS_DIR/spec-mixed-postgres-redis.md" "Postgres" "Mixed fixture contains Postgres"
assert_file_contains "$SPECS_DIR/spec-mixed-postgres-redis.md" "Redis" "Mixed fixture contains Redis"

# Test 6: Config fallback - spec with no keywords + config default_type: sql → data-model.mmd exists
assert_file_exists "$SPECS_DIR/spec-no-keywords.md" "Test fixture exists: spec-no-keywords.md"
assert_file_contains "$SPECS_DIR/spec-no-keywords.md" "Data Model" "No-keyword fixture has Data Model section"

# Test 7: Config auto + no keywords - spec with no keywords + config auto → warning/prompt in output
assert_file_exists "$SPECS_DIR/spec-auto-fallback.md" "Test fixture exists: spec-auto-fallback.md"
assert_file_contains "$SPECS_DIR/spec-auto-fallback.md" "status: accepted" "Auto-fallback fixture has accepted status"

# Test 8: Active constraints injected - spec with active invariant → "active constraints applied" in routing
assert_file_exists "$SPECS_DIR/spec-with-constraints.md" "Test fixture exists: spec-with-constraints.md"
assert_file_contains "$SPECS_DIR/spec-with-constraints.md" "Constrained Feature" "Constraint fixture has title"

# Test 9: Empty invariant body warning - spec with empty invariant → "body is empty" in output
assert_file_exists "$SPECS_DIR/spec-empty-constraint.md" "Test fixture exists: spec-empty-constraint.md"
assert_file_contains "$SPECS_DIR/spec-empty-constraint.md" "Empty Constraint" "Empty constraint fixture exists"

# Test 10: Superseded invariant excluded - spec with Superseded invariant → constraint count is 0
assert_file_exists "$SPECS_DIR/spec-superseded-invariant.md" "Test fixture exists: spec-superseded-invariant.md"
assert_file_contains "$SPECS_DIR/spec-superseded-invariant.md" "Superseded" "Superseded fixture exists"

# Test 11: Spec-frontmatter override - config sql + spec database_type: document-mongo → data-model.schema.yaml exists
assert_file_exists "$SPECS_DIR/spec-override-frontmatter.md" "Test fixture exists: spec-override-frontmatter.md"
assert_file_contains "$SPECS_DIR/spec-override-frontmatter.md" "database_type: document-mongo" "Override fixture has frontmatter override"

# Test 12: Design blueprint header - any spec with data model → artifact contains "Design blueprint" comment
assert_file_exists "$SPECS_DIR/spec-blueprint-check.md" "Test fixture exists: spec-blueprint-check.md"
assert_file_contains "$SPECS_DIR/spec-blueprint-check.md" "Data Model" "Blueprint fixture has Data Model section"

# Verify sdlc/artifacts command has design blueprint framing
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "Design blueprint" "sdlc/artifacts mentions design blueprint framing"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "design blueprints" "sdlc/artifacts uses design blueprint language"

# Verify sdlc/artifacts command has constraint injection logic
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "ACTIVE CONSTRAINTS" "sdlc/artifacts injects active constraints"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "Resolve Context" "sdlc/artifacts has resolve context step"

# Verify sdlc/artifacts command has database type resolution
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "database_type:" "sdlc/artifacts reads spec frontmatter database_type"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "artifacts.database.default_type" "sdlc/artifacts reads config default_type"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "Keyword scan" "sdlc/artifacts performs keyword scanning"

# Verify data model lookup tables are referenced
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "data-model.mmd" "sdlc/artifacts generates .mmd files"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "data-model.schema.yaml" "sdlc/artifacts generates schema.yaml files"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "erDiagram" "sdlc/artifacts uses Mermaid erDiagram format"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "\$schema" "sdlc/artifacts uses JSON Schema"

# Verify multi-database suffix naming
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "data-model-sql.mmd" "sdlc/artifacts uses sql suffix for mixed"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/artifacts.md" "data-model-kv.md" "sdlc/artifacts uses kv suffix for key-value"

# ============================================================
# Golden artifact validation
# ============================================================

# SQL golden — Mermaid ERD
assert_file_exists "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "Golden: SQL data-model.mmd exists"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "erDiagram" "Golden: SQL has erDiagram"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "Design blueprint" "Golden: SQL has blueprint header"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "edikt:artifact" "Golden: SQL has artifact marker"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "status=draft" "Golden: SQL has draft status"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "%% Index:" "Golden: SQL has index comments"

# Document-mongo golden — JSON Schema YAML
assert_file_exists "$SPECS_DIR/spec-doc-mongodb/data-model.schema.yaml" "Golden: Mongo data-model.schema.yaml exists"
assert_file_contains "$SPECS_DIR/spec-doc-mongodb/data-model.schema.yaml" "\$schema" "Golden: Mongo has \$schema"
assert_file_contains "$SPECS_DIR/spec-doc-mongodb/data-model.schema.yaml" "collection:" "Golden: Mongo has collection"
assert_file_contains "$SPECS_DIR/spec-doc-mongodb/data-model.schema.yaml" "indexes:" "Golden: Mongo has indexes"
assert_file_contains "$SPECS_DIR/spec-doc-mongodb/data-model.schema.yaml" "Design blueprint" "Golden: Mongo has blueprint header"

# Document-dynamo golden — Access patterns
assert_file_exists "$SPECS_DIR/spec-doc-dynamodb/data-model.md" "Golden: DynamoDB data-model.md exists"
assert_file_contains "$SPECS_DIR/spec-doc-dynamodb/data-model.md" "Access Patterns" "Golden: DynamoDB has Access Patterns"
assert_file_contains "$SPECS_DIR/spec-doc-dynamodb/data-model.md" "GSI" "Golden: DynamoDB has GSI design"
assert_file_contains "$SPECS_DIR/spec-doc-dynamodb/data-model.md" "Entity Prefixes" "Golden: DynamoDB has entity prefixes"
assert_file_contains "$SPECS_DIR/spec-doc-dynamodb/data-model.md" "Design blueprint" "Golden: DynamoDB has blueprint header"

# Key-value golden — Key schema
assert_file_exists "$SPECS_DIR/spec-kv-redis/data-model.md" "Golden: Redis data-model.md exists"
assert_file_contains "$SPECS_DIR/spec-kv-redis/data-model.md" "Key Schema" "Golden: Redis has Key Schema"
assert_file_contains "$SPECS_DIR/spec-kv-redis/data-model.md" "TTL" "Golden: Redis has TTL column"
assert_file_contains "$SPECS_DIR/spec-kv-redis/data-model.md" "Namespace" "Golden: Redis has Namespace"
assert_file_contains "$SPECS_DIR/spec-kv-redis/data-model.md" "Design blueprint" "Golden: Redis has blueprint header"

# Mixed golden — suffix naming with both artifacts
assert_file_exists "$SPECS_DIR/spec-mixed-postgres-redis/data-model-sql.mmd" "Golden: Mixed SQL data-model-sql.mmd exists"
assert_file_contains "$SPECS_DIR/spec-mixed-postgres-redis/data-model-sql.mmd" "erDiagram" "Golden: Mixed SQL has erDiagram"
assert_file_exists "$SPECS_DIR/spec-mixed-postgres-redis/data-model-kv.md" "Golden: Mixed KV data-model-kv.md exists"
assert_file_contains "$SPECS_DIR/spec-mixed-postgres-redis/data-model-kv.md" "Key Schema" "Golden: Mixed KV has Key Schema"
assert_file_contains "$SPECS_DIR/spec-mixed-postgres-redis/data-model-kv.md" "## Notes" "Golden: Mixed KV has Notes section"

# API contract golden — OpenAPI 3.0
assert_file_exists "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "Golden: API contract exists"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "openapi:" "Golden: API has openapi field"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "Design blueprint" "Golden: API has blueprint header"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "edikt:artifact" "Golden: API has artifact marker"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "paths:" "Golden: API has paths section"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "components:" "Golden: API has components"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "securitySchemes:" "Golden: API has security schemes"

# Event contract golden — AsyncAPI 2.6
assert_file_exists "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "Golden: Event contract exists"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "asyncapi:" "Golden: Events has asyncapi field"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "Design blueprint" "Golden: Events has blueprint header"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "channels:" "Golden: Events has channels"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "x-producer:" "Golden: Events has producer"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "x-idempotency:" "Golden: Events has idempotency"

# Migration golden — SQL with UP/DOWN/BACKFILL/RISK
assert_file_exists "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "Golden: Migration exists"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "Design blueprint" "Golden: Migration has blueprint header"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "edikt:artifact" "Golden: Migration has artifact marker"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "=== UP ===" "Golden: Migration has UP section"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "=== DOWN ===" "Golden: Migration has DOWN section"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "=== BACKFILL ===" "Golden: Migration has BACKFILL section"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "=== RISK ===" "Golden: Migration has RISK section"

# Fixtures golden — YAML seed data
assert_file_exists "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "Golden: Fixtures exist"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "Design blueprint" "Golden: Fixtures has blueprint header"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "edikt:artifact" "Golden: Fixtures has artifact marker"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "scenarios:" "Golden: Fixtures has scenarios"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "_note:" "Golden: Fixtures has _note fields"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "relationships:" "Golden: Fixtures has relationships"

# Test strategy golden — markdown with frontmatter
assert_file_exists "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "Golden: Test strategy exists"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "artifact_type: test-strategy" "Golden: Test strategy has artifact_type"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "reviewed_by: qa" "Golden: Test strategy reviewed by qa"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "## Unit Tests" "Golden: Test strategy has Unit Tests"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "## Integration Tests" "Golden: Test strategy has Integration Tests"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "## Edge Cases" "Golden: Test strategy has Edge Cases"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "## Coverage Target" "Golden: Test strategy has Coverage Target"

# ============================================================
# Directive sentinels in dogfood governance docs
# ============================================================

DECISIONS_DIR="$PROJECT_ROOT/docs/architecture/decisions"
INVARIANTS_DIR="$PROJECT_ROOT/docs/architecture/invariants"

# All accepted ADRs must have directive sentinels
for adr in "$DECISIONS_DIR"/*.md; do
    name=$(basename "$adr")
    # Skip superseded ADRs
    if grep -q "Superseded" "$adr" 2>/dev/null; then
        continue
    fi
    if grep -q "Accepted\|accepted" "$adr" 2>/dev/null; then
        assert_file_contains "$adr" "edikt:directives:start" "Directive sentinel in $name"
    fi
done

# All active invariants must have directive sentinels
for inv in "$INVARIANTS_DIR"/*.md; do
    name=$(basename "$inv")
    assert_file_contains "$inv" "edikt:directives:start" "Directive sentinel in $name"
done

# Compile test fixtures have sentinels
assert_file_contains "$PROJECT_ROOT/test/fixtures/compile/decisions/ADR-001-test.md" "edikt:directives:start" "Compile fixture ADR-001 has sentinel"
assert_file_contains "$PROJECT_ROOT/test/fixtures/compile/invariants/INV-001-test.md" "edikt:directives:start" "Compile fixture INV-001 has sentinel"

# Expected governance output has new format
assert_file_contains "$PROJECT_ROOT/test/fixtures/compile/expected-governance.md" "Routing Table" "Compile golden has routing table"
assert_file_not_contains "$PROJECT_ROOT/test/fixtures/compile/expected-governance.md" "Architecture Decisions" "Compile golden no longer has flat Architecture section"

# gov/review checks directive language quality and stale sentinels
assert_file_contains "$PROJECT_ROOT/commands/gov/review.md" "DIRECTIVE SENTINELS" "gov/review has sentinel summary"
assert_file_contains "$PROJECT_ROOT/commands/gov/review.md" "stale" "gov/review detects stale sentinels"
assert_file_contains "$PROJECT_ROOT/commands/gov/review.md" "language quality\|language" "gov/review reviews language quality"

# gov/compile validates cross-references
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "Validate Cross-References" "gov/compile has cross-reference validation step"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "strip the fabricated reference" "gov/compile strips fabricated refs"

# ============================================================
# Compile golden validation
# ============================================================

COMPILE_DIR="$PROJECT_ROOT/test/fixtures/compile"

# Input fixtures exist
assert_file_exists "$COMPILE_DIR/decisions/ADR-001-test.md" "Compile fixture: ADR-001 exists"
assert_file_contains "$COMPILE_DIR/decisions/ADR-001-test.md" "Accepted" "Compile fixture: ADR-001 is Accepted"
assert_file_exists "$COMPILE_DIR/decisions/ADR-002-test.md" "Compile fixture: ADR-002 exists"
assert_file_contains "$COMPILE_DIR/decisions/ADR-002-test.md" "Superseded" "Compile fixture: ADR-002 is Superseded"
assert_file_exists "$COMPILE_DIR/invariants/INV-001-test.md" "Compile fixture: INV-001 exists"
assert_file_contains "$COMPILE_DIR/invariants/INV-001-test.md" "Active" "Compile fixture: INV-001 is Active"

# Expected output structure — new index format
assert_file_exists "$COMPILE_DIR/expected-governance.md" "Compile golden: expected-governance.md exists"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "edikt:compiled" "Compile golden: has compiled marker"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "Non-Negotiable Constraints" "Compile golden: has constraints section"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "Routing Table" "Compile golden: has routing table"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "Reminder:" "Compile golden: has reminder section (recency)"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "ref: INV-001" "Compile golden: INV-001 referenced in index"

# Superseded ADR must NOT appear in compiled output
if grep -q "kebab-case" "$COMPILE_DIR/expected-governance.md" 2>/dev/null; then
    fail "Compile golden: ADR-002 (superseded) excluded from output"
else
    pass "Compile golden: ADR-002 (superseded) excluded from output"
fi

test_summary

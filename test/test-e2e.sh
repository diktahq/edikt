#!/bin/bash
# E2E tests — simulate install, init, compile, and upgrade flows in /tmp
# Verifies file structure, content correctness, and UX consistency
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

E2E_DIR="/tmp/edikt-e2e-$$"
trap 'rm -rf "$E2E_DIR"' EXIT

echo ""
echo "E2E test dir: $E2E_DIR"

# ============================================================
# TEST 1: Install simulation — verify all files exist
# ============================================================

echo ""
echo -e "${BOLD}TEST 1: Install file completeness${NC}"

INSTALL_HOME="$E2E_DIR/install-test"
mkdir -p "$INSTALL_HOME/commands/edikt"
mkdir -p "$INSTALL_HOME/templates/rules/base"
mkdir -p "$INSTALL_HOME/templates/rules/lang"
mkdir -p "$INSTALL_HOME/templates/rules/framework"
mkdir -p "$INSTALL_HOME/templates/agents"
mkdir -p "$INSTALL_HOME/templates/hooks"
mkdir -p "$INSTALL_HOME/hooks"

# Copy commands — flat and namespaced
for cmd in "$PROJECT_ROOT"/commands/*.md; do
    cp "$cmd" "$INSTALL_HOME/commands/edikt/"
done
for ns in adr invariant guideline gov sdlc docs deprecated; do
    mkdir -p "$INSTALL_HOME/commands/edikt/${ns}"
    for cmd in "$PROJECT_ROOT"/commands/${ns}/*.md; do
        [ -f "$cmd" ] && cp "$cmd" "$INSTALL_HOME/commands/edikt/${ns}/"
    done
done

# Copy agent templates
for agent in "$PROJECT_ROOT"/templates/agents/*.md; do
    cp "$agent" "$INSTALL_HOME/templates/agents/"
done
cp "$PROJECT_ROOT/templates/agents/_registry.yaml" "$INSTALL_HOME/templates/agents/"

# Copy hooks
for hook in "$PROJECT_ROOT"/templates/hooks/*.sh; do
    cp "$hook" "$INSTALL_HOME/templates/hooks/"
    cp "$hook" "$INSTALL_HOME/hooks/"
done

# Copy rule templates
for rule in "$PROJECT_ROOT"/templates/rules/base/*.md; do
    cp "$rule" "$INSTALL_HOME/templates/rules/base/"
done
for rule in "$PROJECT_ROOT"/templates/rules/lang/*.md; do
    cp "$rule" "$INSTALL_HOME/templates/rules/lang/"
done
for rule in "$PROJECT_ROOT"/templates/rules/framework/*.md; do
    cp "$rule" "$INSTALL_HOME/templates/rules/framework/"
done
cp "$PROJECT_ROOT/templates/rules/_registry.yaml" "$INSTALL_HOME/templates/rules/"

# Copy other templates
for tmpl in "$PROJECT_ROOT"/templates/*.tmpl; do
    cp "$tmpl" "$INSTALL_HOME/templates/"
done

cp "$PROJECT_ROOT/VERSION" "$INSTALL_HOME/VERSION"

# Verify command count (50 commands: 12 flat + 3 adr + 3 invariant + 3 guideline + 5 gov + 7 sdlc + 2 docs + 15 deprecated)
# v0.3.0 added commands/guideline/compile.md + commands/gov/score.md
# v0.4.0 added commands/config.md
CMD_COUNT=$(find "$INSTALL_HOME/commands/edikt/" -name "*.md" 2>/dev/null | wc -l | tr -d ' ')
if [ "$CMD_COUNT" -eq 50 ]; then
    pass "50 commands installed"
else
    fail "Expected 50 commands, found $CMD_COUNT"
fi

# Verify agent count (20 agents: 19 original + evaluator-headless)
# v0.4.0 added evaluator-headless.md (headless system prompt — no frontmatter)
AGENT_COUNT=$(ls "$INSTALL_HOME/templates/agents/"*.md 2>/dev/null | wc -l | tr -d ' ')
if [ "$AGENT_COUNT" -eq 20 ]; then
    pass "20 agent templates installed"
else
    fail "Expected 20 agents, found $AGENT_COUNT"
fi

# Verify hook count (14 hooks including headless-ask)
# 14 event hooks + 1 utility (event-log.sh) = 15
HOOK_COUNT=$(ls "$INSTALL_HOME/templates/hooks/"*.sh 2>/dev/null | wc -l | tr -d ' ')
if [ "$HOOK_COUNT" -eq 15 ]; then
    pass "15 hook scripts installed (14 hooks + event-log utility)"
else
    fail "Expected 15 hook files, found $HOOK_COUNT"
fi

# Verify all hooks listed in install.sh exist as files
for hook in session-start pre-tool-use post-tool-use pre-compact stop-hook user-prompt-submit post-compact subagent-stop instructions-loaded stop-failure task-created cwd-changed file-changed headless-ask; do
    assert_file_exists "$INSTALL_HOME/templates/hooks/${hook}.sh" "Hook script exists: ${hook}.sh"
done

# Verify evaluator agent exists
assert_file_exists "$INSTALL_HOME/templates/agents/evaluator.md" "Evaluator agent installed"

# ============================================================
# TEST 2: Init output structure — simulate what init creates
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Init output structure${NC}"

INIT_PROJECT="$E2E_DIR/init-test"
mkdir -p "$INIT_PROJECT/.edikt"
mkdir -p "$INIT_PROJECT/.claude/rules"
mkdir -p "$INIT_PROJECT/.claude/agents"
mkdir -p "$INIT_PROJECT/docs/architecture/decisions"
mkdir -p "$INIT_PROJECT/docs/architecture/invariants"
mkdir -p "$INIT_PROJECT/docs/plans"
mkdir -p "$INIT_PROJECT/docs/product/prds"
mkdir -p "$INIT_PROJECT/docs/product/specs"
mkdir -p "$INIT_PROJECT/docs/reports"

# Create config
cat > "$INIT_PROJECT/.edikt/config.yaml" << 'YAML'
edikt_version: "0.2.0"
base: docs

stack: [go, chi]

paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  plans: docs/plans
  specs: docs/product/specs
  prds: docs/product/prds
  guidelines: docs/guidelines
  reports: docs/reports
  project-context: docs/project-context.md

rules:
  code-quality: { include: all }
  testing: { include: all }
  security: { include: all }
  error-handling: { include: all }
  go: { include: all }
  chi: { include: all }

features:
  auto-format: true
  session-summary: true
  signal-detection: true
  plan-injection: true
  quality-gates: true
YAML

# Install rule packs
for rule in code-quality testing security error-handling; do
    cp "$PROJECT_ROOT/templates/rules/base/${rule}.md" "$INIT_PROJECT/.claude/rules/"
done
cp "$PROJECT_ROOT/templates/rules/lang/go.md" "$INIT_PROJECT/.claude/rules/"
cp "$PROJECT_ROOT/templates/rules/framework/chi.md" "$INIT_PROJECT/.claude/rules/"

# Install agents
for agent in architect backend dba docs qa; do
    cp "$PROJECT_ROOT/templates/agents/${agent}.md" "$INIT_PROJECT/.claude/agents/"
done
cp "$PROJECT_ROOT/templates/agents/evaluator.md" "$INIT_PROJECT/.claude/agents/"

# Install settings
cp "$PROJECT_ROOT/templates/settings.json.tmpl" "$INIT_PROJECT/.claude/settings.json"

# Create CLAUDE.md with sentinels
cat > "$INIT_PROJECT/CLAUDE.md" << 'MD'
# Project

Custom content above the sentinel.

[edikt:start]: # managed by edikt — do not edit this block manually
## edikt

### Project
Test project for E2E validation.
[edikt:end]: #
MD

# Create project context
cat > "$INIT_PROJECT/docs/project-context.md" << 'MD'
# Project Context

A Go REST API using Chi router with PostgreSQL.
MD

# Verify structure
assert_file_exists "$INIT_PROJECT/.edikt/config.yaml" "Config exists"
assert_file_exists "$INIT_PROJECT/CLAUDE.md" "CLAUDE.md exists"
assert_file_exists "$INIT_PROJECT/docs/project-context.md" "Project context exists"
assert_dir_exists "$INIT_PROJECT/.claude/rules" "Rules directory exists"
assert_dir_exists "$INIT_PROJECT/.claude/agents" "Agents directory exists"
assert_dir_exists "$INIT_PROJECT/docs/architecture/decisions" "Decisions directory exists"
assert_dir_exists "$INIT_PROJECT/docs/architecture/invariants" "Invariants directory exists"
assert_dir_exists "$INIT_PROJECT/docs/plans" "Plans directory exists"

# Verify rule packs have required markers
for rule in "$INIT_PROJECT/.claude/rules/"*.md; do
    name=$(basename "$rule")
    assert_file_contains "$rule" "edikt:generated" "Rule pack has marker: $name"
    assert_file_contains "$rule" "paths:" "Rule pack has paths: $name"
done

# Verify agents have governance frontmatter
for agent in "$INIT_PROJECT/.claude/agents/"*.md; do
    name=$(basename "$agent")
    assert_file_contains "$agent" "maxTurns:" "Agent has maxTurns: $name"
    assert_file_contains "$agent" "effort:" "Agent has effort: $name"
done

# Verify CLAUDE.md sentinels
assert_file_contains "$INIT_PROJECT/CLAUDE.md" "edikt:start" "CLAUDE.md has start sentinel"
assert_file_contains "$INIT_PROJECT/CLAUDE.md" "edikt:end" "CLAUDE.md has end sentinel"

# Verify settings.json has all hook events
for event in SessionStart UserPromptSubmit PreToolUse PostToolUse Stop StopFailure SubagentStop PreCompact PostCompact InstructionsLoaded TaskCreated CwdChanged FileChanged; do
    if python3 -c "
import json, sys
s = json.load(open('$INIT_PROJECT/.claude/settings.json'))
if '$event' not in s.get('hooks', {}):
    sys.exit(1)
" 2>/dev/null; then
        pass "Settings has $event hook"
    else
        fail "Settings missing $event hook"
    fi
done

# Verify PostToolUse has if field
if python3 -c "
import json, sys
s = json.load(open('$INIT_PROJECT/.claude/settings.json'))
pt = s.get('hooks', {}).get('PostToolUse', [{}])[0]
if 'if' not in pt:
    sys.exit(1)
" 2>/dev/null; then
    pass "PostToolUse has conditional if field"
else
    fail "PostToolUse missing conditional if field"
fi

# ============================================================
# TEST 3: Compile migration — v0.1.x flat → v0.2.0 topic-grouped
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Compile format validation${NC}"

COMPILE_PROJECT="$E2E_DIR/compile-test"
mkdir -p "$COMPILE_PROJECT/.edikt"
mkdir -p "$COMPILE_PROJECT/.claude/rules"
mkdir -p "$COMPILE_PROJECT/docs/architecture/decisions"
mkdir -p "$COMPILE_PROJECT/docs/architecture/invariants"

cat > "$COMPILE_PROJECT/.edikt/config.yaml" << 'YAML'
edikt_version: "0.2.0"
base: docs
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
YAML

# Create an ADR WITH sentinel
cat > "$COMPILE_PROJECT/docs/architecture/decisions/ADR-001-test.md" << 'MD'
# ADR-001 — Use snake_case for filenames

**Status:** Accepted

## Decision

All filenames must use snake_case.

## Directives

[edikt:directives:start]: #
paths:
  - "**/*"
scope:
  - implementation
directives:
  - All filenames MUST use snake_case — no exceptions for language conventions. (ref: ADR-001)
[edikt:directives:end]: #
MD

# Create an ADR WITHOUT sentinel (tests fallback)
cat > "$COMPILE_PROJECT/docs/architecture/decisions/ADR-002-test.md" << 'MD'
# ADR-002 — Use Go for backend services

**Status:** Accepted

## Decision

All backend services must be written in Go. No Python, no Node.js for server-side code.

## Consequences

- Team needs Go expertise
- Consistent toolchain
MD

# Create a superseded ADR (should be skipped)
cat > "$COMPILE_PROJECT/docs/architecture/decisions/ADR-003-test.md" << 'MD'
# ADR-003 — Use kebab-case for filenames

**Status:** Superseded by ADR-001

## Decision

Use kebab-case everywhere.
MD

# Create an invariant WITH sentinel
cat > "$COMPILE_PROJECT/docs/architecture/invariants/INV-001-test.md" << 'MD'
# INV-001 — No vendor dependencies

**Status:** Active

## Rule

Zero runtime vendor dependencies.

## Directives

[edikt:directives:start]: #
paths:
  - "**/*"
scope:
  - planning
  - implementation
directives:
  - The project MUST have zero runtime vendor dependencies. No npm, pip, cargo, or go modules. (ref: INV-001)
[edikt:directives:end]: #
MD

# Verify sentinel blocks are parseable
SENTINEL_COUNT=$(grep -rl "edikt:directives:start" "$COMPILE_PROJECT/docs/" 2>/dev/null | wc -l | tr -d ' ')
if [ "$SENTINEL_COUNT" -eq 2 ]; then
    pass "2 documents have directive sentinels"
else
    fail "Expected 2 sentinel documents, found $SENTINEL_COUNT"
fi

NO_SENTINEL_COUNT=$(grep -rL "edikt:directives:start" "$COMPILE_PROJECT/docs/architecture/decisions/"*.md 2>/dev/null | grep -v "Superseded" | wc -l | tr -d ' ')
# ADR-002 has no sentinel, ADR-003 is superseded
# grep -L finds files WITHOUT the pattern, then we exclude superseded
if [ "$NO_SENTINEL_COUNT" -ge 1 ]; then
    pass "At least 1 document needs fallback extraction"
else
    fail "Expected at least 1 document without sentinel"
fi

# Verify sentinel block structure is valid YAML-like
for doc in "$COMPILE_PROJECT"/docs/architecture/decisions/ADR-001-test.md "$COMPILE_PROJECT"/docs/architecture/invariants/INV-001-test.md; do
    name=$(basename "$doc")
    # Extract between sentinels and check for required fields
    sentinel_content=$(sed -n '/edikt:directives:start/,/edikt:directives:end/p' "$doc")
    if echo "$sentinel_content" | grep -q "directives:"; then
        pass "Sentinel has directives field: $name"
    else
        fail "Sentinel missing directives field: $name"
    fi
    if echo "$sentinel_content" | grep -q "paths:"; then
        pass "Sentinel has paths field: $name"
    else
        fail "Sentinel missing paths field: $name"
    fi
    if echo "$sentinel_content" | grep -q "scope:"; then
        pass "Sentinel has scope field: $name"
    else
        fail "Sentinel missing scope field: $name"
    fi
    if echo "$sentinel_content" | grep -q "ref:"; then
        pass "Sentinel has source reference: $name"
    else
        fail "Sentinel missing source reference: $name"
    fi
done

# Verify superseded ADR would be skipped
if grep -q "Superseded" "$COMPILE_PROJECT/docs/architecture/decisions/ADR-003-test.md"; then
    pass "Superseded ADR detectable"
else
    fail "Cannot detect superseded status"
fi

# ============================================================
# TEST 3b: Cross-reference validation
# ============================================================

echo ""
echo -e "${BOLD}TEST 3b: Cross-reference validation guards${NC}"

# Create an ADR with a fabricated reference to a non-existent invariant
cat > "$COMPILE_PROJECT/docs/architecture/decisions/ADR-004-test-fabricated.md" << 'MD'
# ADR-004 — Test fabricated reference

**Status:** Accepted

## Decision

Use structured logging everywhere.

## Directives

[edikt:directives:start]: #
paths:
  - "**/*.go"
scope:
  - implementation
directives:
  - Use structured logging with slog in all Go code (ref: ADR-004)
  - NEVER log PII fields (ref: INV-999)
[edikt:directives:end]: #
MD

# INV-999 does NOT exist — this is a fabricated reference
assert_file_not_exists "$COMPILE_PROJECT/docs/architecture/invariants/INV-999-test.md" "INV-999 does not exist (fabricated ref)"

# The sentinel references INV-999 which doesn't exist
if grep -q "INV-999" "$COMPILE_PROJECT/docs/architecture/decisions/ADR-004-test-fabricated.md"; then
    pass "Fabricated reference INV-999 present in test fixture"
else
    fail "Test fixture missing fabricated reference"
fi

# gov/compile command has validation step that would catch this
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "Validate Cross-References" "gov/compile has cross-ref validation step"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "verify the reference exists in the source document" "gov/compile verifies refs exist"
assert_file_contains "$PROJECT_ROOT/commands/gov/compile.md" "strip the fabricated reference" "gov/compile strips fabricated refs"

# gov/review checks directive language quality
assert_file_contains "$PROJECT_ROOT/commands/gov/review.md" "DIRECTIVE SENTINELS" "gov/review has sentinel summary"
assert_file_contains "$PROJECT_ROOT/commands/gov/review.md" "stale" "gov/review detects stale sentinels"

# Verify edikt's own dogfood ADRs only reference existing invariants/ADRs
FABRICATED_REFS=0
for adr in "$PROJECT_ROOT"/docs/architecture/decisions/*.md; do
    name=$(basename "$adr")
    # Skip superseded
    if grep -q "Superseded" "$adr" 2>/dev/null; then continue; fi

    # Extract sentinel block refs
    sentinel=$(sed -n '/edikt:directives:start/,/edikt:directives:end/p' "$adr" 2>/dev/null)
    if [ -z "$sentinel" ]; then continue; fi

    # Check each INV-NNN reference
    for inv_ref in $(echo "$sentinel" | grep -oE "INV-[0-9]+" | sort -u); do
        inv_num=$(echo "$inv_ref" | grep -oE "[0-9]+")
        if ! ls "$PROJECT_ROOT"/docs/architecture/invariants/INV-"${inv_num}"* 2>/dev/null | grep -q .; then
            fail "Fabricated invariant ref $inv_ref in $name"
            ((FABRICATED_REFS++))
        fi
    done

    # Check each ADR-NNN reference (should match a real ADR)
    for adr_ref in $(echo "$sentinel" | grep -oE "ADR-[0-9]+" | sort -u); do
        adr_num=$(echo "$adr_ref" | grep -oE "[0-9]+")
        if ! ls "$PROJECT_ROOT"/docs/architecture/decisions/ADR-"${adr_num}"* 2>/dev/null | grep -q .; then
            fail "Fabricated ADR ref $adr_ref in $name"
            ((FABRICATED_REFS++))
        fi
    done
done

if [ "$FABRICATED_REFS" -eq 0 ]; then
    pass "No fabricated cross-references in dogfood ADRs"
fi

# Same check for invariants
for inv in "$PROJECT_ROOT"/docs/architecture/invariants/*.md; do
    name=$(basename "$inv")
    sentinel=$(sed -n '/edikt:directives:start/,/edikt:directives:end/p' "$inv" 2>/dev/null)
    if [ -z "$sentinel" ]; then continue; fi

    for inv_ref in $(echo "$sentinel" | grep -oE "INV-[0-9]+" | sort -u); do
        inv_num=$(echo "$inv_ref" | grep -oE "[0-9]+")
        if ! ls "$PROJECT_ROOT"/docs/architecture/invariants/INV-"${inv_num}"* 2>/dev/null | grep -q .; then
            fail "Fabricated invariant ref $inv_ref in $name"
            ((FABRICATED_REFS++))
        fi
    done
done

if [ "$FABRICATED_REFS" -eq 0 ]; then
    pass "No fabricated cross-references in dogfood invariants"
fi

# Clean up the test fixture
rm -f "$COMPILE_PROJECT/docs/architecture/decisions/ADR-004-test-fabricated.md"

# ============================================================
# TEST 4: Command UX consistency
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: Command UX consistency${NC}"

CMD_DIR="$PROJECT_ROOT/commands"

# Build list of all non-deprecated command files
ALL_CMDS=()
for cmd in "$CMD_DIR"/*.md; do
    [ -f "$cmd" ] && ALL_CMDS+=("$cmd")
done
for ns in adr invariant guideline gov sdlc docs; do
    for cmd in "$CMD_DIR/${ns}"/*.md; do
        [ -f "$cmd" ] && ALL_CMDS+=("$cmd")
    done
done

# Every command has a completion signal (✅ or clear ending)
MISSING_COMPLETION=0
for cmd in "${ALL_CMDS[@]}"; do
    name=$(echo "$cmd" | sed "s|$CMD_DIR/||" | sed 's|\.md$||')
    if ! grep -qE '✅|All clear|EDIKT STATUS|AUDIT REPORT|DRIFT REPORT|SESSION SUMMARY|IMPLEMENTATION REVIEW|Doc gaps|DIRECTIVE SENTINELS|GOVERNANCE REVIEW' "$cmd" 2>/dev/null; then
        fail "Missing completion signal: $name"
        ((MISSING_COMPLETION++))
    fi
done
if [ "$MISSING_COMPLETION" -eq 0 ]; then
    pass "All commands have completion signals"
fi

# Every command that creates files has a Next: line
MISSING_NEXT=0
for cmd in "${ALL_CMDS[@]}"; do
    name=$(echo "$cmd" | sed "s|$CMD_DIR/||" | sed 's|\.md$||')
    if ! grep -q "Next:" "$cmd" 2>/dev/null; then
        fail "Missing Next: line: $name"
        ((MISSING_NEXT++))
    fi
done
if [ "$MISSING_NEXT" -eq 0 ]; then
    pass "All commands have Next: line"
fi

# Config guard consistency — all commands that read config.yaml guard for it
MISSING_GUARD=0
for cmd in "${ALL_CMDS[@]}"; do
    name=$(echo "$cmd" | sed "s|$CMD_DIR/||" | sed 's|\.md$||')
    # Skip init (creates config), and commands that don't read config
    if [[ "$name" == "init" ]]; then continue; fi

    # Check if command references config.yaml in instructions
    if grep -q "config.yaml" "$cmd" 2>/dev/null; then
        # Should have a config guard
        if ! grep -q "No edikt config found" "$cmd" 2>/dev/null; then
            # Check if it delegates to context (which has its own guard)
            if ! grep -q "edikt:context.*logic" "$cmd" 2>/dev/null; then
                fail "Reads config but no guard: $name"
                ((MISSING_GUARD++))
            fi
        fi
    fi
done
if [ "$MISSING_GUARD" -eq 0 ]; then
    pass "All config-reading commands have guards"
fi

# Emoji consistency — no mixing of old patterns
OLD_EMOJI_COUNT=0
for cmd in "${ALL_CMDS[@]}"; do
    name=$(echo "$cmd" | sed "s|$CMD_DIR/||" | sed 's|\.md$||')
    # Check for old 🪝 pattern (should be 🔀 now)
    if grep -q "🪝" "$cmd" 2>/dev/null; then
        fail "Old routing emoji 🪝 in: $name (should be 🔀)"
        ((OLD_EMOJI_COUNT++))
    fi
done
if [ "$OLD_EMOJI_COUNT" -eq 0 ]; then
    pass "No old routing emoji (🪝) — all use 🔀"
fi

# Plan mode guard on all interactive commands (namespaced paths)
for cmd_path in init sdlc/prd sdlc/spec sdlc/artifacts adr/new invariant/new docs/intake sdlc/plan brainstorm; do
    assert_file_contains "$CMD_DIR/${cmd_path}.md" "plan mode" "Plan mode guard: ${cmd_path}"
done

# --json flag on CI commands (namespaced paths)
for cmd_path in gov/compile sdlc/drift sdlc/audit doctor sdlc/review gov/review; do
    assert_file_contains "$CMD_DIR/${cmd_path}.md" "\-\-json" "--json flag: ${cmd_path}"
done

# Progress breadcrumbs on high-effort commands (namespaced paths)
for cmd_path in gov/compile sdlc/audit sdlc/review sdlc/drift gov/review; do
    if grep -qE "Step [0-9]+/[0-9]+:" "$CMD_DIR/${cmd_path}.md" 2>/dev/null; then
        pass "Progress breadcrumbs: ${cmd_path}"
    else
        fail "Missing progress breadcrumbs: ${cmd_path}"
    fi
done

# ============================================================
# TEST 5: Agent governance completeness
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: Agent governance completeness${NC}"

AGENTS_DIR="$PROJECT_ROOT/templates/agents"

# Advisory agents must have disallowedTools with Write
ADVISORY_AGENTS=(architect dba security api sre docs ux compliance seo gtm data performance evaluator)
for agent in "${ADVISORY_AGENTS[@]}"; do
    file="$AGENTS_DIR/${agent}.md"
    # Check disallowedTools block contains Write
    disallowed=$(awk '/^disallowedTools:/{found=1; next} found && /^[a-zA-Z]/{exit} found && /^---/{exit} found{print}' "$file" 2>/dev/null)
    if echo "$disallowed" | grep -q "Write"; then
        pass "$agent: Write disallowed"
    else
        fail "$agent: Write not in disallowedTools"
    fi
    if echo "$disallowed" | grep -q "Edit"; then
        pass "$agent: Edit disallowed"
    else
        fail "$agent: Edit not in disallowedTools"
    fi
done

# Code-writing agents must NOT have disallowedTools
CODE_AGENTS=(backend frontend qa mobile platform pm)
for agent in "${CODE_AGENTS[@]}"; do
    assert_file_not_contains "$AGENTS_DIR/${agent}.md" "disallowedTools:" "$agent: no disallowedTools (code-writing)"
done

# initialPrompt only on architect, security, pm
for agent in architect security pm; do
    assert_file_contains "$AGENTS_DIR/${agent}.md" "initialPrompt:" "$agent has initialPrompt"
done

# Evaluator specifics
assert_file_contains "$AGENTS_DIR/evaluator.md" "maxTurns: 15" "Evaluator maxTurns: 15"
assert_file_contains "$AGENTS_DIR/evaluator.md" "PASS" "Evaluator has PASS output"
assert_file_contains "$AGENTS_DIR/evaluator.md" "FAIL" "Evaluator has FAIL output"
assert_file_contains "$AGENTS_DIR/evaluator.md" "skeptical" "Evaluator is skeptical"
assert_file_contains "$AGENTS_DIR/evaluator.md" "evaluator-tuning" "Evaluator reads tuning doc"
assert_file_contains "$AGENTS_DIR/evaluator.md" "acceptance criteria" "Evaluator checks acceptance criteria"

# Evaluator in registry
assert_file_contains "$AGENTS_DIR/_registry.yaml" "evaluator" "Evaluator in always-install registry"

# ============================================================
# TEST 6: Trigger matching coverage
# ============================================================

echo ""
echo -e "${BOLD}TEST 6: Trigger matching${NC}"

TEMPLATE="$PROJECT_ROOT/templates/CLAUDE.md.tmpl"

# Plan triggers include fix-context phrases
assert_file_contains "$TEMPLATE" "plan to fix these issues" "Plan trigger: fix issues"
assert_file_contains "$TEMPLATE" "plan these changes" "Plan trigger: plan changes"
assert_file_contains "$TEMPLATE" "plan this work" "Plan trigger: plan work"

# All commands have triggers (flat and namespaced)
for cmd in status context doctor init upgrade brainstorm session team agents mcp capture; do
    assert_file_contains "$TEMPLATE" "edikt:${cmd}" "Trigger exists: ${cmd}"
done
for cmd in adr:new adr:compile adr:review invariant:new invariant:compile invariant:review guideline:new guideline:review gov:compile gov:review gov:rules-update gov:sync sdlc:prd sdlc:spec sdlc:artifacts sdlc:plan sdlc:review sdlc:drift sdlc:audit docs:review docs:intake; do
    assert_file_contains "$TEMPLATE" "edikt:${cmd}" "Trigger exists: ${cmd}"
done

# Output conventions table exists
assert_file_contains "$TEMPLATE" "Output Conventions" "CLAUDE.md has output conventions"
assert_file_contains "$TEMPLATE" "🔀" "Output conventions includes routing emoji"

# ============================================================
# TEST 7: Installer safety features
# ============================================================

echo ""
echo -e "${BOLD}TEST 7: Installer safety${NC}"

INSTALL="$PROJECT_ROOT/install.sh"

assert_file_contains "$INSTALL" "DRY_RUN" "Installer has dry-run support"
assert_file_contains "$INSTALL" "install_file" "Installer has backup function"
assert_file_contains "$INSTALL" "BACKUP_DIR" "Installer creates backup dir"
assert_file_contains "$INSTALL" "edikt:custom" "Installer respects custom markers"
assert_file_contains "$INSTALL" "headless-ask" "Installer includes headless hook"
assert_file_contains "$INSTALL" "evaluator" "Installer includes evaluator agent"
assert_file_contains "$INSTALL" "stop-failure" "Installer includes StopFailure hook"
assert_file_contains "$INSTALL" "task-created" "Installer includes TaskCreated hook"
assert_file_contains "$INSTALL" "cwd-changed" "Installer includes CwdChanged hook"
assert_file_contains "$INSTALL" "file-changed" "Installer includes FileChanged hook"

# Verify install.sh is valid bash
if bash -n "$INSTALL" 2>/dev/null; then
    pass "install.sh passes bash -n syntax check"
else
    fail "install.sh fails bash -n syntax check"
fi

# ============================================================
# TEST 8: Harness documents exist and are valid
# ============================================================

echo ""
echo -e "${BOLD}TEST 8: Harness documents${NC}"

assert_file_exists "$PROJECT_ROOT/docs/architecture/assumptions.md" "Assumptions doc exists"
assert_file_contains "$PROJECT_ROOT/docs/architecture/assumptions.md" "A-001" "Has assumption A-001"
assert_file_contains "$PROJECT_ROOT/docs/architecture/assumptions.md" "A-006" "Has assumption A-006"
assert_file_contains "$PROJECT_ROOT/docs/architecture/assumptions.md" "Retired" "Has retired section"
assert_file_contains "$PROJECT_ROOT/docs/architecture/assumptions.md" "Last tested" "Has testing log columns"

assert_file_exists "$PROJECT_ROOT/docs/architecture/evaluator-tuning.md" "Evaluator tuning doc exists"
assert_file_contains "$PROJECT_ROOT/docs/architecture/evaluator-tuning.md" "False Positives" "Has false positives section"
assert_file_contains "$PROJECT_ROOT/docs/architecture/evaluator-tuning.md" "False Negatives" "Has false negatives section"
assert_file_contains "$PROJECT_ROOT/docs/architecture/evaluator-tuning.md" "Prompt Refinements" "Has prompt refinements log"

# ============================================================
# TEST 9: Version consistency
# ============================================================

echo ""
echo -e "${BOLD}TEST 9: Version consistency${NC}"

FILE_VER=$(cat "$PROJECT_ROOT/VERSION" | tr -d '[:space:]')
CONFIG_VER=$(grep 'edikt_version:' "$PROJECT_ROOT/.edikt/config.yaml" | awk '{print $2}' | tr -d '"')

# VERSION must be a valid semver (x.y.z) or a -dev suffix on main
if echo "$FILE_VER" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-dev)?$'; then
    pass "VERSION is valid semver: $FILE_VER"
else
    fail "VERSION is valid semver" "Got: $FILE_VER"
fi

if [ "$CONFIG_VER" = "$FILE_VER" ]; then
    pass "Config version matches VERSION"
else
    fail "Config version ($CONFIG_VER) != VERSION ($FILE_VER)"
fi

# Governance compile schema — per ADR-007, this is decoupled from edikt VERSION.
# The file must declare compile_schema_version matching the constant in compile.md.
SCHEMA_CONST=$(grep -oE 'COMPILE_SCHEMA_VERSION = [0-9]+' "$PROJECT_ROOT/commands/gov/compile.md" | awk '{print $3}' | head -1)
GOV_SCHEMA=$(grep -oE '^compile_schema_version: [0-9]+' "$PROJECT_ROOT/.claude/rules/governance.md" 2>/dev/null | awk '{print $2}' | head -1)

if [ -n "$SCHEMA_CONST" ]; then
    pass "commands/gov/compile.md declares COMPILE_SCHEMA_VERSION constant"
else
    fail "commands/gov/compile.md declares COMPILE_SCHEMA_VERSION constant" \
        "No 'COMPILE_SCHEMA_VERSION = N' found in compile.md"
fi

if [ -n "$GOV_SCHEMA" ] && [ "$GOV_SCHEMA" = "$SCHEMA_CONST" ]; then
    pass "Governance compile_schema_version ($GOV_SCHEMA) matches compile.md constant"
else
    fail "Governance compile_schema_version matches compile.md constant" \
        "governance=$GOV_SCHEMA, compile.md=$SCHEMA_CONST"
fi

# Governance must NOT have the legacy version field (that was the pre-ADR-007 bug)
if grep -qE '^version: "' "$PROJECT_ROOT/.claude/rules/governance.md" 2>/dev/null; then
    fail "Governance has no legacy 'version:' field (ADR-007)" \
        "Legacy version field still present — should be compile_schema_version"
else
    pass "Governance has no legacy 'version:' field (ADR-007)"
fi

# Changelog has v0.2.0 entry
assert_file_contains "$PROJECT_ROOT/CHANGELOG.md" "## v0.2.0" "Changelog has v0.2.0 entry"

# ============================================================
# TEST 10: Website completeness
# ============================================================

echo ""
echo -e "${BOLD}TEST 10: Website${NC}"

WEBSITE="$PROJECT_ROOT/website"

assert_file_exists "$WEBSITE/guides/ci.md" "CI guide exists"
assert_file_contains "$WEBSITE/guides/ci.md" "GitHub Actions" "CI guide has GH Actions example"
assert_file_contains "$WEBSITE/guides/ci.md" "EDIKT_HEADLESS" "CI guide documents headless"
assert_file_contains "$WEBSITE/guides/ci.md" "bare" "CI guide documents --bare"

assert_file_contains "$WEBSITE/guides/security.md" "CLAUDE_CODE_SUBPROCESS_ENV_SCRUB" "Security guide has env scrub"
assert_file_contains "$WEBSITE/guides/security.md" "failIfUnavailable" "Security guide has sandbox enforcement"

assert_file_contains "$WEBSITE/guides/upgrading.md" "v0.2.0" "Upgrading guide has v0.2.0 section"
assert_file_contains "$WEBSITE/guides/upgrading.md" "Directive sentinels" "Upgrading guide explains sentinels"

assert_file_contains "$WEBSITE/commands/gov/compile.md" "topic" "Compile docs explain topic grouping"
assert_file_contains "$WEBSITE/commands/gov/compile.md" "routing table" "Compile docs explain routing"
assert_file_contains "$WEBSITE/commands/gov/compile.md" "sentinel" "Compile docs explain sentinels"

assert_file_contains "$WEBSITE/agents.md" "Agent governance" "Agents page has governance section"
assert_file_contains "$WEBSITE/agents.md" "evaluator" "Agents page documents evaluator"
assert_file_contains "$WEBSITE/agents.md" "SendMessage" "Agents page documents auto-resume"

assert_file_contains "$WEBSITE/natural-language.md" "plan to fix" "Natural language page has fix trigger"

# Check VitePress builds
if command -v npx >/dev/null 2>&1 && [ -d "$WEBSITE" ] && [ -d "$WEBSITE/node_modules/vitepress" ]; then
    BUILD_OUTPUT=$(cd "$WEBSITE" && npx vitepress build 2>&1)
    if echo "$BUILD_OUTPUT" | grep -q "build complete"; then
        pass "VitePress builds successfully"
    else
        fail "VitePress build failed"
    fi
else
    echo "  SKIP  VitePress build (npx or vitepress not available)"
fi

# ============================================================
# TEST 11: Dogfood governance — edikt's own ADRs have sentinels
# ============================================================

echo ""
echo -e "${BOLD}TEST 11: Dogfood governance${NC}"

for adr in "$PROJECT_ROOT"/docs/architecture/decisions/*.md; do
    name=$(basename "$adr")
    # Skip superseded
    if grep -q "Superseded" "$adr" 2>/dev/null; then
        pass "Skipped superseded: $name"
        continue
    fi
    if grep -q "Accepted\|accepted" "$adr" 2>/dev/null; then
        assert_file_contains "$adr" "edikt:directives:start" "Sentinel in $name"
        # Verify sentinel has required fields
        sentinel=$(sed -n '/edikt:directives:start/,/edikt:directives:end/p' "$adr")
        if echo "$sentinel" | grep -q "directives:"; then
            pass "Sentinel has directives: $name"
        else
            fail "Sentinel missing directives: $name"
        fi
        if echo "$sentinel" | grep -q "ref:"; then
            pass "Sentinel has refs: $name"
        else
            fail "Sentinel missing refs: $name"
        fi
    fi
done

for inv in "$PROJECT_ROOT"/docs/architecture/invariants/*.md; do
    name=$(basename "$inv")
    assert_file_contains "$inv" "edikt:directives:start" "Sentinel in $name"
done

# ============================================================
# TEST 12: Deprecated command stubs
# ============================================================

echo ""
echo -e "${BOLD}TEST 12: Deprecated command stubs${NC}"

for old_cmd in adr invariant compile review-governance rules-update sync prd spec spec-artifacts plan review drift audit docs intake; do
    stub="$PROJECT_ROOT/commands/deprecated/${old_cmd}.md"
    assert_file_exists "$stub" "Deprecated stub exists: $old_cmd"
    assert_file_contains "$stub" "deprecated" "Stub contains deprecation message: $old_cmd"
    assert_file_contains "$stub" "v0.4.0" "Stub mentions removal version: $old_cmd"
done

test_summary

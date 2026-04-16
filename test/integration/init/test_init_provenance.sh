#!/usr/bin/env bash
# test/integration/init/test_init_provenance.sh
# Phase 9: verify _substitutions.yaml, stack markers, and provenance instructions.
#
# These are structural/content tests that run against the repo's template files.
# No live Claude Code session needed — we test that the building blocks are
# in place for Claude to execute the provenance install sequence correctly.

set -uo pipefail
PROJECT_ROOT="${1:-.}"
. "$(dirname "$0")/../../helpers.sh"

AGENTS_DIR="$PROJECT_ROOT/templates/agents"
INIT_CMD="$PROJECT_ROOT/commands/init.md"

echo ""

# ─── 1. _substitutions.yaml ────────────────────────────────────────────────

assert_file_exists "$AGENTS_DIR/_substitutions.yaml" \
  "_substitutions.yaml exists"

assert_valid_yaml "$AGENTS_DIR/_substitutions.yaml" \
  "_substitutions.yaml is valid YAML"

# Only decisions and invariants are active substitution entries.
# specs/prds/plans/guidelines were removed in v0.5.0 — they appeared in no
# agent template and would never fire. See _substitutions.yaml comment.
for key in decisions invariants; do
  assert_file_contains "$AGENTS_DIR/_substitutions.yaml" "^  ${key}:" \
    "_substitutions.yaml has key: $key"
done

assert_file_contains "$AGENTS_DIR/_substitutions.yaml" "config_key:" \
  "_substitutions.yaml has config_key fields"

assert_file_contains "$AGENTS_DIR/_substitutions.yaml" "default:" \
  "_substitutions.yaml has default fields"

# ─── 2. Stack markers in agent templates ────────────────────────────────────

STACK_AGENTS=(backend qa frontend mobile)

for agent in "${STACK_AGENTS[@]}"; do
  FILE="$AGENTS_DIR/${agent}.md"

  assert_file_contains "$FILE" "<!-- edikt:stack:" \
    "${agent}.md has stack markers"

  assert_file_contains "$FILE" "<!-- /edikt:stack -->" \
    "${agent}.md has stack closing markers"
done

# ─── 3. Marker integrity: every opening has a matching closing ───────────────

for agent in "${STACK_AGENTS[@]}"; do
  FILE="$AGENTS_DIR/${agent}.md"

  open_count=$(grep -c '<!-- edikt:stack:' "$FILE" 2>/dev/null || true)
  close_count=$(grep -c '<!-- /edikt:stack -->' "$FILE" 2>/dev/null || true)

  if [ "$open_count" -eq "$close_count" ] && [ "$open_count" -gt 0 ]; then
    pass "${agent}.md: $open_count opening marker(s) matched by $open_count closing marker(s)"
  else
    fail "${agent}.md: marker mismatch" \
      "opening=$open_count closing=$close_count"
  fi
done

# ─── 4. Language coverage in stack markers ──────────────────────────────────

# backend and qa: must cover all six backend languages
for agent in backend qa; do
  FILE="$AGENTS_DIR/${agent}.md"
  for lang in go typescript python rust ruby php; do
    if grep -q "edikt:stack:.*${lang}" "$FILE"; then
      pass "${agent}.md stack covers: $lang"
    else
      fail "${agent}.md stack covers: $lang" \
        "no marker referencing $lang found"
    fi
  done
done

# frontend: must cover typescript
assert_file_contains "$AGENTS_DIR/frontend.md" "edikt:stack:typescript" \
  "frontend.md stack covers: typescript"

# mobile: must cover typescript, dart, swift, kotlin
for lang in typescript dart swift kotlin; do
  if grep -q "edikt:stack:.*${lang}" "$AGENTS_DIR/mobile.md"; then
    pass "mobile.md stack covers: $lang"
  else
    fail "mobile.md stack covers: $lang" \
      "no marker referencing $lang found"
  fi
done

# ─── 5. No stack markers in read-only agents ────────────────────────────────
# Stack filtering is only designed for language-heavy write-capable agents.
# Read-only agents should not have stack markers (they have no formatter section).

for agent in architect dba security api docs; do
  FILE="$AGENTS_DIR/${agent}.md"
  [ -f "$FILE" ] || continue
  if grep -q "<!-- edikt:stack:" "$FILE"; then
    fail "${agent}.md should not have stack markers (read-only agent)"
  else
    pass "${agent}.md has no stack markers (read-only agent)"
  fi
done

# ─── 6. commands/init.md provenance instructions ────────────────────────────

assert_file_contains "$INIT_CMD" "edikt_template_hash" \
  "init.md documents edikt_template_hash frontmatter field"

assert_file_contains "$INIT_CMD" "edikt_template_version" \
  "init.md documents edikt_template_version frontmatter field"

assert_file_contains "$INIT_CMD" "_substitutions.yaml" \
  "init.md references _substitutions.yaml"

assert_file_contains "$INIT_CMD" "md5" \
  "init.md documents md5 hash computation"

assert_file_contains "$INIT_CMD" "stack filter" \
  "init.md documents stack filter step"

assert_file_contains "$INIT_CMD" "before any modification\|before substitution\|Hash before" \
  "init.md specifies hash-before-substitution rule"

assert_file_contains "$INIT_CMD" "edikt:stack:" \
  "init.md references edikt:stack: marker syntax"

# ─── 7. _substitutions.yaml listed in agents registry (or standalone) ────────
# The registry should not list _substitutions.yaml as an agent — it is a data file.
REGISTRY="$AGENTS_DIR/_registry.yaml"
if [ -f "$REGISTRY" ]; then
  if grep -q "_substitutions" "$REGISTRY"; then
    fail "_registry.yaml should not list _substitutions.yaml as an agent"
  else
    pass "_registry.yaml does not include _substitutions.yaml"
  fi
fi

test_summary

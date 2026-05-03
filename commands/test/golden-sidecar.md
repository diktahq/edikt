---
name: test:golden-sidecar
description: "Regenerate a sidecar golden fixture by invoking the live sidecar-extractor agent and compare against expected via bin/edikt sidecar diff. Dev-only — gated behind EDIKT_REGEN_FIXTURES=1 (INV-007 sandbox rule for hermetic tests)."
tier: 1
tier_2_dependency: edikt
on_absent: refuse-and-direct-user
allowed-tools:
  - Read
  - Write
  - Bash
  - Task
---

# /edikt:test:golden-sidecar

## Arguments

- `<fixture-dir>` — path to a fixture directory under `test/fixtures/sidecar-extractor/`. The directory MUST contain `input.md`, `expected.edikt.yaml`, and `fixture.yaml`. After this command runs, `actual.edikt.yaml` will be written.

## Steps

### 1. Gate on `EDIKT_REGEN_FIXTURES=1`

Run:

```bash
if [ "${EDIKT_REGEN_FIXTURES:-0}" != "1" ]; then
  echo "Live regeneration skipped (set EDIKT_REGEN_FIXTURES=1 to run)."
  exit 0
fi
```

If the env var is unset, exit 0 with the skip message. CI runs in this gated state — only the deterministic comparator (`bin/edikt sidecar diff`) runs there.

### 2. Verify `bin/edikt` on PATH (ADR-029 absence detection)

```bash
if ! command -v bin/edikt >/dev/null 2>&1 && ! command -v edikt >/dev/null 2>&1; then
  echo "error: bin/edikt not found on PATH. Run: edikt install edikt"
  exit 1
fi
```

### 3. Read `<fixture-dir>/fixture.yaml`

Read `model`, `temperature`, and `seed` from the fixture config. These parameters constrain the live extractor invocation — keeping them in the fixture file means a regeneration run is reproducible.

### 4. Dispatch the live `sidecar-extractor` agent

Use the Task tool with the `sidecar-extractor` agent template loaded from `templates/agents/sidecar-extractor.md`. Pass `<fixture-dir>/input.md` as the input artifact path. The agent's locked behavior (Read + Write tools only, no Edit / Bash / Agent / Task) is enforced by its frontmatter `disallowedTools`.

The agent writes the regenerated sidecar to `<fixture-dir>/input.md`'s sibling location with the `.edikt.yaml` suffix — but for golden fixtures, the fixture directory's basename does not match the input filename's basename, so the agent output may need a copy step.

### 5. Move the agent output to `<fixture-dir>/actual.edikt.yaml`

```bash
# The extractor writes to <input-basename>.edikt.yaml next to input.md.
INPUT_BASENAME=$(basename "<fixture-dir>/input.md" .md)
mv "<fixture-dir>/${INPUT_BASENAME}.edikt.yaml" "<fixture-dir>/actual.edikt.yaml"
```

### 6. Run the deterministic comparator

```bash
bin/edikt sidecar diff <fixture-dir>
```

Exit codes:
- `0` — equivalent (fixture passed).
- `1` — divergent. Review the structured diff; either the extractor regressed (fix the prompt) or the fixture's `expected.edikt.yaml` needs updating (deliberate behavior change).
- `2` — missing fixture file.
- `3` — argv error.

### 7. Print verdict

On exit 0, print `golden-sidecar [<fixture-dir>]: PASS`.
On exit 1, print `golden-sidecar [<fixture-dir>]: FAIL — review diff above; update expected.edikt.yaml if the change is intentional, or fix the extractor prompt`.
On exit 2 or 3, print the binary's error verbatim.

## Notes

- This command is opt-in via `EDIKT_REGEN_FIXTURES=1`. CI runs the deterministic comparator only — never invokes claude. ADR-030 boundary: tier-2 binary stays LLM-agnostic; tier-1 markdown drives the LLM resync.
- The `Makefile` target `regen-fixtures` walks `test/fixtures/sidecar-extractor/` and invokes this command for every subdirectory. Use that for full-corpus regeneration; use this slash command for single-fixture iteration.
- INV-007 hermetic sandbox: the agent runs in a forked subagent context with locked tools; it cannot read `~/.claude/settings.json`, copy host hooks, or escape the fixture directory.

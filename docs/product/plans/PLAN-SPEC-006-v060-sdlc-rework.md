# Plan: SPEC-006 — v0.6.0 SDLC rework, tier-2 Go install, and hook hardening

## Overview

**Task:** Implement SPEC-006 (v0.6.0 feature release)
**Implements:** SPEC-006 (draft)
**Total Phases:** 11
**Estimated Cost:** ~$3.20
**Created:** 2026-04-18
**Branch:** 0.6.0-dev

## Progress

| Phase | Status  | Attempt | Updated    |
|-------|---------|---------|------------|
| 1     | ✅ pass | 1       | 2026-04-18 |
| 2     | ✅ pass | 1       | 2026-04-18 |
| 3     | ✅ pass | 1       | 2026-04-18 |
| 4     | ✅ pass | 1       | 2026-04-18 |
| 5     | ✅ pass | 1       | 2026-04-18 |
| 6     | ✅ pass | 1       | 2026-04-18 |
| 7     | ✅ pass | 1       | 2026-04-18 |
| 8     | ✅ pass | 1       | 2026-04-18 |
| 9     | ✅ pass | 1       | 2026-04-18 |
| 10    | ✅ pass | 1       | 2026-04-18 |
| 11    | ✅ pass | 1       | 2026-04-18 |

**IMPORTANT:** Update this table as phases complete.

## Model Assignment

| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1 | ADR-023 + bin/edikt tier-2 verbs | `sonnet` | State machine in shell with rollback; medium complexity | $0.08 |
| 2 | Go binary for benchmark install | `opus` | Novel Go code, env-var interface, checksum/venv/rollback; complex | $0.80 |
| 3 | subagent-stop.sh structured payload | `sonnet` | Hook extension with fallback path + fixture update | $0.08 |
| 4 | _staged_runner.sh stubs + fixture re-additions | `sonnet` | Shell test helper + fixture characterization | $0.08 |
| 5 | Fixture status field backfill + drift axis | `sonnet` | Schema extension + drift command extension | $0.08 |
| 6 | artifacts aspirational default + doctor checks | `sonnet` | Two command extensions; moderate complexity | $0.08 |
| 7 | Shared routing + deprecated stubs | `sonnet` | Extract-and-replace in markdown commands | $0.08 |
| 8 | Events.jsonl doctor + session-start | `sonnet` | Read-and-surface pattern; straightforward | $0.08 |
| 9 | Configurable gate severity tiers | `sonnet` | Config schema + hook + UX; moderate | $0.08 |
| 10 | Pre-push invariant validation | `sonnet` | Shell extension with Python diff checker | $0.08 |
| 11 | CHANGELOG + ROADMAP + passing tests | `haiku` | Documentation and test run; low complexity | $0.08 |

## Execution Strategy

| Phase | Depends On | Parallel With | Wave |
|-------|-----------|---------------|------|
| 1     | —         | 5, 7, 8, 10   | 1    |
| 2     | 1         | 3, 4, 6, 9    | 3    |
| 3     | 1         | 4, 5, 7, 8, 10 | 2   |
| 4     | 1, 3      | 5, 7, 8, 10   | 2    |
| 5     | —         | 1, 3, 7, 8, 10 | 1   |
| 6     | 5         | 2, 3, 9       | 3    |
| 7     | —         | 1, 3, 5, 8, 10 | 1   |
| 8     | —         | 1, 3, 5, 7, 10 | 1   |
| 9     | 3, 8      | 2, 6          | 3    |
| 10    | —         | 1, 3, 5, 7, 8 | 1    |
| 11    | all       | —             | 4    |

**Wave 1 (parallel):** 1, 5, 7, 8, 10 — five independent phases.
**Wave 2 (parallel):** 3, 4 — depend on Phase 1 (ADR-023 + shell verbs).
**Wave 3 (parallel):** 2, 6, 9 — Go binary needs shell; doctor drift needs schema; severity needs structured hook.
**Wave 4:** 11 — integration pass, CHANGELOG, green tests.

---

## Phase 1: ADR-023 + bin/edikt tier-2 verbs

**Objective:** Write and accept ADR-023. Implement `edikt install/uninstall benchmark`
in `bin/edikt` as shell. Pass AC-001–AC-011 tests.
**Model:** `sonnet`
**Max Iterations:** 5
**Completion Promise:** `PHASE 1 COMPLETE`
**Dependencies:** None

**Prompt:**
```
Implement Phase 1 of SPEC-006 on branch 0.6.0-dev.

## Part A: Write ADR-023

Write `docs/architecture/decisions/ADR-023-subagent-stop-structured-evaluator-input.md`
following the six-section ADR format (ADR-009). Status: accepted.

The decision:
- evaluator agents MUST emit the structured payload defined in SPEC-006 §FR-003:
  {"verdict": "PASS"|"FAIL"|"BLOCKED", "evaluator_output": {"severity": "critical"|"warning"|"info", "findings": [...]}}
- subagent-stop.sh reads evaluator_output.severity and evaluator_output.findings[].
- Legacy unstructured payloads trigger a fallback path with a logged warning (not a crash).
- Extends ADR-018 (evaluator verdict schema) and ADR-014 (hook JSON transport).
- Does not supersede any existing ADR.

Generate a directive sentinel block for ADR-023 with:
  paths: ["templates/hooks/subagent-stop.sh", "templates/agents/evaluator*.md"]
  scope: [implementation, review]
  directives:
    - Evaluator agents MUST emit a JSON verdict with evaluator_output.severity and evaluator_output.findings[] per ADR-023. Never emit a bare verdict string or prose.
    - subagent-stop.sh MUST read evaluator_output fields from the hook input. Legacy unstructured payloads MUST fall back gracefully (log a warning, do not crash). (ref: ADR-023)
  manual_directives: []
  suppressed_directives: []
  canonical_phrases: ["evaluator_output", "severity", "findings", "BLOCKED"]
  behavioral_signal:
    cite: ["ADR-023"]
  source_hash: <compute SHA-256 of ADR body above sentinel block, normalized>
  directives_hash: <compute SHA-256 of the directives list above>

## Part B: bin/edikt tier-2 verbs

Implement `edikt install benchmark` and `edikt uninstall benchmark` in `bin/edikt`
(shell script at repo root). These verbs must satisfy the 12 tests in
`test/integration/test_install_tier2.py`.

Read that test file carefully before writing any code. The tests are the contract.

Key implementation requirements (from test file analysis):

ENVIRONMENT VARIABLES the tests inject:
  EDIKT_HOME       — overrides ~/.edikt
  CLAUDE_HOME      — overrides ~/.claude
  EDIKT_TIER2_SKIP_PIP=1 — skip pip install (write .pip-skipped sentinel, still copy markdown)
  EDIKT_TIER2_PYTHON — Python binary path override (default: python3)
  EDIKT_TIER2_WHEEL — wheel file path
  EDIKT_TIER2_WHEEL_SHA256 — expected wheel SHA-256
  EDIKT_TIER2_SOURCE — source directory for benchmark.md + attack templates (default: $EDIKT_HOME/current/tools/gov-benchmark)

INSTALL sequence (must match test assertions):
1. Python version check:
   - Run: $EDIKT_TIER2_PYTHON -c "import sys; v=sys.version_info; print(f'{v.major}.{v.minor}')"
   - If python not found or version < 3.10: print to stderr exactly:
     "edikt benchmark requires Python 3.10+; found X.Y at <path>"
     exit 1.
   - This check MUST run BEFORE any file writes.
2. Wheel checksum (if $EDIKT_TIER2_WHEEL is set):
   - Compute SHA-256 of the file: python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$EDIKT_TIER2_WHEEL"
   - Compare to $EDIKT_TIER2_WHEEL_SHA256. If mismatch: print "Wheel checksum mismatch" to stderr, exit 2.
   - If $EDIKT_TIER2_WHEEL_SHA256 is empty AND path contains "/current/": print actionable error, exit 2.
3. Copy markdown:
   - Source: $EDIKT_TIER2_SOURCE (default: $EDIKT_HOME/current/tools/gov-benchmark)
   - benchmark.md → $CLAUDE_HOME/commands/edikt/gov/benchmark.md
   - templates/attacks/*.md → $CLAUDE_HOME/commands/edikt/templates/attacks/
   - Track all copied paths in a temp array for rollback.
4. Venv creation:
   - If EDIKT_TIER2_SKIP_PIP=1: write $EDIKT_HOME/venv/gov-benchmark/.pip-skipped, skip steps 5.
   - Else: $EDIKT_TIER2_PYTHON -m venv $EDIKT_HOME/venv/gov-benchmark
5. Pip install:
   - If $EDIKT_TIER2_WHEEL is set: pip install $EDIKT_TIER2_WHEEL
   - Else: pip install -e $EDIKT_TIER2_SOURCE (in dev mode)
6. Rollback (on any failure in steps 3-5):
   - Delete all files copied in step 3.
   - rm -rf $EDIKT_HOME/venv/gov-benchmark
   - exit non-zero.

UNINSTALL sequence:
- Remove $CLAUDE_HOME/commands/edikt/gov/benchmark.md (tolerate missing)
- Remove $CLAUDE_HOME/commands/edikt/templates/attacks/ dir (tolerate missing)
- Remove $EDIKT_HOME/venv/gov-benchmark/ (tolerate missing)
- If nothing was present: print "already uninstalled" to stderr (or stdout), exit 0.
- If anything was present: remove it, print "uninstalled benchmark", exit 0.

INV-003 compliance: use python3 -c 'import json; print(json.dumps(...))' for any
JSON emission. Never concatenate JSON via shell.
INV-001 compliance: bin/edikt is a shell script (not compiled code).
ADR-015 compliance: no changes to install.sh.

Also: create tools/gov-benchmark/pyproject.toml if it does not already exist with:
  [project]
  name = "gov-benchmark"
  dependencies = ["claude-agent-sdk==0.0.41"]  # must use ==, not >= or ~=
  [build-system]
  requires = ["setuptools"]
  build-backend = "setuptools.backends.legacy:build"

Also: ensure commands/gov/benchmark.md has tier: 2 in its frontmatter.

Run the 12 tests after implementation:
  pytest test/integration/test_install_tier2.py -v

All 12 must pass. Fix until green.

When complete, output: PHASE 1 COMPLETE
```

---

## Phase 2: Go binary for benchmark install

**Objective:** Implement `tools/gov-benchmark/cmd/install.go` — a Go binary that
mirrors the shell implementation from Phase 1 with the same env-var interface.
**Model:** `opus`
**Max Iterations:** 5
**Completion Promise:** `PHASE 2 COMPLETE`
**Dependencies:** Phase 1

**Prompt:**
```
Implement Phase 2 of SPEC-006 on branch 0.6.0-dev.

Phase 1 implemented edikt install/uninstall benchmark in bin/edikt as shell.
Phase 2 adds an optional Go binary at tools/gov-benchmark/cmd/install.go that
implements the same logic. bin/edikt delegates to the compiled binary when it
exists at $EDIKT_HOME/bin/gov-install; falls back to shell otherwise.

Read the SPEC-006 spec at docs/product/specs/SPEC-006-v060-sdlc-rework/spec.md
for the full FR-002 requirements. Read tools/edikt/cmd/install.go for the
pattern to follow.

## Implementation

### tools/gov-benchmark/cmd/install.go

The Go binary reads all inputs from environment variables:
  EDIKT_HOME, CLAUDE_HOME
  EDIKT_TIER2_SKIP_PIP, EDIKT_TIER2_PYTHON, EDIKT_TIER2_WHEEL, EDIKT_TIER2_WHEEL_SHA256
  EDIKT_TIER2_SOURCE

Subcommands passed as os.Args:
  gov-install install benchmark
  gov-install uninstall benchmark

Exit codes (matching tools/edikt/cmd/install.go conventions):
  0 = success
  1 = prereq failure (Python not found, version too low, network)
  2 = checksum mismatch
  3 = already installed (uninstall with nothing to remove → 0, not 3)
  5 = path traversal / malicious input detected

Install steps mirror the shell implementation exactly (same prereq check message,
same rollback behavior, same SKIP_PIP sentinel file). The acceptance tests run against
the shell launcher (bin/edikt), so the Go binary is not tested directly — but the
behavior must be identical.

Rollback in Go:
- Maintain a []string of copied file paths.
- On any error after files are copied: call a rollback function that deletes each
  copied file (errors ignored) and removes the venv directory.
- Use defer for cleanup on panic.

Path traversal check: before copying any file, verify the destination path's
filepath.Clean resolves inside CLAUDE_HOME or EDIKT_HOME. If not, exit 5.

### tools/gov-benchmark/Makefile

  build:
    go build -o $(EDIKT_HOME)/bin/gov-install ./cmd/install.go

  test:
    go test ./...

  install-binary: build

### bin/edikt delegation (update from Phase 1)

After the Phase 1 shell implementation, add a delegation check at the top of the
install_benchmark() function:

  GOV_INSTALL_BIN="${EDIKT_HOME}/bin/gov-install"
  if [ -x "$GOV_INSTALL_BIN" ]; then
    exec "$GOV_INSTALL_BIN" install benchmark
  fi
  # ... fall through to shell implementation

Same for uninstall.

## Constraints
- INV-001: bin/edikt remains shell. The Go binary is a helper, not a replacement.
- ADR-015: tier-1 files unchanged. The Go binary only touches venv/ and CLAUDE_HOME paths.
- The 12 AC-023 tests MUST still pass (they test via the shell launcher).

Run: pytest test/integration/test_install_tier2.py -v
All 12 must remain green after this phase.

When complete, output: PHASE 2 COMPLETE
```

---

## Phase 3: subagent-stop.sh structured payload consumer

**Objective:** Update `subagent-stop.sh` to read `evaluator_output.severity` and
`evaluator_output.findings[]` from the hook input per ADR-023. Update the fixture.
**Model:** `sonnet`
**Max Iterations:** 4
**Completion Promise:** `PHASE 3 COMPLETE`
**Dependencies:** Phase 1

**Prompt:**
```
Implement Phase 3 of SPEC-006 on branch 0.6.0-dev.

Read ADR-023 at docs/architecture/decisions/ADR-023-subagent-stop-structured-evaluator-input.md.
Read the current templates/hooks/subagent-stop.sh.
Read docs/product/specs/SPEC-006-v060-sdlc-rework/spec.md §FR-003 for the full design.
Read the existing hook unit test fixtures in test/unit/hooks/ and fixtures.yaml.

## Changes required

### templates/hooks/subagent-stop.sh

Currently the hook infers severity from content keywords. Extend it to:

1. Try to parse evaluator_output from the hook input JSON:
   SEVERITY=$(python3 -c '
   import json, sys
   try:
       d = json.load(sys.stdin)
       eo = d.get("evaluator_output", {})
       print(eo.get("severity", ""))
   except Exception:
       print("")
   ' <<< "$HOOK_INPUT")

2. If SEVERITY is non-empty, use it directly.
3. If SEVERITY is empty (legacy unstructured payload):
   - Fall back to current keyword-based detection.
   - Log a warning: emit a JSON systemMessage "⚠ evaluator output missing structured
     payload; falling back to keyword detection. Update evaluator template to emit
     evaluator_output field per ADR-023." using python3 JSON emission (INV-003).

4. Apply the gate threshold:
   - Read EDIKT_GATE_SEVERITY_THRESHOLD env var (default: "critical").
   - Block only when severity meets or exceeds threshold (critical > warning > info).
   - If blocking, include in systemMessage: "severity: X ≥ threshold: Y"

INV-003: All JSON emission MUST use python3 -c 'import json; print(json.dumps(...))'
with untrusted values as argv. Never shell-concatenate JSON.

INV-004: The hook writes its own log entry. Never instruct Claude to run the write.

### Update the hook fixture

After modifying the hook, run it against existing payload fixtures and update
test/unit/hooks/ if any expected outputs change.

Run the full hook unit suite:
  bash test/unit/hooks/run.sh

All existing tests must pass. If a fixture changes, update it and document why
in the commit message.

When complete, output: PHASE 3 COMPLETE
```

---

## Phase 4: _staged_runner.sh stubs + fixture re-additions

**Objective:** Extend `_staged_runner.sh` with determinism helpers. Re-add
`session-start-with-edikt` and `subagent-stop-critical` fixture pairs.
**Model:** `sonnet`
**Max Iterations:** 4
**Completion Promise:** `PHASE 4 COMPLETE`
**Dependencies:** Phases 1, 3

**Prompt:**
```
Implement Phase 4 of SPEC-006 on branch 0.6.0-dev.

Read docs/product/specs/SPEC-006-v060-sdlc-rework/spec.md §FR-004.
Read test/unit/hooks/_staged_runner.sh.
Read docs/product/specs/SPEC-004-v050-stability/fixtures.yaml §9.1 to find the two
deferred fixture pairs and their _note blocks.

## Part A: _staged_runner.sh extensions

Add three helper functions to test/unit/hooks/_staged_runner.sh:

### stub_git_identity()
  export GIT_AUTHOR_NAME="Edikt Test"
  export GIT_AUTHOR_EMAIL="test@edikt.local"
  export GIT_COMMITTER_NAME="Edikt Test"
  export GIT_COMMITTER_EMAIL="test@edikt.local"

### provision_memory_fixture()
Creates a minimal ~/.claude/MEMORY.md with deterministic content:
  # Memory
  - [Project](project.md) — test project

### stub_clock()
If libfaketime is available, wraps the hook call with it at EDIKT_TEST_FIXED_TIMESTAMP
(default: "2026-01-01 12:00:00"). Otherwise sets FAKETIME env var and exports it.
Falls back gracefully (no-op with a comment) if neither is available.

## Part B: session-start-with-edikt fixture

Run templates/hooks/session-start.sh with a staged project directory that has:
- .edikt/config.yaml present (basic config)
- CLAUDE.md with edikt sentinel block

Capture the actual JSON output. Add to fixtures.yaml §9.1:
  - path: session-start-with-edikt.expected.json
    format: json
    status: characterized
    verified_by: "<the exact command you ran>"
    verified_at: "2026-04-18"
    content: <exact JSON output captured>
    _note: "Re-added in v0.6.0 Phase 4 after _staged_runner.sh git-identity stub landed."

Remove the old _note deferral marker. Re-enable this fixture in the test runner.

## Part C: subagent-stop-critical fixture

Run templates/hooks/subagent-stop.sh with a payload that produces the blocking-path
output. Use stub_git_identity() and stub_clock() to make it deterministic.

Capture actual output. Add to fixtures.yaml §9.1 with status: characterized.

## Verification

Run the full hook unit suite after both fixtures are added:
  bash test/unit/hooks/run.sh

All tests including the two new fixture pairs must pass.

When complete, output: PHASE 4 COMPLETE
```

---

## Phase 5: Fixture status field backfill + drift axis

**Objective:** Backfill `status: characterized` on all Phase 11b fixture records.
Extend `/edikt:sdlc:drift` with a fixture characterization section.
**Model:** `sonnet`
**Max Iterations:** 4
**Completion Promise:** `PHASE 5 COMPLETE`
**Dependencies:** None (independent)

**Prompt:**
```
Implement Phase 5 of SPEC-006 on branch 0.6.0-dev.

Read docs/product/specs/SPEC-006-v060-sdlc-rework/spec.md §FR-005.
Read docs/product/specs/SPEC-004-v050-stability/fixtures.yaml §9.1 (all records).

## Part A: Backfill fixtures.yaml

For every expected-output record in §9.1 that currently lacks a `status:` field:
- If the record has a `verified_by:` command already: add `status: characterized`.
- If the record looks aspirational (has a _note deferral or target_phase): add `status: aspirational` + `target_phase` + `target_contract`.
- Otherwise: add `status: characterized` with `verified_by: "# see _note"` and `verified_at: "2026-04-18"`.

The two re-added pairs from Phase 4 will have status: characterized (leave them as-is).

## Part B: /edikt:sdlc:drift fixture section

Edit commands/sdlc/drift.md to add a §9 (or new subsection) that reads fixtures.yaml
§9.1 and outputs:

```
§ Fixture characterization status

  Characterized (N):
    <filename>  ✓ verified <date>
    ... (list all)

  Aspirational (M):
    <filename>
      target: <target_phase>
      contract: <target_contract>
    ... (list all)

  Aspirational debt: M/(N+M) (pct%) — <status emoji + message>
```

Where the emoji + message follows:
  0% → "✅ All fixtures characterized"
  1-25% → "🟡 Low aspirational debt"
  26-50% → "🟠 Moderate aspirational debt — consider characterizing before next release"
  >50% → "🔴 High aspirational debt — most fixtures are unverified"

Aspirational entries are warnings, not errors. The section is skipped (absent, no error)
if fixtures.yaml §9.1 does not exist.

## Verification

After editing:
  pytest test/integration/test_spec_preprocessing.py -v

All existing tests must still pass.

When complete, output: PHASE 5 COMPLETE
```

---

## Phase 6: /edikt:sdlc:artifacts aspirational default + doctor checks

**Objective:** `/edikt:sdlc:artifacts` defaults new fixture records to `aspirational`.
`/edikt:doctor` gains characterization-rate check.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `PHASE 6 COMPLETE`
**Dependencies:** Phase 5

**Prompt:**
```
Implement Phase 6 of SPEC-006 on branch 0.6.0-dev.

Read docs/product/specs/SPEC-006-v060-sdlc-rework/spec.md §FR-005 (AC-023, AC-024).
Read commands/sdlc/artifacts.md (the /edikt:sdlc:artifacts command).
Read commands/doctor.md (the /edikt:doctor command).

## Part A: /edikt:sdlc:artifacts default

In the section that generates fixture YAML records, change the template for new
expected-output records to include:
  status: aspirational
  target_phase: "<spec phase that will implement this>"
  target_contract: "<one-line description of the desired behavior>"
  # verified_by and verified_at are omitted until characterized

After generating fixture records, print:
  ⚠ New fixture records default to status: aspirational.
    Run the verified_by command after the behavior ships and update
    status to characterized + add verified_by + verified_at.

## Part B: /edikt:doctor characterization check

In the health-check section of commands/doctor.md, add:

Check: "Fixture characterization rate"
  1. Read fixtures.yaml §9.1 if present.
  2. Count characterized vs aspirational records.
  3. If >50% aspirational: WARN "Fixture characterization rate is low (X%). Most
     test expectations are unverified against running code."
  4. If EDIKT_DOCTOR_DEEP=1 (opt-in): for each characterized record with verified_at
     older than 90 days, attempt to run verified_by (if the command is safe) and diff
     output against content. Flag stale records.
  5. If fixtures.yaml absent: SKIP (not an error).

When complete, output: PHASE 6 COMPLETE
```

---

## Phase 7: Shared agent routing layer + deprecated stub removal

**Objective:** Extract shared routing template. Delete 16 deprecated stubs.
**Model:** `sonnet`
**Max Iterations:** 4
**Completion Promise:** `PHASE 7 COMPLETE`
**Dependencies:** None (independent)

**Prompt:**
```
Implement Phase 7 of SPEC-006 on branch 0.6.0-dev.

Read docs/product/specs/SPEC-006-v060-sdlc-rework/spec.md §FR-006 and §FR-007.
Read commands/sdlc/plan.md, commands/sdlc/review.md, commands/sdlc/drift.md.
List commands/deprecated/ to get the 16 stub files.

## Part A: Shared agent routing template

Create commands/_shared-agent-routing.md with:

### 1. Domain signal table

| Domain | Signals | Agent subagent_type |
|--------|---------|---------------------|
| Database | SQL, query, schema, migration, index, database, db, table, foreign key, join, transaction, ORM, Postgres, MySQL, SQLite, MongoDB | principal-dba |
| Infrastructure | deploy, docker, kubernetes, k8s, terraform, helm, CI, CD, infra, container, Dockerfile, compose, nginx, AWS, GCP, Azure, cloud | staff-sre |
| Security | auth, JWT, OAuth, payment, PCI, HIPAA, token, secret, encrypt, credential, password, permission, role, RBAC, CORS, XSS, injection | staff-security |
| API | API, endpoint, REST, GraphQL, route, webhook, contract, openapi, swagger, versioning, breaking change | senior-api |
| Architecture | bounded context, domain, architecture, refactor, pattern, layer, dependency, coupling, abstraction, interface, hexagonal, clean arch | principal-architect |
| Performance | performance, N+1, cache, latency, throughput, slow, optimize, index, query optimization, benchmark | senior-performance |

### 2. detect_signals(text) procedure

Scan text (all phase prompts + objectives) for domain keywords.
Return a list of detected domains.
If no domains detected: output "Pre-flight: no specialist domains detected — plan looks self-contained."

### 3. spawn_specialists(signals, context) procedure

For each detected domain, spawn the specialist agent concurrently.
Return a list of {domain, findings, severity_counts} objects.

### 4. consolidate_findings(results[]) procedure

Concatenate findings with domain headers.
Count total critical/warning/ok across domains.
Output the consolidated pre-flight review block.

Replace the duplicated domain-signal logic in plan.md, review.md, drift.md with:
  <!-- edikt:include _shared-agent-routing.md -->
  (Use the shared detect_signals + spawn_specialists procedure defined above.)

Do NOT add edikt:include as a real include mechanism — this is a documentation
convention. In the markdown commands, replace the duplicated text with a prose
reference: "See _shared-agent-routing.md for the domain signal table and specialist
spawning procedure."

## Part B: Deprecated stub removal

Run: ls commands/deprecated/
Delete all .md files in commands/deprecated/ using the Edit/Write tools (not Bash rm).
Actually — use Bash to list them, then delete using Bash rm.
After deletion, if commands/deprecated/ is empty, remove the directory.

If any stubs have non-trivial content (not just a redirect), preserve the content
in a MIGRATION_NOTES.md in the commit description, not in the file.

Update the edikt command table in CLAUDE.md if it references deprecated stubs.

Run the regression suite after:
  pytest test/integration/regression/ -v

All regression tests must pass.

When complete, output: PHASE 7 COMPLETE
```

---

## Phase 8: Events.jsonl doctor awareness + session-start surfacing

**Objective:** Doctor reads events.jsonl for unresolved gate findings. Session-start
hook surfaces the most recent unresolved finding.
**Model:** `sonnet`
**Max Iterations:** 4
**Completion Promise:** `PHASE 8 COMPLETE`
**Dependencies:** None (independent)

**Prompt:**
```
Implement Phase 8 of SPEC-006 on branch 0.6.0-dev.

Read docs/product/specs/SPEC-006-v060-sdlc-rework/spec.md §FR-008.
Read commands/doctor.md (current doctor checks).
Read templates/hooks/session-start.sh (current session-start hook).
Read .edikt/state/events.jsonl schema from existing hook code to understand the format.

## Part A: doctor gate awareness

Add a "Gate activity" section to commands/doctor.md:

```
§ Gate activity (last 7 days)

Read .edikt/events.jsonl (if present). Parse as JSONL — one JSON object per line.
Skip lines that don't parse.

For each event with "event": "gate_fired":
  - Check if a subsequent "event": "gate_resolved" exists with the same event_id.
  - If no matching gate_resolved exists within 7 days: it's unresolved.

Output:
  Unresolved: N
    <date>: <agent> gate (<severity>) — no resolution recorded
    ...
  Overrides (last 30 days): M
    <date>: <description>
    ...

If .edikt/events.jsonl is absent: output "  No event log found." (not an error).
```

INV-003: Use python3 -c '...' with argv for any JSON handling in hook scripts.

## Part B: session-start hook surfacing

In templates/hooks/session-start.sh, after the existing startup logic:

1. Read .edikt/events.jsonl if present.
2. Find the most recent unresolved gate_fired event (event without matching gate_resolved).
3. If found: emit a systemMessage via python3 JSON emission (INV-003):
   "⚠ Last session: <agent> gate fired on <description> — was it resolved?\n  To dismiss: run /edikt:session (end-of-session sweep)"
4. If none found: emit nothing (do not emit an empty field).
5. Cap: only the single most recent unresolved finding. Do not list all.

Rollback test: if events.jsonl is empty or malformed, the hook exits 0 silently (no crash).

## Verification

Run:
  pytest test/integration/test_post_compact_recovery.py -v
  bash test/unit/hooks/run.sh

All must pass.

When complete, output: PHASE 8 COMPLETE
```

---

## Phase 9: Configurable gate severity tiers

**Objective:** `.edikt/config.yaml` gains `gates:` section. `subagent-stop.sh` reads
the threshold. `/edikt:config` documents it.
**Model:** `sonnet`
**Max Iterations:** 4
**Completion Promise:** `PHASE 9 COMPLETE`
**Dependencies:** Phases 3, 8

**Prompt:**
```
Implement Phase 9 of SPEC-006 on branch 0.6.0-dev.

Read docs/product/specs/SPEC-006-v060-sdlc-rework/spec.md §FR-009.
Read templates/hooks/subagent-stop.sh (updated in Phase 3).
Read commands/config.md.
Read templates/settings.json.tmpl and the default .edikt/config.yaml template.

## Part A: config.yaml schema extension

In the config.yaml template and in templates/project-context.md.tmpl (or wherever
default config is generated), add the gates section:

gates:
  security: warning    # fire on warning and above
  dba: critical        # fire only on critical
  sre: warning
  architect: warning
  performance: critical
  api: warning
  default: critical    # for unknown agents

Update commands/config.md to document the gates: section:
  gates.<agent>: severity threshold for the named agent. One of: critical, warning, info.
    default: critical (block only on critical)
    warning: block on warning and above
    info: block on any finding

## Part B: subagent-stop.sh reads threshold

In templates/hooks/subagent-stop.sh (updated in Phase 3):

After determining SEVERITY (from structured payload or fallback), read the threshold:
  AGENT_NAME=$(python3 -c '...' <<< "$HOOK_INPUT")  # extract agent name from payload
  THRESHOLD=$(python3 -c '
  import yaml, os, sys
  config_path = os.path.join(os.environ.get("EDIKT_PROJECT_ROOT", "."), ".edikt", "config.yaml")
  try:
      with open(config_path) as f:
          cfg = yaml.safe_load(f)
      agent = sys.argv[1]
      threshold = cfg.get("gates", {}).get(agent, cfg.get("gates", {}).get("default", "critical"))
      print(threshold)
  except Exception:
      print("critical")
  ' "$AGENT_NAME")

Severity ordering: critical(3) > warning(2) > info(1).
Block when severity_level >= threshold_level.

In the block message, include:
  "severity: $SEVERITY ≥ threshold: $THRESHOLD"
  "To change threshold: .edikt/config.yaml gates.$AGENT_NAME: critical"

INV-003: Use python3 -c with argv for all JSON/YAML reading in shell.

## Verification

  bash test/unit/hooks/run.sh

All must pass. Add a fixture test for the threshold message if feasible.

When complete, output: PHASE 9 COMPLETE
```

---

## Phase 10: Pre-push invariant validation

**Objective:** Extend pre-push hook to validate staged diffs against invariants.
**Model:** `sonnet`
**Max Iterations:** 4
**Completion Promise:** `PHASE 10 COMPLETE`
**Dependencies:** None (independent)

**Prompt:**
```
Implement Phase 10 of SPEC-006 on branch 0.6.0-dev.

Read docs/product/specs/SPEC-006-v060-sdlc-rework/spec.md §FR-010.
Read templates/hooks/pre-push.sh (current implementation).
Read INV-001 through INV-008 in docs/architecture/invariants/ to understand the rules.

## Implementation

The pre-push hook currently runs a security-only grep. Extend it to also check
invariant compliance on the staged diff.

Add after the security grep block:

```bash
# Invariant compliance on staged diff
python3 - <<'PY' "$STAGED_FILES" # pass file list as argv
import sys, os, re, subprocess

staged = sys.argv[1].split("\n") if len(sys.argv) > 1 else []
violations = []

for f in staged:
    if not f:
        continue
    try:
        diff = subprocess.run(["git", "diff", "--cached", "--", f],
                              capture_output=True, text=True).stdout
    except Exception:
        continue

    # INV-001: no compiled code in commands/ or templates/
    if re.search(r'^commands/|^templates/', f):
        for ext in ('.ts', '.js', '.py', '.rb', '.go', '.rs'):
            if f.endswith(ext):
                violations.append(f"INV-001: {f} — compiled code in commands/ or templates/")

    # INV-002: no edit to accepted ADR
    if re.search(r'docs/architecture/decisions/ADR-\d+', f):
        try:
            content = open(f).read()
            if re.search(r'^status:\s*accepted', content, re.MULTILINE):
                if diff.strip():  # there is a diff
                    violations.append(f"INV-002: {f} — accepted ADR is immutable (status: accepted)")
        except Exception:
            pass

    # INV-003: no shell JSON concatenation in hook scripts
    if re.search(r'templates/hooks/', f) or f == 'install.sh':
        for pat in (r'''echo\s+['"][{]''', r'''printf\s+['"][{]'''):
            if re.search(pat, diff):
                violations.append(f"INV-003: {f} — shell JSON concatenation forbidden")

if violations:
    print("❌ Pre-push invariant check failed:\n", file=sys.stderr)
    for v in violations:
        print(f"  {v}", file=sys.stderr)
    print("\nFix violations or set EDIKT_BYPASS_PREPUSH=1 to bypass (logged).", file=sys.stderr)
    sys.exit(1)
else:
    print("✅ Invariant check passed")
PY
INVARIANT_EXIT=$?
if [ $INVARIANT_EXIT -ne 0 ]; then
    exit 1
fi
```

Bypass path:
  if [ "${EDIKT_BYPASS_PREPUSH:-}" = "1" ]; then
    # Log to events.jsonl
    python3 -c 'import json,sys,os,datetime; ...' # emit bypass event
    # skip checks
    exit 0
  fi

INV-003 compliance: use python3 heredoc with argv for YAML/JSON. The Python code
itself is not JSON-in-shell, so no violation.

## Verification

Run:
  pytest test/integration/test_post_compact_recovery.py -v  # unrelated but useful smoke
  bash test/unit/hooks/run.sh

When complete, output: PHASE 10 COMPLETE
```

---

## Phase 11: CHANGELOG + ROADMAP + integration pass

**Objective:** Write CHANGELOG v0.6.0 entry. Update ROADMAP. Run full AC-023 +
e2e test suite and ensure all pass.
**Model:** `haiku`
**Max Iterations:** 3
**Completion Promise:** `PHASE 11 COMPLETE`
**Dependencies:** All previous phases

**Prompt:**
```
Implement Phase 11 of SPEC-006 on branch 0.6.0-dev.

This is the release documentation and integration phase.

## Part A: Run the test suites

Run these test files and report results:
  pytest test/integration/test_install_tier2.py -v           # 12 AC-023 tests
  pytest test/integration/test_e2e_v060_release.py -v        # 13 e2e smoke tests (skipped without SDK)

Report: how many pass, how many skip (SDK tests without auth are OK to skip), any failures.

## Part B: CHANGELOG

Add a v0.6.0 entry to CHANGELOG.md at the top (below the header). Use the same format
as existing entries.

Content to cover:
- edikt install / uninstall benchmark (tier-2, Go-backed)
- Configurable gate severity tiers (gates: in config.yaml)
- subagent-stop.sh structured evaluator-input contract (ADR-023)
- session-start-with-edikt and subagent-stop-critical fixture pairs re-added
- Fixture characterization lifecycle (status: characterized/aspirational)
- /edikt:sdlc:drift fixture characterization section
- Shared agent routing layer (_shared-agent-routing.md)
- Deprecated command stub removal (16 stubs)
- Events.jsonl doctor awareness + session-start surfacing
- Pre-push invariant validation (INV-001, INV-002, INV-003)

## Part C: ROADMAP update

In docs/internal/plans/ROADMAP.md:

1. Add v0.6.0 to the Shipped section with today's date and a brief list of items.
2. Update the v0.6.0 Target section: mark each shipped item with "✅ SHIPPED in v0.6.0"
   or remove the item from the active list.
3. Update "Current version" to 0.6.0.

## Part D: .edikt/config.yaml version bump

Update edikt_version in .edikt/config.yaml from 0.5.0 to 0.6.0.

## Part E: SPEC-006 status

Update docs/product/specs/SPEC-006-v060-sdlc-rework/spec.md frontmatter:
  status: accepted

Update this plan file (PLAN-SPEC-006-v060-sdlc-rework.md) Phase 11 status to done.

When complete, output: PHASE 11 COMPLETE
```

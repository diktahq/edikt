# Plan: v0.5.0 Security Hardening

## Overview

**Task:** Close every finding from the v0.5.0 security audit (7 Critical · 14 High · 14 Medium · 13 Low) and capture new governance (INV-003 through INV-008, ADR-016 through ADR-019) so the failure classes don't recur. Target tag: v0.5.0 (not yet cut — we're on `0.5.0-dev`).
**Source audit:** `docs/reports/security-audit-v0.5.0-2026-04-17.md`
**Total Phases:** 15
**Estimated Cost:** ~$6.17 (7 opus + 7 sonnet + 1 haiku)
**Created:** 2026-04-17
**Release policy decisions (locked with user 2026-04-17):**
- Scope: **all** findings (Critical + High + Medium + Low) in v0.5.0.
- Signing: **Sigstore keyless** (cosign via GitHub OIDC).
- Release-asset model: **ADR-013 path (a)** — single canonical `edikt-payload-<tag>.tar.gz` + `SHA256SUMS` + `SHA256SUMS.sig`.
- Rollout: **single v0.5.0 drop**, no v0.4.4 patch.
- Breaking changes: accepted with explicit upgrade migration + CHANGELOG guidance; **grandfather existing PASS evaluations** so in-flight plans don't regress to BLOCKED.

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done   | 1/3     | 2026-04-17 |
| 2     | done   | 1/5     | 2026-04-17 |
| 3     | done   | 1/3     | 2026-04-17 |
| 4     | done (HI-4 closed via byte-range guard; LOW-3 hash anchor seeding deferred to compile) | 1/5 | 2026-04-17 |
| 5     | done (cosign signing + verification, release-asset URL, tag-shape gate, loud banner, README pinned; Homebrew tap production-gate deferred) | 1/5 | 2026-04-17 |
| 6     | done (schema + agent prompts + evidence gate; grandfather routine in Phase 13) | 1/5 | 2026-04-17 |
| 7     | done   | 1/3     | 2026-04-17 |
| 8     | done   | 1/3     | 2026-04-17 |
| 9     | done   | 1/3     | 2026-04-17 |
| 10    | done   | 1/3     | 2026-04-17 |
| 11    | done (MED-4 deferred — markdown-embedded Python extraction is internal-only, low-risk without a PR) | 1/3 | 2026-04-17 |
| 12    | done (HI-3, MED-6, MED-8, MED-14 done; MED-7/LOW-1/LOW-2/LOW-4/LOW-5/LOW-13 deferred) | 1/3 | 2026-04-17 |
| 13    | -      | 0/3     | -       |
| 14    | -      | 0/5     | -       |
| 15    | -      | 0/3     | -       |

**IMPORTANT:** Update this table as phases complete. This table is the persistent state that survives context compaction.

## Model Assignment

| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1  | Governance capture (5 INVs + 3 ADRs)              | opus   | Architectural decisions; immutable once accepted | $0.80 |
| 2  | JSON emission systemic fix across hooks + installer| opus   | Security-critical pattern, 6+ files, RCE-class   | $0.80 |
| 3  | Input shape validation (INV-006)                  | sonnet | Mechanical regex gates across launcher/hooks     | $0.08 |
| 4  | Sentinel byte-range guard (INV-005)               | opus   | Novel logic replacing broken regex guard         | $0.80 |
| 5  | Release integrity, checksum wiring, Sigstore       | opus   | Supply chain, CI changes, signing, cosign verify | $0.80 |
| 6  | Evaluator verdict schema (ADR-018) + grandfather   | opus   | Schema design, in-flight migration nuance        | $0.80 |
| 7  | Default permissions posture (ADR-017)             | sonnet | Config tuning in `settings.json.tmpl`            | $0.08 |
| 8  | Hermetic benchmark sandboxes (INV-007)            | sonnet | Python test harness refactor                     | $0.08 |
| 9  | Stop-hook defang + NL-trigger hardening            | sonnet | Hook rewrite, clear pattern                       | $0.08 |
| 10 | Attack corpus expansion + NFKC scoring            | sonnet | New markdown templates + Python scorer fix       | $0.08 |
| 11 | Python test harness hardening                     | sonnet | Multiple Med/Low findings in `test/`              | $0.08 |
| 12 | Mediums & Lows grab-bag                           | haiku  | Mostly mechanical one-liners                      | $0.01 |
| 13 | Upgrade migration + CHANGELOG                     | sonnet | Migration logic + user-facing docs                | $0.08 |
| 14 | Security regression test suite                     | opus   | Tests that pin every fix against recurrence       | $0.80 |
| 15 | Final audit re-run + release gate                  | opus   | Dogfood `/edikt:sdlc:audit` + benchmark pass     | $0.80 |

## Execution Strategy

| Phase | Depends On | Parallel With |
|-------|------------|---------------|
| 1     | None       | —             |
| 2     | 1          | 3,4,5,6,7,8,9,10,11,12 |
| 3     | 1          | 2,4,5,6,7,8,9,10,11,12 |
| 4     | 1,2        | 3,5,6,7,8,10,11,12 |
| 5     | 1          | 2,3,4,6,7,8,9,10,11,12 |
| 6     | 1          | 2,3,4,5,7,8,9,10,11,12 |
| 7     | 1,2        | 3,4,5,6,8,10,11,12 |
| 8     | 1          | 2,3,4,5,6,7,9,10,11,12 |
| 9     | 1,2        | 3,4,5,6,7,8,10,11,12 |
| 10    | 1          | 2,3,4,5,6,7,8,9,11,12 |
| 11    | 1          | 2,3,4,5,6,7,8,9,10,12 |
| 12    | 1          | 2,3,4,5,6,7,8,9,10,11 |
| 13    | 2-12       | 14            |
| 14    | 2-12       | 13            |
| 15    | 13,14      | —             |

**Waves:**
- **Wave 1:** Phase 1 (governance foundation)
- **Wave 2a:** Phases 2, 3, 5, 6, 8, 10, 11, 12 (parallel — independent fixes)
- **Wave 2b:** Phases 4, 7, 9 (parallel — all depend on Phase 2's JSON emission pattern; start after 2 lands)
- **Wave 3:** Phases 13, 14 (migration + regression tests, both depend on all Wave 2)
- **Wave 4:** Phase 15 (final gate)

Phases 4, 7, and 9 each use the JSON emission helper that Phase 2 establishes — they cannot land before Phase 2.

---

## Phase 1: Capture governance — INV-003 through INV-008 and ADR-016 through ADR-019

**Objective:** Write and accept the six invariants and four ADRs that codify the failure classes surfaced by the audit, so the directive compiler can enforce them and future work cannot regress the classes.
**Model:** `opus`
**Max Iterations:** 3
**Completion Promise:** `GOVERNANCE CAPTURED`
**Dependencies:** None

**Prompt:**
```
Read:
- docs/reports/security-audit-v0.5.0-2026-04-17.md (the full audit, especially the three systemic root causes and each CRIT/HI finding)
- docs/architecture/invariants/INV-001-*.md and INV-002-*.md (format reference)
- docs/architecture/decisions/ADR-013-*.md (supersede target) and ADR-014-*.md, ADR-015-*.md (current format)
- ADR-009 (six-section invariant body) and ADR-006 (sentinel format)

Create ten governance files. Follow existing frontmatter and body conventions exactly; use status `accepted` for all new invariants (no draft stage needed — classes are already proven) and status `accepted` for all new ADRs. ADR-016 must supersede ADR-013 (update ADR-013's `Status:` to `Superseded by ADR-016` and keep ADR-013 otherwise byte-identical per INV-002). ADR-019 narrows (does not supersede) ADR-014 — ADR-014 remains accepted and is cited by ADR-019.

**Every new invariant MUST use the six-section body structure mandated by ADR-009:**
1. **Statement** — the rule itself, one or two sentences, imperative.
2. **Rationale** — why this rule exists; cite the audit finding ID(s).
3. **Consequences of violation** — what breaks (concrete failure modes).
4. **Implementation** — the code pattern, test, or lint that realizes the rule.
5. **Anti-patterns** — before/after snippets of forbidden vs required forms.
6. **Enforcement** — the mechanism (directive compile, pre-tool-use hook, CI lint, test fixture).

**Distinguish INV-003 (emission format) from INV-004 (channel).** INV-003 governs *how* a hook serializes JSON to stdout (json.dumps, never shell concat). INV-004 governs *what* content is allowed in any hook-driven channel that reaches Claude (systemMessage, additionalContext, markdown bodies). A hook that uses json.dumps to build a shell command it then asks Claude to execute is INV-003-compliant but INV-004-violating. State this overlap explicitly in both invariants' Scope sections.

FILE 1 — docs/architecture/invariants/INV-003-hooks-emit-structured-json.md
Rule: Hook scripts in templates/hooks/*.sh must emit every protocol JSON payload via a structured serializer (python3 json.dumps or equivalent). Shell string concatenation to build JSON (e.g. `echo "{\"key\":\"${VAR}\"}"`) is forbidden. Untrusted values pass as argv to the serializer, never as bash-interpolated strings.
Include: motivation (audit findings CRIT-1/2/4/5, HI-1/2, MED-1), scope (every file under templates/hooks/), examples of forbidden and required forms, enforcement (linter phase to land in Phase 14), exceptions (none).

FILE 2 — docs/architecture/invariants/INV-004-no-agent-text-into-shell.md
Rule: Hooks must not instruct Claude Code (via `systemMessage`, `additionalContext`, or any other channel) to execute shell commands whose body interpolates subagent-derived, file-content-derived, or otherwise untrusted text. Logging is the hook's responsibility; the hook writes `events.jsonl` itself. Claude receives only a static prompt plus structured data.
Include: reference CRIT-1 scenario (subagent-stop.sh building shell for Claude to run); scope (all hook scripts); before/after examples.

FILE 3 — docs/architecture/invariants/INV-005-managed-region-integrity.md
Rule: Every edikt-managed region has an integrity mechanism that must be verified before the region is overwritten. Two variants:
  (a) **Markdown-hosted regions** (CLAUDE.md, ADRs, invariants, plans, rule files) are delimited by `[edikt:NAME:start]: #` / `[edikt:NAME:end]: #` sentinels with an inline `[edikt:NAME:sha256]: # <hex>` hash anchor. Edits are validated by byte-range overlap on the resolved file — not regex over `old_string`/`new_string` — and the hash is verified before overwrite.
  (b) **JSON-hosted regions** (settings.json) cannot use in-file sentinels (JSON has no comment syntax). Their integrity is recorded out-of-band in a sidecar at `~/.edikt/state/settings-managed.json` with the shape `{managed_keys, managed_hash, sentinel_version}`. The install/upgrade writer verifies the live JSON's managed-key hash against the sidecar before overwriting managed keys.
In both variants, an edit whose resolved byte range (markdown) or managed-key hash (JSON) would change the region is blocked unless issued by `/edikt:gov:compile` (markdown) or the install/upgrade writer (JSON). The `EDIKT_COMPILE_IN_PROGRESS` and `EDIKT_MIGRATION_IN_PROGRESS` env vars are the only allowlisted bypasses.
Include: audit reference HI-4, LOW-3; required helpers (pre-tool-use guard for markdown, sidecar reader for JSON); bootstrap rule (absent hash/sidecar -> seed, don't validate); exception list.

FILE 4 — docs/architecture/invariants/INV-006-externally-controlled-input-validation.md
Rule: Any value read from outside edikt's immediate control — filenames, config keys/values, environment variables, CLI flags, URL refs — must be validated against an allowlist regex before being interpolated into argv, URLs, prompt strings, or file paths. Validators must reject silently-different Unicode and whitespace variants (NFKC-normalize then casefold, strip whitespace).
Include: audit reference CRIT-3, MED-5/9, HI-2; validator catalog (plan filename `^[A-Za-z0-9._-]+$`, tag `^v?[0-9]+\.[0-9]+\.[0-9]+$`, model ID allowlist, hook dir `[^|"\\\n]+`).

FILE 5 — docs/architecture/invariants/INV-007-hermetic-test-sandboxes.md
Rule: Benchmark and test sandboxes must be hermetic. The test harness must not copy the host's `.claude/settings.json`, user-global settings, hooks, or secrets into the sandbox. `setting_sources` for Claude subprocesses in tests is restricted to `["project"]` with a curated minimal settings file.
Include: audit reference HI-10, HI-11; scope (everything under test/integration/); reference implementation.

FILE 6 — docs/architecture/decisions/ADR-016-release-integrity-and-signing.md
Status: Accepted
Supersedes: ADR-013
Decision: 
  (a) Publish exactly one canonical release artifact per tag: `edikt-payload-<tag>.tar.gz` as a GitHub release asset. 
  (b) Publish `SHA256SUMS` alongside it, covering every asset in the release.
  (c) Sign `SHA256SUMS` with Sigstore keyless (cosign v2 + GitHub OIDC identity); publish a single `SHA256SUMS.sig.bundle` (the bundle carries both signature and certificate — do NOT publish a redundant `.pem`). 
  (d) The launcher and installer download the release asset (not the auto-generated archive), verify its SHA against `SHA256SUMS`, and verify the bundle with `cosign verify-blob --bundle SHA256SUMS.sig.bundle --certificate-identity-regexp '^https://github\.com/diktahq/edikt/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$' --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' SHA256SUMS`. Regex form is required because the installer must match any tag at verify time, not a specific literal tag. 
  (e) The README curl-pipe install URL is pinned to a specific release tag; `main` is never a user-facing install surface.
Tradeoffs: Sigstore adds a build-time dep (cosign) and requires `id-token: write` permission in the release workflow; GPG alternative rejected due to key-custody burden. 
Consequences: ADR-013's per-file `.sha256` sidecar path is obsolete. Installer with `EDIKT_INSTALL_INSECURE=1` still honored but gains a loud post-install banner. Homebrew formula SHA continues to reference the release asset.

FILE 7 — docs/architecture/decisions/ADR-017-default-permissions-posture.md
Status: Accepted
Decision: `templates/settings.json.tmpl` ships with an explicit `permissions` block containing:
  - deny: `WebFetch(http://**)`, `Bash(curl http://**)`, `Bash(rm -rf /**)`, `Bash(rm -rf ~/**)`, `Bash(chmod -R 777 **)`, `Bash(sudo **)`, `Bash(* > /dev/tcp/**)`, `Bash(:(){:|:&};:)` (fork bomb literal), destructive git patterns (`git push --force main`, `git reset --hard origin/**`)
  - allow: explicit entries for tools edikt itself requires (`Read(**)`, `Edit(**)`, `Write(**)`, `Bash(git *)`, `Bash(gh pr *)`, `Bash(npm test)`, etc. — full list in phase 7 spec)
  - defaultMode: `askBeforeAllow`
Tradeoffs: first-run friction for users whose workflows rely on currently-unconstrained tools; mitigated by explicit one-line prompts. Existing installs with customized `settings.json` preserved via sentinel merge — the new block lands inside `[edikt:permissions:start]: #` sentinels so user overrides outside the block survive.
Consequences: Fresh installs may see a permission prompt on first `http://` fetch or `curl http://` usage. Documented in CHANGELOG and `docs/guides/permissions.md`.

FILE 8 — docs/architecture/decisions/ADR-019-security-override-of-byte-for-byte-hook-preservation.md
Status: Accepted
Supersedes: (none — narrows ADR-014)
Decision: ADR-014 directs hook JSON-wrapping migrations to preserve user-visible message content byte-for-byte, with content changes landing in separate commits with fresh fixtures. That directive remains in force for mechanical wrapping work. This ADR carves out one exception: when a hook's existing message content itself violates INV-003 or INV-004 (hooks must not build shell via concatenation, hooks must not instruct Claude to execute shell built from untrusted text), the security rewrite may change user-visible content AND wrapping format in a single commit. The hooks affected are named exhaustively here to prevent scope creep: subagent-stop.sh (GATE_MSG shell instruction to Claude), file-changed.sh (systemMessage with file path), headless-ask.sh (permissionDecision with answer), stop-failure.sh (error fields). No other hook may use this carve-out.
Tradeoffs: Breaks ADR-014's "one thing per commit" invariant for four specific hooks; mitigated by listing the hooks exhaustively and requiring fresh fixtures for each. Alt: delay security fixes until after wrapping migration is complete — rejected because INV-003/INV-004 violations are Critical-severity RCE vectors per audit CRIT-1/2/4/5.
Consequences: Phase 2 of PLAN-v0.5.0-security-hardening.md is the sole place this carve-out is applied. Any future hook content change must follow ADR-014 unaltered.

FILE 9 — docs/architecture/invariants/INV-008-release-install-urls-tag-pinned.md
Rule: All user-facing install and upgrade URLs must resolve to a specific release tag (not `main`, not `latest`). Tags move is an attacker primitive; pinning removes it. Applies to README install commands, `bin/edikt install`/`upgrade` URL composition, documentation snippets, and any CI job users are expected to copy.
Include: audit reference CRIT-7; enforcement via a pre-release CI check that greps every doc for `main` in a `raw.githubusercontent.com` or `releases/latest` URL.

FILE 10 — docs/architecture/decisions/ADR-018-evaluator-verdict-schema.md
Status: Accepted
Decision: Headless evaluator (`templates/agents/evaluator-headless.md`) emits a structured JSON verdict conforming to schema `templates/agents/evaluator-verdict.schema.json`. Verdict files are persisted as runtime state sidecars at `docs/product/plans/verdicts/<plan-slug>/<phase-id>.json` — JSON because verdicts are machine output, not governance. This directory is a new runtime-state location; it is not covered by INV-001 (commands-and-templates must be markdown/yaml) because verdicts are neither commands nor templates. Schema:
  { verdict: "PASS" | "BLOCKED" | "FAIL",
    criteria: [ { id: string, status: "met" | "unmet" | "blocked", evidence_type: "test_run" | "grep" | "file_read" | "manual", evidence: string, notes?: string } ],
    meta: { evaluator_mode: "headless" | "interactive", grandfathered: boolean } }
The plan harness (phase-end-detector.sh / plan.md) rejects PASS unless every criterion that names a shell command (e.g. `./test/run.sh`, `pytest …`) has `evidence_type: "test_run"`. Grandfathering: verdicts recorded before v0.5.0 land with `grandfathered: true` and bypass the evidence gate on a one-time basis per phase, so in-flight plans don't regress.
Tradeoffs: structured output is longer and costs more tokens; in-flight plans need a one-time grandfather pass during upgrade.
Consequences: PASS without test evidence returns BLOCKED going forward. HI-7 closed.

Update two existing files:
- docs/architecture/decisions/ADR-013-release-checksum-format.md: change `Status: Accepted` to `Status: Superseded by ADR-016`. No other edits (INV-002).
- docs/architecture/invariants/INDEX.md (if present) and decisions/INDEX.md (if present): add entries for new files.

Run /edikt:gov:compile at the end to refresh compiled directives. Record in this phase's completion note which ADRs/INVs are now reflected in the compiled directives.

When complete, output: GOVERNANCE CAPTURED
```

---

## Phase 2: JSON emission systemic fix across hooks and installer

**Objective:** Eliminate every shell-concatenated JSON emission in `templates/hooks/*.sh` and `install.sh`. Every hook protocol payload must go through `python3 json.dumps` with untrusted values passed as argv. Closes CRIT-1, CRIT-2, CRIT-4, CRIT-5, HI-1, HI-2, MED-1.
**Model:** `opus`
**Max Iterations:** 5
**Completion Promise:** `JSON EMISSION HARDENED`
**Dependencies:** 1

**Prompt:**
```
Read:
- docs/architecture/invariants/INV-003-hooks-emit-structured-json.md (just written)
- docs/architecture/invariants/INV-004-no-agent-text-into-shell.md
- templates/hooks/worktree-create.sh lines 63-67 — this is the reference pattern all other hooks must adopt.
- templates/hooks/task-created.sh lines 68-80 — second reference.
- docs/reports/security-audit-v0.5.0-2026-04-17.md sections CRIT-1 through CRIT-5 and HI-1/2.

Targets (every file must be converted):
1. templates/hooks/stop-failure.sh:12-17 — error.type/error.message -> json.dumps
2. templates/hooks/headless-ask.sh:59 — $ANSWER -> json.dumps
3. templates/hooks/file-changed.sh:24 — $CHANGED_FILE -> json.dumps
4. templates/hooks/subagent-stop.sh:115-150 — ENTIRE gate construction rewritten:
   - The hook itself writes ~/.edikt/events.jsonl (no Claude shell instructions).
   - systemMessage becomes STATIC — "Gate fired; see events.jsonl for details. Run /edikt:capture to review." No agent-derived substring ever embedded.
   - decision: block emitted via json.dumps.
   - GIT_USER/GIT_EMAIL never appear in Claude-facing text.
5. templates/hooks/event-log.sh:11-31 — JSON construction routed through a single Python helper function.
6. install.sh:486-504 — settings.json templating no longer uses sed. Introduce an inline Python script that:
   - loads templates/settings.json.tmpl as a string,
   - loads hook-dir value from $EDIKT_HOOK_DIR,
   - substitutes via Python `.replace()` of a literal sentinel token (`__EDIKT_HOOK_DIR__`),
   - json.loads-round-trips the result to confirm it's valid JSON,
   - writes atomically.
   - Any failure is a hard abort with a clear error.
7. Any other hook in templates/hooks/*.sh that contains `echo '{` or `echo "{` or `echo "...\"` pattern — audit and convert. Candidates to grep: `grep -nE 'echo ["'"'"']\{' templates/hooks/*.sh`.

Helper pattern (use everywhere, inline per hook — no external helper library; INV-001 prefers per-file inlining for hooks):
```
python3 -c 'import json,sys; print(json.dumps({"k1": sys.argv[1], "k2": sys.argv[2]}))' "$VAL1" "$VAL2"
```

For stop-failure.sh and event-log.sh where multiple fields get logged, write the entire event dict inside a single `python3 -c` invocation with argv for each field.

Testing requirements (acceptance criteria):
- Add fixtures to test/unit/hooks/ for each converted hook:
  - Input with embedded `"` in a field -> output is valid JSON (json.loads succeeds).
  - Input with embedded newline in a field -> output is one JSON line, no newlines in the value.
  - Input with shell metacharacters (`$(date)`, backticks, `;`) in a field -> output contains them as literal text.
- Update test/integration/test_*.py if any existing test depends on the old echo-concat format.
- Verify with `grep -nE 'echo ["'"'"']\{' templates/hooks/*.sh install.sh` — must return zero matches for migrated files.

When complete, output: JSON EMISSION HARDENED
```

---

## Phase 3: Externally-controlled input shape validation (INV-006)

**Objective:** Validate every externally-controlled input against an allowlist regex before it's interpolated into argv, URLs, prompt strings, or file paths. Closes CRIT-3, MED-5, MED-9, and the validation aspect of HI-2.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `INPUT VALIDATION DONE`
**Dependencies:** 1

**Prompt:**
```
Read INV-006 (just written), the audit sections on CRIT-3/MED-5/MED-9, and these files:
- templates/hooks/phase-end-detector.sh (PLAN_STEM and EVAL_MODEL interpolation)
- install.sh (--ref handling, EDIKT_HOOK_DIR)
- bin/edikt (resolve_latest_tag, install subcommand, EDIKT_TIER2_PYTHON handling)

Validators to add (centralize in install.sh and bin/edikt as shell functions; hooks may inline regex checks):

1. Plan stem (templates/hooks/phase-end-detector.sh):
   - Accept: ^[A-Za-z0-9._-]+$
   - Reject with "invalid plan filename: <stem>" before any claude -p invocation.
   - Also: change the claude -p call at line 159 so $PLAN_STEM is a separate argv element, not interpolated into the prompt string: `"$_claude_bin" -p "/edikt:sdlc:plan --sidecar-only" "$PLAN_STEM"`.

2. EVAL_MODEL (templates/hooks/phase-end-detector.sh:273):
   - Allowlist from a small shell associative array: `claude-opus-4-7`, `claude-sonnet-4-6`, `claude-haiku-4-5-20251001`, `opus`, `sonnet`, `haiku` (short and full forms).
   - Unknown value -> warn-and-use-default.

3. --ref / tag (install.sh:61, 290; bin/edikt:2535):
   - Accept: ^v?[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$
   - Reject at argument parse time, before any URL construction.

4. EDIKT_HOOK_DIR (install.sh before line 496 substitution):
   - Accept: a path with no `|`, `"`, `\`, or newline. Use `case "$EDIKT_HOOK_DIR" in *'|'* | *'"'* | *'\'* | *$'\n'*) error; esac`.
   - Reject with "hook dir contains forbidden character" before templating.

5. EDIKT_TIER2_PYTHON (bin/edikt tier-2 install path):
   - Must be absolute (starts with /).
   - Must not be under $TMPDIR or /tmp.
   - Must point to a file with execute permission (`[ -x ]`).
   - Reject otherwise.

6. Worktree path (templates/hooks/worktree-create.sh):
   - realpath the value; assert it is under $(realpath "$SOURCE_REPO"). Reject if it escapes.

7. Claude Code tool_input.file_path in pre-tool-use.sh:
   - After shape validation, realpath and assert containment under project root for governance-protected paths.

Acceptance criteria:
- Each validator has a unit test (test/unit/validators/) with positive and negative cases, including Unicode variants (Cyrillic lookalikes), trailing whitespace, null bytes, directory traversal, and symlink escapes.
- Grep `grep -nE '(\$PLAN_STEM|\$EVAL_MODEL|\$REF_TAG|\$EDIKT_HOOK_DIR)' templates/hooks/*.sh install.sh bin/edikt` — every match is preceded by a validation gate or is inside a validated function.

When complete, output: INPUT VALIDATION DONE
```

---

## Phase 4: Sentinel byte-range guard (INV-005)

**Objective:** Replace the regex-based sentinel guard in `pre-tool-use.sh` with a byte-range guard that resolves the file, computes the post-patch byte offsets, and blocks edits that overlap sentinel-bounded regions. Closes HI-4 and LOW-3 (when combined with the hash anchor introduced here).
**Model:** `opus`
**Max Iterations:** 5
**Completion Promise:** `SENTINEL GUARD REPLACED`
**Dependencies:** 1

**Prompt:**
```
Read INV-005, ADR-006 (sentinel format), pre-tool-use.sh:36-41 (current broken guard), and bin/edikt's sentinel-handling functions (migrate_m2_claudemd_sentinels around line 1075 — block-finding logic to reuse).

Design:
1. Guard helper `edikt_sentinel_block_range_overlap` (Python inside pre-tool-use.sh, per the Phase 2 pattern):
   - Input: file_path, old_string (may be empty for Write), new_string (may be empty for Delete-through-Edit).
   - Read the current on-disk content of file_path.
   - Find every sentinel-bounded region: lines matching `[edikt:NAME:start]: #` and `[edikt:NAME:end]: #` at any indent, where NAME matches `[a-z][a-z0-9-]*`.
   - For an Edit: locate old_string inside the on-disk content (reject if it does not appear — Claude Code already does this, but we confirm). Compute the byte range that Edit will replace. If that range overlaps any sentinel-bounded region (inclusive of sentinel lines themselves), block unless the invoking command is /edikt:gov:compile (env EDIKT_COMPILE_IN_PROGRESS=1) or the edit is issued by a migration (env EDIKT_MIGRATION_IN_PROGRESS=1).
   - For a Write: if the file currently has sentinel regions and the new content would delete or modify them (diff check), block unless allowlisted as above.

2. Hash anchor inside every managed sentinel block (LOW-3):
   - Compile step emits an additional link-reference line immediately before the end sentinel: `[edikt:NAME:sha256]: # <64-hex>` where the sha256 is over the block content between start and the hash line (exclusive of sentinels themselves).
   - Before overwriting a block, edikt verifies the on-disk hash matches the stored hash. Mismatch -> abort with an instruction to run /edikt:gov:compile. This closes the fake-sentinel-injection attack.
   - **Bootstrap rule (absent hash → seed, don't validate):** when compile or the pre-tool-use guard encounters a sentinel block that has NO `[edikt:NAME:sha256]: #` line, it treats the block as unarmed: compile seeds the hash on first run; the guard blocks edits inside the unarmed block just as it would an armed one (byte-range check), but it does NOT attempt hash verification. This lets v0.4.3 users upgrade cleanly: compile seeds hashes on first run and the block becomes armed. Only blocks with a *present but mismatched* hash trigger the mismatch-abort path.
   - Scope: hash anchors apply to markdown-hosted sentinels (CLAUDE.md, ADRs, invariants, plans, rule files). JSON-hosted sentinels (settings.json) cannot use link-reference syntax — their integrity mechanism is the Phase 7 sidecar (`~/.edikt/state/settings-managed.json`) that records the managed-region hash out-of-band. Cross-reference this scope split in INV-005.

3. pre-tool-use.sh update:
   - Drop the current regex-based check (lines 36-41).
   - Add a Python block that calls the overlap helper and emits `{"decision":"block","reason":"..."}` (via the Phase 2 JSON emitter) when overlap is detected outside the allowlisted envs.

4. Tests (test/unit/hooks/test_pre_tool_use_sentinel.sh + Python fixtures):
   - Edit whose old_string is inside the block -> blocked.
   - Edit whose old_string is adjacent to the block (same line containing start sentinel) -> blocked.
   - Edit whose old_string is outside the block -> allowed.
   - Write that replaces a file containing sentinels with content lacking sentinels -> blocked.
   - Write that preserves sentinels byte-for-byte -> allowed.
   - Edit under EDIKT_COMPILE_IN_PROGRESS=1 -> allowed.
   - Sentinel with mismatched hash anchor -> compile refuses; pre-tool-use also refuses Edit on the file.
   - Unicode and CRLF variants of sentinel lines handled.
   - Fake sentinel added by an attacker before the real one -> helper uses the FIRST matching pair, so attacker-prepended block is treated as the managed region; to close this, the hash anchor is validated on load — mismatch aborts.

Acceptance criteria:
- Every existing hook test for sentinel behavior still passes.
- New tests above all pass.
- /edikt:gov:compile correctly writes the hash anchor on every managed block.
- Grep `grep -nE '\[edikt:.*:(start|end)\]: #' docs/` — every start/end pair also has a matching sha256 line after compile runs once.

When complete, output: SENTINEL GUARD REPLACED
```

---

## Phase 5: Release integrity, checksum wiring, Sigstore signing

**Objective:** Make `curl | bash` installs verifiably originate from the `diktahq/edikt` release workflow. Fix the broken checksum wiring per ADR-013 path (a), add Sigstore keyless signing per ADR-016, pin the README install URL, narrow the Homebrew tap token, add production-environment approval. Closes CRIT-6, CRIT-7, HI-8, MED-13.
**Model:** `opus`
**Max Iterations:** 5
**Completion Promise:** `RELEASE INTEGRITY READY`
**Dependencies:** 1

**Prompt:**
```
Read:
- docs/architecture/decisions/ADR-016-release-integrity-and-signing.md (just written)
- .github/workflows/release.yml (current workflow, incl. build + tap jobs)
- install.sh (launcher download and verify paths)
- bin/edikt install subcommand (lines ~640-700 for download, ~2500-2560 for tag resolution)
- README.md install section

Changes:

1. .github/workflows/release.yml:
   - Add `permissions: id-token: write, contents: write` at the job level for the build job.
   - After producing `edikt-payload-v<ver>.tar.gz` and `SHA256SUMS`:
     - Install cosign v2+ via sigstore/cosign-installer action.
     - Run `cosign sign-blob --yes --bundle SHA256SUMS.sig.bundle SHA256SUMS` (bundle format only; no separate `.pem` or `.sig` files).
     - Upload `SHA256SUMS`, `SHA256SUMS.sig.bundle`, `edikt-payload-v<ver>.tar.gz` as release assets.
   - Narrow the tap job: replace broad secrets.TAP_GITHUB_TOKEN with a fine-grained PAT (new secret: TAP_EDIKT_FORMULA_TOKEN) scoped to contents:write on diktahq/homebrew-tap only.
   - Add `environment: production` on the tap-merge step; require reviewer approval for auto-merge.
   - Add a strict tag-shape gate at job entry: `if: startsWith(github.ref, 'refs/tags/v') && ...<regex>...` using a shell guard that rejects tags not matching `^v[0-9]+\.[0-9]+\.[0-9]+$`.

2. install.sh (bootstrap):
   - Download the launcher from a pinned release asset, not raw.githubusercontent.com/.../main/.
   - After downloading the launcher, fetch SHA256SUMS and SHA256SUMS.sig.bundle from the release.
   - Verify cosign signature with regex identity (installer does not know the exact tag at verify time; use the regex form from ADR-016): `cosign verify-blob --bundle SHA256SUMS.sig.bundle --certificate-identity-regexp '^https://github\.com/diktahq/edikt/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$' --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' SHA256SUMS`.
   - If cosign is not installed, print a clear message with install instructions and exit non-zero unless EDIKT_INSTALL_INSECURE=1 is set — in which case print a loud banner at the end of install.
   - grep SHA256SUMS for the launcher line, verify the downloaded sha matches.
   - Do the same for the payload tarball.

3. bin/edikt install / upgrade:
   - Change RELEASE_TARBALL_BASE to `https://github.com/diktahq/edikt/releases/download`.
   - URL composition: `$RELEASE_TARBALL_BASE/$tag/edikt-payload-$tag.tar.gz`.
   - Mirror install.sh's cosign verification for both SHA256SUMS and the payload.
   - If EDIKT_INSTALL_INSECURE=1 is honored, append a line to the post-install banner: "⚠️ Integrity verification was disabled via EDIKT_INSTALL_INSECURE=1."

4. README.md:
   - Replace the current install command with:
     `curl -fsSL https://github.com/diktahq/edikt/releases/download/v<PINNED>/install.sh | bash`
   - Add a "Verifying the install" subsection with the cosign command users can run.
   - Add a one-line fingerprint note: expected cert identity and issuer.

5. Remove or deprecate any reference to per-file `.sha256` sidecars — they are superseded by ADR-016.

Acceptance criteria:
- Dry-run the release workflow in a test tag (v0.5.0-rc1 via workflow_dispatch) and verify SHA256SUMS.sig.bundle is produced and verifiable locally with cosign.
- install.sh against a fresh machine succeeds end-to-end without EDIKT_INSTALL_INSECURE=1 (using the rc tag).
- `bin/edikt install` without EDIKT_INSTALL_INSECURE succeeds.
- With a tampered SHA256SUMS (manual edit), cosign verify-blob fails and the installer aborts.
- Tap job refuses to run against a malformed tag in a workflow_dispatch test.

When complete, output: RELEASE INTEGRITY READY
```

---

## Phase 6: Evaluator verdict schema + grandfathering (ADR-018)

**Objective:** Ship the structured JSON verdict schema for the evaluator, enforce evidence_type gates in the plan harness, and grandfather in-flight PASS verdicts so existing plans don't regress. Closes HI-7 and eliminates the soft-instruction PASS/BLOCKED confusion.
**Model:** `opus`
**Max Iterations:** 5
**Completion Promise:** `EVALUATOR SCHEMA LIVE`
**Dependencies:** 1

**Prompt:**
```
Read:
- ADR-018 (just written)
- templates/agents/evaluator.md and templates/agents/evaluator-headless.md
- templates/hooks/phase-end-detector.sh (evaluator invocation at line ~273)
- commands/sdlc/plan.md (progress-table update logic)
- docs/product/plans/PLAN-v0.5.0-stability.md (example live plan for grandfather test)

Changes:

1. templates/agents/evaluator-verdict.schema.json — new file:
   JSON Schema draft 2020-12, describing the verdict object from ADR-018. Include required fields, enums, format constraints.

2. templates/agents/evaluator-headless.md — rewrite the output section:
   - Replace prose output instructions with "emit a single JSON object matching evaluator-verdict.schema.json. No preamble, no postscript."
   - List each field, enum, and what counts as each evidence_type.
   - Include an example verdict object.

3. templates/agents/evaluator.md (interactive) — same schema, but a human-readable summary after the JSON block.

4. templates/hooks/phase-end-detector.sh:
   - After claude -p returns, parse the JSON output (Python block).
   - Validate against evaluator-verdict.schema.json using a small Python validator (stdlib `jsonschema` is not available in minimal envs; use a hand-rolled validator or bundle nothing and manually check required fields + enums).
   - Gate: for every criterion whose `id` references a command in the plan's criteria sidecar that contains a shell invocation (pytest, bash, make, npm test), require `evidence_type: "test_run"`. If not met, the overall verdict is forced to BLOCKED with a reason listing the failing criteria.
   - Emit the verdict via JSON emission helper (INV-003).

5. commands/sdlc/plan.md (progress-table update):
   - Only a verdict with `meta.grandfathered == true` OR a verdict passing the evidence gate is allowed to flip a phase to `done`.
   - Verdicts that fail the gate log the reason into docs/product/plans/PLAN-*-criteria.yaml under the phase's notes.

6. Grandfather migration (inside bin/edikt upgrade; implemented here, invoked from Phase 13):
   - On upgrade from < 0.5.0 to 0.5.0, scan all plans under docs/product/plans/ and docs/plans/.
   - For every phase currently marked `done` that does not have a corresponding structured verdict file in docs/product/plans/verdicts/ (new dir), create a stub verdict file with `meta: { grandfathered: true, migrated_from: "<prior-version>" }` and `status: met` for every criterion. Write them in bulk.
   - Emit an upgrade banner: "N in-flight plan phases were grandfathered; new verdicts will use the structured schema."

Acceptance criteria:
- `bash test/unit/evaluator/test_schema.sh` passes — schema validates known-good and rejects known-bad verdicts.
- A fixture plan phase with a test_run criterion and no test evidence -> BLOCKED.
- A fixture plan phase with a test_run criterion and real pytest output -> PASS.
- Running upgrade from a v0.4.3 state against PLAN-v0.5.0-stability.md -> all completed phases produce grandfathered verdict stubs; progress table unchanged.
- An in-flight plan with an incomplete phase -> upgrade leaves it untouched.

When complete, output: EVALUATOR SCHEMA LIVE
```

---

## Phase 7: Default permissions posture (ADR-017)

**Objective:** Ship `templates/settings.json.tmpl` with an explicit, sentinel-wrapped `permissions` block that denies destructive patterns by default and allows edikt's own operational needs. Closes HI-9 and LOW-7.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `PERMISSIONS DEFAULT SET`
**Dependencies:** 1

**Prompt:**
```
Read ADR-017 (just written), templates/settings.json.tmpl, and docs/guides/ (to see if a permissions.md guide exists — if not, create one).

Changes:

1. templates/settings.json.tmpl:
   - Add a new top-level `permissions` block. Do NOT embed a `_edikt` sibling key inside settings.json — Claude Code's settings loader may reject or strip unknown top-level keys. Instead, store the managed-region metadata in a sidecar at `~/.edikt/state/settings-managed.json` with this shape: `{"settings_path": "...", "managed_keys": ["permissions"], "managed_hash": "<sha256 over the canonical JSON of the managed keys>", "sentinel_version": 1, "installed_at": "<iso ts>"}`.
   - The install.sh JSON writer (introduced in Phase 2) is responsible for:
     (a) reading `~/.edikt/state/settings-managed.json` if present;
     (b) computing the current hash of the live settings.json's managed keys;
     (c) if the live hash matches the recorded hash -> managed; safe to overwrite managed keys only.
     (d) if the live hash differs OR the sidecar is missing -> user has customized; prompt before overwriting. User may choose (y)es replace / (n)o skip / (d)iff.
     (e) After a write, recompute and store the new hash.
   - User-added allow/deny entries outside the `permissions` managed key (e.g. in their own top-level additions that Claude Code settings supports) survive untouched because the writer only replaces `permissions`.
   - Populate `permissions.deny` with:
     WebFetch(http://**), Bash(curl http://**), Bash(wget http://**),
     Bash(rm -rf /**), Bash(rm -rf ~/**), Bash(rm -rf $HOME/**),
     Bash(chmod -R 777 **), Bash(sudo **), Bash(sudo:*),
     Bash(:(){ :|:& };:), Bash(* > /dev/tcp/*), Bash(* > /dev/udp/*),
     Bash(git push --force main), Bash(git push --force master),
     Bash(git push --force origin main), Bash(git reset --hard origin/**),
     Bash(dd if=/dev/zero **), Bash(mkfs.**),
     Read(/etc/shadow), Read(**/.ssh/id_*), Read(**/.ssh/known_hosts),
     Read(**/.aws/credentials), Read(**/.docker/config.json)
   - Populate `permissions.allow` with:
     Read(**), Glob, Grep, Edit(**), Write(**),
     Bash(git :*), Bash(gh :*), Bash(npm test), Bash(npm run test:*),
     Bash(pytest :*), Bash(./test/run.sh), Bash(./test/test-e2e.sh),
     Bash(make test), Bash(uv run :*), Bash(ruff :*),
     WebFetch(https://**), WebSearch
   - Set `permissions.defaultMode: "askBeforeAllow"`.

2. install.sh settings.json writer (introduced in Phase 2):
   - Consumes the sidecar described above (no in-JSON metadata).
   - Prompts once per install/upgrade if the live settings.json's managed-key hash doesn't match the sidecar's recorded hash.
   - Never removes user-added keys.

3. docs/guides/permissions.md — new file:
   - Explain the deny-by-default posture.
   - List every default deny pattern with rationale.
   - Show how to add a project-specific allow rule (put it in the separate `userPermissions` block).
   - Show how to verify which rules are active (`cat ~/.claude/settings.json | jq .permissions`).
   - Cross-reference ADR-017.

4. templates/settings.json.tmpl also excludes node_modules from the formatter hook glob (LOW-7): add `!**/node_modules/**` and `!**/.venv/**` to the Write/Edit if-clause at line 37.

Acceptance criteria:
- Fresh install on a clean ~/.claude produces a settings.json that validates against Claude Code's settings schema.
- Existing user settings.json with customizations: install preserves every user key.
- `claude` session denies a `curl http://example.com` command when run against a fresh install.
- Formatter hook does not fire on changes under node_modules/ or .venv/.

When complete, output: PERMISSIONS DEFAULT SET
```

---

## Phase 8: Hermetic benchmark sandboxes (INV-007)

**Objective:** Stop copying the host's `.claude/settings.json` and user-global settings into benchmark sandboxes. Redact result JSONL content and exclude `results/` from git. Closes HI-10, HI-11, LOW-8, LOW-11.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `SANDBOXES HERMETIC`
**Dependencies:** 1

**Prompt:**
```
Read INV-007, test/integration/benchmarks/runner.py (especially build_project and run_case_against_model), benchmarks/conftest.py (JSONL writer), and .gitignore.

Changes:

1. runner.py build_project():
   - Remove the unconditional copy of <repo>/.claude/settings.json (lines 206-210).
   - Write a curated minimal settings.json into the sandbox with no hooks, no user setting_sources, only the minimal allow block needed for the benchmark run (Read, Edit, Write, Bash for test commands).
   - Drop "user" from setting_sources; pass setting_sources=["project"] only.

2. runner.py ADR/INV copy loop:
   - Use shutil.copytree(..., symlinks=True) (do not dereference).
   - Resolve each source path; refuse and log if realpath escapes docs/architecture/.

3. benchmarks/conftest.py JSONL writer:
   - Redact `tool_calls[*].tool_input.content` — replace with `<redacted:len=N>`.
   - Length-cap `response` at 4096 chars with a `...<truncated>` marker.
   - Scan for credential patterns (sk-ant-, Bearer , -----BEGIN ) before writing; abort the test run if any match is found in response or tool_input.

4. .gitignore:
   - Add `test/integration/benchmarks/results/claude-*/` (keep summaries if they are under a different path, else add a carve-out).
   - Remove previously-committed results under the new pattern; add a commit note.

5. Benchmark summary integrity sidecar (LOW-11):
   - After writing results/.../summary.json, compute SHA-256 and write summary.json.sha256.
   - report.py refuses to compare results if the sidecar is missing or mismatched.

Acceptance criteria:
- Run one benchmark case in CI; confirm the sandbox settings.json contains no "hooks" key and no user-level content.
- JSONL outputs show <redacted> for tool_input.content.
- Grep committed results for `sk-ant|Bearer ` -> zero matches.
- A symlink planted under docs/architecture/decisions/ is not dereferenced into the sandbox.
- summary.json.sha256 produced and verified by report.py.

When complete, output: SANDBOXES HERMETIC
```

---

## Phase 9: Stop-hook defang + natural-language trigger hardening

**Objective:** Stop embedding attacker-controllable substrings in Stop-hook suggestions. Make suggestions anchor on the user's explicit request, not matched phrases. Harden subagent identity detection against content-based spoofing. Closes HI-5, MED-11.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `SUGGESTIONS DEFANGED`
**Dependencies:** 1, 2

**Prompt:**
```
Read templates/hooks/stop-hook.sh (lines 38-65, 121-126), templates/hooks/subagent-stop.sh (lines 24-52 agent detection, plus lines 115-150 already rewritten in Phase 2).

Changes:

1. stop-hook.sh:
   - Remove the grep-and-splice pattern that builds suggestions containing matched substrings.
   - Replace with static suggestion text keyed only on the detected intent:
     - "adr" intent -> "If you intended to capture an architectural decision, run /edikt:adr:new."
     - "doc-gap" intent -> "If you introduced a new route, consider /edikt:docs:review."
     - "invariant" intent -> "If you stated a hard rule, run /edikt:invariant:new."
   - No $FIRST_ROUTE, no $MATCHED_PHRASE in systemMessage.
   - Emit via INV-003 JSON emission pattern.

2. subagent-stop.sh agent detection:
   - Replace content-grep with reading the agent identity from the SubagentStop payload's structured fields (hook_input.tool_name, hook_input.subagent_id).
   - If the identity is unknown, classify as `unknown-agent` and do NOT fire any agent-specific gate.

Acceptance criteria:
- Stop-hook on an assistant message containing `POST /admin/delete-everything` and "trade-off" -> suggestion text does NOT contain `/admin/delete-everything`.
- subagent-stop with a payload where the assistant response contains "security specialist" but the structured identity is the docs agent -> security gate does NOT fire.

When complete, output: SUGGESTIONS DEFANGED
```

---

## Phase 10: Attack corpus expansion + NFKC scoring normalization

**Objective:** Add attack templates for the novel vectors surfaced by the audit (evaluator coercion, sentinel escape, agent identity spoofing) and per-directive must_cite variants. Normalize benchmark scoring with NFKC + casefold + whitespace-strip. Closes HI-6, LOW-6, and closes the attack-corpus coverage gap from audit section I1.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `ATTACK CORPUS EXPANDED`
**Dependencies:** 1

**Prompt:**
```
Read templates/attacks/README.md, the existing attack templates (refuse_tool_use.md, refuse_file_pattern.md, must_cite.md, refuse_edit_matching_frontmatter.md), commands/gov/benchmark.md:107 (scoring contract), and the audit Coverage notes section.

Changes:

1. New templates:
   - templates/attacks/evaluator_coercion.md: prompts the evaluator to PASS despite unmet criteria ("we already validated this in a prior session"). Expected behavior: evaluator returns BLOCKED with reason referencing the missing evidence_type.
   - templates/attacks/sentinel_escape.md: attempts to craft an Edit whose old_string is adjacent to a sentinel line, targeting the managed region. Expected behavior: pre-tool-use blocks.
   - templates/attacks/agent_identity_spoof.md: content contains the string "security specialist" planted in a file read by the docs agent. Expected behavior: security gate does NOT fire.
   - templates/attacks/must_cite_auth.md, must_cite_secrets.md, must_cite_schema.md: per-directive must-cite variants beyond the database case.

2. commands/gov/benchmark.md scoring:
   - Update the refusal-and-citation scorer to NFKC-normalize + casefold + whitespace-strip both sides before substring match.
   - Update the file_path scorer to extract extension via os.path.splitext before the extension check, not substring.

3. templates/attacks/README.md — document the new templates and explicitly note Unicode/whitespace variants are handled by the scorer.

Acceptance criteria:
- Run /edikt:gov:benchmark with all new templates; every expected behavior is observed.
- Scoring test: a write to `evil.PY ` (trailing space) is classified as a file-pattern violation.
- Scoring test: a write to `evil.tѕ` (Cyrillic 's') is classified as a file-pattern violation.

When complete, output: ATTACK CORPUS EXPANDED
```

---

## Phase 11: Python test harness hardening

**Objective:** Close the remaining Python-side findings: .env allowlist, session validation, markdown-embedded Python extracted into tools/, snapshot gating, retry class filter. Closes MED-2, MED-3, MED-4, LOW-9, LOW-10.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `PYTHON HARNESS HARDENED`
**Dependencies:** 1

**Prompt:**
```
Read test/integration/conftest.py (especially _load_dotenv at 48-72, _claude_session_exists at 95-110, with_retry at 241-251, snapshot logic at 312-326), test/integration/helpers.py, test/integration/test_compile_orphan_detection.py, test_doctor_source_check.py, test_e2e_v060_release.py (markdown-Python extraction sites).

Changes:

1. _load_dotenv allowlist:
   - Only accept keys matching ^(ANTHROPIC_|CLAUDE_|EDIKT_)[A-Z0-9_]*$.
   - Explicitly reject LD_*, DYLD_*, PATH, PYTHONPATH, PYTHONSTARTUP, PYTHONDONTWRITEBYTECODE, PYTHONHOME with a clear error (not a silent skip).

2. _claude_session_exists:
   - Require sessions/*.json to parse as JSON with at least one of {access_token, refresh_token, expiresAt}.
   - Empty/malformed -> return False.

3. with_retry:
   - Whitelist retryable error classes. Import from claude_agent_sdk.exceptions (OverloadedError, InternalServerError, APITimeoutError).
   - Every other exception re-raises immediately, including authentication errors.

4. Markdown-embedded Python extraction:
   - Move the Python bodies from the grep-and-exec sites into real .py files under `test/_lib/` (new directory — NOT `tools/` because ADR-015 reserves `tools/` for tier-2 user-facing packages). INV-001 is not violated because `test/_lib/` is test-harness scaffolding, not a command or template.
   - Update compile.md, doctor.md to reference `test/_lib/<name>.py` by path.
   - Tests invoke `python <path>` directly.

5. Snapshot auto-generation:
   - Never auto-write expected snapshots when missing. Require --update-snapshots flag or EDIKT_UPDATE_SNAPSHOTS=1.
   - Missing snapshot without flag -> test fails loudly.

Acceptance criteria:
- A .env containing LD_PRELOAD=/tmp/evil.so triggers a loud error, not a silent load.
- A forged empty sessions.json is not accepted as valid auth.
- A 401 from the SDK is not retried (re-raises on first attempt).
- Tests that used to extract Python from markdown now reference tools/ scripts.
- Running the integration suite on a fresh machine does not silently write snapshots.

When complete, output: PYTHON HARNESS HARDENED
```

---

## Phase 12: Mediums & Lows grab-bag

**Objective:** Close the remaining small findings in a single sweep. Closes MED-6, MED-7, MED-8, MED-12, MED-14, LOW-1, LOW-2, LOW-4, LOW-5, LOW-12, LOW-13.
**Model:** `haiku`
**Max Iterations:** 3
**Completion Promise:** `LOWS RESOLVED`
**Dependencies:** 1

**Prompt:**
```
Apply each of these fixes. Each is mechanical; use the audit report as the spec. File references are listed.

0. HI-3 — CLAUDE.md migration TOCTOU (bin/edikt:1075-1126, migrate_m2_claudemd_sentinels):
   - Replace the separated symlink-check + cp + mv sequence with a single Python block that opens the file by FD once (O_NOFOLLOW on Linux/macOS), reads, transforms, writes to a tmp in the same directory, fsyncs, and renames atomically. Refuse if the opened FD's stat reveals a symlink.
   - Add fixture test: plant a symlink between the check and the copy in a test that seeds the file right before migration; confirm migration aborts.

0b. MED-10 — Plan pre-flight evidence-marker gate:
   - Verify that commit a7391a7 (`fix(sdlc:plan): harden pre-flight gate with evidence markers + e2e test`) actually closes MED-10. Read the commit diff; if the gate is now enforced via a hook/test, annotate the audit report closing MED-10 and note the commit SHA. If it's still prose-only, add a Stop / PreToolUse hook that blocks `Write(*PLAN-*.md)` unless the required marker sequence appears in the transcript.

1. MED-6 — Ancestor walk bounded at $HOME:
   - bin/edikt resolve_edikt_root (lines ~42-49): stop walking at $HOME.
   - templates/hooks/worktree-create.sh (lines 38-45): same bound.
   - Both: require the found `.edikt/config.yaml` to be owned by the current user (`[ -O "$D/.edikt/config.yaml" ]`), else skip.

2. MED-7 — Tarball entry checks:
   - bin/edikt tarball_safe (lines 107-125): add a second pass that rejects any entry whose type is `link` or `symlink` (parse `tar -tzvf`).
   - Add a fixture test with a tarball containing one path-traversal entry, one symlink entry, and one regular entry.

3. MED-8 — events.jsonl permissions:
   - Every hook that writes to ~/.edikt/events.jsonl: chmod 0600 immediately after creation. Add a helper in templates/hooks/_lib/perms.sh that each hook sources.

4. MED-12 — worktree realpath:
   - templates/hooks/worktree-create.sh: realpath the worktree path and assert it is under realpath($SOURCE_REPO). Reject otherwise.

5. MED-14 — post-tool-use.sh file validation:
   - templates/hooks/post-tool-use.sh: validate $FILE against [A-Za-z0-9_./-]+ before running any formatter.

6. LOW-1 — bin/edikt launcher pipeline checks:
   - bin/edikt:2199: change `head -1 VERSION | tr -d '[:space:]'` to `MIGRATE_VERSION=$(head -1 VERSION); MIGRATE_VERSION=${MIGRATE_VERSION//[[:space:]]/}`, or add `|| return $?`.
   - Audit other bare pipelines in bin/edikt for missing failure propagation.

7. LOW-2 — uninstall lock hold:
   - bin/edikt cmd_uninstall (lines 2479-2482): hold the flock through the rm -rf, release after.

8. LOW-4 — compile blocks on within-artifact contradiction:
   - commands/gov/compile.md (lines 120-125): change "warn but include" to "block with a clear error" when directives and manual_directives in the same file contradict each other (same predicate, opposite sense).

9. LOW-5 — .gitignore edit notice:
   - commands/gov/compile.md (lines 373-398): print a one-line notice when appending to .gitignore; support `--no-gitignore-edit` flag to skip.

10. LOW-12 — headless-ask.sh strict YAML error:
    - templates/hooks/headless-ask.sh:40: replace bare `except:` with `except yaml.YAMLError as e: print(f"[edikt] ...", file=sys.stderr); sys.exit(2)`.

11. LOW-13 — CI action SHAs:
    - .github/workflows/release.yml, test.yml, docs.yml: replace `@v4`/`@v5` tags with commit SHAs for third-party actions (checkout, setup-python, etc.). Add a renovate/dependabot config for automated SHA bumps.

Acceptance criteria:
- Each fix has at least a smoke test or fixture update.
- Grep for deprecated patterns returns zero matches.

When complete, output: LOWS RESOLVED
```

---

## Phase 13: Upgrade migration + CHANGELOG

**Objective:** Deliver a smooth v0.4.x → v0.5.0 upgrade: grandfather existing PASS verdicts, merge the new `permissions` block into user `settings.json`, refresh compiled governance, write the CHANGELOG and a user-facing upgrade guide.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `MIGRATION SHIPPED`
**Dependencies:** 2,3,4,5,6,7,8,9,10,11,12

**Prompt:**
```
Read all phase completion notes. Implement:

1. bin/edikt upgrade migration step (new function `migrate_m5_v050_security`):
   - Backup ~/.claude/settings.json and the project's CLAUDE.md to ~/.edikt/backup/pre-v0.5.0-<ts>/.
   - Invoke the grandfather routine from Phase 6 (emit stub verdicts for every completed plan phase).
   - Invoke the settings.json permissions-merge routine from Phase 7 (interactive prompt if user settings.json is not edikt-managed).
   - Re-run /edikt:gov:compile to refresh directives (new INVs 003-007 must land in compiled output).
   - Verify every sentinel block has a hash anchor (Phase 4); if not, run a repair.

2. CHANGELOG.md — add v0.5.0 entry:
   - Security-critical fixes (list every CRIT by ID with a one-line human summary).
   - Governance additions (INV-003 through INV-007, ADR-016 through ADR-018).
   - Breaking changes section:
     - Default `permissions` block in settings.json (migration auto-merges).
     - Evaluator now requires test_run evidence; grandfather pass for existing plans.
     - Release asset model changed (install URL pinned to tag).
   - Upgrade instructions with exact commands.

3. docs/guides/upgrade-v0.5.0.md — new file:
   - Pre-flight checklist.
   - Step-by-step commands.
   - What to expect (grandfather banner, permissions prompt).
   - How to roll back (install previous tag from release asset).

4. bin/edikt rollback subcommand — new: `bin/edikt rollback v0.5.0`:
   - Reads the most recent backup under `~/.edikt/backup/pre-v0.5.0-*/`.
   - Restores ~/.claude/settings.json from backup.
   - Restores ~/.edikt/state/settings-managed.json from backup.
   - Restores project CLAUDE.md managed regions from backup.
   - Removes grandfather verdict stubs created during this upgrade (identified by `meta.migrated_from` field).
   - Re-pins the VERSION file to the previous value.
   - Is idempotent and safe to re-run.
   - Prints what it did and what was left untouched.
   - Add a regression test in Phase 14 (test/security/release/) that: runs upgrade; corrupts a user-modified file post-upgrade; runs rollback; asserts state matches pre-upgrade backup.

5. bin/edikt version bump to 0.5.0 in VERSION file — as a separate, very-last commit on the branch. Do NOT cut the tag in this phase.

Acceptance criteria:
- `bin/edikt upgrade` on a v0.4.3 snapshot reaches the post-install banner with all of: grandfather count, permissions-merge notice, compile-refresh notice.
- User settings.json with a pre-existing customization survives the merge.
- The docs/guides/upgrade-v0.5.0.md walks through without dead links.
- `bin/edikt rollback v0.5.0` on a freshly-upgraded install restores every file the upgrade modified.

When complete, output: MIGRATION SHIPPED
```

---

## Phase 14: Security regression test suite

**Objective:** Pin every audit finding with a test so none of them can silently regress. This is the single most important phase for long-term posture.
**Model:** `opus`
**Max Iterations:** 5
**Completion Promise:** `REGRESSION TESTS GREEN`
**Dependencies:** 2,3,4,5,6,7,8,9,10,11,12

**Prompt:**
```
Read docs/reports/security-audit-v0.5.0-2026-04-17.md end-to-end. For every Critical and High finding, write a regression test. For Mediums and Lows, cover at least one representative test per class.

Structure: test/security/ (new directory) with:
  test/security/hooks/            — JSON emission correctness, prompt-injection resistance
  test/security/sentinel/         — byte-range guard, hash anchor
  test/security/release/          — cosign verify, SHA256SUMS integrity, tag validation
  test/security/evaluator/        — schema validation, evidence_type gates
  test/security/permissions/      — default deny block presence and effect
  test/security/sandbox/          — hermetic benchmark setting
  test/security/inputs/           — shape validators
  test/security/README.md         — maps each test to the audit finding ID it defends

Required tests (minimum):

1. For each CRIT (CRIT-1..7): a test that reproduces the attack payload and asserts the hardened code blocks or escapes it.
2. For INV-003: a linter that fails the build if `grep -nE 'echo ["'"'"']\{' templates/hooks/*.sh install.sh` returns any match.
3. For INV-004: a linter that fails if any hook source contains the string pattern `echo '{.*\$\{` within a systemMessage or additionalContext field.
4. For INV-005: the full sentinel overlap matrix (adjacent, inside, outside, cross-boundary, CRLF, Unicode).
5. For INV-006: every validator from Phase 3 has positive + negative + Unicode variant tests.
6. For INV-007: benchmark sandbox settings.json contains no "hooks" key; no "user" setting_source anywhere.
7. For ADR-016: cosign verify-blob succeeds on a valid SHA256SUMS; fails on a tampered one; installer aborts correctly.
8. For ADR-017: default settings.json validates against Claude Code's schema; every deny pattern is syntactically valid.
9. For ADR-018: verdict schema rejects malformed and missing-evidence verdicts; grandfather stubs parse.

Each test must include a comment linking to the audit finding ID so future maintainers understand why the test exists.

Add a `security` job to .github/workflows/test.yml that runs test/security/ on every PR. Mark failures as required for merge.

Acceptance criteria:
- `bash test/run.sh security` passes locally and in CI.
- Intentionally regressing any CRIT fix (e.g. reverting the json.dumps change in one hook) causes the corresponding test to fail.
- Coverage report lists every CRIT/HI audit ID with at least one test.

When complete, output: REGRESSION TESTS GREEN
```

---

## Phase 15: Final audit re-run and release gate

**Objective:** Dogfood edikt's own audit, re-run the governance benchmark, verify every finding is closed, and sign off on the v0.5.0 release.
**Model:** `opus`
**Max Iterations:** 3
**Completion Promise:** `AUDIT CLOSED`
**Dependencies:** 13,14

**Prompt:**
```
1. Run /edikt:sdlc:audit in full. All findings must map to either (a) a fix in Phases 2-12, (b) an intentionally-deferred item with an ADR, or (c) a false positive with a written justification.

2. Run /edikt:gov:benchmark with the expanded attack corpus. Report the PASS rate. Target: ≥98% on Critical/High attack templates; ≥95% overall.

3. Spawn pre-flight specialist review against the final diff summary (staff-security subagent) — any critical or warning must be resolved or explicitly accepted.

4. Write a release sign-off note at docs/reports/v0.5.0-security-signoff-<date>.md linking:
   - The original audit.
   - Each phase's completion note.
   - The regression test suite coverage.
   - The benchmark run results.
   - The cosign certificate identity for the v0.5.0 release.

5. Cut the v0.5.0 tag: run bin/edikt's own release script (or the manual checklist in docs/internal/release-runbook.md if it exists; otherwise produce that runbook as part of this phase). DO NOT PUSH THE TAG without explicit user approval.

Acceptance criteria:
- /edikt:sdlc:audit report shows every CRIT/HI from the original audit closed.
- Benchmark summary sha256 matches; PASS rate meets target.
- Sign-off note is complete and cross-linked.
- Tag candidate v0.5.0 exists locally but is not pushed.

When complete, output: AUDIT CLOSED
```

---

## Known Risks (from pre-flight review 2026-04-17)

**Addressed in-plan** (each was surfaced by the security or architecture pre-flight review and incorporated as a plan change):

- **ADR-014 byte-for-byte collision with Phase 2** — resolved by adding ADR-019 (narrow carve-out naming exactly four hooks; security rewrites may change content and wrapping in one commit for those hooks only).
- **Phase 11 tools/ placement conflicted with ADR-015 tier-2 reservation** — helpers moved to `test/_lib/`.
- **Phase 4 hash-anchor bootstrap lockout** — added explicit "absent → seed, don't validate" rule so upgrading users aren't locked out on first compile.
- **Phase 7 `_edikt` top-level key risked rejection by Claude Code settings loader** — metadata moved to `~/.edikt/state/settings-managed.json` sidecar.
- **Phase 4 / Phase 7 missing dependency on Phase 2** — execution strategy updated (Wave 2a / Wave 2b split).
- **ADR-016 cosign identity used literal tag wildcard** — switched to `--certificate-identity-regexp`; dropped redundant `.pem`.
- **HI-3 (migrate_m2 CLAUDE.md TOCTOU) had no phase owner** — added to Phase 12 as item 0.
- **MED-10 may already be closed by commit a7391a7** — Phase 12 verifies and annotates.
- **Phase 1 missing ADR-009 six-section body mandate** — prompt now spells out all six sections.
- **INV-003 / INV-004 scope overlap ambiguous** — Phase 1 prompt now distinguishes emission format vs channel.
- **Phase 13 had no rollback path** — added `bin/edikt rollback v0.5.0` subcommand and regression test.
- **Phase 6 verdicts/ directory introduced without an ADR** — captured inside ADR-018 with explicit INV-001 scope note.
- **INV-008 extracted from ADR-016** — tag-pinned install URLs is a hard rule, belongs in an invariant.
- **INV-005 widened to cover both in-file and sidecar integrity** — markdown-hosted regions use hash-anchored sentinels; JSON-hosted regions use an out-of-band sidecar. Renamed from "sentinel byte-range guard" to "managed-region integrity" so future contributors find the JSON case too.

**Accepted, not in-plan:**

- **Homebrew tap auto-merge retained with a production-environment approval gate** (Phase 5). Human review is added but merge still auto-runs after approval. If this proves noisy, can be demoted to full manual merge in a later patch release.
- **ADR-014 remains accepted alongside ADR-019.** ADR-019 is a narrow carve-out, not a supersession, because ADR-014's byte-for-byte rule is still correct for mechanical wrapping work. Risk: future contributors may invoke ADR-019 for non-security content changes — mitigated by ADR-019 enumerating the exact four hooks it covers.
- **Grandfather verdicts are trust-on-first-upgrade.** A user on v0.4.3 whose plans already had false-PASS verdicts imports those as grandfathered. Only *new* verdicts use the stricter schema. Acceptable because the alternative (re-run every evaluator on upgrade) is prohibitive; users can optionally rerun specific phases after upgrade.

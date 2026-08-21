---
name: upgrade
description: "Upgrade edikt in this project — launcher version check, then hooks, agents, and rules"
effort: normal
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash
  - AskUserQuestion
---

# edikt:upgrade

Upgrade edikt to the latest version and update this project's hooks, agents, and rule packs. Detects major-version jumps and redirects to the installer when needed.

## Arguments

- `$ARGUMENTS` — Optional. `--offline` skips the remote version check. `--no-review` is not applicable to this command.

## Instructions

### 0. Mark upgrade as in-progress

Create the marker file `.edikt/state/upgrade-in-progress` at the start of orchestration:

```bash
mkdir -p .edikt/state && touch .edikt/state/upgrade-in-progress
```

The stop-hook (`templates/hooks/stop-hook.sh`) short-circuits to `{"continue": true}` while this marker is present. Without the marker, the stop-hook's drift detector and ADR-candidate signal detector fire on every Claude turn during this command's orchestration — by definition the user is mid-fix, and seeing "⚠ Some artifacts have stale sidecars" 30+ times during the resync is noise.

You MUST remove the marker on every exit path — success, user-cancel, error. The simplest discipline: trap-style cleanup. At the end of EVERY exit point in this command (final summary, cancellation prompt, error halt), run:

```bash
rm -f .edikt/state/upgrade-in-progress
```

If you forget, the user's stop-hook stays muted across future sessions. Use a final cleanup step in this command's flow to belt-and-suspenders the removal regardless of the path taken.

### 0z. Check for Updates

If `--offline` is in `$ARGUMENTS`, skip this step entirely and proceed to Step 1.

#### 0y. Resolve the launcher binary

Every step below that invokes the launcher (`edikt version`, `edikt upgrade --yes`) MUST resolve it through this order, most-specific first, and MUST use the resolved path — not a bare `edikt` call — from this point on:

```bash
EDIKT_BIN=""
if [ -x ".edikt/bin/edikt" ]; then
  EDIKT_BIN=".edikt/bin/edikt"                  # the canonical project-mode
                                                 # marker — install.sh itself
                                                 # defines project-mode
                                                 # installed as this exact
                                                 # path existing (install.sh
                                                 # line 225), nothing else.
                                                 # Unambiguous: no content
                                                 # other than edikt's own
                                                 # installer ever creates it,
                                                 # so it is safe to check
                                                 # first for every consumer.
elif [ -x "bin/edikt" ]; then
  EDIKT_BIN="bin/edikt"                         # edikt-dev's own dogfooding
                                                 # convention — an ordinary,
                                                 # ambiguous relative path.
                                                 # Checked SECOND, never
                                                 # first: a downstream Go
                                                 # project that happens to
                                                 # build its own unrelated
                                                 # binary into bin/ named
                                                 # `edikt` would have that
                                                 # binary shadow its real
                                                 # install if this rung won.
elif command -v edikt >/dev/null 2>&1; then
  EDIKT_BIN="edikt"                             # global install on PATH
fi
```

If `EDIKT_BIN` is empty, treat it the same as "launcher missing" in 0c/0d below — do not fall through to a bare `edikt` call, which silently resolves to whatever a stale global install or dev build happens to be on PATH and can report a version, or a feature set, that does not match this project's actual install.

#### 0y-bis. Resolve the template root

The same class of bug as launcher resolution, one layer up: every step below that reads a shipped template (agent files, rule packs, `settings.json.tmpl`, `commands/gov/compile.md`, schemas) MUST resolve the template root through this order — never a hardcoded `~/.edikt/templates/...` path — and MUST use the resolved root from this point on:

```bash
EDIKT_TEMPLATES=""
if [ -d ".edikt/current/templates" ]; then
  EDIKT_TEMPLATES=".edikt/current/templates"        # project-mode: the
                                                       # canonical versioned
                                                       # template payload
                                                       # `install.sh --project`
                                                       # creates. Same
                                                       # `current/` symlink
                                                       # chain the launcher
                                                       # itself resolves
                                                       # through (use.go's
                                                       # repairExternalSymlinks)
                                                       # — not a separate
                                                       # convention.
else
  _edikt_root_g="${EDIKT_ROOT:-${EDIKT_HOME:-$HOME/.edikt}}"
  if [ -d "$_edikt_root_g/current/templates" ]; then
    EDIKT_TEMPLATES="$_edikt_root_g/current/templates"  # global install
  fi
fi
```

**Why this matters, concretely:** a project-mode install with no global install has no `~/.edikt/templates/` at all. Every agent-template and rule-pack comparison that reads the hardcoded global path was silently comparing against a directory that does not exist — reading zero templates, not the wrong templates, with nothing in the output distinguishing that from "everything is already up to date." If `EDIKT_TEMPLATES` is empty, treat it the same as "templates not found" in §1 below — do not fall through to the hardcoded global path.

#### 0a. Read the installed version

```bash
# Prefer the resolved launcher's VERSION output; fall back to project-mode .edikt/VERSION
INSTALLED_VERSION=$(
  "${EDIKT_BIN:-edikt}" version 2>/dev/null \
  || cat "$HOME/.edikt/current/VERSION" 2>/dev/null \
  || cat .edikt/VERSION 2>/dev/null \
  || cat ~/.edikt/VERSION 2>/dev/null \
  | tr -d '[:space:]'
)
```

Strip any leading `v` from INSTALLED_VERSION for comparisons.

#### 0b. Fetch the latest stable release

The launcher subcommand (`edikt upgrade`) uses the GitHub releases API, but for the slash command we also support a direct VERSION check:

```bash
# Primary: GitHub releases API (used by launcher)
LATEST_TAG=$(curl -fsSL --max-time 15 \
  "https://api.github.com/repos/diktahq/edikt/releases/latest" 2>/dev/null \
  | grep '"tag_name"' | head -1 | awk -F'"' '{print $4}')
LATEST_VERSION=$(echo "$LATEST_TAG" | sed 's/^v//')

# Fallback: raw VERSION file (used by legacy upgrade path)
# curl -fsSL --max-time 5 "https://raw.githubusercontent.com/diktahq/edikt/main/VERSION" 2>/dev/null
```

**Fetch failed** (no network, timeout, empty response):
```
⚠ Could not check for updates (network unavailable). Proceeding with installed version.
  To skip this check: /edikt:upgrade --offline
```
Proceed to Step 1 normally.

#### 0c. Launcher presence check

Check whether the resolved launcher (0y) responds:

```bash
LAUNCHER_OK=0
[ -n "$EDIKT_BIN" ] && "$EDIKT_BIN" version >/dev/null 2>&1 && LAUNCHER_OK=1
```

#### 0d. Major-version detection

Parse the major component of both versions (X in X.Y.Z, stripping leading `v`):

```bash
INSTALLED_MAJOR=$(echo "$INSTALLED_VERSION" | awk -F. '{print $1+0}')
LATEST_MAJOR=$(echo "$LATEST_VERSION" | awk -F. '{print $1+0}')
```

**If launcher is missing OR `$LATEST_MAJOR > $INSTALLED_MAJOR`:**

This is a major upgrade. The launcher cannot self-upgrade across major versions — the installer must re-bootstrap the binary.

```
This is a major upgrade (v{INSTALLED_VERSION} → v{LATEST_VERSION}).
Run the installer to complete the upgrade:

  curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash

Then re-run /edikt:upgrade to apply project changes.
```

Stop here — do not proceed to Step 1. Do not mutate any files.

#### 0e. Minor-version bump: delegate to launcher

If `$LATEST_MAJOR == $INSTALLED_MAJOR` AND `$LATEST_VERSION` differs from `$INSTALLED_VERSION`:

```bash
"$EDIKT_BIN" upgrade --yes
```

If the launcher upgrade fails (non-zero exit), report the error and stop. Do not proceed to Step 1 until the launcher upgrade succeeds.

#### 0f. Post-launcher-upgrade summary

After a successful `edikt upgrade --yes`, print:

```
Launcher upgrade complete: v{INSTALLED_VERSION} → v{LATEST_VERSION}
```

Note any migrations that were applied by reading recent entries from `~/.edikt/events.jsonl`:

```bash
tail -20 ~/.edikt/events.jsonl 2>/dev/null | grep '"event":"layout_migrated"' | head -3
```

For each migration event found, print: `  Migration applied: {event details}`

Suggest verification: `Run edikt doctor to verify the installation.`

Then proceed to Step 1 to apply project-level changes.

---

### 1. Check Prerequisites

Read `.edikt/config.yaml`. If not found:
```
No edikt config found. Run /edikt:init to set up this project.
```

Check that `$EDIKT_TEMPLATES` (resolved in §0y-bis) is non-empty and the directory it points to exists. If not:
```
edikt templates not found. Re-install edikt:
  curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
```

Use the Bash tool to read both versions — do NOT infer or guess them:
```bash
cat ~/.edikt/VERSION 2>/dev/null | tr -d '[:space:]'
grep '^edikt_version:' .edikt/config.yaml | awk '{print $2}' | tr -d '"'
```

Use the actual output of these commands as INSTALLED_VERSION and PROJECT_VERSION.

Show at the top of the output:
```
Installed edikt: {INSTALLED_VERSION}
Project edikt:   {PROJECT_VERSION}
```

If INSTALLED_VERSION == PROJECT_VERSION AND there are no changes detected in step 2 AND `edikt_version` is already set in `.edikt/config.yaml`, show:
```
✅ Already up to date (edikt {INSTALLED_VERSION}) — nothing to upgrade.
```
and stop.

If INSTALLED_VERSION != PROJECT_VERSION, always proceed with the upgrade — the version difference alone is reason enough.

If `edikt_version` is missing from `.edikt/config.yaml` (project predates versioning), always proceed — adding the version is itself an upgrade.

### 1.5. Sidecar Migration Check (v0.6.0+ + two-phase model)

v0.6.0 replaces the in-body `[edikt:directives:start]: # ... [edikt:directives:end]: #` sentinel block with a co-located `<artifact>.edikt.yaml` sidecar. Projects upgrading from any earlier version must migrate their sentinel blocks before any other v0.6.0 command will work — `/edikt:gov:compile` refuses to run while legacy in-body sentinels remain.

**Two-phase migration model (v0.6.0+):** the migration is a CLEANUP step, not an extraction step. Phase A is itself two deterministic sub-steps — run in order, both required.

  Phase A, step 1 — `edikt migrate sidecars --apply` (pure Go, no LLM):
    Strips every sentinel block from the parent `.md` and writes a
    skeleton sidecar with `topic: needs-extraction`. The legacy
    sentinel's content (directives + manual_directives +
    suppressed_directives + reminders + verification + topic/signals
    hints) is preserved verbatim into a transient `migration_preserved:`
    field on the skeleton. Fast, deterministic, handles all sentinel
    shapes (v0.4.3 / v0.5.x / pre-v0.4 / hand-edited) identically.

    **This step alone leaves the project with ZERO enforceable
    directives.** The skeleton it writes has `directives: []` —
    verified directly, not assumed: run `--apply` on a fixture and read
    the resulting `.edikt.yaml`, and `directives:` is an empty list
    every time, with the old content sitting only in
    `migration_preserved`, which nothing compiles or enforces. A
    project that stops here — script exits early, a later step fails
    and the run isn't resumed — has silently disabled its own
    governance. This is the single most dangerous gap in this whole
    flow precisely because it produces no error: `migrate sidecars
    --apply` exits 0, prints a success line, and leaves the corpus
    inert. Phase A, step 2 and Phase B are not optional follow-ups;
    treat this command as incomplete until both have run.

  Phase A, step 2 — `edikt migrate to-v2` (pure Go, no LLM):
    Upgrades every skeleton `migrate sidecars --apply` just wrote from
    the retired single-anchor `gov-sidecar.v1` shape to the current
    `gov-sidecar.v2` shape (`source_excerpts[]`, multi-anchor). This
    step is required, not optional: `gov compile`'s Phase B dispatch
    gate refuses outright to run the extractor while any v1-shaped
    sidecar exists in the corpus — confirmed directly, not assumed —
    printing "Run `bin/edikt migrate to-v2` first" and exiting non-zero.
    Skipping this step does not silently degrade anything the way
    skipping step 1's follow-up does; it hard-blocks Phase B with a
    clear message. It is documented here anyway because a user reading
    only "run gov compile next" has no way to know this step exists
    until they hit that refusal.

  Phase B — `/edikt:gov:compile` (dispatches sidecar-extractor):
    The extractor agent runs against every `migration_preserved`-bearing
    sidecar, uses the preserved lists as the canonical baseline (per
    `templates/agents/sidecar-extractor.md` "On migration-preserved
    baselines" rules), and produces the final canonical sidecar.
    A post-extraction `lossless-check` gate (`--on-loss=auto`) verifies
    the extractor didn't drop any preserved directive. `migration_preserved`
    is stripped from the canonical sidecar afterwards — steady-state
    sidecars never carry the transient field.

    **Phase B needs a live model to actually extract anything.** Run as
    `/edikt:gov:compile` inside a Claude Code session (the normal,
    documented path — including a headless `claude -p "/edikt:upgrade"`
    invocation, which still carries a live model turn), the session
    itself dispatches the extractor agent and this just works. Run as
    the raw `bin/edikt gov compile` from a shell or CI job with **no**
    Claude Code session wrapping it at all — a legitimate, separately
    documented headless path (see `edikt migrate sidecars --apply` "for
    headless flows" below) — Phase B needs a `claude` CLI on `PATH`
    for its own extraction dispatch. Confirmed directly, both ways: with
    it absent, compile refuses cleanly (`error: phase A: extractor
    unavailable`, non-zero exit) rather than fabricating content or
    hanging, and the migration_preserved data is left untouched and
    recoverable. With no live model reachable at all — no session, no
    CLI — there is currently no way to complete Phase B; that is a real
    limitation of today's flow, not a bug in this documentation.

After Phase A, this slash command runs Phase B (`/edikt:gov:compile`) as the mandatory next step. The migration is not complete until all three steps have run.

**Not in 0.7.0.** A design exists for retiring the version-specific parts of this flow
entirely in favor of state-based detection (does a sentinel exist? does a sidecar exist?
— no version lookup at all). It is a proposal only
(`docs/internal/plans/PROPOSAL-state-based-sidecar-migration.md`), not implemented, and this
section describes the flow as it actually ships in 0.7.0, not the proposal.

This step runs the migration BEFORE Step 2 (detection) so the rest of the upgrade flow operates on a v0.6.0-shaped project.

**Unconditional scan (Phase 11 — release engineering).** This dry-run fires on every `/edikt:upgrade` invocation regardless of the source version, even when the launcher just bootstrapped a clean install. The scan is harmless on fresh projects (no legacy sentinels → exit 0 with `0 sidecars to create` and one acknowledgement line). Cross-major upgrades (v0.5.x or earlier → v0.6.0+) take this path after the user re-runs `/edikt:upgrade` post-`install.sh`, and the scan guarantees the user is offered migration before any compile runs.

#### 1.5a. Scan for legacy in-body sentinels

Detect any artifact `.md` file under `paths.decisions`, `paths.invariants`, and `paths.guidelines` (resolved from `.edikt/config.yaml`) that still contains an in-body sentinel marker. Use the `edikt` binary's migration tool — it already handles fence detection and the documentation skip-list (blocks inside fenced regions):

```bash
"$EDIKT_BIN" migrate sidecars --dry-run > /tmp/edikt-sidecar-plan.out 2>&1
DRY_EXIT=$?
```

Three outcomes:

- **Exit 0 AND output reports `0 sidecars to create`** — no legacy sentinels found. Continue to the schema-currency check below.
- **Exit 0 AND output reports `N sidecars to create`** with `N > 0` — legacy sentinels found, prompt the user (Step 1.5b).
- **Non-zero exit** — print the captured output to the user and stop. Do NOT proceed to Step 2 until the dry-run is healthy.

Print the dry-run plan verbatim before the prompt so the user can see exactly which artifacts will change.

**Schema currency is a separate question from sentinel-lifting — ask it directly.** A sidecar with no legacy in-body sentinel to lift can still be schema_version 1. "0 sidecars to create" above answers "does anything need a NEW sidecar," never "is every EXISTING sidecar's schema shape current." Check the second question explicitly:

```bash
"$EDIKT_BIN" migrate to-v2 --dry-run 2>&1 | tee /tmp/edikt-schema-plan.out
SCHEMA_STALE=$(grep -oE 'would convert [0-9]+' /tmp/edikt-schema-plan.out | grep -oE '[0-9]+' || echo 0)
```

- **`SCHEMA_STALE` is `0`** — every existing sidecar is already v2-shaped.
- **`SCHEMA_STALE` is `> 0`** — `SCHEMA_STALE` sidecar(s) are still schema_version 1. Include this in the Step 1.5b prompt (or, if no legacy sentinels were found but schema is stale, prompt for the schema upgrade alone: `"$EDIKT_BIN" migrate to-v2`).

Track whether this upgrade run leaves the project fully migrated — set `MIGRATION_COMPLETE=true` only when, by the end of Step 1.5, both the legacy-sentinel count and `SCHEMA_STALE` are `0`. Step 6 reads this flag before writing `edikt_version`.

If both checks report zero, print one line and continue to Step 2:
```
✅ Sidecar migration: nothing to migrate. Schema is current.
```

**The 24-hour dry-run gate.** `edikt migrate sidecars --dry-run` writes a gate file to `.edikt/state/migration-dry-run.json` recording the plan's timestamp and the artifacts inspected. `edikt migrate sidecars --apply` reads that file and **refuses to run if the timestamp is older than 24 hours**, returning:

```text
migrate sidecars: --dry-run required first (or pass --force).
Run: edikt migrate sidecars --dry-run
```

The window exists because sidecar generation is destructive on the prose body (the in-body sentinel block is removed atomically with the sidecar write). The recommended workflow is: run `--dry-run`, review the plan, run `--apply` in the same session. If a CI flow has already validated the plan upstream and you cannot re-run the dry-run, pass `--force`.

`/edikt:upgrade` always runs `--dry-run` immediately before prompting, so the gate is fresh when this command applies the migration. Direct `edikt migrate sidecars --apply` invocations outside the upgrade flow must respect the window.

#### 1.5b. Prompt for migration

Show the plan, then ask:

```
v0.6.0 requires migrating in-body sentinel blocks to co-located *.edikt.yaml
sidecars. The dry-run above shows what will change.

Apply the migration now? [y/N]
```

- **On `y` / `yes`** — run the apply, then the schema upgrade, then complete the resync via the host agent's subagent dispatch:

  ```bash
  "$EDIKT_BIN" migrate sidecars --apply
  "$EDIKT_BIN" migrate to-v2
  ```

   the tier-2 binary is LLM-agnostic. Under the two-phase model (Phase A: structural strip + preserve verbatim into `migration_preserved`, then schema-shape upgrade; Phase B: extractor runs during `/edikt:gov:compile`), apply writes a skeleton sidecar with `topic: needs-extraction` for EVERY artifact regardless of sentinel schema. Schema branching is gone — v0.5.x-full / v0.5.x-partial / v0.4.3-legacy / unknown / hand-edited all take the same path.

  After apply completes:

  1. **Post-migration sentinel verification** runs automatically. The migrate tool's last step scans every migrated `.md` and fails the run with a per-file list if any column-0 `[edikt:directives:start]: #` survived the strip. If you see this error: the user's pre-migration backup at `.edikt/backups/` is intact and recovery is straightforward. Do NOT proceed past this gate.

  2. **Run `edikt migrate to-v2`.** `--apply` writes v1-shaped (single-anchor) skeletons; `gov compile`'s Phase A dispatch gate refuses to run at all while any v1-shaped sidecar exists in the corpus. This step upgrades every skeleton to the current v2 (multi-anchor `source_excerpts[]`) shape. Confirmed directly: running Phase B before this step fails fast with "Run `bin/edikt migrate to-v2` first" and a non-zero exit — not a partial or silent failure, but also not a step to skip.

  3. **Dispatch Phase B by running `/edikt:gov:compile`**. The compile's Phase A picks up every `migration_preserved`-bearing sidecar (`IsStale` returns true unconditionally when the transient field is present), dispatches the `sidecar-extractor` agent per artifact, and the extractor:
     - Reads `migration_preserved.directives` as a canonical baseline → outputs each entry verbatim in `directives[]`, anchored to prose with a real `source_excerpt`
     - Reads `migration_preserved.manual_directives` → outputs in `manual_directives[]` verbatim
     - Same pattern for `suppressed_directives`, `reminders`, `verification`
     - Uses `migration_preserved.topic` and `.signals` as hints; synthesizes from prose if needed
     - MUST NOT include `migration_preserved` in its output sidecar
     The full preservation rules are in `templates/agents/sidecar-extractor.md` § "On migration-preserved baselines". **This step needs a live model** — inside a Claude Code session (including headless `claude -p`), the session dispatches the extractor and this just works; run as the raw `bin/edikt gov compile` with no session at all, it needs a `claude` CLI on `PATH` or it refuses cleanly rather than fabricating content (see the Phase B note above).

  4. **The post-extractor `lossless-check` gate** (`--on-loss=auto`, defaults to `abort` in CI / `accept` in TTY) verifies the extractor's output covers every directive in `migration_preserved.directives`. If anything was dropped (modality/ref-id/noun-phrase mismatch), compile aborts non-zero with a per-sidecar report. Surface to the user; recovery options:
     - Re-run with `edikt gov compile --on-loss=accept` to keep the extractor's output (loss is documented)
     - Edit affected sidecars by hand to restore dropped entries
     - File an extractor regression bug if loss is unexpected

  5. **After compile completes successfully**, `migration_preserved` is stripped from every sidecar. Steady-state sidecars never carry the transient field; subsequent compiles are deterministic Phase B merges unless prose actually drifted.

  - On exit 0 from `migrate sidecars --apply` and `migrate to-v2`: set `MIGRATION_COMPLETE=true` and print:
    ```
    ✅ Sidecar migration applied (Phase A: structural strip + preserve + schema upgrade). Run /edikt:gov:compile to dispatch sidecar-extractor and produce canonical sidecars (Phase B). Until it runs, this project has ZERO enforceable directives.
    ```
    Then run `/edikt:gov:compile`.

    **Then run `/edikt:gov:grade-compile` and surface the result — do not skip this.** Phase B just re-extracted every artifact's sidecar; `gov compile`'s regression check only asks whether the migration destroyed anything (`lost`/`degraded`/`factual` counts), never whether the resulting extraction is any *good*. A grade has never been run yet on a freshly migrated project — `doctor` reports "No compile-quality grade yet" unconditionally until someone runs it, so leaving this step out means an upgraded project ships with a grading tool installed, working, and silently never invoked. Print the summary:
    ```
    📊 Compile-quality grade: {overall}/100 ({dimension}: {score}, ...)
    ```
    This is advisory only — a low grade does not block the upgrade or require action — but it must be visible, not merely available. Continue to Step 2 regardless of the grade.
  - On non-zero exit from `migrate sidecars --apply` or `migrate to-v2` (including the post-migration sentinel verification failure): leave `MIGRATION_COMPLETE=false`, print the migration tool's output verbatim, and stop. The user's pre-migration backup at `.edikt/backups/` is intact. Do NOT proceed to Step 2.
- **On `N` / `no` / empty** — leave `MIGRATION_COMPLETE=false` and print:
  ```
  Migration deferred. Run /edikt:gov:compile to apply when ready.
  Compile will refuse until migration is applied.
  Note: edikt_version will NOT be bumped to the full target version until migration completes — re-run /edikt:upgrade once you're ready.
  ```
  Then continue to Step 2 — the rest of the upgrade still operates safely on the unmigrated project. The user can re-run `/edikt:upgrade` later or invoke `edikt migrate sidecars --dry-run` followed by `edikt migrate sidecars --apply` directly.

**Headless mode.** If `EDIKT_HEADLESS=1` or `--yes` was passed to `/edikt:upgrade` (no future flag exists today, but reserve the convention) AND the dry-run reports a non-empty plan, fall through to deferred — never auto-apply without an interactive confirmation. Migration is destructive on the prose body (sentinel blocks are removed) and must not happen silently.

### 1.6. Post-install sidecar regression check (v0.6.0+)

After migration is applied (or deferred), run a sidecar regression check to surface any quality regressions in the existing sidecars. This step fires on every upgrade to catch regressions introduced between rc versions.

**Requires the launcher resolved in 0y.** If `$EDIKT_BIN` is empty, skip this step with:
```
⚠ Sidecar regression check skipped: no edikt launcher found (checked bin/edikt, .edikt/bin/edikt, PATH).
  Install with: edikt install edikt
```

When the binary is present, run:

```bash
"$EDIKT_BIN" migrate sidecars --dry-run --report-json /tmp/edikt-upgrade-report.json
```

Wait for exit.  Rule 2, display output verbatim; do not parse it.

Then read `/tmp/edikt-upgrade-report.json` and extract the `summary` block's `lost`, `degraded`, and `factual` fields. Print:

```
Sidecar regression summary: {lost} lost, {degraded} degraded, {factual} factual.
```

If any count is non-zero, print:

```
Run: claude /edikt:sidecar:regenerate to fix detected regressions.
```

If all counts are zero, print:

```
Sidecars already current.
```

**MUST NOT auto-run `/edikt:sidecar:regenerate`.** Sidecar regeneration dispatches LLM subagents and must remain a user-initiated action. The check is advisory only.

### 2. Detect What Needs Upgrading

Run all checks in parallel and collect findings.

#### 2a. Hooks check

Read `.claude/settings.json`. Read `$EDIKT_TEMPLATES/settings.json.tmpl`.

For each hook type, check two things: (1) is the content correct, and (2) is it using the modern `.sh` script reference format?

**Pre-flight: unsubstituted `${EDIKT_HOOK_DIR}` placeholder.** If any hook `command` string in `.claude/settings.json` contains the literal substring `${EDIKT_HOOK_DIR}`, the file shipped with the template placeholder un-resolved and every hook fires the error `/bin/sh: /<hook>.sh: No such file or directory` (the shell expands the unset variable to empty). Auto-repair before continuing — substitute `${EDIKT_HOOK_DIR}` with the absolute path resolution rule from §`commands/init.md` (global mode → `$HOME/.edikt/hooks`; project mode → `<project_root>/.edikt/hooks`). Re-validate JSON after substitution. Log: `repaired settings.json: substituted N occurrences of ${EDIKT_HOOK_DIR}`. Do NOT prompt — this is a strictly mechanical fix and silent error in the user's session.

**Pre-flight: missing `statusLine.type` field.** If `.claude/settings.json` has a `statusLine` object that lacks a `type` key, Claude Code refuses to load the entire settings file with the error `statusLine › type: Invalid value. Expected one of: "command"`. Settings written before Claude Code 2.x added the requirement (or shipped from a stale template) hit this. Auto-repair: insert `"type": "command"` as the first key inside the `statusLine` object. Re-validate JSON. Log: `repaired settings.json: added statusLine.type`. Do NOT prompt — same rationale as the placeholder repair above.

**Migration check — inline bash vs. script references:**
If any `type: command` hook has its logic inline (a long bash string) rather than referencing `$HOME/.edikt/hooks/*.sh`, it is outdated regardless of content. Note: "using inline bash — migrate to script reference".

**Content checks — per hook TYPE, whether it exists at all:**
- `SessionStart`: command should reference `$HOME/.edikt/hooks/session-start.sh` — if not → outdated
- `PreToolUse`: must be present with `Write|Edit` matcher — if missing → missing
- `PostToolUse`: must be present with `Write|Edit` matcher — if missing → missing
- `Stop`: must be type:command referencing `$HOME/.edikt/hooks/stop-hook.sh` — if type:prompt or inline → outdated
- `UserPromptSubmit`: must be present — if missing → missing (v4: injects active plan phase)
- `PostCompact`: must be present — if missing → missing (v4: re-injects context after compaction)
- `SubagentStop`: must be present — if missing → missing (v4: logs agent activity + quality gates)
- `InstructionsLoaded`: must be present — if missing → missing (v4: logs rule pack loading)

**Content checks — per hook script BASENAME, even when the type already exists.** Type presence alone does not mean the type is current: a project's `PreToolUse`/`PostToolUse` typically predate a release that added a new script *within* an already-present type, and a type-level check alone reports nothing to do while the new script never installs — this is exactly what happened on a real downstream 0.7.1 upgrade: `verify-gate.sh` and both `inject-directives-{pre,post}.sh` never installed because `PreToolUse`/`PostToolUse` already existed, and `doctor` then pointed back at this same upgrade command as the (non-working) remedy. For every hook type present in `$EDIKT_TEMPLATES/settings.json.tmpl`, collect the set of command basenames the template registers for that type (across every matcher/`if` group within it) and the set actually present in `.claude/settings.json` for that type (same). Any template basename absent from the installed set → outdated (missing entry), even though the type itself is present.

For each outdated or missing hook, note what changed in plain English:
- "SessionStart: inline bash → migrate to `$HOME/.edikt/hooks/session-start.sh`"
- "PostToolUse: missing (auto-formats files after edits)"
- "PreToolUse: missing entry `verify-gate.sh` (write-time completion gate) — type already present, this script was never added"
- "PostToolUse: missing entry `inject-directives-post.sh` (write-time directive delivery) — type already present, this script was never added"
- "UserPromptSubmit: missing (v4 — injects active plan phase into every prompt)"
- "PostCompact: missing (v4 — re-injects plan + invariants after compaction)"
- "SubagentStop: missing (v4 — logs agent activity, quality gates)"
- "InstructionsLoaded: missing (v4 — logs which rule packs load)"
- "Stop: outdated format (may cause JSON validation error) → migrate to `$HOME/.edikt/hooks/stop-hook.sh`"

#### 2b. CLAUDE.md — content-currency check, provenance-first (ADR-067)  edikt-guard:allow

Read `CLAUDE.md`. Detect sentinel format:

```bash
grep -qF '[edikt:start]' CLAUDE.md 2>/dev/null && echo "new"
grep -qF '<!-- edikt:start' CLAUDE.md 2>/dev/null && echo "old"
```

- No edikt block found (`SENTINEL=none`) → skip. `/edikt:init` installs the block; upgrade never creates one from nothing.
- Old HTML comment sentinels found (`SENTINEL=old`) → run **Sentinel Syntax Migration** below first, then continue into the content-currency check on the now-migrated file. A syntax migration says nothing about whether the *content* underneath was ever current — it falls through to the Bootstrap Reconciliation branch below exactly like any other syntax-current, provenance-absent block.
- New visible sentinels found (`SENTINEL=new`) → continue directly into the content-currency check below.

**Why this replaces a syntax-only check.** The prior version of this step tested only which sentinel *syntax* was present — a project migrated to the new syntax at some point in the past reported up to date forever, with no comparison against `$EDIKT_TEMPLATES/CLAUDE.md.tmpl`'s actual content. Confirmed live: this repo's own `CLAUDE.md` carried new syntax with its Intent → Command table 17 rows behind the shipping template, undetected by every upgrade run before this one (ADR-067).  edikt-guard:allow

**Sentinel Syntax Migration** (only when `SENTINEL=old`, runs before anything below):

- Replace `<!-- edikt:start — managed by edikt, do not edit manually -->` → `[edikt:start]: # managed by edikt — do not edit this block manually`
- Replace `<!-- edikt:start -->` (short form, if present) → `[edikt:start]: # managed by edikt — do not edit this block manually`
- Replace `<!-- edikt:end -->` → `[edikt:end]: #`

Leave all content between the sentinels untouched by this step. Report: `"CLAUDE.md — migrated to new sentinel syntax"`.

**The `{{var}}` split (ADR-067) — the only table that needs to change if `CLAUDE.md.tmpl` gains a new slot:**  edikt-guard:allow

```
config-derived — recomputed from .edikt/config.yaml on every run, never carried forward:
  {{project_context_path}} = config.paths['project-context']
  {{plans_path}}           = config.paths['plans']
  {{specs_path}}           = config.paths['specs']

opaque — never regenerated; extracted from the installed block and carried forward verbatim:
  {{project_summary}}
  {{build_command}}
  {{test_command}}
  {{lint_command}}
  {{commit_convention_block}}
```

A `{{var}}` slot appearing in the template but not in either list is a gap in this table, not a silent default — stop and report it rather than guessing which bucket it belongs in.

**`detemplate(installed_block, template)` → `{var: value}` or `FAIL`.** Split `template` on every `{{var}}` occurrence into alternating literal/placeholder segments. Require each literal segment to appear in `installed_block`, in order, verbatim (config-derived vars' surrounding text still counts as literal for this purpose — only the `{{...}}` slots themselves are gaps). If every literal segment matches in order, return the text captured in each gap as that var's current value. If any literal segment cannot be located in order, return `FAIL` — either the user hand-edited structural (non-var) content, or the template's own literal wording changed in a way this operation cannot attribute to a specific var. **This is a whole-document, all-or-nothing match — use it only for the safe-replace-vs-three-way DECISION (Step 5) and the bootstrap already-current check below, never for carrying values forward into a write.** Any single structural drift anywhere in the template — including an ordinary, benign one, like a new Intent → Command table row landing three sections away from `{{project_summary}}` — fails the whole match and should: that is exactly the signal Step 5 exists to catch.

**`detemplate_lenient(installed_block, template)` → `{var: value}` (partial, never fails).** Used ONLY for best-effort carry-forward on an `a` (apply) resolution, in both Bootstrap Reconciliation and the Three-Way Diff Prompt — never for a decision. Extracts each var independently, anchored only by its own immediate surrounding literal text (not the whole document), so a structural change elsewhere cannot cost an unrelated var its carried-forward value. A var whose own immediate anchors aren't found is simply absent from the result; `render` leaves its raw `{{var}}` token in place rather than guessing. Found necessary, not merely nice-to-have: a strict `detemplate` fallback in the bootstrap "apply" case blanked every opaque value — including `project_summary` — over one added Intent → Command table row unrelated to any of them, on the very first bootstrap reconciliation for an otherwise-ordinary, mildly-behind project.

**`render(template, opaque_values)` → text.** Substitute each config-derived var from the CURRENT `.edikt/config.yaml` — never from `opaque_values`, even if a value happens to be present there. Substitute each opaque var from `opaque_values[var]` if present (from either `detemplate` or `detemplate_lenient`, depending on caller), else leave the raw `{{var}}` token in place.

**The migration bypass, and why it's a file, not an env var.** INV-005's guard blocks any Edit/Write whose byte range overlaps CLAUDE.md's managed region unless an allowlisted bypass is present. `EDIKT_MIGRATION_IN_PROGRESS=1` is `bin/edikt upgrade`'s own bypass, set inside its own Go process for its own direct `os.WriteFile` calls — writes that never cross `PreToolUse` at all. This command's writes go through Claude Code's own `Edit` tool instead, a different path: each `Bash` tool call is a fresh subshell, so an `export` in one call is invisible to the separate hook subprocess spawned for a later `Edit` call — an env var set that way is never actually reachable. Bracket every Edit into the managed region with the file-based signal instead, which survives across tool calls because it's real filesystem state:  edikt-guard:allow

```bash
mkdir -p .edikt/state && touch .edikt/state/.migration-in-progress
# ... issue the Edit(s) into CLAUDE.md's managed region ...
rm -f .edikt/state/.migration-in-progress
```

Every "under `EDIKT_MIGRATION_IN_PROGRESS=1`" instruction below means this bracket, not a literal env var export.

**Follow the control flow exactly — do not paraphrase or reorder the branches**, matching §2c's own instruction for the identical reason (this is the same class of algorithm, adapted).

```
# Step 1 — read stored provenance (out-of-band, keyed by project root)
record = read_json("~/.edikt/state/claude-md-managed.json").get(project_root)

# Step 2 — bootstrap: no record for this project
if record is None:
  emit_event "upgrade_claude_md_path" { path: "bootstrap_no_record" }
  run Bootstrap Reconciliation (below)
  continue

# Step 3 — current template hash (raw bytes, pre-substitution — same rule as §2c)
current_template_hash = md5_raw($EDIKT_TEMPLATES/CLAUDE.md.tmpl)

# Step 4 — fast preserve
if record.template_hash == current_template_hash:
  emit_event "upgrade_claude_md_path" { path: "fast_preserve" }
  report "   ✓ CLAUDE.md — managed block unchanged (preserved)"
  continue

# Step 5 — extract against the STORED template
stored_template = reconstruct_template_at(record.template_hash)
  # Same lookup order as §2c Step 5: ~/.edikt/versions/<template_version>/templates/
  # CLAUDE.md.tmpl, else ~/.edikt/cache/template-by-hash/{hash}.md, else None.
  # No git-show fallback — same reasoning as §2c: the edikt source repo is not
  # guaranteed present after a normal user install.

if stored_template is None:
  run Bootstrap Reconciliation (below), noting "stored template unrecoverable"
  continue

opaque_values = detemplate(installed_block, stored_template)

if opaque_values is FAIL:
  emit_event "upgrade_claude_md_path" { path: "threeway_prompt" }
  run Three-Way Diff Prompt (below)
  continue

# Step 6 — safe replace: structural content was never hand-edited
new_block = render(current_template, opaque_values)
Edit CLAUDE.md, replacing only the byte range between [edikt:start] and [edikt:end]
  inclusive, with EDIKT_MIGRATION_IN_PROGRESS=1 set (the INV-005 bypass bin/edikt upgrade already uses).  edikt-guard:allow
write_record(project_root, template_hash=current_template_hash,
             template_version=CURRENT_EDIKT_VERSION, managed_hash=sha256(new_block))
emit_event "upgrade_claude_md_path" { path: "resynth_safe_replace" }
report "   ⬆ CLAUDE.md — managed block updated (no user edits detected)"
```

**Bootstrap Reconciliation** (no record, or a record whose stored template is unrecoverable). There is nothing to diff the installed block against except the one template that exists right now:

```
opaque_values = detemplate(installed_block, current_template)

if opaque_values is not FAIL AND render(current_template, opaque_values) == installed_block:
  # Already byte-identical to what the current template produces with its own
  # extracted opaque values — genuinely current, just never recorded. Seed silently.
  write_record(project_root, template_hash=current_template_hash,
               template_version=CURRENT_EDIKT_VERSION, managed_hash=sha256(installed_block))
  emit_event "upgrade_claude_md_path" { path: "bootstrap_already_current" }
  report "   ✓ CLAUDE.md — managed block already current (provenance seeded)"
  return
```

Otherwise, ask via `AskUserQuestion`, showing installed vs. `render(current_template, detemplate_lenient(installed_block, current_template))` — the lenient extractor, not the strict `opaque_values` computed above (which may be `FAIL`):

```
CLAUDE.md's managed block has no recorded provenance (first run under this check, or
an upgrade from before it existed) — currency can't be proven either way.

  [a] Apply current template — overwrite with the current template (carries forward
                                project summary / build / test / lint / commit-
                                convention text where extraction could find it)
  [k] Keep as-is              — record current content as the new baseline; ask again
                                only if the template changes from here
  [s] Skip                    — decide later; ask again next upgrade

Choice [a/k/s]:
```

| Choice | Action |
| ------ | ------ |
| `a` | Edit CLAUDE.md's block to `render(current_template, detemplate_lenient(installed_block, current_template))`, under `EDIKT_MIGRATION_IN_PROGRESS=1`. Write the record. |
| `k` | Do not modify CLAUDE.md. Write the record as a deliberate acceptance of the installed content as the new baseline (`template_hash = current_template_hash`, `managed_hash = sha256(installed_block)`) — a future upgrade compares against this baseline going forward, not against a re-derived "should have matched" value. |
| `s` | No file change, no record written. Re-asks next upgrade. |

Emit `upgrade_claude_md_bootstrap_resolved { resolution: "a" | "k" | "s" }`.

**Three-Way Diff Prompt** (mirrors §2c's, scoped to the CLAUDE.md block's byte range):

```bash
STORED=$(mktemp)    ; printf '%s' "$stored_render"    > "$STORED"
RESYNTH=$(mktemp)   ; printf '%s' "$current_render"    > "$RESYNTH"
INSTALLED=$(mktemp) ; printf '%s' "$installed_block"   > "$INSTALLED"

if command -v diff3 >/dev/null 2>&1; then
  diff3 -L "old template" -L "your edits" -L "new template" "$STORED" "$INSTALLED" "$RESYNTH"
else
  echo "── old template ──"          ; cat "$STORED"
  echo "── your edits (installed) ──"; cat "$INSTALLED"
  echo "── new template ──"          ; cat "$RESYNTH"
fi
rm -f "$STORED" "$RESYNTH" "$INSTALLED"
```

(`$stored_render` / `$current_render` are best-effort: `render` with whatever `detemplate_lenient` extracted against the stored/current template respectively — this prompt is a diff for a human to read, not a value that gets written.)

```
CLAUDE.md's managed block — template moved AND its structural content doesn't match
what the stored template would have produced (an Intent → Command table row edited,
reordered, or other non-{{var}} text changed).

  [a] Apply new template   — overwrites with the current template; only the opaque
                              values are carried forward, best-effort
  [k] Keep current          — keep the file as-is; miss this template update
  [m] Merge interactively   — open $EDITOR with conflict markers
  [s] Skip                  — decide later; ask again next upgrade

Choice [a/k/m/s]:
```

| Choice | Action | `template_hash` written |
| ------ | ------ | ------------------------ |
| `a` | Edit CLAUDE.md's block to `render(current_template, detemplate_lenient(installed_block, current_template))`, under `EDIKT_MIGRATION_IN_PROGRESS=1`. | `current_template_hash` |
| `k` | Do not modify. | left at the OLD stored value — explicitly not advanced, so a future upgrade asks again rather than treating "kept edits" as "now current" |
| `m` | Write `CLAUDE.md.merge` with `diff3 -m` conflict markers over just the block. Do not touch `CLAUDE.md` or the record until the merge completes and the user re-runs `/edikt:upgrade`. | unchanged |
| `s` | No file change. | unchanged, no record write |

Emit `upgrade_claude_md_conflict_resolved { resolution: "a" | "k" | "m" | "s" }`.

#### 2c. Agent check — provenance-first

List files in `.claude/agents/`. For each, check if a matching template exists in `$EDIKT_TEMPLATES/agents/`.

**Skip customized agents first.** An agent is customized (and therefore skipped before any hash logic runs) if:
1. It contains `<!-- edikt:custom -->` anywhere in the file, OR
2. It is listed in `.edikt/config.yaml` under `agents.custom`

```yaml
# .edikt/config.yaml
agents:
  custom:
    - dba
    - my-team-reviewer
```

For every remaining agent with a matching template, execute the provenance-first flow below. **Follow the control flow exactly — do not paraphrase or reorder the branches.**

```
for each installed agent at .claude/agents/{slug}.md:
  # Step 1 — read provenance frontmatter
  stored_hash = yaml_frontmatter(installed).get("edikt_template_hash")

  # Step 2 — legacy fallback (pre-v0.6.0 install, no provenance)
  if stored_hash is absent:
    emit_event "upgrade_agent_path" { agent: slug, path: "legacy_classifier_entered" }
    run Legacy Classifier Fallback (see below) — do NOT simplify
    continue

  # Step 3 — compute current template hash (md5 of raw template bytes,
  # matching the init hashing rule — BEFORE substitution and BEFORE
  # stack filtering).
  current_template_hash = md5_raw($EDIKT_TEMPLATES/agents/{slug}.md)

  # Step 4 — fast preserve
  if stored_hash == current_template_hash:
    emit_event "upgrade_agent_path" { agent: slug, path: "fast_preserve" }
    emit_event "upgrade_agent_preserved" {
      agent: slug,
      hash: stored_hash,
      reason: "template unchanged"
    }
    # Template has not moved. Any on-disk difference is a user edit.
    # Do not touch the file.
    report "   ✓ {slug}.md — template unchanged (preserved)"
    continue

  # Step 5 — re-synthesize what init WOULD have produced from the stored
  # template plus the current project config. If the installed file is
  # byte-identical to that re-synthesis, the user never touched it and
  # the new template is safe to apply.
  stored_template = reconstruct_template_at(stored_hash)
  #   Lookup order:
  #     1. ~/.edikt/versions/<edikt_template_version>/templates/agents/{slug}.md
  #        (preferred — written at install time by the versioned layout)
  #     2. cached copy under ~/.edikt/cache/template-by-hash/{stored_hash}.md
  #   If neither is available, fall through to Step 7 (threeway_prompt) using
  #   the current template for both "stored" and "current" columns.
  #
  #   Note: a `git show` fallback is NOT supported. The edikt source repo is
  #   not guaranteed to be present after a normal user install, so any fallback
  #   relying on it would silently fail for most users.

  resynth = apply_stack_filter(
             apply_substitutions(stored_template, config.paths),
             config.stack)

  installed_body = read_file_without_provenance_frontmatter(installed)
  resynth_body   = resynth   # (no provenance frontmatter written yet)

  if installed_body == resynth_body:
    # Step 6 — safe replace. User never edited; template moved forward.
    new_content = apply_stack_filter(
                    apply_substitutions(current_template, config.paths),
                    config.stack)
    write_with_provenance(installed,
      content = new_content,
      edikt_template_hash    = current_template_hash,
      edikt_template_version = CURRENT_EDIKT_VERSION)

    emit_event "upgrade_agent_path" { agent: slug, path: "resynth_safe_replace" }
    emit_event "upgrade_agent_replaced" {
      agent: slug,
      hash_old: stored_hash,
      hash_new: current_template_hash,
      user_accepted: false         # auto-applied, no prompt needed
    }
    report "   ⬆ {slug}.md — template updated (no user edits detected)"
    continue

  # Step 7 — 3-way prompt. User edited AND template moved. Show the user
  # all three sides and let them choose.
  emit_event "upgrade_agent_path" { agent: slug, path: "threeway_prompt" }

  run Three-Way Diff Prompt (see below)
  # Prompt records resolution and performs the selected action.
```

**Three-Way Diff Prompt.**

Write the three bodies to temp files and invoke `diff3` to show a merged view. On systems without `diff3`, print each section separately under clearly labelled headers.

```bash
STORED=$(mktemp) ; echo "$stored_template_resynth"  > "$STORED"
RESYNTH=$(mktemp); echo "$current_template_resynth" > "$RESYNTH"
INSTALLED=$(mktemp); cat .claude/agents/{slug}.md   > "$INSTALLED"

if command -v diff3 >/dev/null 2>&1; then
  diff3 -L "old template" -L "your edits" -L "new template" \
    "$STORED" "$INSTALLED" "$RESYNTH"
else
  echo "── old template (stored_hash=$stored_hash) ──" ; cat "$STORED"
  echo "── your edits (installed) ──"                  ; cat "$INSTALLED"
  echo "── new template (current) ──"                  ; cat "$RESYNTH"
fi

rm -f "$STORED" "$RESYNTH" "$INSTALLED"
```

Then prompt:

```
{slug}.md — template moved AND you have local edits.

  [a] Apply new template   — overwrites your edits with the new template
  [k] Keep current         — keep your file; miss this template update
  [m] Merge interactively  — open $EDITOR with conflict markers
  [s] Skip this agent      — decide later; ask again next upgrade

Choice [a/k/m/s]:
```

Record the resolution with `upgrade_agent_conflict_resolved` and perform the action:

| Choice | Action                                                                                                                                                                                               | `upgrade_agent_replaced.user_accepted` |
| ------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------- |
| `a`    | Write `apply_stack_filter(apply_substitutions(current_template))` to the installed path, update `edikt_template_hash` + `edikt_template_version`. Emit `upgrade_agent_replaced` with `user_accepted: true`. | `true`                                 |
| `k`    | Do not modify the file. Emit `upgrade_agent_preserved` with `reason: "user kept edits on moved template"`.                                                                                           | —                                      |
| `m`    | Write a merge file at `.claude/agents/{slug}.md.merge` with `diff3 -m` conflict markers, open `$EDITOR`, and after the editor exits require the user to re-run `/edikt:upgrade`. Do not update frontmatter or the installed file until merge completes cleanly. Emit `upgrade_agent_merge_requested { agent, merge_file, hash_old, hash_new }`. | —                                      |
| `s`    | Exclude from this run. No file change. No frontmatter update.                                                                                                                                        | —                                      |

In every case emit:

```
upgrade_agent_conflict_resolved { agent: slug, resolution: "a" | "k" | "m" | "s" }
```

**Legacy Classifier Fallback (pre-v0.6.0 agents only — the v0.4.3 classification heuristic, preserved).**

The core heuristic below (additions/deletions via line-set comparison, the PURE EXPANSION / PATH SUBSTITUTION / USER DIVERGENCE buckets) is retained verbatim from v0.4.3, commit `d81f6e3`, and is exercised byte-for-byte by `test_upgrade_legacy_agent_uses_classifier`. Do NOT simplify or refactor the heuristic itself. Two things around it are fixed, both real defects found on a downstream upgrade, neither a change to the heuristic's own decision rule: **what gets fed into it** (the template side must be stack-filtered, same as the installed side already is — see the stack-filter step below) and **one guard added before it finalizes USER DIVERGENCE** (the prior-template-match check below). Preserving "exact behavior" means preserving the classification outcome for correctly-fed input, not perpetuating a feed that was wrong.

For each agent that lacks `edikt_template_hash` in its frontmatter and is NOT customized, compare content hashes — NOT modification times:
```bash
# Materialize a stack-filtered copy of the template BEFORE hashing/diffing.
# The raw template on disk still carries every <!-- edikt:stack:... -->
# block; a project's installed file only carries the ones matching its
# configured stack (install already filters before writing it). Comparing
# the raw template against a correctly-filtered install produces a
# constant, spurious divergence on every upgrade for every stack-gated
# agent (backend.md, frontend.md, qa.md each carry several such blocks) —
# this is the same apply_stack_filter(config.stack) the provenance-first
# flow's Step 5/6/7 already runs before every one of its comparisons.
FILTERED_TEMPLATE=$(mktemp)
apply_stack_filter "$EDIKT_TEMPLATES/agents/{slug}.md" config.stack > "$FILTERED_TEMPLATE"

template_hash=$(md5 -q "$FILTERED_TEMPLATE" 2>/dev/null || md5sum "$FILTERED_TEMPLATE" 2>/dev/null | awk '{print $1}')
installed_hash=$(md5 -q .claude/agents/{slug}.md 2>/dev/null || md5sum .claude/agents/{slug}.md 2>/dev/null | awk '{print $1}')
```

- If customized → skip (note as "custom — skipped")
- If hashes differ → **compute the diff** and classify (see below)
- If hashes match → up to date, remove `$FILTERED_TEMPLATE`

**Classify the diff (for each divergent agent):**

Run `diff -u .claude/agents/{slug}.md "$FILTERED_TEMPLATE"` — installed FIRST, filtered template SECOND — and count:
- **Additions** (lines starting with `+`): content in the template but NOT in the installed file. These are template expansions (new sections, new bullets, new formatters).
- **Deletions** (lines starting with `-`): content in the installed file but NOT in the template. These are either user customizations or content that was removed from the template.
- **Path substitutions**: lines where only a file path differs (e.g., `docs/architecture/decisions/` → `adr/`). Detect by checking if the removed line matches a default path from `$EDIKT_TEMPLATES/.edikt/config.yaml` AND the added line matches the user's configured path in `.edikt/config.yaml`.

Classify into three buckets:
- **PURE EXPANSION**: only additions, no deletions (except trivial whitespace). Safe to apply — the template added content. Write `$FILTERED_TEMPLATE`'s content to the installed path.
- **PATH SUBSTITUTION**: deletions match the user's configured paths. Safe to apply if we re-substitute paths after upgrade. For now: flag as USER DIVERGENCE.
- **USER DIVERGENCE** *(before finalizing this bucket, run the prior-template-match check below — it can reclassify to PURE EXPANSION)*: deletions exist that aren't just path substitutions. The installed file has content the template doesn't — likely user customization. Require explicit confirmation with diff preview.

**Prior-template-match check (runs only when the diff above would otherwise land in USER DIVERGENCE).** The heuristic above cannot tell "this is an older template version, never edited" from "this is a real project customization" — both look identical to it as raw deletions. Before prompting the user to adjudicate a conflict that may not exist: for each version directory under `~/.edikt/versions/*/templates/agents/{slug}.md` (skip the current version, already checked above), materialize `apply_stack_filter(that_version's_template, config.stack)` and compare byte-for-byte against the installed file. If ANY prior version matches, the installed file is an un-edited old template, not a real edit — reclassify as PURE EXPANSION (write `$FILTERED_TEMPLATE`'s content, same as the auto-apply case) instead of prompting. If NO prior version matches (including when no prior versions remain on disk at all — retention is not guaranteed, see `edikt prune`), keep the safe, conservative USER DIVERGENCE default: absence of a match means "we don't know," never "therefore it was edited."

When a legacy classifier run ends with a decision (auto-apply, keep, or skip), emit `upgrade_agent_replaced` / `upgrade_agent_preserved` with the matching fields plus `reason: "legacy_classifier"` (or `reason: "legacy_classifier_prior_template_match"` for a reclassification by the check above, so downstream events.jsonl analysis can distinguish the two) so analysis can distinguish provenance-path resolutions from legacy-path resolutions.

Do NOT touch agents that have no matching template (user-created agents) or that are marked as custom.

**Detect new agents.** List files in `$EDIKT_TEMPLATES/agents/`. For each template, check if a matching file exists in `.claude/agents/`. If a template has no installed counterpart, it's a new agent added in this version.

New agents are classified as **core** or **optional**:

- **Core agents** are installed automatically — they're required for edikt's governance mechanisms to work. The evaluator agents (`evaluator.md`, `evaluator-headless.md`) are core: the plan harness and quality gates depend on them. Note the two are not equivalent artifacts: `evaluator.md` carries frontmatter and is Task-dispatched via `subagent_type: evaluator` (registered in `templates/agents/_registry.yaml`'s `always:` list); `evaluator-headless.md` has no frontmatter and is never Task-dispatched — it is a raw prompt body that hook scripts pipe into `claude -p` directly, so it is correctly absent from `_registry.yaml`. "Core" here means "installed unconditionally," not "registered as a subagent."
- **Optional agents** (all other specialist agents) are offered to the user with a description of what they do. The user chooses which to install.

```
New agents in v{version}:

  Installed automatically (core):
  ✓ evaluator-headless.md — headless phase-end evaluator (required by plan harness)

  Available (choose which to add):
  [1] gtm.md — go-to-market strategy review
  [2] mobile.md — mobile platform specialist
  [a] Install all    [s] Skip all    [1,2] Install selected
```

Core agents that the user doesn't want can be disabled after install:
- Delete `.claude/agents/{slug}.md` to remove
- Add to `agents.custom` in `.edikt/config.yaml` to skip on future upgrades

If the user declines an optional agent, add it to `agents.custom` in config so future upgrades don't ask again.

After installing or updating any agent, output: `Note: Agents register at session start — restart Claude Code to dispatch them natively. Until then, commands fall back automatically.`

#### 2d. Config check

Read `.edikt/config.yaml`. Check for missing keys that were added in newer versions:

- `artifacts:` block missing → outdated (added in v0.1.1)
- `artifacts.database.default_type` missing → outdated
- `artifacts.fixtures.format` missing → outdated

Note each missing key with a description:
- "`artifacts:` block missing — enables database-type-aware spec-artifacts"

Do NOT flag keys that exist but have unexpected values — those may be intentional user customizations.

#### 2e-bis. Project templates check (v0.3.0+)

Check whether the project has the three per-artifact project templates that v0.3.0 requires for new artifact creation via the lookup chain (see commands/<artifact>/new.md Section 1a for the lookup contract).

```bash
HAS_ADR_TEMPLATE=$([ -f .edikt/templates/adr.md ] && echo "yes" || echo "no")
HAS_INVARIANT_TEMPLATE=$([ -f .edikt/templates/invariant.md ] && echo "yes" || echo "no")
HAS_GUIDELINE_TEMPLATE=$([ -f .edikt/templates/guideline.md ] && echo "yes" || echo "no")
```

**Classify the project:**

- **All three present** → mark project templates as "up to date, skip". Report nothing in the upgrade summary.
- **At least one missing AND `edikt_version >= 0.3.0`** → mark as "templates partially configured — /edikt:init will complete setup". Note in the summary:
  ```
  ⬆  Project templates — {n}/3 missing
     Missing: {list of missing templates}
     Fix: run /edikt:init --reset-templates to complete setup
  ```
- **All three missing AND `edikt_version < 0.3.0`** (v0.2.x legacy upgrading to v0.3.0) → this is the **grandfather flow**. Note in the summary:
  ```
  ⬆  Project templates — v0.3.0 introduces per-artifact project templates
     This project is on v{old_version} — templates have never been configured.
     v0.3.0+ requires explicit templates for /edikt:adr:new, /edikt:invariant:new,
     and /edikt:guideline:new. edikt doesn't ship a default — your project owns it.

     After upgrade: run /edikt:init to set up project templates interactively.
     You'll pick Adapt (generate from existing artifacts), Start fresh (pick a
     reference template), or Write my own for each artifact type.
  ```
- **All three missing AND `edikt_version >= 0.3.0`** (broken state — v0.3.0+ project with no templates) → note:
  ```
  ⬆  Project templates — broken state detected
     Project is on v{version} but no templates are configured.
     Fix: run /edikt:init --reset-templates immediately.
     Until then, /edikt:<artifact>:new commands will refuse.
  ```

**Never overwrite existing templates.** If `.edikt/templates/adr.md`, `.edikt/templates/invariant.md`, or `.edikt/templates/guideline.md` already exist, `/edikt:upgrade` must NOT touch them. They are user-owned content committed to the project's git. The only way to regenerate them is `/edikt:init --reset-templates`, which the user invokes explicitly.

**Never auto-run init.** Even when templates are missing, `/edikt:upgrade` does NOT invoke `/edikt:init` on the user's behalf. It reports the state in the summary and leaves the user in control. Init is interactive; upgrade should not drag the user through another interactive flow without consent.

**Legacy in-body sentinels** (v0.6.0+): the dedicated sidecar-migration step at §1.5 already detects v0.2.x / v0.4.3 / v0.5.x in-body sentinel blocks across all governance directories. There is no separate three-list-schema migration in v0.6.0 — all legacy shapes flow through the single Phase A structural strip + Phase B canonical extraction defined. Skip any additional schema classification here.

#### 2e. Rule packs check

If `.claude/rules/` does not exist or contains no `.md` files → mark rule packs as "nothing installed, skip" (not outdated).

Otherwise, same logic as `/edikt:rules-update`:
- Compare `version:` frontmatter in installed vs template
- Only flag as outdated if installed version < template version
- Skip files without `<!-- edikt:generated -->` marker (manually edited)
- Skip files not in the registry (custom rules)
- **Hash comparison:** For files with `edikt:generated` marker, compute content hash and compare against the template. If hashes differ (content was edited but marker kept), flag as modified:
  ```
  ⚠ .claude/rules/go.md has edikt:generated marker but content differs from template.

    [1] Overwrite — replace with latest template
    [2] Keep mine — remove the marker, edikt won't touch this file again
    [3] Show diff — see what changed before deciding
  ```
  If user picks [2], remove the `<!-- edikt:generated -->` marker and report:
  ```
  ✅ .claude/rules/go.md is now yours. edikt will never overwrite it again.
  ```

### 3. Show Upgrade Summary

Show what will change in this project before touching anything:

```
EDIKT UPGRADE
─────────────────────────────────────────────────────
Hooks (.claude/settings.json)
  ⬆  SessionStart   — inline bash → script reference
  ⬆  PostToolUse    — missing, will add auto-format hook
  ⬆  Stop           — fix "Prompt hook condition was not met" error (ok:false → ok:true always)
  ⬆  PreToolUse     — missing entry verify-gate.sh (type already present, script was never added)

Agents (.claude/agents/)
  ⬆  dba.md   — template added 12 lines (pure expansion, safe to apply)
  ⚠  security.md  — installed file has 8 lines not in template (USER DIVERGENCE — preview diff before accepting)
  +  evaluator-headless.md — new in v0.4.0
  ✓  architect.md  — up to date

Rule packs (.claude/rules/)
  ⬆  go.md          1.0 → 1.2
  ⬆  code-quality.md 1.0 → 1.1
  ✓  testing.md      — up to date
  —  my-custom.md    — custom, skipped
  —  security.md     — manually edited, skipped

Config (.edikt/config.yaml)
  ⬆  artifacts: block missing — enables database-type-aware spec-artifacts

CLAUDE.md
  ⬆  old HTML sentinels → visible markers (Claude Code v2.1.72+ hides HTML comments)

─────────────────────────────────────────────────────
4 hook changes, 2 agents, 2 rule packs, 1 config addition, 1 CLAUDE.md migration
```

If no rule packs are installed (`.claude/rules/` is missing or empty), show:
```
Rule packs (.claude/rules/)
  —  no rule packs installed
```
Do NOT show any `⬆` icon for rules in this case.

If everything is already up to date:
```
✅ Already up to date — nothing to upgrade.
```

### 4. Confirm

**If any agent has USER DIVERGENCE**, prompt for each diverged agent individually BEFORE the main confirmation:

```
⚠  security.md has content not in the template.
   Showing diff (installed vs template):

   [diff output — deletions shown as - lines, additions as +]

   Your options:
     [1] Apply template — REPLACES your customizations (you'll lose the - lines)
     [2] Keep mine     — add `<!-- edikt:custom -->` marker so upgrade skips this forever
     [3] Skip          — don't change this file now, ask again next upgrade

   Choice [1/2/3]:
```

If user picks [2], add the `<!-- edikt:custom -->` marker at the top of the file (after frontmatter if any) and report:
```
✓ security.md is now yours. Upgrade will skip it from now on.
```

If user picks [3], exclude it from the agent upgrade list.

Then ask the main confirmation:
```
Apply these upgrades? (y/n/select)
  y      — apply all
  n      — cancel
  select — choose which sections to apply (hooks / agents / rules / config / claude.md)
```

**Agents classified as PURE EXPANSION** can be auto-applied with `y` without individual confirmation — they're provably safe (no deletions).

Wait for response. If `select`, ask separately for each section.

If cancelled:
```
Upgrade cancelled — no changes made.
```

### 5. Apply Upgrades

#### Hooks

**Do this merge via the `Bash` tool (e.g. a `python3` read-modify-write), not the `Edit` or `Write` tool.** `.claude/settings.json` is JSON-hosted, and INV-005's JSON-region verification variant is not yet implemented — the `PreToolUse` guard (`templates/hooks/pre-tool-use.sh`) unconditionally denies any `Edit`/`Write` to it, since it cannot yet distinguish a safe edit from one that empties the `deny` list or drops a hook registration. That deny only intercepts the `Edit`/`Write` tools — it has no visibility into a write issued through `Bash` — so performing the merge below as a `Bash`-invoked script is the sanctioned path, not a workaround around a bug. See the guard's own comment (`pre-tool-use.sh`, the `settings.json` branch) for the full reasoning.  edikt-guard:allow

Read the current `.claude/settings.json`. Read the template.

Merge at the individual command-BASENAME level, never by replacing a whole hook type's array — a whole-array replace silently discards user-added entries under that type, and a type-level "already present, skip" check silently leaves out any template basename the installed array doesn't yet have (§2a above). Two passes: add what §2a found missing, then remove what a release retired (never a user's own addition):

```python
# Pseudocode
import os.path

settings = read_json('.claude/settings.json')
template_text = read_file('$EDIKT_TEMPLATES/settings.json.tmpl')

# CRITICAL: substitute ${EDIKT_HOOK_DIR} BEFORE parsing as JSON.
# Claude Code does not expand env vars in `command:` strings — unsubstituted
# placeholders cause /bin/sh: /<hook>.sh: No such file or directory.
hook_dir = f"{HOME}/.edikt/hooks"   # global mode (or {project}/.edikt/hooks for project mode)
template_text = template_text.replace("${EDIKT_HOOK_DIR}", hook_dir)
template_hooks = json.loads(template_text)['hooks']

def basename(command):
    return os.path.basename(command)

# ── Pass 1: add every template basename missing from the installed set,
# for EVERY hook type the template declares — not a hardcoded subset, so a
# future template addition never goes stale here the way an enumerated list
# would. Grouped by (matcher, if) selector, so an unconditional group's
# commands never land inside a filtered one or vice versa.
for hook_type, template_groups in template_hooks.items():
    installed_groups = settings.setdefault('hooks', {}).setdefault(hook_type, [])
    installed_basenames = {
        basename(h['command'])
        for group in installed_groups
        for h in group.get('hooks', [])
    }
    for template_group in template_groups:
        selector = (template_group.get('matcher'), template_group.get('if'))
        missing = [h for h in template_group.get('hooks', [])
                   if basename(h['command']) not in installed_basenames]
        if not missing:
            continue
        target = next((g for g in installed_groups
                        if (g.get('matcher'), g.get('if')) == selector), None)
        if target is None:
            target = {k: v for k, v in template_group.items() if k != 'hooks'}
            target['hooks'] = []
            installed_groups.append(target)
        target['hooks'].extend(missing)
        for h in missing:
            report(f"{hook_type}: added {basename(h['command'])} (new in this template)")

# ── Pass 2: remove a RETIRED edikt hook — one this release's template no
# longer ships anywhere under its type AND whose script file no longer
# exists on disk. Ownership is decided by directory, not by "was it in the
# template last time": a command whose directory resolves under hook_dir is
# edikt's own; anything else (a real user-added hook, wherever it points)
# is NEVER touched by this pass, template membership or not.
template_basenames_by_type = {
    hook_type: {basename(h['command']) for g in groups for h in g.get('hooks', [])}
    for hook_type, groups in template_hooks.items()
}
for hook_type, groups in list(settings.get('hooks', {}).items()):
    for group in groups:
        kept = []
        for h in group.get('hooks', []):
            cmd = h['command']
            owned_by_edikt = os.path.dirname(cmd) == hook_dir
            still_shipped = basename(cmd) in template_basenames_by_type.get(hook_type, set())
            file_exists = os.path.exists(cmd)
            if owned_by_edikt and not still_shipped and not file_exists:
                report(f"{hook_type}: removed {basename(cmd)} (retired from the template, file no longer exists)")
                continue
            kept.append(h)
        group['hooks'] = kept
    settings['hooks'][hook_type] = [g for g in groups if g.get('hooks')]

# Sanity check before writing — block any leftover placeholders.
serialized = json.dumps(settings)
assert "${EDIKT_HOOK_DIR}" not in serialized, "settings.json still contains unsubstituted placeholders"
write_json('.claude/settings.json', settings)
```

**Never remove** a hook command whose directory does not resolve under `hook_dir` (the user may have added their own, wherever it points) — Pass 2 above is the one narrow, ownership-checked exception: an edikt-owned command (`hook_dir`-resolved) whose basename this release no longer ships anywhere, and whose file no longer exists, is retired, not user-added, and gets removed with the removal reported. This upgrade never overwrites user-added hooks — everything else it only updates or adds.

#### Agents

For each outdated agent:
1. Read the installed file
2. Read the template
3. Replace the installed file with the template content

For each new **core** agent (evaluator, evaluator-headless):
1. Copy the template to `.claude/agents/{slug}.md`
2. Report: `✓ Installed evaluator-headless.md — core (required by plan harness)`

`evaluator-headless.md` is copied as a plain file, not registered as a Task subagent — it has no frontmatter and is dispatched only by hook scripts piping it into `claude -p`. Its absence from `templates/agents/_registry.yaml`'s `always:` list is correct, not a gap.

For each new **optional** agent the user accepted:
1. Copy the template to `.claude/agents/{slug}.md`
2. Report: `✓ Installed gtm.md — go-to-market strategy review`

For each new optional agent the user declined:
1. Do NOT install
2. Add slug to `agents.custom` in `.edikt/config.yaml` so future upgrades don't ask again
3. Report: `— Skipped gtm.md (added to agents.custom)`

Skip agents without a matching template. Skip user-created agents (no matching template slug).

#### CLAUDE.md

Already fully handled in §2b (ADR-067) — sentinel syntax migration, content-currency  edikt-guard:allow
detection, and the resulting write (fast-preserve / resynth-safe-replace / bootstrap
reconciliation / three-way prompt) all run inline during detection, the same way §2c's
agent provenance flow does. Nothing further happens here.

#### Config

For each missing config key, append the block to `.edikt/config.yaml`. Preserve all existing content — only add what's missing.

If `artifacts:` block is missing, append:

```yaml

artifacts:
  database:
    # Default database type for artifact generation.
    # spec-artifacts checks spec frontmatter first, then this value, then keyword-scans the spec.
    # Set by edikt:init from code signals. Change only if detection was wrong.
    # Values: sql | document | key-value | mixed | auto
    # auto = detect from spec each time (greenfield or genuinely undecided)
    default_type: auto

  fixtures:
    # Fixture format. yaml is portable — transform to your stack at implementation time.
    # Values: yaml | json | sql
    format: yaml
```

Note: the `sql.migrations.tool` sub-key is only written by `/edikt:init` when a SQL database is detected. Do not add it during upgrade — `auto` is the correct default for unknown stacks.

#### Rule packs

Same as `/edikt:rules-update` logic — replace outdated packs, skip manually edited and custom ones.

#### Compile schema check

Check if the project's generated governance is stale vs the current compile schema.

1. Read the constant `COMPILE_SCHEMA_VERSION` from `$EDIKT_TEMPLATES/commands/gov/compile.md` (or `commands/gov/compile.md` in the installed templates).
2. Read `.claude/rules/governance.md` (if it exists) and extract `compile_schema_version` from its YAML frontmatter.
3. Compare:
   - **Missing field** (legacy v0.1.x output): note `governance.md uses legacy version stamp — run /edikt:gov:compile to regenerate with schema v{N}`
   - **Lower than current**: note `governance.md compiled with schema v{old} (current: v{new}) — run /edikt:gov:compile to regenerate`
   - **Equal**: no note
   - **Higher than current**: note `governance.md compiled with schema v{n}, but this edikt only supports v{current}. Upgrade edikt globally first.`

Do NOT auto-run `/edikt:gov:compile`. Surface the recommendation in the upgrade summary and let the user decide. Compile is potentially expensive and may have contradictions that need review.

**Important**: Never enforce `compiled_by` or `compiled_at` equality with the current edikt version. Those fields are informational HTML comments only — they tell humans when/who produced the file, but do not drive any decision.

#### Project templates (v0.6.0+)

**Never overwrite project templates under `.edikt/templates/`.** This is a hard contract:

- Existing `.edikt/templates/adr.md`, `.edikt/templates/invariant.md`, or `.edikt/templates/guideline.md` MUST NOT be touched by `/edikt:upgrade`. They are user-owned, team-shared, committed to git. The only way to modify them is direct user edit or `/edikt:init --reset-templates` (which the user invokes explicitly).
- If any of the three exists, skip it in this step. Do NOT even verify its contents — trust the user.

**When templates are missing**, the behavior depends on the grandfather state detected in Step 2e-bis:

- **Grandfather flow** (`edikt_version < 0.3.0` → upgrading to v0.3.0+): print a clear migration notice:
  ```
  📋 v0.3.0 introduces per-artifact project templates

  v0.3.0+ requires .edikt/templates/adr.md, .edikt/templates/invariant.md,
  and .edikt/templates/guideline.md for /edikt:<artifact>:new to work.
  Your project is being upgraded from v{old} and doesn't have them yet.

  This upgrade does NOT create templates automatically. Templates are a
  choice your team makes: adapt from existing artifacts, pick a reference,
  or write your own.

  Next step after upgrade:
    /edikt:init    (interactive — pick Adapt / Start fresh / Write my own
                    for each artifact type)

  Until you run init, /edikt:<artifact>:new will continue to use the
  legacy inline fallback template with a one-time warning per invocation
  (v0.2.x behavior preserved). You can migrate at your own pace.
  ```

- **Broken state** (`edikt_version >= 0.3.0` but templates missing — shouldn't happen in normal workflows but may occur if the user deleted `.edikt/templates/*`): print an error-grade notice:
  ```
  ⚠ Project templates are missing but project is on v{version}

  edikt v0.3.0+ requires project templates for new artifact creation.
  This is a broken state — until you fix it, /edikt:<artifact>:new
  commands will HARD REFUSE with an error message.

  Fix: run /edikt:init --reset-templates immediately.
  ```

- **Partially configured** (`edikt_version >= 0.3.0`, some templates present, some missing): print:
  ```
  ⚠ Some project templates are missing

  Missing: {list of missing templates}
  Fix: run /edikt:init --reset-templates to complete the setup.

  The existing templates ({list of existing}) are preserved — init
  only regenerates the missing ones.
  ```

**Bump `edikt_version` in config** after the upgrade completes successfully, so subsequent `<artifact>:new` invocations can distinguish "recently upgraded, templates not yet set up" from "legacy project on v0.2.x". Do this as the final step of the upgrade, not before — if the upgrade fails midway, the version stays at the old value so the user can retry.

**Do not run `/edikt:init` automatically.** Always leave the user in control. Upgrade reports the state; init is the next action the user takes when they're ready.

#### Config key migration: `paths.soul` → `paths.project-context` (v0.4.0)

Check if `.edikt/config.yaml` contains `soul:` under `paths:`. If found, rename the key to `project-context:` — the value stays the same.

```
ℹ Config migration: paths.soul → paths.project-context

The config key `paths.soul` has been renamed to `paths.project-context`
in v0.4.0. Your config has been updated automatically.

Old: soul: {value}
New: project-context: {value}
```

This is a safe auto-migration — the key name changes, the value and behavior are identical. Commands that read this config check for both `project-context` and `soul` (fallback) so older configs continue to work even without the migration.

#### Directive sentinel schema migration (removed in v0.6.0)

The v0.2.x → v0.3.0 three-list schema migration block previously lived here. Under v0.6.0+, all legacy sentinel shapes — v0.2.x / v0.3.x / v0.4.3 / v0.5.x — are handled by the single two-phase migration orchestrated in Step 1.5 above (`edikt migrate sidecars --apply` + `/edikt:gov:compile`). No separate per-schema migration runs in this section. If the project still carries in-body sentinels at this point, Step 1.5 either applied or deferred them; this step is a no-op.

#### Command reference migration (v0.1.x → v0.2.x)

v0.2.0 renamed 15 flat commands into namespaces. Projects initialized with v0.1.x have hardcoded references to the old flat names in `CLAUDE.md` (the intent table inside the edikt-managed block) and in compiled rule packs. These references still resolve today via deprecated stubs, but they'll break in v0.4.0 when the stubs are removed.

Apply this migration table to the following targets:

| Old (v0.1.x)           | New (v0.2.x)             |
|------------------------|--------------------------|
| `/edikt:adr`           | `/edikt:adr:new`         |
| `/edikt:invariant`     | `/edikt:invariant:new`   |
| `/edikt:compile`       | `/edikt:gov:compile`     |
| `/edikt:review-governance` | `/edikt:gov:review`  |
| `/edikt:rules-update`  | `/edikt:gov:rules-update`|
| `/edikt:sync`          | `/edikt:gov:sync`        |
| `/edikt:prd`           | `/edikt:sdlc:prd`        |
| `/edikt:spec`          | `/edikt:sdlc:spec`       |
| `/edikt:spec-artifacts`| `/edikt:sdlc:artifacts`  |
| `/edikt:plan`          | `/edikt:sdlc:plan`       |
| `/edikt:review`        | `/edikt:sdlc:code-review`     |
| `/edikt:drift`         | `/edikt:sdlc:drift`      |
| `/edikt:audit`         | `/edikt:sdlc:audit`      |
| `/edikt:docs`          | `/edikt:docs:review`     |
| `/edikt:intake`        | `/edikt:docs:intake`     |

**Targets** (only files edikt owns — never touch user content):

1. **CLAUDE.md managed block** — the content strictly between `[edikt:start]: #` and `[edikt:end]: #` sentinels (or the old HTML sentinels if they weren't migrated yet). Leave everything outside the sentinels untouched.
2. **Generated rule packs** — any file under `.claude/rules/` or `.claude/rules/governance/` that contains the `edikt:generated` or `edikt:compiled` marker. Skip files without the marker (those are user-written).

**Safety rules:**

- **Idempotency is critical.** Do NOT replace `/edikt:adr` if it's already followed by `:` (e.g. `/edikt:adr:new` or `/edikt:adr:compile`). Use string contexts that make the match unambiguous: backtick-wrapped (`` `/edikt:adr` ``), end of line (`/edikt:adr\n`), or punctuation-delimited (`/edikt:adr,`, `/edikt:adr.`, `/edikt:adr)`).
- **Longest first is WRONG here.** The old commands have no overlap with each other, but they DO have overlap with the new names (`/edikt:adr` is a prefix of `/edikt:adr:new`). Always match the old pattern with a non-`:` terminator.
- **Use Edit with literal strings**, not Write. Preserve line endings, trailing whitespace, and all other content. For each replacement, include enough context (at least the backtick/paren/whitespace around the token) to avoid ambiguity.
- **Skip if already migrated.** Before making any edit, grep the file for any of the NEW command names in the table. If even one new name is present (e.g. `/edikt:adr:new` exists in the file), the file was already migrated on a previous upgrade — still run the full mapping pass to catch any stragglers, but don't report it as "migrated" unless actual changes were made.

**Process for each target file:**

1. Read the file.
2. Determine the edit scope (for CLAUDE.md: the managed block only; for rule packs: the whole file if it has the `edikt:generated` marker).
3. For each row in the mapping table, search the edit scope for old-name occurrences that are NOT followed by `:`. Track the count.
4. If any matches were found, apply them via Edit with full surrounding context for disambiguation.
5. Record: filename + count of replacements.

Report the results as part of the upgrade summary:

```
Command references:
  CLAUDE.md:                               7 replacements
  .claude/rules/governance.md:             3 replacements
  .claude/rules/governance/api-design.md:  0 (already current)
```

If a project has no v0.1.x references anywhere, report `Command references: ✓ up to date` instead of the per-file breakdown.

### 6. Post-Upgrade

After applying:

1. Update `edikt_version` in `.edikt/config.yaml` to the installed version — even if no other changes were applied — **but only when `MIGRATION_COMPLETE` (set in Step 1.5) is `true`**:
   - If a `edikt_version:` line exists, replace it
   - If it doesn't exist (project predates versioning), add it as the first non-comment line after any leading `#` comment block at the top of the file

   **If `MIGRATION_COMPLETE` is `false`** — sidecars still carry legacy in-body sentinels, are still schema_version 1, or the migration commands failed — do NOT write the new version. Writing it here is exactly the defect this gate exists to prevent: `edikt_version` is the only signal a future `/edikt:upgrade` uses to decide whether migration is still needed, so bumping it prematurely permanently hides the pending work — the next run reports "current" and never re-offers migration, and per the v0.7.0 changelog a schema_version 1 sidecar sitting beside any v2 sidecar makes the compile pipeline refuse to dispatch at all. Instead, leave `edikt_version` at its prior value and print:
   ```
   ⚠ edikt_version NOT updated to v{LATEST_VERSION} — sidecar migration is incomplete.
     Config still reads v{INSTALLED_VERSION}. Re-run /edikt:upgrade once migration
     is applied, or run it directly: edikt migrate sidecars --apply && edikt migrate to-v2
   ```
   `edikt doctor` MUST report a config/corpus schema disagreement (`edikt_version` implies a schema line the corpus doesn't have) as a loud warning — this is the state this gate is designed to make unreachable, but `doctor` still checks for it as a backstop against manual edits or an interrupted upgrade run.

2. **Drop stale project-level `.edikt/VERSION`** (v0.3-era artefact). Older projects (initialised under v0.2/v0.3) carry a `.edikt/VERSION` file in the project root. Step 0a's `INSTALLED_VERSION` resolution falls back to this file when the launcher VERSION isn't readable, which means a stale value left after upgrade silently anchors subsequent runs. v0.6.0+ canonical version sources are the launcher's `~/.edikt/current/VERSION` plus `.edikt/config.yaml`'s `edikt_version:` (just bumped in step 1). Remove the legacy file:

   ```bash
   if [ -f .edikt/VERSION ]; then
     rm -f .edikt/VERSION
     echo "Removed stale .edikt/VERSION (v0.3-era; superseded by .edikt/config.yaml edikt_version)"
   fi
   ```

   This is state-only-safe — `.edikt/VERSION` is a state file, not an ADR or invariant artefact. Idempotent: re-running upgrade after the cleanup is a no-op.

3. Check if linter configs exist and linter rules are outdated (template mtime > linter rule mtime):
   ```
   Linter configs found. Run /edikt:sync to regenerate linter rules.
   ```

4. **Check project templates (v0.6.0+)**: if any of `.edikt/templates/adr.md`, `.edikt/templates/invariant.md`, or `.edikt/templates/guideline.md` is missing, surface a prominent next-step prompt in the post-upgrade output:
   ```
   📋 Project templates — /edikt:init required

   v0.3.0+ requires per-artifact project templates. This project
   doesn't have all three yet:
     Missing: {list of missing templates}

   Next step: /edikt:init (interactive setup)
     You'll pick Adapt, Start fresh, or Write my own for each
     artifact type. Existing artifacts are not touched.

   Until templates are set up, /edikt:<artifact>:new will:
     - Use the legacy inline fallback + warning (for v0.2.x-era projects
       whose edikt_version is still < 0.3.0)
     - HARD REFUSE with an error pointing at /edikt:init (for projects
       whose edikt_version is now >= 0.3.0 after this upgrade)
   ```
   This notice fires AFTER the `edikt_version` bump in step 1, so the user sees it with the correct version context.

   **This is advisory only.** Do not auto-run `/edikt:init`. Leave the user in control.

5. **Report linked worktrees, do not touch them.** A linked git worktree (`.claude/worktrees/<name>/` or any other `git worktree` checkout of this repo) carries its own independent `.edikt/` state — its own sidecars, its own compiled governance, its own `edikt_version`. This upgrade only ever touches the current working tree. Detect worktrees and report what's found, without traversing into them:

   ```bash
   git worktree list --porcelain 2>/dev/null | awk '/^worktree /{print $2}' | tail -n +2
   ```

   For each linked worktree path found, check whether it has `.edikt/config.yaml` and read its `edikt_version` (same `readPinnedVersion`-style read as Step 0a, applied to `<worktree>/.edikt/config.yaml`). If present and older than the version just installed here, report:
   ```
   ⚠ N linked worktree(s) found with older governance, not upgraded by this run:
     .claude/worktrees/comments-ux-landing — edikt_version {old}, this checkout is now {new}
   Run /edikt:upgrade from inside each worktree to bring it current. Working in a
   stale worktree with no staleness signal is easy to miss — this is that signal.
   ```

   Do not traverse into a worktree, do not run any migration or compile step there, and do not refuse to complete this upgrade because linked worktrees exist. Worktrees are a routine part of this project's workflow, not an edge case — silently ignoring them (no report) previously left a worktree's governance arbitrarily stale with nothing telling the person working there. Reporting and leaving the decision to the user, rather than reaching into a checkout this command was not invoked from, matches how `edikt doctor` handles the analogous dual-hook-registration case: detect, offer, never act unprompted outside the scope you were invoked in.

6. Output results:

If only `edikt_version` was added (everything else was already current):
```
UPGRADE COMPLETE
─────────────────────────────────────────────────────
Version:     {old or "unset"} → {new}
Hooks:       ✓ up to date
Agents:      ✓ up to date
Rule packs:  ✓ up to date

Commit to record the version:
  git add .edikt/config.yaml && git commit -m "chore: set edikt_version to {new}"

Run /edikt:doctor to verify governance health.

{If docs/architecture/assumptions.md exists:}
💡 Model capabilities may have changed. Review docs/architecture/assumptions.md
   to re-test harness assumptions.

WHAT'S NEW in {new}
─────────────────────────────────────────────────────
{content of the most recent changelog section from ~/.edikt/CHANGELOG.md}
─────────────────────────────────────────────────────

Next: Run /edikt:doctor to verify governance health.
```

If changes were applied:
```
UPGRADE COMPLETE
─────────────────────────────────────────────────────
Version:     {old} → {new}
Hooks:       4 updated
Agents:      2 updated, {N} preserved (still stamped edikt_template_version {old or earlier} — unchanged since last write, not upgraded)
Rule packs:  2 updated (1 skipped — manually edited)
Config:      1 addition (artifacts: block)
CLAUDE.md:   sentinels migrated to visible format

Commit these changes to share the upgrade with your team:
  git add .claude/ .edikt/config.yaml && git commit -m "chore: upgrade edikt to {new}"

Run /edikt:doctor to verify governance health.

{If docs/architecture/assumptions.md exists:}
💡 Model capabilities may have changed. Review docs/architecture/assumptions.md
   to re-test harness assumptions.

WHAT'S NEW in {new}
─────────────────────────────────────────────────────
{content of the most recent changelog section from ~/.edikt/CHANGELOG.md}
─────────────────────────────────────────────────────

Next: Run /edikt:doctor to verify governance health.
```

**`edikt_template_version` is write-provenance, not a currency claim.** It records when a file was last generated by this upgrade flow — the fast-preserve path (Step 2c, "Template has not moved. Any on-disk difference is a user edit. Do not touch the file.") intentionally does not write it, so a preserved agent keeps whatever version stamped its last real write. This is deliberate and fails safe: stamping the new version onto unreviewed old-era content would make that agent invisible to a future template-moved comparison. Never summarize an upgrade run as "all agents stamped {new}" — say how many were updated and how many were preserved, as above; a preserved count with an unchanged stamp is the correct, honest result, not a partial failure.

### 7. Offer re-extraction (optional, after everything above completes)

**This is an offer, not a dispatch.** `/edikt:gov:reextract` (DESIGN-QUESTIONS-2026-08-16.md
Q3) stays exactly what its own frontmatter says: manual, opt-in, never
invoked automatically by this or any other flow. What changes here is that
this command now *tells the user it exists*, with real numbers, instead of
requiring them to already know its name — closing the discoverability gap
that otherwise means a release's extraction-quality improvement reaches
almost nobody. Nothing is dispatched in this step without an explicit yes.

Run after Step 6's completion output, only if the upgrade itself succeeded
(don't offer a follow-on when the run above failed or is incomplete):

```bash
bin/edikt gov reextract --status --json
```

If the binary is absent, or the status check fails, or `eligible` is `0`,
say nothing further — this is a silent skip, not a warning; an empty offer
would be noise on every project that's already current.

If `eligible > 0`, print, using `--status`'s real numbers (not a guess):

```
📋 N artifact(s) on an earlier extraction contract than what's installed.
   Re-extracting refreshes their sidecar content through the current
   contract — separate from the schema/version work above, and entirely
   optional. Estimated cost: roughly N * ~4 minutes (EXP-006 median),
   less in parallel. Nothing lands without your explicit review per
   artifact.
```

Ask via `AskUserQuestion`: "Run /edikt:gov:reextract now?" (yes / no /
remind me later — all three are terminal for this step, none block
completion of the upgrade already reported above).

- **yes** — invoke `/edikt:gov:reextract` with no arguments (full corpus).
  That command owns its own dispatch-and-review flow from here; this step
  ends once it's invoked.
- **no** or **remind me later** — end the upgrade run here. Do not repeat
  the offer within the same session; the user can run
  `/edikt:gov:reextract` directly whenever they want, and the next
  `/edikt:upgrade` invocation will offer again if artifacts are still
  eligible then.

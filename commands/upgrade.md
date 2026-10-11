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
stops:
  - id: managed-block-no-provenance
    class: risk
    default: skip
    on_silence: safe-side
    prompt: "The managed block in CLAUDE.md has no recorded provenance — apply the current template, keep it as-is, or skip?"
  - id: managed-block-template-moved
    class: risk
    default: skip
    on_silence: safe-side
    prompt: "The managed block's template moved and its content diverged — apply, keep, merge, or skip?"
  - id: agent-template-conflict
    class: risk
    default: skip
    on_silence: safe-side
    prompt: "This agent's template moved and you have local edits — apply, keep, merge, or skip?"
  - id: agent-user-divergence
    class: risk
    default: skip
    on_silence: safe-side
    prompt: "This agent has content not in the template — apply the template, keep yours, or skip?"
  - id: apply-upgrades
    class: risk
    default: "no"
    on_silence: safe-side
    prompt: "Apply these upgrades?"
  - id: repair-user-scope-settings
    class: risk
    default: "no"
    on_silence: safe-side
    prompt: "The user-scope settings file has an unsubstituted hook dir or a statusLine without type — repair it in place?"
  - id: remove-user-scope-duplicates
    class: risk
    default: "no"
    on_silence: safe-side
    prompt: "Remove the duplicate user-scope hook registrations from the user's settings file?"
  - id: offer-reextract
    class: selection
    default: "no"
    options: ["yes", "no", "remind-me-later"]
    on_silence: take-default
    prompt: "Artifacts are on an earlier extraction contract — run the re-extraction now?"
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
# There is deliberately no relative-path rung here. A project has no local
# bin/ holding the launcher — install and upgrade put it on PATH — and a
# relative rung would also let an unrelated binary of the same name in a
# downstream project's build directory shadow the real install.
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
# The resolved launcher's VERSION output, falling back to the active payload's VERSION file
INSTALLED_VERSION=$(
  "${EDIKT_BIN:-edikt}" version 2>/dev/null \
  || cat "$HOME/.edikt/current/VERSION" 2>/dev/null \
  | tr -d '[:space:]'
)
```

Strip any leading `v` from INSTALLED_VERSION for comparisons.

#### 0b. Fetch the latest stable release

The launcher subcommand (`edikt upgrade`) and this slash command both discover the
newest release through the GitHub releases API. Discovery returns a **tag name**;
every URL that then downloads or executes anything is built from that pinned tag,
never from a branch ref.

```bash
# Primary: GitHub releases API (used by launcher)
LATEST_TAG=$(curl -fsSL --max-time 15 \
  "https://api.github.com/repos/diktahq/edikt/releases/latest" 2>/dev/null \
  | grep '"tag_name"' | head -1 | awk -F'"' '{print $4}')
LATEST_VERSION=$(echo "$LATEST_TAG" | sed 's/^v//')
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

#### 0d. Self-upgrade decision — channel guard first, then major-version detection

An installed version is either a release (`X.Y.Z`, optional leading `v`) or a
channel name such as `dev` (a dev-linked checkout, D36). Only a release is
comparable to the latest release. Before this guard existed, `dev` went through
`awk -F. '{print $1+0}'`, became major 0, and every 0.x release then took the
self-upgrade branch below — `edikt upgrade --yes` would have replaced the
operator's dev install with a release build (D41 (a); bok escaped it only by
running `--offline`).

```bash
# edikt-test-block: version-channel-guard
# Inputs: INSTALLED_VERSION, LATEST_VERSION, LAUNCHER_OK. Output: SELF_UPGRADE.
SELF_UPGRADE=none   # none | skip | major | minor
if ! printf '%s' "$INSTALLED_VERSION" | grep -qE '^v?[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "installed version is a channel (${INSTALLED_VERSION}); self-upgrade skipped"
  SELF_UPGRADE=skip
elif [ -z "$LATEST_VERSION" ]; then
  echo "self-upgrade check skipped: latest version unknown (the release fetch in 0b failed or was skipped)" >&2
  SELF_UPGRADE=none
else
  INSTALLED_REL="${INSTALLED_VERSION#v}"; LATEST_REL="${LATEST_VERSION#v}"
  INSTALLED_MAJOR=$(printf '%s' "$INSTALLED_REL" | awk -F. '{print $1+0}')
  LATEST_MAJOR=$(printf '%s' "$LATEST_REL" | awk -F. '{print $1+0}')
  if [ "${LAUNCHER_OK:-0}" != 1 ] || [ "$LATEST_MAJOR" -gt "$INSTALLED_MAJOR" ]; then
    SELF_UPGRADE=major
  elif [ "$LATEST_MAJOR" -eq "$INSTALLED_MAJOR" ]; then
    # Same major: compare ORDER, not inequality — a downgrade (LATEST < INSTALLED,
    # e.g. a local snapshot ahead of the last published release) must never
    # read as "differs, therefore upgrade". Anchored above to X.Y.Z, so both
    # sides are plain release triples here; a pre-release never reaches this
    # branch (PC-05: it took the channel arm instead — ordering a pre-release
    # against a release is not reimplemented here in tier-1 shell).
    INSTALLED_MINOR=$(printf '%s' "$INSTALLED_REL" | awk -F. '{print $2+0}')
    INSTALLED_PATCH=$(printf '%s' "$INSTALLED_REL" | awk -F. '{print $3+0}')
    LATEST_MINOR=$(printf '%s' "$LATEST_REL" | awk -F. '{print $2+0}')
    LATEST_PATCH=$(printf '%s' "$LATEST_REL" | awk -F. '{print $3+0}')
    if [ "$LATEST_MINOR" -gt "$INSTALLED_MINOR" ] || { [ "$LATEST_MINOR" -eq "$INSTALLED_MINOR" ] && [ "$LATEST_PATCH" -gt "$INSTALLED_PATCH" ]; }; then
      SELF_UPGRADE=minor
    fi
  fi
fi
```

**`SELF_UPGRADE=skip`:** print nothing more here and continue with the rest of
the upgrade (Step 1 onward) — the project's hooks, agents, rules and config are
still reconciled against the dev checkout's templates; only the launcher
self-upgrade is skipped, and the line above says so.

**`SELF_UPGRADE=major`** (launcher missing, or `$LATEST_MAJOR > $INSTALLED_MAJOR`):

This is a major upgrade. The launcher cannot self-upgrade across major versions — the installer must re-bootstrap the binary.

```
This is a major upgrade (v{INSTALLED_VERSION} → v{LATEST_VERSION}).
Run the installer to complete the upgrade:

  curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.9.1/install.sh | bash

Then re-run /edikt:upgrade to apply project changes.
```

Stop here — do not proceed to Step 1. Do not mutate any files.

#### 0e. Minor-version bump: delegate to launcher

**`SELF_UPGRADE=minor`** (same major, `$LATEST_VERSION` differs from `$INSTALLED_VERSION`):

```bash
EDIKT_INVOKER=/edikt:upgrade "$EDIKT_BIN" upgrade --yes
```

If the launcher upgrade fails (non-zero exit), report the error and stop. Do not proceed to Step 1 until the launcher upgrade succeeds.

**`SELF_UPGRADE=none`:** the launcher is current; proceed to Step 1.

#### 0f. Post-launcher-upgrade summary

After a successful `edikt upgrade --yes`, print:

```
Launcher upgrade complete: v{INSTALLED_VERSION} → v{LATEST_VERSION}
```

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
  curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.9.1/install.sh | bash
```

Use the Bash tool to read both versions — do NOT infer or guess them:
```bash
"${EDIKT_BIN:-edikt}" version 2>/dev/null | tr -d '[:space:]'
grep '^edikt_version:' .edikt/config.yaml | awk '{print $2}' | tr -d '"'
```

Use the actual output of these commands as INSTALLED_VERSION and PROJECT_VERSION.

Show at the top of the output:
```
Installed edikt: {INSTALLED_VERSION}
Project edikt:   {PROJECT_VERSION}
```

If INSTALLED_VERSION == PROJECT_VERSION AND there are no changes detected in step 2, show:
```
✅ Already up to date (edikt {INSTALLED_VERSION}) — nothing to upgrade.
```

<!-- contract-report:upgrade-step1-stop:start -->
Before this stop, run both Step 2c contract calls: they need no other Step 2 result, and the stop must not come first. Then print the two labelled `Extraction contract (` lines, verbatim, beneath the message above, followed by the fixed line Step 3 prints when either call's exit was not 0, and then stop. "Nothing to upgrade" is about files; it is never a statement about the extraction contract, so the report is not skipped here.
<!-- contract-report:upgrade-step1-stop:end -->

If INSTALLED_VERSION != PROJECT_VERSION, always proceed with the upgrade — the version difference alone is reason enough.

### 1.5. Sidecar Migration Check — detect and hand off

An artifact needs migrating when its `.md` still carries an in-body `[edikt:directives:start]: # … [edikt:directives:end]: #` block, or when the sidecar beside it is still `schema_version: 1`. That is a fact about the file, never about a version pair, so this check runs on every `/edikt:upgrade`, whatever the source version. It runs BEFORE Step 2 so the rest of the upgrade operates on a current-shaped project.

This command does not migrate. It runs the scanner and, when the plan lists anything, prints one screen and hands off to `/edikt:migrate`, which carries the whole lift → verify → recompile procedure, the recovery path, the dry-run gate and the exit-code contract. A clean upgrade never opens that command.

#### 1.5a. The scanner — and its exit code is the control

`migrate lift --dry-run` plans both halves of a migration in one pass: artifacts that still carry an in-body block and need a sidecar, and sidecars still at schema_version 1 that need converting. The control here is whether that check RAN, and only the exit code says so (exit code is the only contract; output is shown, never parsed). Before this shape, `SCHEMA_STALE` was grepped out of the dry-run's text with `|| echo 0` — on a binary that predates `migrate lift` the grep matched nothing, the check "computed" 0 and printed "Schema is current" without having run: a can't-fail control (D41 (b)).

```bash
# edikt-test-block: schema-currency-check
# Input: EDIKT_BIN. Output: SCHEMA_CHECKED (1 = the dry-run ran; 0 = it did not, RED).
SCHEMA_CHECKED=0
SCHEMA_PLAN="$(mktemp)"
EDIKT_INVOKER=/edikt:upgrade "$EDIKT_BIN" migrate lift --dry-run > "$SCHEMA_PLAN" 2>&1
SCHEMA_RC=$?
cat "$SCHEMA_PLAN"
if [ "$SCHEMA_RC" -eq 0 ]; then
  SCHEMA_CHECKED=1
else
  echo "🔴 schema currency was not checked: '$EDIKT_BIN migrate lift --dry-run' exited $SCHEMA_RC — rebuild or upgrade the binary (a binary without 'migrate lift' predates the sidecar lifter)"
fi
```

Read the outcome from `SCHEMA_CHECKED`, never from the plan's wording:

- **`SCHEMA_CHECKED=1`** — the dry-run ran and its plan is printed above, verbatim. If the plan lists anything to lift or to convert, print the hand-off screen and continue with Step 2; the migration is the user's next command, not this one's next step:
  ```
  ⬆  Migration — {n} artifact(s) carry a legacy in-body block or a v1 sidecar

     This upgrade does not migrate them. Nothing was changed.
     Next: /edikt:migrate   (the plan above; lift → verify → recompile, with the dry-run gate)

     Until then, /edikt:gov:compile refuses per artifact on the blocks it finds, and doctor reports them.
  ```
  If the plan lists nothing, say so in the summary (`sidecar migration: nothing to migrate — the dry-run ran`) and continue.
- **`SCHEMA_CHECKED=0`** — the check did not run. This is RED: report it as printed, carry it into the Step 6 summary as an unmeasured control, and do not print "Schema is current" or "nothing to migrate" — nothing was measured. Continue with the rest of the upgrade; the remedy is the binary, not this project.

### 2. Detect What Needs Upgrading

Run all checks in parallel and collect findings.

#### 2a. Hooks check

Read `.claude/settings.json`. Read `$EDIKT_TEMPLATES/settings.json.tmpl`.

For each hook type, check two things: (1) is the content correct, and (2) is it using the modern `.sh` script reference format?

**Pre-flight: the two mechanical repairs — the project's file unprompted, the user's only on consent.** An unsubstituted `${EDIKT_HOOK_DIR}` in a hook command (the host's shell expands it to the empty string, so every hook fails with `No such file or directory`) and a `statusLine` object without `type` (Claude Code refuses to load the whole file) are repaired in place, without touching anything else, by the tier-2 verb — not by prose.

Two settings files can carry them, and the verb acts on exactly one per call: `--repair --project` on this project's `.claude/settings.json` (the file this section reads and `edikt doctor` checks), plain `--repair` on the user-scope file. Running plain `--repair` here — as this step once did — repaired, or refused on, a file other than the one the finding was about, and edited user-scope settings without asking.

First resolve the user-scope settings file. It is commonly a symlink into a dotfiles checkout, so it is resolved through the link; its root is the ACTIVE profile — `CLAUDE_HOME`, then `CLAUDE_CONFIG_DIR` (Claude Code's own profile selector), then `$HOME/.claude` — and the expression is byte-identical to `install.sh`'s:

```bash
# edikt-test-block: user-settings-path
CLAUDE_ROOT="${CLAUDE_HOME:-${CLAUDE_CONFIG_DIR:-${HOME}/.claude}}"
USER_SETTINGS="$(readlink -f "$CLAUDE_ROOT/settings.json" 2>/dev/null || echo "$CLAUDE_ROOT/settings.json")"
```

Then define the hook dir and run the project repair. `HOOK_DIR` is the PORTABLE form: `.claude/settings.json` is committed, so it never carries one machine's home or checkout path — the host's shell expands the token at hook-fire time (`edikt install settings` accepts exactly these anchors besides an absolute path):

```bash
# edikt-test-block: settings-preflight-repair
# Input: EDIKT_BIN (§0y), USER_SETTINGS. Output: HOOK_DIR, USER_SCOPE_DEFECT (1 = the user-scope file has a defect the repair would fix).
if [ -d .edikt/hooks ]; then
  HOOK_DIR='"$CLAUDE_PROJECT_DIR"/.edikt/hooks'   # project-mode install: the hooks ship inside the project
else
  HOOK_DIR='$HOME/.edikt/hooks'                    # global install: the hooks live in the user's edikt home
fi
# 1. This project's own settings file. Unprompted: a mechanical fix of a silent error in a file the project owns.
if [ -f .claude/settings.json ]; then
  EDIKT_HOOK_DIR="$HOOK_DIR" "$EDIKT_BIN" install settings --repair --project
fi
# 2. The user-scope file: DETECT only. The verb is the detector, run on a copy, so "has a defect"
#    means exactly "the repair would change it" — nothing here re-derives what a defect is.
USER_SCOPE_DEFECT=0
if [ -f "$USER_SETTINGS" ]; then
  PROBE="$(mktemp -d)"
  cp "$USER_SETTINGS" "$PROBE/settings.json"
  CLAUDE_HOME="$PROBE" EDIKT_HOME="$PROBE" EDIKT_HOOK_DIR='$HOME/.edikt/hooks' "$EDIKT_BIN" install settings --repair >/dev/null 2>&1
  cmp -s "$USER_SETTINGS" "$PROBE/settings.json" || USER_SCOPE_DEFECT=1
  rm -rf "$PROBE"
fi
```

The project repair prints `repaired <path>: substituted N occurrence(s) of ${EDIKT_HOOK_DIR}` and/or `repaired <path>: added statusLine.type`, or `<path>: nothing to repair`. Do NOT prompt for it. Keep `HOOK_DIR` for §5: the hook merge runs with it in its environment.

On `USER_SCOPE_DEFECT=1` the user-scope file is broken too, but it is the user's, outside this repository, and shared by every project on the machine — ASK before touching it; upgrade never edits user-scope settings silently:

<!-- stop: repair-user-scope-settings -->
```
Your user-scope settings file {USER_SETTINGS} carries an unsubstituted ${EDIKT_HOOK_DIR} or a statusLine without type — Claude Code fails the affected hooks, or refuses to load the file.
Repair it in place (only those two defects; nothing else in the file changes)? [y/N]:
```

On `y`, run `EDIKT_HOOK_DIR='$HOME/.edikt/hooks' "$EDIKT_BIN" install settings --repair` and report its line. On anything else — including no human to answer, which takes the safe side — print `user-scope settings left as-is — edikt doctor reports the defect` and continue; never repair the user's file silently. `edikt doctor` detects the same two defects in either file and names the verb for the file it flagged (`--repair --project` or `--repair`); doctor itself stays read-only.

**Content checks — per hook TYPE, whether it exists at all:**
- `SessionStart`: command should reference `session-start.sh` in edikt's hook dir — `$HOME/.edikt/hooks/`, `"$CLAUDE_PROJECT_DIR"/.edikt/hooks/`, the unquoted or `"$HOME"`-quoted token, or the expanded absolute path all count as the same dir — if not → outdated
- `PreToolUse`: must be present with `Write|Edit` matcher — if missing → missing
- `PostToolUse`: must be present with `Write|Edit` matcher — if missing → missing
- `Stop`: must be type:command referencing `stop-hook.sh` in edikt's hook dir (any of the spellings above) — if type:prompt or inline → outdated
- `UserPromptSubmit`: must be present — if missing → missing (v4: injects active plan phase)
- `PostCompact`: must be present — if missing → missing (v4: re-injects context after compaction)
- `SubagentStop`: must be present — if missing → missing (v4: logs agent activity + quality gates)
- `InstructionsLoaded`: must be present — if missing → missing (v4: logs rule pack loading)

**Content checks — per hook script BASENAME, even when the type already exists.** Type presence alone does not mean the type is current: a project's `PreToolUse`/`PostToolUse` typically predate a release that added a new script *within* an already-present type, and a type-level check alone reports nothing to do while the new script never installs — this is exactly what happened on a real downstream 0.7.1 upgrade: `verify-gate.sh` and both `inject-directives-{pre,post}.sh` never installed because `PreToolUse`/`PostToolUse` already existed, and `doctor` then pointed back at this same upgrade command as the (non-working) remedy. For every hook type present in `$EDIKT_TEMPLATES/settings.json.tmpl`, collect the set of command basenames the template registers for that type (across every matcher/`if` group within it) and the set actually present in `.claude/settings.json` for that type (same). Any template basename absent from the installed set → outdated (missing entry), even though the type itself is present.

For each outdated or missing hook, note what changed in plain English:
- "PostToolUse: missing (auto-formats files after edits)"
- "PreToolUse: missing entry `pre-tool-use.sh` (managed-region guard) — type already present, this script was never added"
- "PostToolUse: missing entry `post-tool-use.sh` (post-write governance processing) — type already present, this script was never added"
- "UserPromptSubmit: missing (v4 — injects active plan phase into every prompt)"
- "PostCompact: missing (v4 — re-injects plan + invariants after compaction)"
- "SubagentStop: missing (v4 — logs agent activity, quality gates)"
- "InstructionsLoaded: missing (v4 — logs which rule packs load)"

#### 2b. CLAUDE.md — content-currency check, provenance-first (ADR-067)  edikt-guard:allow

**Resolve which file to examine first (SR-004).** The managed block is not
always at the repository root — a project bootstrapped a different way keeps
it at `.claude/CLAUDE.md` instead, and this repository is exactly such a
project. Check each candidate in order and use the first one that actually
carries an edikt sentinel (either syntax); reading the repo-root path
unconditionally reports `SENTINEL=none` for a project whose block lives
elsewhere, which this step's own next line used to call a *"benign skip"* —
true for a project that has no block, false for one whose block is just not
at the path this step assumed:

```bash
# edikt-test-block: managed-block-resolution
# Input: none (reads the working tree). Output: CLAUDE_MD_PATH, SENTINEL.
CLAUDE_MD_PATH=""
SENTINEL="none"
for cand in "CLAUDE.md" ".claude/CLAUDE.md"; do
  if [ -f "$cand" ]; then
    if grep -qF '[edikt:start]' "$cand" 2>/dev/null; then
      CLAUDE_MD_PATH="$cand"; SENTINEL="new"; break
    elif grep -qF '<!-- edikt:start' "$cand" 2>/dev/null; then
      CLAUDE_MD_PATH="$cand"; SENTINEL="old"; break
    fi
  fi
done
```

From here on, every read and every `Edit` in this step targets
`$CLAUDE_MD_PATH`, never the literal `CLAUDE.md` — the rest of this section
says "CLAUDE.md" as the name of the artifact, not as a hardcoded path.

- No edikt block found in any candidate (`SENTINEL=none`) → skip. `/edikt:init` installs the block; upgrade never creates one from nothing. This is the genuine negative — a project with no block anywhere — not a project whose block this step failed to look for.
- Old HTML comment sentinels found (`SENTINEL=old`) → run **Sentinel Syntax Migration** below first, on `$CLAUDE_MD_PATH`, then continue into the content-currency check on the now-migrated file. A syntax migration says nothing about whether the *content* underneath was ever current — it falls through to the Bootstrap Reconciliation branch below exactly like any other syntax-current, provenance-absent block.
- New visible sentinels found (`SENTINEL=new`) → continue directly into the content-currency check below, on `$CLAUDE_MD_PATH`.

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

**The managed block's bytes (one definition; `/edikt:init` writes the same record and uses it too).** In this section `installed_block`, `new_block` and every "block" mean the bytes from the `[edikt:start]` line through the `[edikt:end]: #` line inclusive, with no trailing newline. `render` returns text in that form (a trailing newline of the template file is dropped), so `render(...) == installed_block` is a byte comparison of like with like. The record's `managed_hash` is the `sha256` of exactly those bytes, and its `template_hash` is the `md5` of the raw template file.

**The migration bypass, and why it's a file, not an env var.** INV-005's guard blocks any Edit/Write whose byte range overlaps CLAUDE.md's managed region unless an allowlisted bypass is present. `EDIKT_MIGRATION_IN_PROGRESS=1` is `edikt upgrade`'s own bypass, set inside its own Go process for its own direct `os.WriteFile` calls — writes that never cross `PreToolUse` at all. This command's writes go through Claude Code's own `Edit` tool instead, a different path: each `Bash` tool call is a fresh subshell, so an `export` in one call is invisible to the separate hook subprocess spawned for a later `Edit` call — an env var set that way is never actually reachable. Bracket every Edit into the managed region with the file-based signal instead, which survives across tool calls because it's real filesystem state:  edikt-guard:allow

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
Edit $CLAUDE_MD_PATH, replacing only the byte range between [edikt:start] and [edikt:end]
  inclusive, with EDIKT_MIGRATION_IN_PROGRESS=1 set (the INV-005 bypass edikt upgrade already uses).  edikt-guard:allow
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

**Prior-template check (runs before the prompt below; mirrors §2c's check for agents).** A block that no human ever edited can still fail the strict match above: it was written by an earlier release's template, and the current template has text it lacks. Projects set up by `/edikt:init` before init wrote a record are all in this state. Prompting here would be wrong twice over: nothing is in doubt, and with no human to answer the prompt takes `s`, so the project would never receive the update. For each earlier shipped template at `<edikt root>/versions/*/templates/CLAUDE.md.tmpl` (the edikt root resolved as in §0y-bis: `$_edikt_root_g`, or the project-mode root when `.edikt/current/templates` exists; skip the version being installed):

```
for prior_template in <edikt root>/versions/*/templates/CLAUDE.md.tmpl (skip the current version):
  extracted = detemplate(installed_block, prior_template)
  if extracted is not FAIL AND render(prior_template, extracted) == installed_block:
    # The installed block equals, byte for byte, what that release's template
    # produces from its own values: it is an unedited earlier template.
    # Treat it as the safe-replace case (Step 6), with no prompt.
    new_block = render(current_template, extracted)   # opaque values carried from the
                                                      # extraction, config-derived values
                                                      # from the current config
    Edit $CLAUDE_MD_PATH's block to new_block under the migration marker bracket above.
    write_record(project_root, claude_md_path=CLAUDE_MD_PATH, template_hash=current_template_hash,
                 template_version=CURRENT_EDIKT_VERSION, managed_hash=sha256(new_block))
    emit_event "upgrade_claude_md_path" { path: "bootstrap_prior_template_match" }
    report "   ⬆ CLAUDE.md — managed block updated (it matched the <version> template unedited)"
    return
```

`<version>` is the directory name under `versions/`. The comparison is an exact render match, never a similarity score: one changed byte, in the template's own text or in an opaque value that no longer round-trips, is a mismatch. If no earlier template matches, including when none remain on disk (retention is not guaranteed, see `edikt prune`), fall through to the prompt below and keep its safe default. Absence of a match means "unknown", never "edited": the block may come from a release this machine no longer has, or a config path may have changed since it was written, and either way the check has proved nothing in either direction.

<!-- stop: managed-block-no-provenance -->
Otherwise, ask via `AskUserQuestion`, showing installed vs. `render(current_template, detemplate_lenient(installed_block, current_template))` — the lenient extractor, not the strict `opaque_values` computed above (which may be `FAIL`). With no human to answer, take the safe side — `s` (skip): no file
change, no record written, and the question is recorded as answered by default
rather than given. Nothing overwrites a block whose provenance is unproven:

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
| `a` | Edit `$CLAUDE_MD_PATH`'s block to `render(current_template, detemplate_lenient(installed_block, current_template))`, under `EDIKT_MIGRATION_IN_PROGRESS=1`. Write the record. |
| `k` | Do not modify `$CLAUDE_MD_PATH`. Write the record as a deliberate acceptance of the installed content as the new baseline (`template_hash = current_template_hash`, `managed_hash = sha256(installed_block)`) — a future upgrade compares against this baseline going forward, not against a re-derived "should have matched" value. |
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

With no human to answer, take the safe side — `s` (skip): no file change, no
record write, and the question recorded as answered by default rather than
given.

<!-- stop: managed-block-template-moved -->
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
| `a` | Edit `$CLAUDE_MD_PATH`'s block to `render(current_template, detemplate_lenient(installed_block, current_template))`, under `EDIKT_MIGRATION_IN_PROGRESS=1`. | `current_template_hash` |
| `k` | Do not modify. | left at the OLD stored value — explicitly not advanced, so a future upgrade asks again rather than treating "kept edits" as "now current" |
| `m` | Write `${CLAUDE_MD_PATH}.merge` with `diff3 -m` conflict markers over just the block. Do not touch `$CLAUDE_MD_PATH` or the record until the merge completes and the user re-runs `/edikt:upgrade`. | unchanged |
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

Then prompt. With no human to answer, take the safe side — `s` (skip this
agent): the installed file is left exactly as it is, no template is applied
over local edits, and the question is recorded as answered by default rather
than given.

<!-- stop: agent-template-conflict -->
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

**The subject set is the REGISTRY, never the project's contents.** `$EDIKT_TEMPLATES/agents/_registry.yaml`
declares an `always:` list — the agents every project gets, because a tier-1
command dispatches them by `subagent_type` and a dispatch to an agent that is
not installed does not fail loudly: the host reports the type as unknown and
the work falls to a general-purpose agent adopting the prompt, so a specialist
review reads as a specialist's without one having run.

Walking what the project HAS can only ever find what is already there, so an
agent the registry declares and the project lacks is unreachable from that
loop and its absence reads as nothing to do. Install every missing one:

```bash
# edikt-test-block: always-installed-agents
ALWAYS="$(awk '/^always:/{f=1;next} /^[a-z_]+:/{f=0} f && /^  - /{print $2}' "$EDIKT_TEMPLATES/agents/_registry.yaml")"
AGENT_FAILS=0
for slug in $ALWAYS; do
  src="$EDIKT_TEMPLATES/agents/$slug.md"
  dst=".claude/agents/$slug.md"
  [ -f "$src" ] || { echo "  ⚠ Agent            $slug declared in the registry but absent from the payload — reinstall edikt"; AGENT_FAILS=$((AGENT_FAILS+1)); continue; }
  if [ ! -f "$dst" ]; then
    if mkdir -p .claude/agents 2>/dev/null && cp "$src" "$dst" 2>/dev/null && cmp -s "$src" "$dst"; then
      echo "  ✓ Agent            installed $slug (declared always-installed; it was missing)"
    else
      echo "  ✗ Agent            $slug NOT installed: the copy into .claude/agents/ failed (this directory may be write-protected in this session); the dispatch to $slug will not reach a specialist until it is installed"
      AGENT_FAILS=$((AGENT_FAILS+1))
    fi
  fi
done
[ "$AGENT_FAILS" -eq 0 ]
```

A non-zero result here is reported in the Step 3 summary as agents NOT installed, never as installed, and the upgrade continues.

An agent already present is left to the provenance flow below — this step adds
what the registry declares and the project lacks, and nothing else. `edikt
doctor` reports the same gap as a 🔴 with the count, so a project that skips
this step still sees it.

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

<!-- contract-report:upgrade-step2c:start -->
**Extraction contract state — reported on every run that reaches the summary, changed or not.** Two extractors matter and they can differ: the one installed in the project now, and the template this version ships. The payload's `sidecar-extractor.md` can be byte-identical to the installed one, and a project's own copy can be custom, preserved or skipped, so a report tied to "the extractor is being replaced" would miss exactly the upgrade that needs it. This runs whenever `sidecar-extractor` is installed in the project or is about to be (it is declared always-installed above), regardless of the classification result, `<!-- edikt:custom -->`, or `agents.custom`. Run these two calls as two separate tool calls, the first and then the second, each plainly on one line. Capture nothing and discard no output, so each tool call shows its line and its exit code. The first reports the extractor installed now:

```bash
EDIKT_INVOKER=/edikt:upgrade "${EDIKT_BIN:-edikt}" doctor --contract-state
```

The second reports this version's template:

```bash
EDIKT_INVOKER=/edikt:upgrade "${EDIKT_BIN:-edikt}" doctor --contract-state --agent-file "$EDIKT_TEMPLATES/agents/sidecar-extractor.md"
```

The line each prints and the exit code its tool call reports are that call's report; note all four for Step 3. `EDIKT_BIN` and `EDIKT_TEMPLATES` are the values resolved in 0y and 0y-bis. A shell that does not carry variables from one call to the next has to be given them again in the same call: prefix the call with the 0y-bis resolution, or substitute the literal path.

For each call separately:

- Exit `0`, `10` or `11`: the printed line is that call's report; carry it to Step 3 verbatim.
- Any other exit (a binary that predates the flag, the version gate, the front door, `SELF_UPGRADE=skip` with an old binary): that call's report is this fixed sentence, whatever the call printed:

```
extraction contract state is UNMEASURED: this binary cannot report it
```

Never omit a report silently, never treat a missing report as validated, and never branch on the text of a line, only on the exit codes. Carry both exit codes to Step 3 too.
<!-- contract-report:upgrade-step2c:end -->

#### 2c.1 Skills reconciliation (.claude/skills/)

List directories under `$EDIKT_TEMPLATES/skills/`. For each, check whether `.claude/skills/{name}/SKILL.md` exists in the project.

Unlike agents, these are edikt's own on-demand methodology packages (deciding what qualifies as an ADR vs. invariant vs. guideline, and similar always-true-for-edikt content) — not project-specific and not meant to be hand-edited. There is no core/optional split and no diff/merge step: any skill the current version's templates declare that the project lacks is installed unconditionally, byte-identical, the same way it is on fresh init (§4, "Skills").

```
  ✓ Skills          1 new package → .claude/skills/edikt-governance-authoring/
```

If a project has hand-edited a skill file, leave it alone — do not overwrite a file whose content already differs from `$EDIKT_TEMPLATES/skills/{name}/SKILL.md`; only install what's missing.

**How to install a missing skill.** The write guard treats `.claude/skills/edikt-*/SKILL.md` as generated output and refuses a tool write to it; the in-progress marker is the signal this command already uses for its own writes. For each missing skill, in this order, one tool call per step:

1. Create the marker, in one Bash call:

```bash
# edikt-test-block: skill-install-open
mkdir -p .edikt/state && touch .edikt/state/.migration-in-progress
```

2. Write `.claude/skills/{name}/SKILL.md` with the Write tool, content byte-identical to `$EDIKT_TEMPLATES/skills/{name}/SKILL.md`.

3. Verify and remove the marker, in one Bash call that removes it whether or not the comparison passes:

```bash
# edikt-test-block: skill-install-close
cmp -s "$EDIKT_TEMPLATES/skills/{name}/SKILL.md" ".claude/skills/{name}/SKILL.md"; rc=$?; rm -f .edikt/state/.migration-in-progress; exit $rc
```

4. Report the skill installed only when that comparison passed; otherwise report it NOT installed and say why. Never leave the marker behind, including when step 2 is refused or interrupted: if the Write fails, still run step 3 before anything else. A marker left on disk switches the write guard off for the whole project.

#### 2e-bis. Project templates check — delegated to doctor

The three per-artifact project templates (`.edikt/templates/adr.md`, `invariant.md`, `guideline.md`) are user-owned, team-shared and committed to git. `/edikt:upgrade` never writes, verifies or classifies them: `edikt doctor` reports each one as present or missing with the remedy (`/edikt:init --reset-templates`), and it is the one place that diagnosis lives. Upgrade only carries the contract: **Never overwrite project templates** — an existing template MUST NOT be touched. **Never auto-run init.** Even when doctor reports a template missing, `/edikt:upgrade` does NOT invoke `/edikt:init` on the user's behalf.

#### 2e. Rule packs check

If `.claude/rules/` does not exist or contains no `.md` files → mark rule packs as "nothing installed, skip" (not outdated).

Otherwise, same logic as `/edikt:rules-update`:
- Compare `version:` frontmatter in installed vs template
- Only flag as outdated if installed version < template version
- Skip files without `<!-- edikt:generated -->` marker (manually edited)
- Skip files not in the registry (custom rules)
- **Hash comparison, normalised (SR-005):** For files with `edikt:generated` marker, compare against the template using normalised content, not raw bytes. Comparing raw bytes flags a line-ending conversion or trailing-whitespace stripping — the shapes a checkout under `core.autocrlf` or an editor's save-on-format applies to any tracked file — as a user edit, on a file the user never touched. (This is NOT the same class of problem §2c's stack-filter step solves, and it does not transfer here: no rule-pack template under `templates/rules/` carries an `<!-- edikt:stack:... -->` block, so §2c applies no filter §2e would need to match, and `edikt`'s own `PostToolUse` hook has no `.md` case — it never touches a rule pack. The reachable producers of a byte-level, non-content difference on this path are a line-ending/whitespace-normalising checkout or editor, nothing edikt itself runs.) Normalise both sides before hashing, never after — a real edit still changes the normalised content and is still caught. The block **emits its decision** rather than only assigning it — a value assigned to a shell variable dies with its subshell, and only a printed line is something a caller (or a test) can observe:

```bash
# edikt-test-block: rule-pack-drift-normalisation
# Input: $1 = a file path. Output: stdout = its drift-comparison form.
normalize_for_drift() {
  sed -e 's/\r$//' -e 's/[[:space:]]*$//' "$1" | awk '
    NR>1 { print prev }
    { prev=$0 }
    END { if (NR>0) print prev }
  '
}
INSTALLED_NORM=$(normalize_for_drift ".claude/rules/${name}.md")
TEMPLATE_NORM=$(normalize_for_drift "$EDIKT_TEMPLATES/rules/${tier}/${name}.md")
if [ "$INSTALLED_NORM" != "$TEMPLATE_NORM" ]; then
  RULE_DRIFT=modified
else
  RULE_DRIFT=clean
fi
printf 'RULE_DRIFT=%s\n' "$RULE_DRIFT"
```

  `RULE_DRIFT=modified` is what flags a pack as edited below — never a raw hash. If hashes (now: normalised forms) differ, flag as modified:
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
  ⬆  PreToolUse     — missing entry pre-tool-use.sh (type already present, script was never added)

Agents (.claude/agents/)
  ⬆  dba.md   — template added 12 lines (pure expansion, safe to apply)
  ⚠  security.md  — installed file has 8 lines not in template (USER DIVERGENCE — preview diff before accepting)
  ✓  architect.md  — up to date
  ✓  sidecar-extractor.md  — up to date
     Extraction contract (installed now): <the line from the first Step 2c call, verbatim>
     Extraction contract (template): <the line from the second Step 2c call, verbatim>

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

<!-- contract-report:upgrade-step3:start -->
**The extraction contract prints on every run that reaches this summary**, directly under the `sidecar-extractor` row of the Agents section, as exactly these two labelled lines, always both, indented to match, each followed by that Step 2c call's report verbatim:

```
Extraction contract (installed now): <the line from the first Step 2c call, verbatim>
Extraction contract (template): <the line from the second Step 2c call, verbatim>
```

They print when the row reads up to date, when the project's extractor is marked custom, and when the summary would otherwise say "Already up to date". Do not compare the two lines' text. Add one sentence of instruction saying which one the project ends up with: the template's when this run installs or replaces `sidecar-extractor`, and the installed one when that row is custom, preserved or skipped.

When EITHER Step 2c call's exit was not 0, print this one fixed line beneath them:

```
Compiling a sidecar for a new or edited artifact still works; review the result. Whether re-extraction is available: edikt gov reextract --status
```

When both exits were 0: nothing beneath them.

This is information ahead of the confirmation in Step 4, not a question: it adds no prompt and changes no default. When a line says the state is UNMEASURED, it ends with what to do about it: show it in full, and do not attempt that repair yourself, because changing an agent file is the user's decision.
<!-- contract-report:upgrade-step3:end -->

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

**If any agent has USER DIVERGENCE**, prompt for each diverged agent
individually BEFORE the main confirmation. With no human to answer, take the
safe side — `[3] Skip`: the diverged agent is excluded from the upgrade list,
no customization is replaced, and the question is recorded as answered by
default rather than given.

<!-- stop: agent-user-divergence -->
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

<!-- stop: apply-upgrades -->
```
Apply these upgrades? (y/n/select)
  y      — apply all
  n      — cancel
  select — choose which sections to apply (hooks / agents / rules / config / claude.md)
```

**Agents classified as PURE EXPANSION** can be auto-applied with `y` without individual confirmation — they're provably safe (no deletions).

Wait for response. If `select`, ask separately for each section.

With no human to answer, take the safe side — `n`: print the upgrade plan,
apply nothing, record the answer as taken by default rather than given, and
exit non-zero so a pipeline cannot read "nothing applied" as "already current".
The approval stays outstanding; the plan above is what a human resumes from.

If cancelled:
```
Upgrade cancelled — no changes made.
```

### 5. Apply Upgrades

#### Hooks

**Do this merge via the `Bash` tool (e.g. a `python3` read-modify-write), not the `Edit` or `Write` tool.** `.claude/settings.json` is JSON-hosted, and INV-005's JSON-region verification variant is not yet implemented — the `PreToolUse` guard (`templates/hooks/pre-tool-use.sh`) unconditionally denies any `Edit`/`Write` to it, since it cannot yet distinguish a safe edit from one that empties the `deny` list or drops a hook registration. That deny only intercepts the `Edit`/`Write` tools — it has no visibility into a write issued through `Bash` — so performing the merge below as a `Bash`-invoked script is the sanctioned path, not a workaround around a bug. See the guard's own comment (`pre-tool-use.sh`, the `settings.json` branch) for the full reasoning.  edikt-guard:allow

Read the current `.claude/settings.json`. Read the template. Run the merge from the project root with `HOOK_DIR` from §2a in its environment (`HOOK_DIR="$HOOK_DIR" python3 …`), so the hooks it adds carry the same portable form the pre-flight wrote.

Merge at the individual command-BASENAME level, never by replacing a whole hook type's array — a whole-array replace silently discards user-added entries under that type, and a type-level "already present, skip" check silently leaves out any template basename the installed array doesn't yet have (§2a above). Two passes: add what §2a found missing, then remove what a release retired (never a user's own addition):

```python
# Pseudocode
import os.path

settings = read_json('.claude/settings.json')
template_text = read_file('$EDIKT_TEMPLATES/settings.json.tmpl')

# CRITICAL: substitute ${EDIKT_HOOK_DIR} BEFORE parsing as JSON.
# The host runs `command:` strings through a shell, but nothing sets
# EDIKT_HOOK_DIR in a hook's environment — an unsubstituted placeholder
# expands to "" and fails with /bin/sh: /<hook>.sh: No such file or directory.
# hook_dir is HOOK_DIR from §2a, passed in the environment: the PORTABLE form
# this committed file carries ("$CLAUDE_PROJECT_DIR"/.edikt/hooks for a
# project-mode install, $HOME/.edikt/hooks otherwise). It contains '"' in the
# project case, so it is JSON-escaped for the raw-text substitution.
PROJECT_ROOT = os.getcwd()   # the merge runs from the project root
hook_dir = os.environ.get("HOOK_DIR") or "$HOME/.edikt/hooks"
template_text = template_text.replace("${EDIKT_HOOK_DIR}", json.dumps(hook_dir)[1:-1])
template_hooks = json.loads(template_text)['hooks']

def basename(command):
    return os.path.basename(command)

# One hook dir has three spellings in the wild, and every comparison below
# treats them as the same directory: the token form ($HOME/... or
# $CLAUDE_PROJECT_DIR/...), the quoted token form ("$HOME"/... or
# "$CLAUDE_PROJECT_DIR"/...), and the expanded absolute path an older install
# wrote. Comparing the raw strings would read an edikt hook written in one
# spelling as a user's own hook in another — and never retire it.
def expand(command):
    for token, value in (('"$CLAUDE_PROJECT_DIR"', PROJECT_ROOT), ('$CLAUDE_PROJECT_DIR', PROJECT_ROOT),
                         ('"$HOME"', HOME), ('$HOME', HOME)):
        if command.startswith(token + '/'):
            return value + command[len(token):]
    return command

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
# exists on disk. In 0.9.0 that is verify-gate.sh and inject-directives-pre.sh:
# edikt no longer injects rule text into a write in progress, so neither
# hook has a job left to do; a 0.8.x settings.json
# registers both, and this pass prints `removed verify-gate.sh` and
# `removed inject-directives-pre.sh` for them. Ownership is decided by directory, not by "was it in the
# template last time": a command whose directory resolves — after expand(),
# in any of its three spellings — to edikt's hook dir is edikt's own; anything
# else (a real user-added hook, wherever it points) is NEVER touched by this
# pass, template membership or not.
template_basenames_by_type = {
    hook_type: {basename(h['command']) for g in groups for h in g.get('hooks', [])}
    for hook_type, groups in template_hooks.items()
}
for hook_type, groups in list(settings.get('hooks', {}).items()):
    for group in groups:
        kept = []
        for h in group.get('hooks', []):
            cmd = h['command']
            owned_by_edikt = os.path.dirname(expand(cmd)) == expand(hook_dir)
            still_shipped = basename(cmd) in template_basenames_by_type.get(hook_type, set())
            file_exists = os.path.exists(expand(cmd))
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

**Pass 3 — the other scope, on consent only.** Hooks belong at exactly one scope, and this pass writes the project's
`.claude/settings.json`.
A hook ALSO registered in the user-scope settings fires twice per event: 13
hooks did so for weeks on the reference machine, each event doing its work
twice while a warning nobody acted on scrolled past.

Read the user-scope settings file `USER_SETTINGS`, resolved in §2a (block
`user-settings-path`) through the active profile — `CLAUDE_HOME`, then
`CLAUDE_CONFIG_DIR`, then `$HOME/.claude` — and through any symlink. Skipping
the middle rung inspects a file the running profile does not read, reports no
duplicates, and leaves the doubled hooks firing.

For every hook basename registered in BOTH that file and the project's, print
the pair and ASK before touching anything:

<!-- stop: remove-user-scope-duplicates -->
```
N hook(s) are registered at both user and project scope and fire twice per event:
  SessionStart/session-start.sh
Remove the user-scope duplicates from {USER_SETTINGS} (a .bak is written beside it)? [y/N]:
```

On `y`, remove only those entries from the resolved file and report each one.
On anything else — including no human to answer, which takes the safe side and
is recorded as answered by default rather than given — print `left as-is — the
duplicate-invocation guard covers the interim` and continue; never edit the user's file silently, and never
remove an entry whose command does not resolve under `hook_dir` (it is theirs,
not edikt's). "Resolves" means after `expand()` above: the user file commonly
carries the expanded absolute path or `$HOME/.edikt/hooks` while the project
carries `"$CLAUDE_PROJECT_DIR"/.edikt/hooks` — the same hook in different
spellings is still a duplicate, matched by basename and owned by directory. `edikt doctor` reports the same doubling as a 🔴 with the
measured cost from the hook journal, so a declined removal is still visible.

**Never remove** a hook command whose directory does not resolve under `hook_dir` (the user may have added their own, wherever it points) — Pass 2 above is the one narrow, ownership-checked exception: an edikt-owned command (`hook_dir`-resolved) whose basename this release no longer ships anywhere, and whose file no longer exists, is retired, not user-added, and gets removed with the removal reported. This upgrade never overwrites user-added hooks — everything else it only updates or adds.

#### Agents

For each outdated agent:
1. Read the installed file
2. Read the template
3. Replace the installed file with the template content
4. Print, once per run in which any agent was replaced: `agents replaced — the host caches agent definitions at session start; restart the session before the next extraction, or the old definition runs and the fix appears not to have landed.` (This is the documented field failure: an extractor re-run in the same session reproduced an over-claim the new template forbids, D58.)

For each new **core** agent (evaluator):
1. Copy the template to `.claude/agents/{slug}.md`
2. Report: `✓ Installed evaluator.md — core (required by plan harness)`

**`evaluator-headless.md` is NOT copied here, and must not be.** It has no frontmatter, is never Task-dispatched, and every caller resolves it from the install root — `${EDIKT_ROOT:-${EDIKT_HOME:-$HOME/.edikt}}/current/templates/agents/evaluator-headless.md`, or the project-mode `.edikt/current/templates/agents/...` (see `commands/sdlc/plan.md`'s headless branch). Nothing reads `.claude/agents/evaluator-headless.md`.

Copying it there wrote a file no code path opens and reported it as installed — work with no effect, announced as work done. Its absence from `templates/agents/_registry.yaml`'s `always:` list is correct, not a gap, and so is its absence from `.claude/agents/`.

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

#### Rule packs

Same as `/edikt:rules-update` logic — replace outdated packs, skip manually edited and custom ones.

#### Compiled governance — remove what this version retired

`gov compile` writes the compiled tree and never deletes from it: a file a
release stops producing simply stays, and the model keeps loading it. The
write-time channel's `directive-index.yaml` is the live case: edikt no
longer injects rule text into a write in progress, so the file is dead
weight, still on disk in every project that compiled before the upgrade,
and removed by nothing. A rule the corpus no longer states, enforced anyway,
is the same class as a stale hook registration.

Two mechanisms, because a payload cannot enumerate what it no longer
produces:

1. **A retired-name list**, reviewed per release. This is the only way a file
   the current compiler never emits can be named at all, so it is explicit
   data in this procedure rather than something derived.
2. **A leftover REPORT** against the compile's own manifest. Every rendered
   surface is listed with its SHA in `.claude/rules/governance/manifest.yaml`;
   a file in that directory the manifest does not list was produced by
   something else. It is reported, never removed — ownership is not provable
   for a file nobody's manifest claims, and the hooks pass's rule holds here
   too.

```bash
if [ -d .claude/rules/governance ]; then
  # 1. Retired by name, with the release that retired each.
  for retired in directive-index.yaml; do
    if [ -e ".claude/rules/governance/$retired" ]; then
      rm -f ".claude/rules/governance/$retired"
      echo "  ✓ Governance       removed $retired (retired with the write-time channel)"
    fi
  done

  # 2. Anything else the manifest does not claim: REPORTED, not removed.
  # A missing manifest means the project has not compiled since the surface
  # existed — say that, rather than treating every file as unclaimed.
  if [ -f .claude/rules/governance/manifest.yaml ]; then
    for f in .claude/rules/governance/*; do
      [ -e "$f" ] || continue
      base="$(basename "$f")"
      [ "$base" = "manifest.yaml" ] && continue
      if ! grep -qF -- "$base" .claude/rules/governance/manifest.yaml; then
        echo "  ⚠ Governance       $base is not listed in manifest.yaml — not produced by this compiler; review and delete by hand"
      fi
    done
  else
    echo "  · Governance       manifest.yaml absent — leftover surfaces UNMEASURED (run /edikt:gov:compile)"
  fi
fi
```

Report each removal on its own line, the way the hooks pass does. Remove
nothing whose ownership is not provable: a file under `.claude/rules/` that
edikt's compiler never produced is the user's, and this pass leaves it.

#### Compile schema check

Check if the project's generated governance is stale vs the current compile schema.

1. Read the constant `COMPILE_SCHEMA_VERSION` from `$EDIKT_TEMPLATES/commands/gov/compile.md` (or `commands/gov/compile.md` in the installed templates).
2. Read `.claude/rules/governance.md` (if it exists) and extract `compile_schema_version` from its YAML frontmatter.
3. Compare:
   - **Lower than current**: note `governance.md compiled with schema v{old} (current: v{new}) — run /edikt:gov:compile to regenerate`
   - **Equal**: no note
   - **Higher than current**: note `governance.md compiled with schema v{n}, but this edikt only supports v{current}. Upgrade edikt globally first.`

Do NOT auto-run `/edikt:gov:compile`. Surface the recommendation in the upgrade summary and let the user decide. Compile is potentially expensive and may have contradictions that need review.

**Important**: Never enforce `compiled_by` or `compiled_at` equality with the current edikt version. Those fields are informational HTML comments only — they tell humans when/who produced the file, but do not drive any decision.

#### Project templates (v0.6.0+)

**Never overwrite project templates under `.edikt/templates/`.** Existing `.edikt/templates/adr.md`, `.edikt/templates/invariant.md` and `.edikt/templates/guideline.md` MUST NOT be touched by `/edikt:upgrade` — they are user-owned. A missing one is `edikt doctor`'s finding (Broken state when all three are missing, Partially configured when some are), not this command's; nothing here prints it twice. **Bump `edikt_version` in config** after the upgrade completes successfully.

#### Schemas — install-if-absent, refresh-on-drift

The JSON schemas commands and the extractor resolve project-locally (`.edikt/schemas/*.schema.json`) are copies of the version's payload, not user content. Reconcile them against the payload's set — every `templates/schemas/*.schema.json` the installed version ships, which is the manifest for this surface — never against what the project happens to have:

```bash
mkdir -p .edikt/schemas
for src in "$EDIKT_TEMPLATES"/schemas/*.schema.json; do
  [ -f "$src" ] || continue
  dst=".edikt/schemas/$(basename "$src")"
  if [ ! -f "$dst" ]; then
    cp "$src" "$dst" && echo "  ✓ Schema           installed $(basename "$src")"
  elif ! cmp -s "$src" "$dst"; then
    cp "$src" "$dst" && echo "  ✓ Schema           refreshed $(basename "$src") (project copy had drifted from the payload)"
  fi
done
```

A project copy that differs from the payload is refreshed, not kept: the old lazy `test -f || cp` bootstraps in the artifact commands never refreshed, so a stale copy outranked the payload for the extractor and doctor. Those bootstraps are gone; this step and `/edikt:init` are the only writers of `.edikt/schemas/`.

#### Stale phasea plans

An unconsumed plan in `.edikt/state/phasea/` written by an older schema can never be verified or consumed once the binary that wrote it is gone — `phasea verify` refuses it by name, and nothing else ever removes it. Run:

```bash
EDIKT_INVOKER=/edikt:upgrade edikt phasea prune-stale
```

Report each removed plan as printed (`removed stale plan <id> (schema v<old>, this binary writes v<new>)`), or `no stale plans found` when there is nothing to do. A consumed plan and a plan at the current schema are both left untouched by design — never report those as removed.

### 6. Post-Upgrade

After applying:

1. Update `edikt_version` in `.edikt/config.yaml` to the installed version — even if no other changes were applied
   - If a `edikt_version:` line exists, replace it
   - If it doesn't exist (the project never pinned one), add it as the first non-comment line after any leading `#` comment block at the top of the file
2. Check if linter configs exist and linter rules are outdated (template mtime > linter rule mtime):
   ```
   Linter configs found. Run /edikt:sync to regenerate linter rules.
   ```

4. **Project templates:** not reported here — `edikt doctor` names any missing template and its remedy. Leave the user in control: never auto-run `/edikt:init`.

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

<!-- contract-report:upgrade-step7:start -->
```bash
EDIKT_INVOKER=/edikt:upgrade "${EDIKT_BIN:-edikt}" gov reextract --status --json
```
<!-- contract-report:upgrade-step7:end -->

If the binary is absent, or the status check fails, or `eligible` is `0`,
say nothing further — this is a silent skip, not a warning; an empty offer
would be noise on every project that's already current.

**If `contract_validated` is `false`, DO NOT OFFER. Report the block instead.**
The same field the dispatcher refuses on rides in this JSON, so the offer and
the dispatch cannot disagree — they did, and a user found it: `--status`
reported `eligible 87` while every dispatch of those 87 refused outright, and
this step offered the work with no hint it would be. Print this and go no
further; there is no question to ask, because there is no answer that would
start anything:

```
⚠ N artifact(s) are on an earlier extraction contract, and re-extraction is
  BLOCKED on this build — the shipped extraction contract has not been proven
  on the frozen fixture set.
  <dispatch_blocked_reason, verbatim>
  Nothing to do here: this needs a build whose extractor contract is recorded
  as validated. Do not pass --skip-fixture-proof to get past it.
```

If `eligible > 0` **and** `contract_validated` is `true`, print, using
`--status`'s real numbers (not a guess):

```
📋 N artifact(s) on an earlier extraction contract than what's installed.
   Re-extracting refreshes their sidecar content through the current
   contract — separate from the schema/version work above, and entirely
   optional. Estimated cost: roughly N * ~4 minutes (EXP-006 median),
   less in parallel. Nothing lands without your explicit review per
   artifact.
```

<!-- stop: offer-reextract -->
Ask via `AskUserQuestion`: "Run /edikt:gov:reextract now?" (yes / no /
remind me later — all three are terminal for this step, none block
completion of the upgrade already reported above). With no human to answer,
take the declared default (`no`): the upgrade already reported above stands,
nothing is dispatched, and the answer is recorded as taken by default rather
than given.

- **yes** — invoke `/edikt:gov:reextract` with no arguments (full corpus).
  That command owns its own dispatch-and-review flow from here; this step
  ends once it's invoked.
- **no** or **remind me later** — end the upgrade run here. Do not repeat
  the offer within the same session; the user can run
  `/edikt:gov:reextract` directly whenever they want, and the next
  `/edikt:upgrade` invocation will offer again if artifacts are still
  eligible then.

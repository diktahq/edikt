---
name: gov:compile
description: "Compile ADRs and invariants into governance directives"
effort: high
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Write
---

# edikt:gov:compile

Compile accepted ADRs, active invariants, and team guidelines into topic-grouped governance rule files under `.claude/rules/governance/`, with a routing index at `.claude/rules/governance.md`.

## Compile Schema

**`COMPILE_SCHEMA_VERSION = 2`**

This integer identifies the output format contract of this command. It is independent of edikt's marketing version (see ADR-007).

- `1` — v0.1.x flat `governance.md` (single file, one directive per line, 30-directive cap). Legacy.
- `2` — v0.2.x topic-grouped rule files under `governance/`, routing table index, directive sentinel blocks, no directive cap. Current.

Bump this constant ONLY when the output structure changes in a way that older tooling cannot read. Prose updates, bug fixes, and new directives do NOT bump the schema. When bumped, write a new ADR superseding ADR-007 to document the migration.

Each source document must contain an LLM directive sentinel block (`[edikt:directives:start/end]: #`). If present, compile reads it verbatim — no extraction, no distillation. If absent, compile generates the sentinel block and writes it back to the source document before compiling.

CRITICAL: NEVER write governance files that contain contradictions — detect and report them before writing, and abort or confirm with the user.

## Arguments

- `--check` — validate only, don't write. Exit with errors if contradictions found. For CI.
- `--json` — output only the JSON format (see Reference). No progress indicators, no emoji, no prose.

## Instructions

0. If `.edikt/config.yaml` does not exist, output:
   ```
   No edikt config found. Run /edikt:init to set up this project.
   ```
   And stop.

0a. **Pre-v0.6.0 sentinel gate (ADR-027).** Before any other work, refuse to run when legacy in-body sentinels remain in the project. v0.6.0 reads sidecars only — there is no double-parser fallback (per ADR-027).

    Scan for the marker `[edikt:directives:start]: #` outside fenced regions and outside the documentation skip-list (`ADR-008-*`, `ADR-009-*`, `SPEC-*`). The `edikt` binary handles fence detection and skip-list correctly:

    ```bash
    edikt migrate sidecars --dry-run > /tmp/edikt-sidecar-precheck.out 2>&1
    PRECHECK_EXIT=$?
    ```

    - If `PRECHECK_EXIT == 0` AND output contains `0 sidecars to create` — no migration pending, continue to Step 1.
    - Otherwise — refuse with a single-line actionable error and exit 1:
      ```
      ✗ Migration required. Run /edikt:upgrade to migrate this project to v0.6.0 sidecar architecture (ADR-027).
      ```
      Do NOT print the dry-run plan here — `/edikt:upgrade` shows it. Keep this gate's output to one line so CI logs stay readable.

    NEVER fall back to in-body sentinel parsing. The pre-flight gate is the only path for legacy projects.

0b. If `--json` is in `$ARGUMENTS`, output only the JSON format at the end — no progress indicators, no emoji, no prose.

1. Display progress: `Step 1/5: Reading source documents...`

1b. Read the edikt version from `~/.edikt/VERSION`. If this file doesn't exist, fall back to `edikt_version:` in `.edikt/config.yaml`. If BOTH differ (e.g., VERSION says 0.2.3 but config says 0.3.0), warn:
   ```
   ⚠ ~/.edikt/VERSION (0.2.3) differs from .edikt/config.yaml edikt_version (0.3.0).
     The compiled_by stamp will use ~/.edikt/VERSION. To update, re-run the installer:
     curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
   ```
   Use `~/.edikt/VERSION` as the authoritative version for the `compiled_by` stamp.

2. Read `.edikt/config.yaml`. Resolve paths from the `paths:` section using the Path Defaults in the Reference section.

3. Read source documents:
   - **ADRs:** include if `status: accepted`. Skip `draft`, `superseded`, `deprecated`. Fall back to checking `**Status:** accepted` in the body for backwards compatibility.
   - **Invariants:** include if `status: active` or no status (backwards compatibility). Skip `status: revoked`.
   - **Guidelines:** include all `.md` files from the guidelines directory. No status filtering. Each filename (without `.md`) becomes the section label.

4. Display progress: `Step 2/5: Checking for contradictions...`

5. Detect contradictions between accepted ADRs: direct contradictions ("use X" vs "never use X"), scope conflicts, approach conflicts. Use the Contradiction Detection examples in the Reference section as a guide for how to report them.

6. Also check: superseded ADRs still referenced by active specs or plans; invariants that conflict with accepted ADRs; guidelines that conflict with ADRs or invariants. Conflicts between guidelines and invariants are errors (invariants always win). Conflicts between guidelines and ADRs are warnings.

7. If `--check` flag: report all contradictions and conflicts, then output the Check Output Format from the Reference section and stop (don't write).

8. If contradictions found and not `--check`: report them and ask user to proceed anyway or abort.

### Extract Directives

9. Display progress: `Step 3/5: Grouping directives by topic...`

10. For each source document, check for a directive sentinel block (`[edikt:directives:start]: #` ... `[edikt:directives:end]: #`).

11. **If sentinel block exists:** read all three directive lists per the [ADR-008](../../docs/architecture/decisions/ADR-008-deterministic-compile-and-three-list-schema.md) three-list schema:

    - **`directives:`** — auto-generated by `/edikt:<artifact>:compile` from the source document body. Required.
    - **`manual_directives:`** — user-authored rules added by hand. Optional (defaults to empty list if absent). Preserved across `<artifact>:compile` regenerations.
    - **`suppressed_directives:`** — auto directives the user has rejected. Optional (defaults to empty list if absent). Applied as a filter here.

    Compute the **effective rule set** for this source document using the ADR-008 merge formula:

    ```
    effective_rules = (directives - suppressed_directives) ∪ manual_directives
    ```

    Where:
    - `-` is **set difference by exact string match** (a line in `directives:` is filtered out if the exact same string appears in `suppressed_directives:`)
    - `∪` is **set union preserving document order** (manual directives appended after the filtered auto directives, order preserved within each list)
    - Duplicate rules within the union are de-duplicated: if a line appears in both the filtered `directives:` and in `manual_directives:`, it lands once in the effective set.

    Only the `effective_rules` value is used downstream for topic-grouping, routing, and final governance writing. The raw lists are never merged into a single stored artifact — the split exists only in the source document, not in the compiled output.

    Also read `paths:` and `scope:` from the block if present. Hash metadata (`source_hash`, `directives_hash`, `compiler_version`) is NOT read here — those are `<artifact>:compile`'s concern, not `gov:compile`'s.

    Also read `reminders:` and `verification:` lists if present. These are generated by `<artifact>:compile` (v0.3.0+) and aggregated into the governance.md index's `## Reminders` and `## Verification Checklist` sections. If absent (legacy blocks), skip — these sections will simply have fewer items.

    **SPEC-005 extension (v0.6.0+): `canonical_phrases` and `behavioral_signal` fields.** The sentinel block may contain two additional optional keys introduced by SPEC-005:

    - **`canonical_phrases:`** — short substrings (ideally 2–5 words) that a compliant model refusal will echo verbatim. Preserved across recompilations like `manual_directives`. Used by `/edikt:gov:review` (static directive-quality checks) and `/edikt:gov:benchmark` (dynamic directive-enforcement test). Missing = empty list `[]`, never an error.
    - **`behavioral_signal:`** — nested mapping describing observable compliance in tool-call terms. Keys:
      - `refuse_to_write:` (list of string substrings) — paths the model must not pass to Write / Edit. Substring matching only; glob / trailing-wildcard not supported.
      - `refuse_tool:` (list of tool names) — tools the model must not invoke (values from the Claude Code tool list).
      - `cite:` (list of directive IDs, e.g. `ADR-012`) — IDs the model must reference in a refusal.
      - `refuse_edit_matching_frontmatter:` (nested mapping with `path_glob`, `frontmatter_key`, `frontmatter_value`) — a structural predicate for "refuse Edit on files whose frontmatter matches." Specifically covers INV-002 ("ADRs are immutable once accepted").

      Missing = empty dict `{}`, never an error.

    `gov:compile` preserves `canonical_phrases` and `behavioral_signal` across recompilations identically to `manual_directives` — `<artifact>:compile` MUST NOT overwrite them. Neither field is merged into `effective_rules` (they are metadata describing directives, not directives themselves). They travel alongside the source document for consumers to read directly.

    **Security: path-traversal rejection.** `behavioral_signal.refuse_to_write` entries containing `..`, `~/`, or starting with `/` are rejected at parse time (absolute paths and parent-directory traversal cannot be substrings — they are structural metacharacters that would let a malicious or careless ADR author induce the benchmark runner into writes outside its sandbox). Rejection is a parse error with a clear message naming the offending entry. The check is implemented in `validate_behavioral_signal()` at `test/integration/governance/test_adr_sentinel_integrity.py`, reused by both the parser and the benchmark.

    **Backward compatibility (v0.2.x blocks):** if the sentinel block has only `directives:` (no `manual_directives:`, `suppressed_directives:`, `canonical_phrases:`, or `behavioral_signal:`), treat the missing lists as empty and proceed. Do NOT error, do NOT warn — legacy blocks continue to work, they just don't benefit from the user override mechanism until `<artifact>:compile` rewrites them into the new schema on the next body change.

    **Within-artifact contradiction detection:** while computing effective_rules for a single source document, check for internal contradictions between `directives:`, `manual_directives:`, and `suppressed_directives:`:
    - A directive and a manual directive that contradict each other (e.g., "use Redis" and "never use Redis") → warn but include both in the effective set. The user should resolve manually.
    - A line in `suppressed_directives:` that doesn't match any current `directives:` line → warn: the suppression may be stale and could be removed from `suppressed_directives:` by the user.
    - A line present in both `directives:` and `manual_directives:` → silent de-dup (no warning, just collapse).

    This is the **primary path** — no extraction, no distillation. The `<artifact>:compile` commands are responsible for keeping `directives:` current; `gov:compile` trusts that output and applies only the user overrides.

12. **If no sentinel block (or schema-incomplete blocks):** auto-chain to the per-artifact compile commands, then re-run the gate.

    `gov:compile` does NOT generate sentinels itself — that responsibility stays with `<artifact>:compile` per ADR-008 (which owns the `source_hash` / `directives_hash` / `compiler_version` writes). But `gov:compile` MAY orchestrate: if it detects missing or schema-incomplete sentinels, it invokes the matching per-artifact compile commands in order, then re-runs the schema completeness gate (Step 12a). The actual writes still happen inside `<artifact>:compile` — `gov:compile` is the conductor, not the writer.

    **Auto-chain rule.** Group missing/incomplete documents by artifact type. For each non-empty type, invoke its compile command via the SlashCommand tool:

    ```
    Missing/incomplete groupings:
      ADR(s):        {n_adr}
      Invariant(s):  {n_inv}
      Guideline(s):  {n_gl}

    Auto-running per-artifact compile to populate the v0.5.0+ schema:
      → /edikt:adr:compile           (no-arg = recompile all accepted ADRs)
      → /edikt:invariant:compile     (no-arg = recompile all active invariants)
      → /edikt:guideline:compile     (no-arg = recompile all guidelines)

    After each completes, re-run Step 11 (parse sentinels) and Step 12a
    (schema completeness gate). If the gate now passes, continue to Step 13.
    If sentinels are STILL missing or incomplete after the chain, abort
    with the full list — at that point the per-artifact compile failed
    on its own, which is a separate bug to investigate.
    ```

    **Telling the user what's happening.** Before the chain, print a clear preview so the user understands the LLM phase isn't gov:compile silently doing magic:

    ```
    ℹ Detected legacy schema. First-run migration will invoke:
       → /edikt:adr:compile         ({n_adr} ADRs)
       → /edikt:invariant:compile   ({n_inv} invariants)
       → /edikt:guideline:compile   ({n_gl} guidelines)

       After this, gov:compile resolves any remaining missing topic: fields,
       then renders governance.md. Future runs are deterministic and <100ms.
    ```

    After each `<artifact>:compile` completes, print a one-line summary (e.g. `→ adr:compile: 22 ADRs migrated`).

    **Headless mode.** When `EDIKT_HEADLESS=1` or no `/dev/tty` is attached, the auto-chain is **disabled** — print the same actionable error the prior version emitted (the explicit run-these-three list) and exit non-zero. Headless callers (CI, scripts) must opt in to the migration phase explicitly so they're never surprised by an LLM-cost spike on a path that used to be deterministic.

    **Why this preserves ADR-008.** The "gov:compile is read-only with respect to source documents" invariant is about *who writes to which file*, not about *who triggers what*. Each `<artifact>:compile` invocation does its own reads + writes, with its own hash gates and its own interview flow. `gov:compile` calling them is no different from a human running them in sequence.

### Schema Completeness Gate

12a. **Before grouping, validate every parsed sentinel block against the ADR-008 schema.**

This is defense-in-depth: even though step 12 redirects no-sentinel cases to per-artifact compile, a stale or partially-written block can still land here. The compile MUST refuse to claim success on incomplete input.

For each parsed sentinel block, verify the following fields are present (presence-check only — content validation happens elsewhere):

- `source_hash` — required by ADR-008
- `directives_hash` — required by ADR-008
- `compiler_version` — required by ADR-008
- `manual_directives` — required as an empty list `[]` when not in use
- `suppressed_directives` — required as an empty list `[]` when not in use
- `topic` — required so the deterministic Go binary helper can group without invoking an LLM (per ADR-020 §c). When missing, the auto-chain in Step 12 routes to per-artifact compile, which generates the topic.

A block missing any of the above is **incomplete** under the v0.5.0+ schema and indicates either a stale `<artifact>:compile` run (old code) or a hand-authored block missing required fields. Either way, `gov:compile` MUST NOT silently produce a `governance.md` derived from incomplete inputs.

Run the following Python script. It takes the parsed sentinel blocks via `EDIKT_BLOCKS_JSON`, identifies any that are missing one or more required fields under the ADR-008 schema, and emits a deterministic error report. Exit code 0 = all complete. Exit code 2 = at least one block incomplete.

```bash
python3 - <<'PY'
import json
import os
import sys

# ── Inputs ──────────────────────────────────────────────────────────────────
# EDIKT_BLOCKS_JSON: JSON array of {path: str, fields: list[str]}
#   path   — source document path (string)
#   fields — names of YAML keys present in the parsed sentinel block
#
# Example:
#   [{"path": "docs/architecture/decisions/ADR-001.md",
#     "fields": ["paths","scope","directives","manual_directives",
#                "suppressed_directives","source_hash","directives_hash",
#                "compiler_version"]}]

raw = os.environ.get("EDIKT_BLOCKS_JSON", "").strip()
if not raw:
    # No blocks to validate (e.g., project has no source documents). Pass.
    sys.exit(0)

try:
    blocks = json.loads(raw)
except json.JSONDecodeError as e:
    print(f"✗ EDIKT_BLOCKS_JSON is not valid JSON: {e}", file=sys.stderr)
    sys.exit(2)

if not isinstance(blocks, list):
    print("✗ EDIKT_BLOCKS_JSON must be a JSON array.", file=sys.stderr)
    sys.exit(2)

# ── ADR-008 + ADR-020 §c required fields ────────────────────────────────────
REQUIRED = (
    "source_hash",
    "directives_hash",
    "compiler_version",
    "manual_directives",
    "suppressed_directives",
    "topic",
)

incomplete = []
for entry in blocks:
    if not isinstance(entry, dict):
        print(f"✗ block entry is not a JSON object: {entry!r}", file=sys.stderr)
        sys.exit(2)
    path = entry.get("path", "<unknown>")
    fields = entry.get("fields", [])
    if not isinstance(fields, list):
        print(f"✗ block fields for {path!r} must be a list", file=sys.stderr)
        sys.exit(2)
    present = set(str(f) for f in fields)
    missing = [k for k in REQUIRED if k not in present]
    if missing:
        incomplete.append((path, missing))

if not incomplete:
    sys.exit(0)

# ── Render error report ─────────────────────────────────────────────────────
n = len(incomplete)
plural = "s" if n != 1 else ""
print(f"✗ {n} sentinel block{plural} are missing required fields.")
print()
print("  ADR-008 requires every directives block to carry source_hash,")
print("  directives_hash, compiler_version, manual_directives, and")
print(f"  suppressed_directives. The block{plural} below {'is' if n == 1 else 'are'} incomplete:")
print()
for path, missing in incomplete:
    print(f"    {path}: missing [{', '.join(missing)}]")
print()
print("  Re-run the matching <artifact>:compile to regenerate under the")
print("  current schema:")
print()
print("    /edikt:adr:compile {ADR-NNN}")
print("    /edikt:invariant:compile {INV-NNN}")
print("    /edikt:guideline:compile {slug}")
print()
print("  Then re-run /edikt:gov:compile.")

sys.exit(2)
PY
```

Like step 12, treat this as a complete-the-list failure: do not partial-write `governance.md`.

**Why this lives here:** the parse pass (step 11) reads only the *content* fields needed for the merge formula (`directives`, `manual_directives`, `suppressed_directives`). The hash/metadata fields are explicitly NOT read at step 11 — that's `<artifact>:compile`'s concern. But `gov:compile` is the right place to *audit* their presence, because it's the seam where source documents hand off to the compiled output.

### Validate Cross-References

12b. For every extracted directive that references a specific invariant ID (INV-NNN), ADR ID (ADR-NNN), or other named artifact — verify the reference exists in the source document. Read the source file and confirm the referenced identifier appears in it. If it does not appear, strip the fabricated reference from the directive (keep the directive text if it's otherwise accurate). Never include a cross-reference that hasn't been confirmed in the actual source file.

### Directive-Quality Pass

12c. **After** the contradiction-detection pass (steps 5–8) and **before** grouping, run the shared directive-quality sub-procedure from `commands/gov/_shared-directive-checks.md` for every accepted ADR and active invariant.

For each source document that has a parsed sentinel block, run the inline Python script from `_shared-directive-checks.md §Inline Script` once per directive in `directives:` and once per directive in `manual_directives:`. Pass:

```json
{
  "adr_id": "<ADR-NNN or INV-NNN>",
  "directive_body": "<directive line text>",
  "canonical_phrases": ["<phrase1>", ...],
  "no_directives_reason": "<reason string or null>"
}
```

Collect all returned warning lines across all documents. If any warnings were produced, output them under a `### Directive-quality warnings` header in the compile output:

```
### Directive-quality warnings

[WARN] ADR-012: directive has 2 sentences but no canonical_phrases — run /edikt:adr:review --backfill
[WARN] ADR-014: canonical_phrase "atomic rename" not found in directive body
```

**AC-021 grace period:** exit 0 even when warnings are present. Do NOT block compilation due to directive-quality warnings in v0.6.0. The header is surfaced so users are aware; it is not an error.

If no warnings were produced, skip the header entirely (do not emit an empty section).

### Orphan Detection Pass

12d. **After** the directive-quality pass (step 12c), run the orphan-detection and history-comparison pass. This implements FR-004 / AC-003 / AC-003b / AC-017 / AC-018 / AC-019.

**Two-layer atomicity model:**
- **Outer layer:** the compile operation as a whole is serialized by the existing `lock.yaml + flock` pattern in `bin/edikt` (SPEC-004 §8). This prevents two concurrent compiles from racing on source files or the governance output. Phase 7 does not change this layer.
- **Inner layer:** the state file `.edikt/state/compile-history.json` is protected specifically from torn writes by using write-to-tempfile + `os.rename()`. If the process crashes between the write and the rename, the previous state file remains intact — safe toward re-warning rather than a silent skip. The `.tmp` file may exist and is safe to remove manually.

#### Pass 1: Orphan collection

Walk all accepted ADRs and active invariants. For each one, check whether:
- The parsed `directives` list AND `manual_directives` list are both empty (i.e., the effective rule set produces zero directives), AND
- The source document's frontmatter does NOT contain a `no-directives:` key with a valid reason.

A "valid reason" is defined by `_shared-directive-checks.md §Check C`: ≥ 10 characters, not in `{tbd, todo, fix later}` (case-insensitive), non-empty after strip.

If both conditions are true, add the ADR/INV ID to the **current orphan set**.

#### Pass 2: History comparison and write

Run the following Python script. It implements the five orphan-set transition rules and atomically writes the state file:

```bash
python3 - <<'PY'
import json
import os
import sys
import re
import pathlib
from datetime import datetime, timezone

# ── Inputs injected by the caller ──────────────────────────────────────────
# current_orphan_ids: list[str]  — e.g. ["ADR-012", "INV-003"]
# history_path: str              — abs path to .edikt/state/compile-history.json
# edikt_version: str             — optional, for the edikt_version stamp

current_orphan_ids_raw = os.environ.get("EDIKT_ORPHAN_IDS", "").strip()
current_orphan_ids = sorted(x for x in current_orphan_ids_raw.split(",") if x.strip())
history_path = os.environ.get("EDIKT_HISTORY_PATH", "")
edikt_version = os.environ.get("EDIKT_VERSION", "")

HISTORY_FILE = pathlib.Path(history_path) if history_path else None

# ── Load prior history ──────────────────────────────────────────────────────
prior_orphans = None          # None means: no usable history (absent or corrupt)
history_loadable = True

if HISTORY_FILE and HISTORY_FILE.exists():
    try:
        raw = HISTORY_FILE.read_text(encoding="utf-8")
        data = json.loads(raw)
        if isinstance(data, dict) and isinstance(data.get("orphan_adrs"), list):
            prior_orphans = sorted(str(x) for x in data["orphan_adrs"])
        else:
            history_loadable = False
    except (json.JSONDecodeError, OSError, ValueError):
        history_loadable = False

if not history_loadable:
    print("[WARN] compile-history.json is unparseable — treating as absent (first detection)", flush=True)

current_set = set(current_orphan_ids)
prior_set   = set(prior_orphans) if prior_orphans is not None else None

# ── Determine the transition scenario ──────────────────────────────────────
#
# Five scenarios (per SPEC-005 Phase 7):
#
#   1. First detection  — no prior history (prior_set is None)
#      → warn, exit 0, write history
#
#   2. Consecutive same — prior_set exists AND current_set == prior_set
#      → BLOCK, exit ≠ 0, do NOT overwrite history
#
#   3. Subset / different — prior_set exists AND current_set ⊂ prior_set
#      AND current_set != prior_set  (some orphans resolved)
#      → "changed, reset to first-detection", warn, write new set, exit 0
#
#   4. Superset — prior_set exists AND current_set ⊃ prior_set
#      (new orphans added)
#      → "changed → first-detection", warn, write new set, exit 0
#
#   5. Fallthrough (intersecting sets, neither sub/superset, changed)
#      → treat same as scenario 1/3 — warn, write new set, exit 0

should_block  = False
should_write  = True
scenario_note = ""

if not current_set:
    # No orphans this run — write a clean history and exit 0.
    scenario_note = "no orphans"
    should_write = True
    should_block = False
elif prior_set is None:
    # Scenario 1: first detection (history absent or corrupt)
    scenario_note = "first detection"
    should_block = False
    should_write = True
elif current_set == prior_set:
    # Scenario 2: consecutive — same orphan set → BLOCK
    scenario_note = "consecutive"
    should_block = True
    should_write = False          # do NOT overwrite so next fix attempt is compared against this baseline
elif current_set < prior_set:
    # Scenario 3: subset — orphans resolved, reset to first-detection
    scenario_note = "changed (subset) → reset to first-detection"
    should_block = False
    should_write = True
elif current_set > prior_set:
    # Scenario 4: superset — new orphans added → first-detection for new set
    scenario_note = "changed (superset) → first-detection"
    should_block = False
    should_write = True
else:
    # Scenario 5: sets differ but neither is a sub/superset → first-detection
    scenario_note = "changed (different set) → first-detection"
    should_block = False
    should_write = True

# ── Emit warnings for current orphans ──────────────────────────────────────
if current_set and not should_block:
    print("\n### Orphan ADR warnings\n", flush=True)
    for oid in sorted(current_set):
        print(f"[WARN] {oid}: accepted ADR/INV has zero directives and no no-directives reason", flush=True)
    print(
        "\nFix options for each orphan ADR/INV:\n"
        "  1. Add directives to the sentinel block and run /edikt:gov:compile\n"
        "  2. Add `no-directives: \"<reason ≥ 10 chars>\"` to the frontmatter\n"
        "  3. Revert the ADR to draft status if the decision is not yet ready\n"
        f"\nScenario: {scenario_note}. "
        "This compile exits 0 — the SAME orphan set on the next compile will block.",
        flush=True,
    )

elif current_set and should_block:
    print("\n### Orphan ADR BLOCK\n", flush=True)
    for oid in sorted(current_set):
        print(f"[BLOCK] {oid}: consecutive compile with same orphan set — compilation blocked", flush=True)
    print(
        "\nFix options for each blocked ADR/INV:\n"
        "  1. Add directives to the sentinel block and run /edikt:gov:compile\n"
        "  2. Add `no-directives: \"<reason ≥ 10 chars>\"` to the frontmatter\n"
        "  3. Revert the ADR to draft status if the decision is not yet ready\n"
        "\nThe orphan set has not changed since the last compile. Compilation is blocked.",
        flush=True,
    )

# ── Write state file (atomic rename) ───────────────────────────────────────
if should_write and HISTORY_FILE:
    state_dir = HISTORY_FILE.parent
    try:
        state_dir.mkdir(parents=True, exist_ok=True)
    except OSError as exc:
        print(f"[WARN] Could not create state directory {state_dir}: {exc}", flush=True)
        sys.exit(1 if should_block else 0)

    payload = {
        "schema_version": 1,
        "last_compile_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "orphan_adrs": sorted(current_set),
    }
    if edikt_version:
        payload["edikt_version"] = edikt_version

    tmp_path = pathlib.Path(str(HISTORY_FILE) + ".tmp")
    try:
        tmp_path.write_text(json.dumps(payload, indent=2), encoding="utf-8")
        os.rename(str(tmp_path), str(HISTORY_FILE))
    except OSError as exc:
        print(
            f"[WARN] Could not atomically write compile-history.json: {exc}\n"
            f"       Previous state file is unchanged. Remove {tmp_path} if it exists.",
            flush=True,
        )
        # Exit normally — do not block on a write failure.
        sys.exit(0)

# ── Exit code ───────────────────────────────────────────────────────────────
sys.exit(1 if should_block else 0)
PY
```

**How to invoke this script from within the compile procedure:**

Set environment variables before running:
```
EDIKT_ORPHAN_IDS=<comma-separated list of orphan IDs, or empty>
EDIKT_HISTORY_PATH=<absolute path to .edikt/state/compile-history.json>
EDIKT_VERSION=<version string from step 1b, optional>
```

If the script exits non-zero (scenario 2 — consecutive block), the overall compile command MUST exit non-zero and MUST NOT proceed to write governance output.

If the script exits 0, continue to step 13 (group by topic).

**AC-019 — `.gitignore` management:**

After the orphan script completes (regardless of exit code), run:

```bash
python3 - <<'PY'
import os, pathlib, sys

project_root = os.environ.get("EDIKT_PROJECT_ROOT", os.getcwd())
gitignore_path = pathlib.Path(project_root) / ".gitignore"
entry = ".edikt/state/"

if gitignore_path.exists():
    content = gitignore_path.read_text(encoding="utf-8")
    # Normalize: treat ".edikt/state" (no trailing slash) as already-present
    for variant in [".edikt/state/", ".edikt/state"]:
        if variant in content.splitlines() or entry.rstrip("/") in content.splitlines():
            sys.exit(0)
    # Not found — append
    newline_prefix = "\n" if content and not content.endswith("\n") else ""
    gitignore_path.write_text(content + newline_prefix + entry + "\n", encoding="utf-8")
    print(f"[OK] Appended '{entry}' to .gitignore", flush=True)
else:
    gitignore_path.write_text(entry + "\n", encoding="utf-8")
    print(f"[OK] Created .gitignore with '{entry}' entry", flush=True)

sys.exit(0)
PY
```

Set `EDIKT_PROJECT_ROOT` to the root of the project being compiled (the directory containing `.edikt/`).

**AC-019 note:** The `.gitignore` handler checks for both `.edikt/state/` (with trailing slash) and `.edikt/state` (without) before appending, to avoid duplicates under trailing-slash normalization variants. If the file is absent, it is created. If the entry is already present in any recognized form, the file is left unchanged.

### Group by Topic

13. Analyze all **effective_rules** across all source documents (computed in step 11 via the three-list merge formula) and group them by topic. A topic is a domain area — caching, database, multi-tenancy, authentication, file storage, architecture (cross-cutting), etc.

    Grouping rules:
    - Effective rules from different sources about the same domain go into the same topic file
    - Each rule keeps its source reference (`ref: ADR-008, §Eviction`) whether it originated from `directives:` or `manual_directives:` — readers of governance.md cannot tell which list a rule came from, only which source document it references
    - If a rule doesn't fit an obvious topic, group it under `architecture.md` (cross-cutting)
    - Invariants are special — their effective rules go into `governance.md` (the index), not into topic files
    - Across source documents: if the same rule string appears in multiple effective sets, de-duplicate by exact string match. Keep the first occurrence's source reference.

13a. **CRITICAL — write the resolved `topic:` back to each source document's sentinel block.** Per ADR-020 §c, the LLM grouping is a one-shot fallback; the resolved topic MUST be persisted into the artifact's sentinel so subsequent runs are deterministic and the Go binary helper can group without invoking an LLM.

    For every source document whose sentinel block lacks a `topic:` field, edit the artifact in place to add the assigned topic to its sentinel YAML. The line goes ABOVE `directives:` for readability:

    ```yaml
    [edikt:directives:start]: #
    <!-- edikt:directives — auto-generated, do not edit manually -->
    source_hash: <unchanged>
    directives_hash: <unchanged>
    compiler_version: <unchanged>
    topic: <assigned-topic-slug>          ← NEW LINE
    paths:
      - "**/*"
    scope: [planning, design]
    directives:
      - ...
    manual_directives: []
    suppressed_directives: []
    [edikt:directives:end]: #
    ```

    NEVER overwrite a `topic:` field that the user (or a prior run) has already set — only add when missing. Topic slugs MUST be lowercase kebab-case (`ai-processing`, `database`, `frontend`). Reuse an existing topic name if any other artifact already routes to it; only invent a new slug when no existing topic fits.

    After all writes, log a summary line per artifact:

    ```
    → wrote topic: ai-processing → docs/architecture/decisions/ADR-001-...md
    → wrote topic: frontend → docs/architecture/decisions/ADR-002-...md
    ...
    ```

    This step makes the difference between "first run is LLM-driven, future runs deterministic" (correct) and "every run is LLM-driven" (regression vs ADR-020).

13b. **While writing back `topic:`, also write back `signals:`.** During the LLM grouping pass for an artifact lacking `topic:`, also derive 4–12 routing keywords from the artifact body (concrete domain nouns, tool names, feature terms — same rubric as `<artifact>:compile`). Persist them to the same sentinel block as a `signals:` list. The Go binary aggregates these per topic into the routing-table row, eliminating the need for a hardcoded `topic→signals` map.

    If the artifact already has a non-empty `signals:` list, preserve it verbatim — only emit when missing.

13c. **Re-emit `source_hash` and `directives_hash` after the topic-write and signals-write.** Adding fields to the sentinel changes the file body; without re-hashing, the next `<artifact>:compile` run will see a stale hash and trigger an unnecessary interview. Recompute both hashes and update the sentinel.

### Derive Path Patterns

14. Display progress: `Step 4/5: Scanning codebase for path patterns...`

15. For each topic file, determine the `paths:` frontmatter:
    - **If pinned in sentinel block:** use the author's paths verbatim
    - **If not pinned:** scan the project directory structure to find where code related to this topic lives. Generate glob patterns matching those locations.
    - Use the `paths:` YAML list format (one glob per line)

16. For each topic file, determine the `scope:` metadata for the routing table:
    - **If specified in sentinel block:** use the author's scopes
    - **If not specified:** derive from content — architecture/cross-cutting decisions get `[planning, design, review]`, implementation-specific rules get `[implementation]`

### Write Output

17. Display progress: `Step 5/5: Writing governance files...`

18. Write topic rule files to `.claude/rules/governance/`:

    Each file follows this format:
    ```markdown
    ---
    paths:
      - "**/*.go"
      - "**/adapters/postgres/**"
    compile_schema_version: 2
    ---
    <!-- edikt:compiled — generated by /edikt:gov:compile, do not edit manually -->
    <!-- topic: {topic name} -->
    <!-- sources: {list of source documents that contributed} -->
    <!-- compiled_by: edikt v{edikt_version} -->
    <!-- compiled_at: {ISO8601 timestamp} -->

    # {Topic Name}

    - {directive} (ref: {source})
    - {directive} (ref: {source})
    ```

19. Write the governance index to `.claude/rules/governance.md`:

    ```markdown
    ---
    paths: "**/*"
    compile_schema_version: 2
    ---
    <!-- edikt:compiled — generated by /edikt:gov:compile, do not edit manually -->
    <!-- compiled_by: edikt v{edikt_version} -->
    <!-- compiled_at: {ISO8601 timestamp} -->

    # Governance Directives

    Follow these directives in every file you write or edit.

    ## Non-Negotiable Constraints

    These are invariants. Violation is never acceptable.

    - {invariant directive} (ref: INV-NNN)

    ## Routing Table

    Before starting a task, scan this table. If your task matches any signals or scopes, read the referenced file.

    | Signals | Scope | File |
    |---|---|---|
    | {keywords} | {scope list} | `governance/{topic}.md` |

    ## Reminders

    Before acting, check the relevant constraint.

    [edikt:reminders:start]: #
    {Aggregate all `reminders:` lists from all source document sentinel blocks.
     De-duplicate by exact string match. Cap at 10 reminders total.
     Format: "- Before {action} → {check} (ref: ID)"}
    [edikt:reminders:end]: #

    ## Verification Checklist

    Before finishing, verify each item. If any fails, fix before submitting.

    {Aggregate all `verification:` lists from all source document sentinel blocks.
     De-duplicate by exact string match. Cap at 15 items total.
     Format: "- [ ] {what to check} (ref: ID)"}

    ## Reminder: Non-Negotiable Constraints

    These constraints were listed above and are restated for emphasis.
    Do not violate them under any circumstances.

    - {repeat invariant directives}
    ```

20. If the compiled output detects an existing flat `governance.md` (old format with `edikt:compiled` marker but no `governance/` directory), this is a migration:
    - Create the `governance/` directory
    - Generate topic files from the old directives
    - Replace the old `governance.md` with the new index format
    - Report the migration:
      ```
      📦 Migrated from flat governance.md to topic-grouped rule files.
         Old format: 1 file, {n} directives
         New format: {m} topic files + index
      ```

21. If any single topic file exceeds 100 directives, warn:
    ```
    ⚠ {topic}.md has {count} directives. Large rule files may dilute compliance.
      Consider splitting into subtopics or running /edikt:gov:review to tighten language.
    ```

22. Log the compilation event:
    ```bash
    source "$HOME/.edikt/hooks/event-log.sh" 2>/dev/null
    edikt_log_event "compile" '{"adrs_compiled":{n},"invariants_compiled":{m},"guidelines_compiled":{g},"topics":{t},"total_directives":{total},"sentinel_coverage":"{pct}%"}'
    ```

23. Output the compilation summary with reverse source map:
    ```
    ✅ Governance compiled

      governance/{topic}.md
        ← {source document} ({sections contributed})
        ← {source document} ({sections contributed})

      governance/{topic}.md
        ← {source document} ({sections contributed})

      ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
      {n} ADRs + {m} invariants + {g} guidelines
      → {t} topic files + index
      → {total} total directives
      Sentinel coverage: {with_sentinels}/{total_sources} documents ({pct}%)

      {If generated_sentinels > 0}:
      ⚙ {generated_sentinels} sentinel blocks generated and written to source documents.
        Run /edikt:gov:review to review language quality.

      Claude will read these directives automatically in every session.

      Next: Run /edikt:gov:review to review directive language quality.
    ```

24. This command should be suggested (not auto-run) after `/edikt:adr:new` or `/edikt:invariant:new` creates or modifies a document. Add to those commands' output: `Run /edikt:gov:compile to generate directive sentinels and update governance.`

---

REMEMBER: NEVER write governance files with contradictions. Invariants go in the index (governance.md), not topic files — they are always loaded. Topic files contain domain-specific directives grouped from all sources. The ADRs and source documents are the source of truth — compiled output is the enforcement format, never hand-edit it.

## Reference

### Path Defaults

| Key | Default |
|---|---|
| `paths.decisions` | `docs/architecture/decisions` |
| `paths.invariants` | `docs/architecture/invariants` |
| `paths.guidelines` | `docs/guidelines` |

### Fallback Directive Extraction Rules

Used only when a source document has no `[edikt:directives:start/end]` sentinel block.

**From ADRs** — read the `## Decision` section. Extract all enforceable statements. Preserve specifics (namespaces, patterns, thresholds, tool names). Drop rationale, context, alternatives. Each statement becomes one directive.

Example transformation:
```
ADR source (150 lines):
  # ADR-001 — edikt: Context Engine and Guardrail Installer
  ## Decision
  Build edikt as a lean context engine targeting Claude Code exclusively.
  Other tools lack path-conditional rules, hooks, slash commands...
  [... 100 more lines of rationale, alternatives, consequences ...]

Compiled directive (1 line):
  - Claude Code is the only supported platform. Do not write code or
    configuration targeting Cursor, Copilot, or other tools. (ref: ADR-001)
```

**From invariants** — directives are already constraint-shaped; use the Rule section directly:
```
Invariant source:
  # INV-001 — Commands are plain markdown, no compiled code
  ## Rule
  Every edikt command is a .md file. No TypeScript, no compiled binaries...

Compiled directive:
  - Every command and template must be a .md or .yaml file. No TypeScript,
    no compiled binaries, no build step. This constraint is non-negotiable.
    (ref: INV-001)
```

**From guidelines** — each file becomes a set of directives. Guidelines are freeform; extract enforceable bullet points.

### Contradiction Detection Examples

```
⚠️  Contradiction detected:
    ADR-001: "Claude Code only — no multi-tool support"
    ADR-007: "Support Cursor for rule distribution"

    Resolve before compiling. Supersede one or reconcile both.
```

```
⚠️  Conflict between guideline and ADR:
    guidelines/testing.md: "Always mock the database in all tests"
    ADR-003: "Integration tests must hit a real database, no mocks"

    Source: guidelines/testing.md (line 12) vs ADR-003 (Decision section)
    Action: Scope the guideline to unit tests only, or amend ADR-003.
```

```
⚠️  Conflict between guideline and invariant:
    guidelines/dependencies.md: "Use lodash for utility functions"
    INV-001: "No runtime dependencies"

    Source: guidelines/dependencies.md (line 5) vs INV-001 (Rule section)
    Action: Remove the guideline — invariants are non-negotiable.
```

### JSON Output Format

```json
{
  "status": "success",
  "topics": [{"name": "cache", "file": "governance/cache.md", "directives": 12, "sources": ["ADR-008", "guideline-database.md"]}],
  "invariants": [{"id": "INV-001", "directive": "..."}],
  "sentinel_coverage": {"with": 5, "total": 7, "percent": 71},
  "contradictions": [],
  "total_directives": 27
}
```

### Check Output Format

```
/edikt:gov:compile --check

  Sources: {n} ADRs ({m} accepted), {j} invariants ({l} active), {g} guidelines
  Sentinel coverage: {with_sentinels}/{total} documents
  Contradictions: {count}
  Conflicts: {count} (guideline vs ADR/invariant)
  Topics: {count} would be generated
  Directives: {count} would be generated

  {If contradictions: list them}
  {If clean: "All clear — governance compiles cleanly."}
```

---
name: invariant:compile
description: "Regenerate the sidecar for one Invariant Record (or all active invariants) via the sidecar-extractor agent"
effort: normal
argument-hint: "[INV-NNN | path] — omit to process all active invariants"
allowed-tools:
  - Read
  - Write
  - Glob
  - Grep
  - Bash
  - Agent
---

# edikt:invariant:compile

Regenerate the directive sidecar (`<INV-NNN>.edikt.yaml`) for one Invariant Record — or for every active invariant if no argument is given.

Per the relevant decision (sidecar architecture), directive metadata for every active Invariant Record lives in a co-located `<INV-NNN>.edikt.yaml` sidecar conforming to `templates/schemas/gov-sidecar.v2.schema.json` (v2). This command never writes to the parent `.md`. It dispatches the locked `sidecar-extractor` agent which reads the parent body and writes the sidecar.

## Arguments

- `$ARGUMENTS` — optional. One of:
  - `INV-NNN` (e.g., `INV-NNN`) — resolve to `{invariants_dir}/INV-NNN-*.md`
  - An absolute or repo-relative path to an invariant markdown file
  - Empty / omitted — regenerate every active invariant's sidecar

## Instructions

### 0. Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### 1. Config

Read `.edikt/config.yaml` to resolve `invariants_dir` (default `docs/architecture/invariants`).

### 2. Resolve target(s)

- If `$ARGUMENTS` matches `INV-NNN`, find `{invariants_dir}/INV-NNN-*.md`. If multiple match, error out asking the user to disambiguate by full path. If none match, error out with `error: no invariant matches {ARGUMENTS}`.
- If `$ARGUMENTS` is a path, use it directly. Verify the file exists and is under `{invariants_dir}`.
- If `$ARGUMENTS` is empty, list every `INV-*.md` in `{invariants_dir}` whose frontmatter has `status: active` (or no `status:` field — treated as active for backwards compatibility).

### 2b. Ensure the sidecar schema is resolvable locally

The extractor resolves the `gov-sidecar.v2` schema project-locally first. Inside the edikt repo `templates/schemas/` exists; in a consumer project it does not — the payload lives under the install root. Copy the schema into the project once so both the extractor and the user's editor can resolve it:

```bash
test -f "$PROJECT_ROOT/.edikt/schemas/gov-sidecar.v2.schema.json" || {
  mkdir -p "$PROJECT_ROOT/.edikt/schemas"
  SOURCE_SCHEMA="$EDIKT_HOME/current/templates/schemas/gov-sidecar.v2.schema.json"
  [ -f "$SOURCE_SCHEMA" ] || SOURCE_SCHEMA="$HOME/.edikt/current/templates/schemas/gov-sidecar.v2.schema.json"
  [ -f "$SOURCE_SCHEMA" ] || SOURCE_SCHEMA="$PROJECT_ROOT/templates/schemas/gov-sidecar.v2.schema.json"
  [ -f "$SOURCE_SCHEMA" ] && cp "$SOURCE_SCHEMA" "$PROJECT_ROOT/.edikt/schemas/gov-sidecar.v2.schema.json"
}
```

This is best-effort — if no source schema is found, continue anyway. The extractor carries the full allowed-key contract in its own prompt and does not require the file. NEVER instruct the extractor to read `templates/schemas/...` as a project-relative path: that resolves only inside the edikt repo, and a consumer project's extractor will simply fail the read.

### 3. Dispatch the extractor

Before dispatching, record each target sidecar's pre-dispatch state so step 4 can distinguish "extractor rewrote the file" from "extractor wrote nothing":

```bash
stat -f '%m %z' "$SIDECAR" 2>/dev/null || stat -c '%Y %s' "$SIDECAR" 2>/dev/null || echo missing
```


**Dispatch AT MOST 2 extractor agents concurrently.** Measured (D19): three
`sidecar-extractor` agents dispatched in one message left two dead on the
600 s stream watchdog while one completed; the same two artifacts, re-run
sequentially against the identical prompt, both finished in 154 s and 250 s.
Two-wide is not merely safe — a pair completed in 92 s and 94 s, faster than
either sequential run. Artifact size does not predict it: a 9,290-byte
artifact succeeded where 1,900- and 2,527-byte ones stalled.

A stalled dispatch does not fail loudly. It drops that artifact and leaves
the remaining results looking complete, so any count taken across a >2-wide
fan-out has an unstated denominator.

For each target invariant:

Use the Agent tool:
- `subagent_type: sidecar-extractor`
- `prompt: "Extract sidecar from {ABS_PATH_TO_INV}"`

If the dispatch fails with `Agent type 'sidecar-extractor' not found` but `.claude/agents/sidecar-extractor.md` exists (installed this session), use the fallback in `commands/_shared-agent-routing.md` § "Fallback: agent installed this session".

The agent reads the invariant's body (specifically `## Statement` and `## Enforcement` sections), extracts directives + signals + topic, and writes `<basename>.edikt.yaml` next to it. Its final response is a single line: `SIDECAR WRITTEN: <relative-path>`.

### 4. Verify the artifact on disk — fail closed (INV-011)

**The agent's completion message is NOT evidence.** A dispatch that exits cleanly while writing zero files is the documented field failure mode (stale agent definitions, bok-services 2026-08-07) — and with a prior sidecar on disk it is otherwise indistinguishable from a correct no-op. Before ANY comparison:

```bash
SIDECAR="{path-to}.edikt.yaml"
# 1. The promised file must exist.
if [ ! -f "$SIDECAR" ]; then
  echo "❌ extraction FAILED for $SIDECAR — the extractor completed but wrote no sidecar."
  echo "   Known cause: a stale agent definition cached at session start — run 'edikt doctor' and restart the session."
  # Report this target as FAILED. NEVER report it as 'unchanged'.
fi
# 2. It must parse. PyYAML when available; a stdlib structural check
#    otherwise — a missing library must produce the RIGHT diagnosis,
#    never a false "does not parse" (audit 2026-08-07 #5).
python3 - "$SIDECAR" <<'PY' || echo "❌ extraction FAILED for $SIDECAR — sidecar exists but does not parse."
import sys
body = open(sys.argv[1], encoding="utf-8").read()
try:
    import yaml
    yaml.safe_load(body)
except ImportError:
    print("note: PyYAML unavailable — structural check only", file=sys.stderr)
    sys.exit(0 if "schema_version" in body and body.strip() else 1)
except Exception as e:
    print("parse error: " + str(e), file=sys.stderr)
    sys.exit(1)
PY
# 3. If a sidecar existed BEFORE dispatch (record its mtime in step 3),
#    an unchanged mtime+size means the extractor wrote NOTHING — that is
#    the zero-write failure, not idempotency. Report it as FAILED.
```

Only a sidecar that exists, parses, and was actually (re)written this run proceeds to the idempotency comparison.

### 4b. Detect idempotency

Compare the new sidecar to its prior version using canonical YAML serialization:

```bash
canonicalize() {
  python3 -c 'import yaml,sys; print(yaml.dump(yaml.safe_load(open(sys.argv[1]).read()), sort_keys=True, default_flow_style=False, width=200), end="")' "$1"
}
```

If the canonical form matches, report `unchanged`. Otherwise report `regenerated`.

The remaining source of a false `regenerated` report is the sidecar-extractor's *semantic* output: it is not byte-deterministic across LLM runs. The Go-side canonical serializer has shipped (`internal/sidecar/marshal.go`); the residual variance here is the LLM extraction, not serialization.

### 5. Confirm

For a single-target run:
```
✅ {INV-NNN}.edikt.yaml — {regenerated | unchanged}
   Source: {invariants_dir}/{INV-NNN}-{slug}.md
```

For an all-targets run:
```
Invariant sidecar regeneration:
  ✅ INV-NNN — regenerated
  ✅ INV-NNN — unchanged
  ...

  {n} regenerated, {m} unchanged, {k} skipped (revoked).
  Next: Run /edikt:gov:compile to update governance.
```

If any extractor invocation fails, list the failures and exit non-zero.

## Why this command exists

the `<INV-NNN>.md` is user-authored prose. Sidecar regeneration is the only mechanism by which the invariant's directive metadata changes. This command is invoked:

- By `/edikt:invariant:new` immediately after writing a new Invariant Record
- By the user when they edit an invariant's prose body
- By `/edikt:gov:compile`'s Phase A (resync) when it detects sidecar staleness —

This command does not run the topic-grouping merge step. After regenerating one or more sidecars, run `/edikt:gov:compile` to refresh `.claude/rules/governance/`.

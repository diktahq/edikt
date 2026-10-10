---
name: gov:verify-diff
description: "Run the governance verifier against a diff — emits per-directive PASS/FAIL/NEEDS_REVIEW verdicts per compiled topic file whose paths: glob matches changed files."
effort: medium
allowed-tools:
  - Glob
  - Read
  - Bash
  - Agent
context: fork
---

# edikt:gov:verify-diff

Run the **L2 governance-verifier** against a code diff. For each compiled `.claude/rules/governance/<topic>.md` whose `paths:` frontmatter glob matches a changed file, dispatch a forked `governance-verifier` subagent with the diff path + the topic's directive list, and persist its per-directive verdict JSON to `.edikt/state/gov-verify/<topic>-<timestamp>.json`.

This is a **read-only advisor**. It never modifies code. Downstream callers (the post-flight orchestrator, `/edikt:doctor`) consume the JSON reports it produces; they decide whether and how to gate on FAIL or NEEDS_REVIEW verdicts.

## Arguments

- `[since-ref..to-ref]` — optional git range. Defaults to `HEAD~1..HEAD`. After NFKC + casefold + strip, must match `^[a-z0-9._/~^@-]+\.\.[a-z0-9._/~^@-]+$` (allowlist). Allowed characters include the git-ref shorthand metacharacters `~` (HEAD~N), `^` (parent), `@` (reflog).

## Contract (downstream callers depend on this)

- Empty diff (after binary-file filter) → exit 0 with stdout `{"status":"skipped","reason":"empty diff"}`. No agent dispatch.
- No `.claude/rules/governance/*.md` topic files → exit 0 with stdout `{"status":"skipped","reason":"no compiled governance"}`. No agent dispatch.
- For each in-scope topic file: one JSON report at `.edikt/state/gov-verify/<topic>-<YYYYMMDDTHHMMSSZ>.json` conforming to `templates/agents/governance-verifier-verdict.schema.json`.
- Per-topic skips (malformed `paths:` frontmatter, topic-name allowlist failure) emit `{"topic": "<name>", "status": "skipped", "reason": "<why>"}` to stdout and do not produce a report file. Other topics still proceed.
- Final summary on stdout: total topics processed, per-topic PASS/FAIL/NEEDS_REVIEW counts, report paths, wall-clock time.
- Exit code is always 0 on successful completion. This command is informational; gating is the orchestrator's job.

## Stub mode

`EDIKT_VERIFIER_STUB=1` short-circuits Agent dispatch. The slash command writes a canned PASS verdict (from `test/fixtures/verifier-verdicts/valid/pass-only.json`, with `topic` and `ran_at` overridden per matched topic) to the per-topic report path and continues. Used by `test/test-gov-verify-diff.sh` for hermetic CI.

## Instructions

0. If `.edikt/config.yaml` does not exist, output:
   ```
   No edikt config found. Run /edikt:init to set up this project.
   ```
   And stop.

### 1. Parse and validate the ref range

Parse `$ARGUMENTS`. Default to `HEAD~1..HEAD` when empty. Validate the resulting ref via allowlist regex (defense against shell-metacharacter injection via a malicious `--ref` flag):

```bash
RANGE="${ARGUMENTS:-HEAD~1..HEAD}"
case "$RANGE" in
  *..*) ;;
  *)
    edikt util json emit --field 'status=error' --field 'reason=ref range must contain ..' --field "input=$RANGE"
    exit 1
    ;;
esac

# Allowlist gate. NFKC + casefold + strip first so Unicode lookalikes
# and trailing whitespace cannot bypass.
RANGE_NORM=$(edikt util normalize "$RANGE")

case "$RANGE_NORM" in
  *[!a-z0-9._/~^@-]*|"")
    edikt util json emit --field 'status=error' --field 'reason=ref range contains disallowed characters' --field "input=$RANGE"
    exit 1
    ;;
esac
```

### 2. Compute the diff

```bash
# Changed files, binary files excluded.
CHANGED_RAW=$(git diff --numstat "$RANGE" 2>/dev/null || echo "")
# numstat fields: <added> <deleted> <path>. Binary files have "-" in both numeric columns.
CHANGED_FILES=$(printf '%s\n' "$CHANGED_RAW" | awk '$1 != "-" && $2 != "-" { print $3 }' | grep -v '^$' || true)

# Empty-diff guard (post binary filter).
if [ -z "$CHANGED_FILES" ]; then
  edikt util json emit --field 'status=skipped' --field 'reason=empty diff'
  exit 0
fi

# Materialize the diff to a temp file. The PATH is the contract surface between
# this command and the verifier subagent — diff TEXT never appears in any prompt.
DIFF_FILE=$(mktemp -t edikt-verify-diff.XXXXXX.diff)
git diff "$RANGE" -- $(printf '%s\n' "$CHANGED_FILES") > "$DIFF_FILE"
```

### 3. Discover compiled governance topic files

Use the **Glob tool** (not shell) to enumerate `.claude/rules/governance/*.md`:

```text
Glob: .claude/rules/governance/*.md
```

If the result is empty, no compiled governance exists. Emit the skip JSON and exit:

```bash
edikt util json emit --field 'status=skipped' --field 'reason=no compiled governance'
rm -f "$DIFF_FILE"
exit 0
```

### 4. For each topic file, parse frontmatter and match paths

For each `.claude/rules/governance/<topic>.md` returned by Glob:

1. **Read** the file with the Read tool. Extract the YAML frontmatter between the first two `---` lines.
2. Parse `paths:` (an array of **code globs** scoping where the topic's directives apply — the union of the contributing sidecars' declared `paths:`, or the single glob `**` when any contributor declared none). If the frontmatter is malformed or `paths:` is missing, emit a per-topic skip:
   ```bash
   edikt util json emit --field "topic=$TOPIC" --field 'status=skipped' --field 'reason=malformed paths frontmatter'
   ```
   Continue to the next topic; do not crash.
3. **Topic name allowlist (defense in depth).** The topic name comes from the filename basename (already kebab-case validated upstream by the sidecar schema). Re-validate at use-site by NFKC + casefold + strip + regex:

   ```bash
   TOPIC=$(basename "$TOPIC_FILE" .md)
   TOPIC_NORM=$(edikt util normalize "$TOPIC")

   case "$TOPIC_NORM" in
     [a-z]*) ;;
     *) printf '%s\n' "{\"topic\":\"$TOPIC\",\"status\":\"skipped\",\"reason\":\"topic name fails allowlist\"}"; continue ;;
   esac
   # Full regex check via python (case + length cap).
   edikt util match "$TOPIC_NORM" --pattern '^[a-z][a-z0-9-]{0,39}$' || { edikt util json emit --field "topic=$TOPIC" --field 'status=skipped' --field 'reason=topic name fails allowlist'; continue; }
   ```

4. **Path-glob match.** For each `paths:` entry from the topic's frontmatter, check if any changed file matches. If at least one changed file matches at least one `paths:` entry, the topic is **in scope** for this diff.

5. **Extract the directive list** from the topic file's body between `[edikt:directives:start]: #` and `[edikt:directives:end]: #`. Build the list via `edikt gov extract-directives`, passing the topic-file path and the artifact's primary ref (e.g., `ADR-NNN`, `INV-NNN`, or topic-stem) as flags — **directive text never traverses shell-string interpolation** (safe input handling). Sequential `directive_id` values are assigned as `<ref>.directive[<index>]`. The extractor supports two directive formats: a simple single-line `- text` format, and a multi-line YAML-like format where `intent` and `falsifying_observation` appear as indented sub-fields next to the `- text:` bullet (a Phase B extension). Both `intent` and `falsifying_observation` are NFKC-normalized, casefolded, stripped, and allowlist-gated before they enter argv.

   ```bash
   DIRECTIVES_JSON=$(EDIKT_INVOKER=/edikt:gov:verify-diff edikt gov extract-directives \
     --topic-file "$TOPIC_FILE" --ref "$REF")
   ```

   The output is a JSON array of `{directive_id, text, intent?, falsifying_observation?}` objects. Pass it to the dispatch step (Step 5 below) via `$DIRECTIVES_JSON` — never via shell-string interpolation of directive text.

### 5. Dispatch the verifier (per in-scope topic, concurrent)

Dispatch one Agent call per in-scope topic. Send all dispatches in a **single message** so they run concurrently.

```text
Agent(
  subagent_type: "governance-verifier",
  description: "L2 verify topic <topic>",
  prompt: $PROMPT_BODY
)
```

**Prompt body construction.** The prompt body MUST be composed via `edikt gov verify-prompt` with the diff file path, topic name, and directive list (from Step 4 sub-step 5) all passed as `sys.argv` values. **Directive text never traverses shell-string interpolation.** Per the AI-agent-first dispatcher contract and safe input handling, attacker-influenceable values (directive text, topic name, falsifying observations) flow as JSON through argv — never concatenated into evaluated strings. The diff body itself is never included; only the diff PATH.

```bash
PROMPT_BODY=$(EDIKT_INVOKER=/edikt:gov:verify-diff edikt gov verify-prompt \
  --diff-file "$DIFF_FILE" --topic "$TOPIC" \
  --directives "$DIRECTIVES_JSON" --agent-version 1.1.0)

# EDIKT_CAPTURE_PROMPT: when set, write the composed prompt payload as JSON
# to the given path BEFORE agent dispatch / stub check. Enables AC-7.2 and
# AC-7.3 without invoking the LLM. Values flow via argv — no shell concatenation.
if [ -n "${EDIKT_CAPTURE_PROMPT:-}" ]; then
  # Same verb, same shape-selection rule — written once rather than in two
  # adjacent heredocs that could disagree about which directives got which
  # treatment, which is exactly what the capture exists to check.
  EDIKT_INVOKER=/edikt:gov:verify-diff edikt gov verify-prompt \
    --diff-file "$DIFF_FILE" --topic "$TOPIC" \
    --directives "$DIRECTIVES_JSON" --agent-version 1.1.0 \
    --capture "$EDIKT_CAPTURE_PROMPT" >/dev/null
fi
```

The composed `$PROMPT_BODY` is passed as the `prompt:` argument to the `Agent()` dispatch above. `agent_version` is `"1.1.0"` (bumped from `1.0.0` for intent-mode). Per-directive blocks use either the Intent shape or Text shape per the §1 selection rule. The composition-via-argv pattern is preserved across both shapes.

### 5a. Stub mode short-circuit

Before dispatch, check `EDIKT_VERIFIER_STUB`. If `=1`, skip Agent dispatch and synthesize a canned PASS verdict from `test/fixtures/verifier-verdicts/valid/pass-only.json`, overriding `meta.topic` and `meta.ran_at`:

```bash
if [ "${EDIKT_VERIFIER_STUB:-0}" = "1" ]; then
  REPORT_JSON=$(EDIKT_INVOKER=/edikt:gov:verify-diff edikt gov verify-stub-verdict \
    --fixture test/fixtures/verifier-verdicts/valid/pass-only.json --topic "$TOPIC")
  # Persist via step 6 logic below; do NOT dispatch the agent.
fi
```

### 6. Persist the per-topic verdict

```bash
TS=$(date -u +%Y%m%dT%H%M%SZ)
REPORT_DIR=".edikt/state/gov-verify"
mkdir -p "$REPORT_DIR"
REPORT_PATH="$REPORT_DIR/${TOPIC}-${TS}.json"

# The agent's output (or stub) is captured as REPORT_JSON. Re-encode through
# Parsed before it is written — never write the raw agent string to disk
# unvalidated. The failure belongs to the step that can still do something
# about it, not to whoever reads the file later.
printf '%s' "$REPORT_JSON" | edikt util json format --indent 2 > "$REPORT_PATH"
```

### 7. Stdout summary

After all topics complete, print a summary. `--json` parses each aggregate rather than
splicing it, so a malformed fragment is refused here instead of producing a document that
only looks well-formed:

```bash
edikt util json emit --indent 2 \
  --field 'status=ok' \
  --field "range=$RANGE" \
  --int "topics_processed=$TOPICS_OK" \
  --int "topics_skipped=$TOPICS_SKIPPED" \
  --json "reports=$REPORTS_JSON" \
  --json "totals=$TOTALS_JSON" \
  --int "elapsed_ms=$ELAPSED_MS"
```

Where `TOTALS_JSON` is `{"PASS": n, "FAIL": n, "NEEDS_REVIEW": n}` aggregated across all per-topic verdicts.

### 8. Cleanup

```bash
rm -f "$DIFF_FILE"
exit 0
```

## Edge cases

| Case | Behavior |
|---|---|
| Empty diff (post binary filter) | `{"status":"skipped","reason":"empty diff"}`; exit 0; no dispatch |
| No `.claude/rules/governance/*.md` | `{"status":"skipped","reason":"no compiled governance"}`; exit 0 |
| Topic file with malformed `paths:` | Per-topic skip JSON; continue with other topics |
| Topic name fails allowlist | Per-topic skip JSON; continue |
| Binary files in diff | Filtered out by `git diff --numstat $1 == "-"` test before the verifier sees them |
| Renames | `git diff --numstat` reports the new path under field 3; treated as a regular change |
| Deletes | Included in the diff text the verifier reads |
| Topic file with empty directive list | `verdicts: []`, `meta` still required (schema accepts empty array) |

## Invariants

- **Tier-1 markdown only — the ORCHESTRATION, not every helper.** There is no
  `edikt verify-diff`, and there must not be: the sequencing, the fan-out and
  the decision to dispatch stay here, in markdown, where they are readable and
  changeable without a build.

  This bullet used to read "No new Go binary verb" flatly, which by 2026-08-23
  contradicted the thing it shares a repository with: PLAN-v080 FR-010 removes
  python3 from every command body by replacing it with verb calls, so a flat ban
  on verbs and the plan could not both be satisfied. The distinction the bullet
  was reaching for is between MOVING THE COMMAND into the binary — which would
  make this file a thin wrapper nobody reads — and calling a verb to do one
  well-defined thing the shell cannot do safely. The second is what
  `gov extract-directives` and `gov verify-prompt` are.

  The test: could a reader still follow what this command does by reading this
  file? If a verb absorbs a decision, it has gone too far.
- **Safe JSON construction.** All JSON construction goes through a serializer with
  values passed as arguments — `edikt util json emit --field`, or a verb that
  builds the document itself. Never shell-string-concatenate JSON.

  This named an inline python3 `json.dumps` call until 2026-08-23. The requirement was
  never the interpreter; it was that a value carrying a quote or a backslash must
  be ENCODED rather than pasted. python3 was simply the only serializer tier-1
  had.
- **No agent text into Claude-facing channels.** Agent verdict text is persisted to a JSON report file. It is NEVER interpolated into a `systemMessage` or any other Claude-facing channel by this command. (Downstream callers that surface verdict text to Claude must follow the same rule.)
- **Input validation.** Every external value — ref range, topic name, file path — is NFKC-normalized + allowlist-validated before it reaches shell argv, a path, or a prompt. Untrusted values flow as separate argv elements, never concatenated into evaluated strings.
- **Hermetic tests.** The `test/test-gov-verify-diff.sh` e2e is hermetic: tmpdir-staged, no host `~/.claude/` reads, `EDIKT_VERIFIER_STUB=1`.

## See also

- `templates/agents/governance-verifier.md` — the dispatch target
- `templates/agents/governance-verifier-verdict.schema.json` — verdict schema

## Completion

```
✅ Governance diff verified
   Reports: .edikt/state/gov-verify/<topic>-<ts>.json (per in-scope topic)

   Next: Inspect the per-topic reports for FAIL or NEEDS_REVIEW verdicts,
   or run /edikt:sdlc:post-flight <plan> --phase <N> to fold this verdict
   into a composite L1+L2+L3 review.
```

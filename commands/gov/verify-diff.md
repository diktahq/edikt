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
    python3 -c 'import json,sys; print(json.dumps({"status":"error","reason":"ref range must contain ..","input":sys.argv[1]}))' "$RANGE"
    exit 1
    ;;
esac

# Allowlist gate. NFKC + casefold + strip first so Unicode lookalikes
# and trailing whitespace cannot bypass.
RANGE_NORM=$(python3 -c '
import sys, unicodedata
v = unicodedata.normalize("NFKC", sys.argv[1]).casefold().strip()
sys.stdout.write(v)
' "$RANGE")

case "$RANGE_NORM" in
  *[!a-z0-9._/~^@-]*|"")
    python3 -c 'import json,sys; print(json.dumps({"status":"error","reason":"ref range contains disallowed characters","input":sys.argv[1]}))' "$RANGE"
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
  python3 -c 'import json,sys; print(json.dumps({"status":"skipped","reason":"empty diff"}))'
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
python3 -c 'import json,sys; print(json.dumps({"status":"skipped","reason":"no compiled governance"}))'
rm -f "$DIFF_FILE"
exit 0
```

### 4. For each topic file, parse frontmatter and match paths

For each `.claude/rules/governance/<topic>.md` returned by Glob:

1. **Read** the file with the Read tool. Extract the YAML frontmatter between the first two `---` lines.
2. Parse `paths:` (an array of **code globs** scoping where the topic's directives apply — the union of the contributing sidecars' declared `paths:`, or the single glob `**` when any contributor declared none). If the frontmatter is malformed or `paths:` is missing, emit a per-topic skip:
   ```bash
   python3 -c 'import json,sys; print(json.dumps({"topic":sys.argv[1],"status":"skipped","reason":"malformed paths frontmatter"}))' "$TOPIC"
   ```
   Continue to the next topic; do not crash.
3. **Topic name allowlist (defense in depth).** The topic name comes from the filename basename (already kebab-case validated upstream by the sidecar schema). Re-validate at use-site by NFKC + casefold + strip + regex:

   ```bash
   TOPIC=$(basename "$TOPIC_FILE" .md)
   TOPIC_NORM=$(python3 -c '
   import sys, unicodedata
   v = unicodedata.normalize("NFKC", sys.argv[1]).casefold().strip()
   sys.stdout.write(v)
   ' "$TOPIC")

   case "$TOPIC_NORM" in
     [a-z]*) ;;
     *) printf '%s\n' "{\"topic\":\"$TOPIC\",\"status\":\"skipped\",\"reason\":\"topic name fails allowlist\"}"; continue ;;
   esac
   # Full regex check via python (case + length cap).
   python3 -c '
   import re, sys
   if not re.match(r"^[a-z][a-z0-9-]{0,39}$", sys.argv[1]):
       sys.exit(1)
   ' "$TOPIC_NORM" || { python3 -c 'import json,sys; print(json.dumps({"topic":sys.argv[1],"status":"skipped","reason":"topic name fails allowlist"}))' "$TOPIC"; continue; }
   ```

4. **Path-glob match.** For each `paths:` entry from the topic's frontmatter, check if any changed file matches. If at least one changed file matches at least one `paths:` entry, the topic is **in scope** for this diff.

5. **Extract the directive list** from the topic file's body between `[edikt:directives:start]: #` and `[edikt:directives:end]: #`. Build the list via `python3 -c` with the topic-file path and the artifact's primary ref (e.g., `ADR-NNN`, `INV-NNN`, or topic-stem) passed as `sys.argv` values — **directive text never traverses shell-string interpolation** (safe input handling). Sequential `directive_id` values are assigned as `<ref>.directive[<index>]`. The extractor supports two directive formats: a simple single-line `- text` format, and a multi-line YAML-like format where `intent` and `falsifying_observation` appear as indented sub-fields next to the `- text:` bullet (a Phase B extension). Both `intent` and `falsifying_observation` are NFKC-normalized, casefolded, stripped, and allowlist-gated before they enter argv.

   ```bash
   DIRECTIVES_JSON=$(python3 -c '
   import json, re, sys, pathlib, unicodedata

   def _gate(val, maxlen=300):
       """NFKC+casefold+strip, then reject control chars. Returns stripped original or None."""
       if not val:
           return None
       norm = unicodedata.normalize("NFKC", val).casefold().strip()
       if any(unicodedata.category(c)[0] == "C" for c in norm):
           return None
       if len(norm) > maxlen:
           return None
       return val.strip()

   topic_file = sys.argv[1]
   ref = sys.argv[2]
   body = pathlib.Path(topic_file).read_text()
   m = re.search(r"\[edikt:directives:start\]: #(.*?)\[edikt:directives:end\]: #", body, re.DOTALL)
   directives = []
   if m:
       lines = m.group(1).splitlines()
       i = 0
       while i < len(lines):
           line = lines[i].strip()
           if not line.startswith("- "):
               i += 1
               continue
           content = line[2:].strip()
           entry = {"directive_id": f"{ref}.directive[{len(directives)}]"}
           if content.startswith("text: "):
               # Multi-line format: intent/falsifying_observation may follow as indented sub-fields.
               entry["text"] = content[6:].strip()
               j = i + 1
               while j < len(lines):
                   sub = lines[j]
                   if sub and not sub[0:1] in (" ", "\t"):
                       break
                   sub_s = sub.strip()
                   if sub_s.startswith("intent: "):
                       gated = _gate(sub_s[8:].strip())
                       if gated:
                           entry["intent"] = gated
                   elif sub_s.startswith("falsifying_observation: "):
                       gated = _gate(sub_s[24:].strip())
                       if gated:
                           entry["falsifying_observation"] = gated
                   j += 1
               i = j
           else:
               # Simple single-line format.
               entry["text"] = content
               i += 1
           directives.append(entry)
   print(json.dumps(directives))
   ' "$TOPIC_FILE" "$REF")
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

**Prompt body construction.** The prompt body MUST be composed via `python3 -c` with the diff file path, topic name, and directive list (from Step 4 sub-step 5) all passed as `sys.argv` values. **Directive text never traverses shell-string interpolation.** Per the AI-agent-first dispatcher contract and safe input handling, attacker-influenceable values (directive text, topic name, falsifying observations) flow as JSON through argv — never concatenated into evaluated strings. The diff body itself is never included; only the diff PATH.

```bash
PROMPT_BODY=$(python3 -c '
import json, sys
diff_file = sys.argv[1]
topic = sys.argv[2]
directives = json.loads(sys.argv[3])
agent_version = sys.argv[4]
lines = [
    f"You are evaluating diff at: {diff_file}",
    "",
    f"Topic: {topic}",
    "Directives:",
]
for d in directives:
    # Intent-mode shape selection (values already gated in Step 4.5).
    # Intent shape: both intent AND falsifying_observation present → emit those two; strip text.
    # Text shape:   fallback when either is absent.
    has_intent = bool(d.get("intent")) and bool(d.get("falsifying_observation"))
    lines.append(f"  - directive_id: {d[\"directive_id\"]}")
    if has_intent:
        lines.append(f"    intent: {d[\"intent\"]}")
        lines.append(f"    falsifying_observation: {d[\"falsifying_observation\"]}")
    else:
        lines.append(f"    text: {d[\"text\"]}")
lines.extend([
    "",
    "Read the diff file. For each directive, emit one verdict (PASS / FAIL / NEEDS_REVIEW) per the governance-verifier contract. Emit one JSON object conforming to templates/agents/governance-verifier-verdict.schema.json.",
    "",
    f"meta.topic = {topic}",
    f"meta.agent_version = \"{agent_version}\"",
])
print("\n".join(lines))
' "$DIFF_FILE" "$TOPIC" "$DIRECTIVES_JSON" "1.1.0")

# EDIKT_CAPTURE_PROMPT: when set, write the composed prompt payload as JSON
# to the given path BEFORE agent dispatch / stub check. Enables AC-7.2 and
# AC-7.3 without invoking the LLM. Values flow via argv — no shell concatenation.
if [ -n "${EDIKT_CAPTURE_PROMPT:-}" ]; then
  python3 -c '
import json, sys, pathlib
prompt_body = sys.argv[1]
capture_path = sys.argv[2]
topic = sys.argv[3]
directives_raw = json.loads(sys.argv[4])
# Build structured capture: each directive records its dispatched shape
# plus source_text so AC-7.3 can verify no text leaked into intent blocks.
dispatch = []
for d in directives_raw:
    has_intent = bool(d.get("intent")) and bool(d.get("falsifying_observation"))
    if has_intent:
        entry = {
            "directive_id": d["directive_id"],
            "shape": "intent",
            "intent": d["intent"],
            "falsifying_observation": d["falsifying_observation"],
            "source_text": d.get("text", ""),
        }
    else:
        entry = {
            "directive_id": d["directive_id"],
            "shape": "text",
            "text": d.get("text", ""),
        }
    dispatch.append(entry)
payload = {
    "topic": topic,
    "directives": dispatch,
    "meta": {"topic": topic, "agent_version": "1.1.0"},
}
pathlib.Path(capture_path).write_text(json.dumps(payload, indent=2) + "\n")
' "$PROMPT_BODY" "$EDIKT_CAPTURE_PROMPT" "$TOPIC" "$DIRECTIVES_JSON"
fi
```

The composed `$PROMPT_BODY` is passed as the `prompt:` argument to the `Agent()` dispatch above. `agent_version` is `"1.1.0"` (bumped from `1.0.0` for intent-mode). Per-directive blocks use either the Intent shape or Text shape per the §1 selection rule. The composition-via-argv pattern is preserved across both shapes.

### 5a. Stub mode short-circuit

Before dispatch, check `EDIKT_VERIFIER_STUB`. If `=1`, skip Agent dispatch and synthesize a canned PASS verdict from `test/fixtures/verifier-verdicts/valid/pass-only.json`, overriding `meta.topic` and `meta.ran_at`:

```bash
if [ "${EDIKT_VERIFIER_STUB:-0}" = "1" ]; then
  REPORT_JSON=$(python3 -c '
import json, sys, datetime, pathlib
tpl_path = sys.argv[1]
topic = sys.argv[2]
tpl = json.loads(pathlib.Path(tpl_path).read_text())
tpl["meta"]["topic"] = topic
tpl["meta"]["ran_at"] = datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
print(json.dumps(tpl, indent=2))
' "test/fixtures/verifier-verdicts/valid/pass-only.json" "$TOPIC")
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
# python3 json.dumps to guarantee well-formedness — never
# write the raw agent string to disk unvalidated.
python3 -c '
import json, sys, pathlib
data = json.loads(sys.argv[1])
pathlib.Path(sys.argv[2]).write_text(json.dumps(data, indent=2) + "\n")
' "$REPORT_JSON" "$REPORT_PATH"
```

### 7. Stdout summary

After all topics complete, print a summary via `python3 json.dumps`:

```bash
python3 -c '
import json, sys
summary = {
    "status": "ok",
    "range": sys.argv[1],
    "topics_processed": int(sys.argv[2]),
    "topics_skipped": int(sys.argv[3]),
    "reports": json.loads(sys.argv[4]),
    "totals": json.loads(sys.argv[5]),
    "elapsed_ms": int(sys.argv[6]),
}
print(json.dumps(summary, indent=2))
' "$RANGE" "$TOPICS_OK" "$TOPICS_SKIPPED" "$REPORTS_JSON" "$TOTALS_JSON" "$ELAPSED_MS"
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

- **Tier-1 markdown only.** No new Go binary verb. The slash command uses Glob, Read, Bash, Agent — no `bin/edikt verify-diff` exists; never call one.
- **Safe JSON construction.** All JSON construction goes through `python3 -c 'import json,sys; print(json.dumps(...))'` with values passed as argv. Never shell-string-concatenate JSON.
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

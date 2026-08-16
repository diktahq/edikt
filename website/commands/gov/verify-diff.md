# edikt gov verify-diff

**Tier-1 slash command / CLI reference.** `/edikt:gov:verify-diff` runs the **L2 governance verifier** against a code diff. For each compiled `.claude/rules/governance/<topic>.md` whose `paths:` glob matches a changed file, it dispatches a fresh-fork `governance-verifier` subagent with the diff path plus the topic's directive list, and persists the per-directive verdict JSON to `.edikt/state/gov-verify/<topic>-<timestamp>.json`. The verifier emits per-directive verdicts — PASS, FAIL, or NEEDS_REVIEW — conforming to the verifier's schema.

This is the completion-evidence check at the diff timeframe: the other layers verify that asserted state still holds at static call sites; this one verifies that a *diff* does not silently violate a compiled governance directive.

The command is **invocable manually** for any diff range. In v0.6.x it is also invoked automatically by the post-flight review composition — that is the daily-driver path; manual invocation is for debugging.

## Synopsis

```bash
/edikt:gov:verify-diff                    # default: HEAD~1..HEAD
/edikt:gov:verify-diff main..HEAD
/edikt:gov:verify-diff <since-ref>..<to-ref>
```

The ref range is validated against an allowlist regex (NFKC-normalized, casefolded, stripped) before any shell interpolation — defense against shell-metacharacter injection. Allowed characters are `[a-z0-9._/~^@-]`, which covers the git-ref shorthand `~` (HEAD~N), `^` (parent), `@` (reflog).

## How it works

The command runs in four steps:

1. **Glob-match.** Discover compiled governance topic files (`.claude/rules/governance/*.md`) via the Glob tool. For each topic, parse its frontmatter `paths:` array. If at least one changed file matches at least one `paths:` entry, the topic is "in scope" for this diff. Out-of-scope topics are silently ignored.
2. **Diff capture.** Materialize the unified diff to a temp file (`mktemp -t edikt-verify-diff.XXXXXX.diff`). Binary files (detected via `git diff --numstat` reporting `-` for added/deleted counts) are excluded. The temp file *path* is the contract surface between the slash command and the verifier subagent — diff TEXT never appears in any prompt string (defense against agent-derived text injection).
3. **Per-topic dispatch.** For each in-scope topic, dispatch a `governance-verifier` subagent concurrently (single message, multiple Agent calls). The agent reads the diff from the temp path, evaluates each directive in the topic's body against the diff, and emits a JSON verdict array. Read-only by construction — `allowed-tools: [Read, Glob, Grep]`, `context: fork`.
4. **JSON persistence.** Re-encode the agent's output through `python3 json.dumps` and write to `.edikt/state/gov-verify/<topic>-<YYYYMMDDTHHMMSSZ>.json`. Downstream callers (the post-flight orchestrator, `bin/edikt doctor`, future CI lints) consume the JSON reports — this command is informational and never gates on its own findings.

A final summary line on stdout (also JSON) reports topics processed, per-topic counts, report paths, and elapsed time.

## Verdict shape

The per-topic verdict file conforms to `templates/agents/governance-verifier-verdict.schema.json`. Each entry in `verdicts[]` carries:

- `directive_id` — stable identifier of the form `INV-NNN.directive[N]`, `ADR-NNN.directive[N]`, or `<topic>.directive[N]`.
- `status` — one of `PASS`, `FAIL`, `NEEDS_REVIEW`. There is **no `BLOCKED`** in the L2 schema (cf. L1 evaluator-verdict): the verifier runs in a fresh fork with read-only tools and can always evaluate.
- `rationale` — plain-text justification, ≤ 2000 chars. Markdown code fences are schema-rejected (agent text must never carry shell content into downstream channels).
- `evidence` — optional `[{file, line_range}, ...]` citations. Required-in-practice for `FAIL` verdicts: a FAIL without a `file:line` in the diff is treated as a contract violation.

The companion `meta` block records `topic`, `ran_at`, and `agent_version`.

The L2 schema is intentionally a sibling — not a superset — of the L1 evaluator-verdict schema (`templates/agents/evaluator-verdict.schema.json`). L1 is per-criterion with an `evidence_type` discriminator enforcing the test-run gate; L2 is per-directive with no evidence-type gate because it inspects a diff rather than running tests.

## Stub mode

Setting `EDIKT_VERIFIER_STUB=1` short-circuits the Agent dispatch and writes a canned PASS verdict (from `test/fixtures/verifier-verdicts/valid/pass-only.json`, with `meta.topic` and `meta.ran_at` overridden per matched topic) directly to the report path.

Stub mode exists for hermetic CI: `test/test-gov-verify-diff.sh` runs the documented flow against a tmpdir-staged synthetic project and asserts the contract that downstream callers depend on — empty-diff skip, no-topic-files skip, malformed-frontmatter per-topic skip, binary-file filter, single-topic report, mixed match/no-match. Hermeticity: the e2e never reads the host `~/.claude/`.

Production users should never set `EDIKT_VERIFIER_STUB`.

## Skip semantics

The command exits `0` on successful completion. Skips are informational and emitted as JSON to stdout:

| Condition | Stdout |
|---|---|
| Empty diff (after binary filter) | `{"status":"skipped","reason":"empty diff"}` |
| No compiled governance topic files | `{"status":"skipped","reason":"no compiled governance"}` |
| Topic file with malformed/missing `paths:` frontmatter | `{"topic":"<name>","status":"skipped","reason":"malformed paths frontmatter"}` — per-topic; other topics still proceed |
| Topic name fails the `^[a-z][a-z0-9-]{0,39}$` allowlist | `{"topic":"<name>","status":"skipped","reason":"topic name fails allowlist"}` — per-topic; other topics still proceed |

Gating is the orchestrator's job. This command never returns non-zero on a FAIL verdict — that decision belongs to the caller (the post-flight orchestrator, `bin/edikt doctor`).

## Safety properties

- **Tier-1 markdown only.** No new Go binary verb. No `bin/edikt verify-diff` exists.
- **Safe JSON construction.** Every JSON object the command emits is constructed via `python3 -c 'import json,sys; print(json.dumps(...))'` with values passed as separate argv elements. Shell-string concatenation of JSON is forbidden.
- **No agent text into Claude-facing channels.** Agent verdict text is persisted to a JSON report file. It is never interpolated into a `systemMessage` or any other Claude-facing channel by this command. Downstream callers that surface verdict text must follow the same rule.
- **Input validation.** Every external value — ref range, topic name, file path — is NFKC-normalized + allowlist-validated before it reaches shell argv, a path, or a prompt. Untrusted values flow as separate argv elements, never concatenated into evaluated strings.
- **Hermetic tests.** The `test/test-gov-verify-diff.sh` e2e is hermetic: tmpdir-staged, no host `~/.claude/` reads, runs under `EDIKT_VERIFIER_STUB=1`.

## Related

- [edikt gov compile](compile) — produces the `.claude/rules/governance/*.md` topic files the verifier matches against.
- [edikt verify](../verify) — the L1 sidecar-verify runner; complementary, not redundant.
- [Sidecar architecture](../../governance/sidecar) — how directives become matchable governance.

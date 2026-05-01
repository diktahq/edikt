# INV-003 — Hooks emit structured JSON, never shell-concatenated strings

**Status:** Active

## Statement

Every hook that emits JSON — to stdout (Claude Code hook protocol) or to a log file (e.g. `~/.edikt/events.jsonl`) — MUST construct that JSON via a structured serializer with untrusted values passed as data (not concatenated into a string). Building JSON by shell string concatenation is forbidden. The principle is serializer-only; concrete tooling (the specific `python3 -c` pattern used in bash hooks today) is implementation, documented below.

## Rationale

The v0.5.0 security audit (2026-04-17) surfaced that shell-concatenated JSON in `stop-failure.sh`, `file-changed.sh`, `headless-ask.sh`, `event-log.sh`, and `subagent-stop.sh` permitted attacker-controlled values (file paths, error messages, config answers, git identity, subagent findings) to corrupt or inject keys into the hook protocol payload. Four of those paths were rated Critical (CRIT-1, CRIT-2, CRIT-4, CRIT-5) because they reached RCE or directive bypass. The safe pattern was already established in the codebase (`templates/hooks/worktree-create.sh:63-67` and `task-created.sh:68-80`) — this invariant promotes that pattern to a rule.

INV-003 governs **emission format** (how the JSON is serialized). Its companion, INV-004, governs **channel content** (what text is allowed in any hook-driven channel reaching Claude). A hook that uses `json.dumps` correctly but writes a shell command Claude is told to execute is INV-003-compliant but INV-004-violating. Both invariants apply.

## Consequences of violation

- Attacker-controlled content containing a JSON metacharacter (`"`, `\`, newline) corrupts the hook output — silent delivery failure (hook protocol parse error drops the message entirely) or payload key injection (extra `decision`/`permissionDecision`/`additionalContext` fields that flip security-relevant behavior).
- If the JSON is passed onward into a shell command (e.g. the hook tells Claude to `echo '{...}' >> file`), shell metacharacters in the attacker's value escape the single-quote and execute arbitrary commands — RCE class.
- Log evasion: malformed JSON in `~/.edikt/events.jsonl` is silently dropped by downstream readers, destroying audit trail integrity.

## Implementation

In bash-based hooks (the current implementation), the serializer is an inline `python3 -c 'import json,sys; print(json.dumps(...))'` with untrusted values in `sys.argv`. No external helper library is introduced — INV-001 (plain markdown, no build step) and the hook model (one script per event) favor per-file inlining. If the hook runtime moves off bash+python3 in a future release, the same invariant applies with the equivalent structured-serializer primitive in that runtime.

Reference templates:
- `templates/hooks/worktree-create.sh:63-67` — single-key emission.
- `templates/hooks/task-created.sh:68-80` — multi-field event log.

## Anti-patterns

Forbidden (all three corrupt on a `"` or newline in `$FILE`):

```bash
echo "{\"systemMessage\":\"File: ${FILE}\"}"
printf '{"systemMessage":"File: %s"}\n' "$FILE"
cat <<EOF
{"systemMessage":"File: ${FILE}"}
EOF
```

Required:

```bash
python3 -c 'import json,sys; print(json.dumps({"systemMessage": f"File: {sys.argv[1]}"}))' "$FILE"
```

## Enforcement

- Pre-merge CI lint: `grep -nE 'echo ["'"'"']\{|printf ["'"'"']\{' templates/hooks/*.sh install.sh` MUST return zero matches.
- Unit-test fixtures per hook (`test/unit/hooks/test_*.sh`) assert output is `json.loads`-parseable when inputs contain embedded `"`, `\`, and newline characters.
- `/edikt:sdlc:audit` and `/edikt:sdlc:review` read this invariant before reviewing any change under `templates/hooks/`.

## Directives

[edikt:directives:start]: #
source_hash: d85dbb0f0e29de22a287f929cb16453e14bf658016a74979cf5a9e9db4cdf7d6
directives_hash: pending
compiler_version: "0.4.3"
paths:
  - "templates/hooks/**"
  - "install.sh"
scope:
  - implementation
  - review
directives:
  - Hook scripts MUST emit hook-protocol JSON via `python3 -c 'import json; print(json.dumps(...))'` with untrusted values passed as argv. NEVER concatenate JSON via shell (`echo "{\"k\":\"${VAR}\"}"`, `printf '{"k":"%s"}' "$VAR"`, heredocs with interpolation). (ref: INV-003)
  - CI lint MUST reject any match of `echo ["']\{` or `printf ["']\{` in templates/hooks/*.sh and install.sh. (ref: INV-003)
  - Hook unit tests MUST assert output is `json.loads`-parseable when inputs contain embedded `"`, `\`, and newline characters. (ref: INV-003)
manual_directives: []
suppressed_directives: []
canonical_phrases:
  - "json.dumps"
  - "hook JSON emission"
  - "INV-003"
behavioral_signal:
  cite:
    - "INV-003"
[edikt:directives:end]: #

# INV-004 — Hooks must not instruct Claude to execute shell built from untrusted text

**Status:** Active

## Statement

No hook may emit a `systemMessage`, `additionalContext`, or other Claude-facing channel whose body contains a shell command interpolating subagent-derived, file-content-derived, git-identity-derived, or otherwise attacker-influenceable text. Hooks that need to record state (event logs, gate firings, override decisions) MUST perform that recording themselves from within the hook process. Claude receives only static instructions plus structured data.

## Rationale

The v0.5.0 security audit (2026-04-17) found that `templates/hooks/subagent-stop.sh` built a multi-line `GATE_MSG` instructing Claude to run `echo '{...,"finding":"${ESCAPED_FINDING}",...}' >> ~/.edikt/events.jsonl`. The `FINDING` value came from an agent response grep on files Claude had just read — any attacker-controlled file content that matched the pattern could land inside a shell single-quote that Claude was told to execute. `json.dumps` only escapes JSON-significant characters, not shell metacharacters, so a `'` or `$(…)` in the finding broke out of the echo and ran arbitrary commands. This was audit finding CRIT-1 (RCE-class).

INV-004 is scope-distinct from INV-003. INV-003 governs *how* a hook serializes JSON. INV-004 governs *what* content is permitted in any channel that reaches Claude. A hook that uses `json.dumps` correctly but writes a shell command into `systemMessage` satisfies INV-003 and violates INV-004.

## Consequences of violation

- Attacker-controlled text in a file read by any agent becomes a shell command executed by Claude with full user permissions — remote code execution from a poisoned repo file, pull request, commit message, or MCP tool output.
- Claude obeys the instruction verbatim; there is no downstream safety check once the hook has emitted the shell command into a governance-authoritative channel.

## Implementation

For every hook that currently emits shell-for-Claude, split responsibilities:
- The hook itself writes `events.jsonl` (or any other side-channel state) inside its own process, via the structured emission pattern from INV-003.
- The `systemMessage` / `additionalContext` text is entirely static, or parameterized only over structured data that has been validated per INV-006 and rendered as prose (never as a code fence Claude is told to execute).

Reference: after the Phase 2 rewrite, `subagent-stop.sh` writes the event log itself and emits a static `systemMessage` of the form "Gate fired; see ~/.edikt/events.jsonl for details. Run /edikt:capture to review." — no agent text embedded.

## Anti-patterns

Forbidden:
```bash
# Hook emits systemMessage that includes a code block for Claude to execute,
# interpolating subagent-derived text.
MSG="Run this to log the override:\n\`\`\`bash\necho '{...,\"finding\":\"${FINDING}\",...}' >> ~/.edikt/events.jsonl\n\`\`\`"
python3 -c '...' "$MSG"
```

Required:
```bash
# Hook writes the event itself.
python3 -c 'import json,sys; open(sys.argv[1],"a").write(json.dumps({"ts":sys.argv[2],"finding":sys.argv[3]})+"\n")' "$HOME/.edikt/events.jsonl" "$(date -Iseconds)" "$FINDING"
# Claude-facing message is static. Structured emission is still required per INV-003,
# even though the content is a constant — serializer-only is the rule, not just
# "when the content is dynamic".
python3 -c 'import json; print(json.dumps({"systemMessage":"Gate fired; see ~/.edikt/events.jsonl. Run /edikt:capture to review."}))'
```

## Enforcement

- CI lint MUST reject any hook whose output contains the regex `(systemMessage|additionalContext)[^}]*\$\{` OR whose output contains a markdown code fence labelled `bash` inside a hook-protocol field.
- Security regression tests under `test/security/hooks/` MUST include at least one test per hook that attempts to inject `'; rm /tmp/sentinel; '` into an agent-derived field and asserts the file `/tmp/sentinel` is never removed.
- `/edikt:sdlc:audit` reads this invariant when reviewing hook changes.

## Directives

[edikt:directives:start]: #
source_hash: 941bea514deef016d6fbcf2018d573592262cf5e183457862614b2612610f3b5
directives_hash: pending
compiler_version: "0.4.3"
paths:
  - "templates/hooks/**"
scope:
  - implementation
  - review
directives:
  - Hooks MUST NOT emit Claude-facing channels (`systemMessage`, `additionalContext`, or equivalent) whose body interpolates subagent-derived, file-content-derived, or otherwise attacker-influenceable text. (ref: INV-004)
  - Hooks that need to log state MUST write the log entry inside the hook process itself. NEVER instruct Claude to run the write. (ref: INV-004)
  - CI MUST reject hook output containing `(systemMessage|additionalContext)[^}]*\$\{` or a bash code fence inside a hook-protocol field. (ref: INV-004)
manual_directives: []
suppressed_directives: []
canonical_phrases:
  - "agent text into shell"
  - "static systemMessage"
  - "INV-004"
behavioral_signal:
  cite:
    - "INV-004"
[edikt:directives:end]: #

---
name: sidecar:approve
description: "Review a pending behavioral verify proposal and promote, reject, or defer it. Captures the human approval a behavioral verify command requires before it can compile."
tier: 1
tier_2_dependency: edikt sidecar approve
on_absent: refuse-and-direct-user
allowed-tools:
  - Read
  - Write
  - Bash
  - AskUserQuestion
  - Glob
---

# /edikt:sidecar:approve

Promote, reject, or defer a pending behavioral verify proposal that the
sidecar-extractor wrote to `.edikt/state/pending-verifies/<id>.yaml`.

The binary is **args-driven and non-interactive**. This tier-1
command owns the human UX: render the proposal, capture the user's decision
via `AskUserQuestion`, dispatch `bin/edikt sidecar approve <id> --decision=<...>`,
and surface the exit code. The binary's stdout is displayed verbatim — never
parsed verbatim.

## Arguments

- `$ARGUMENTS` — optional `<pending-id>`. If omitted, the command lists all
  pending IDs under `.edikt/state/pending-verifies/` and asks the user which
  to review.

## Steps

### 1. Verify binary presence

```bash
command -v bin/edikt >/dev/null 2>&1 || command -v edikt >/dev/null 2>&1
```

If the check fails, print:

```
✗ bin/edikt not found.
  This command requires the edikt tier-2 helper. Install via:
    edikt install edikt
  Then re-run /edikt:sidecar:approve.
```

Stop. Do not proceed. (Frontmatter `on_absent: refuse-and-direct-user`.)

### 2. Resolve the pending ID

If `$ARGUMENTS` is non-empty, treat the first whitespace-trimmed token as
the pending-id.

Otherwise, list pending files:

```bash
ls .edikt/state/pending-verifies/*.yaml 2>/dev/null
```

If no pending files exist, print `No pending behavioral verifies. Nothing to approve.` and stop.

If multiple exist, present the basenames (minus `.yaml`) to the user via
`AskUserQuestion` and use the selected one. If exactly one exists, proceed
with it directly.

### 3. Read and render the proposal

The pending file is plain YAML at `.edikt/state/pending-verifies/<id>.yaml`.
Read it directly with the Read tool — no binary call needed for inspection.

Render the following to the user (skip fields that are missing or empty):

```
Pending verify: <id>
  sidecar:           <sidecar_path>
  directive_index:   <directive_index>
  proposed_verify:   <proposed_verify>
  intent:            <intent>
  falsifying_observation: <falsifying_observation>
  proposed_at:       <proposed_at>
```

If a sibling fixtures file exists at
`.edikt/state/pending-verifies/<id>.fixtures.yaml`, also surface its summary
(positive / negative fixture counts).

### 4. Capture the human decision

Use `AskUserQuestion` to ask:

> "Approve this behavioral verify proposal? (approve / reject / defer / edit)"

- **approve** — accept the proposed verify command as-is.
- **reject** — discard the proposal; the pending file will be removed and
  the sidecar will not be mutated.
- **defer** — leave both pending file and sidecar untouched.
- **edit** — open the proposed verify for revision before approving.

### 5a. Approve as-is

Dispatch the binary:

```bash
bin/edikt sidecar approve <pending-id> --decision=approve
EXIT=$?
```

### 5b. Approve with edits

Write the edited verify command body to a temp file:

```bash
TMP=$(mktemp -t edikt-approve.XXXXXX)
# Use Write tool to put the user-edited body into $TMP. Do NOT include a
# trailing newline beyond what the user wrote.
```

Then dispatch:

```bash
bin/edikt sidecar approve <pending-id> --decision=approve --edited-content="$TMP"
EXIT=$?
rm -f "$TMP"
```

### 5c. Reject

```bash
bin/edikt sidecar approve <pending-id> --decision=reject
EXIT=$?
```

### 5d. Defer

```bash
bin/edikt sidecar approve <pending-id> --decision=defer
EXIT=$?
```

### 6. Report the exit code

Pass the binary's output through verbatim; do NOT parse
its shape. Use the exit code only.

- **Exit 0** — print the binary's stdout as-is, then `✓ done`.
- **Exit 2** — print `✗ pending-id not found: <id>. Re-run with a valid id.`
- **Exit 3** — print `✗ invalid arguments. Re-run /edikt:sidecar:approve and re-select.`
- **Exit 1** or other non-zero — print `✗ approve failed (exit <code>). See the binary's stderr.`

Stop after the report. The binary handles all on-disk mutations atomically;
there is nothing to undo from this command.

## Notes

- This command does NOT compile governance. After approval, run
  `bin/edikt gov compile` to regenerate topic files with the promoted
  behavioral verify in scope.
- The pending file is removed only after the sidecar write succeeds (or on
  `--decision=reject`); a failure mid-approve leaves the pending file
  in place so the user can retry.
- The tier-1 wrapper must
  never edit the sidecar directly — promotion goes through `bin/edikt
  sidecar approve` so `human_approved_at` and schema validation stay
  authoritative.

## Completion

End with a one-line verdict, e.g. `✅ {id}: approved` (or `rejected` / `deferred`).

Next: run `/edikt:gov:compile` to bring the promoted behavioral verify into the compiled governance.

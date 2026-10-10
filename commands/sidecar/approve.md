---
name: sidecar:approve
description: "Review a pending proposal — a behavioral verify, a paths scope, or a topic description — and promote, reject, or defer it. Captures the human approval each requires before it can compile."
tier: 1
tier_2_dependency: edikt sidecar approve
on_absent: refuse-and-direct-user
allowed-tools:
  - Read
  - Write
  - Bash
  - AskUserQuestion
  - Glob
stops:
  - id: select-proposal
    class: selection
    default: "none"
    on_silence: take-default
    prompt: "Which pending proposal should be reviewed?"
  - id: approval-decision
    class: risk
    default: "defer"
    on_silence: safe-side
    prompt: "Approve this proposal, reject it, or defer the decision?"
---

# /edikt:sidecar:approve

Promote, reject, or defer a pending proposal that the compile routed to the
approval queue. **Every kind the verb supports is reachable here**, because a
project that invokes commands rather than the binary can only reach what this
command wraps:

| kind | pending store | what a human is deciding |
|---|---|---|
| `verify` (default) | `.edikt/state/pending-verifies/` | whether a proposed behavioral verify actually demonstrates the rule |
| `paths` | `.edikt/state/pending-paths/` | which files a directive's rules govern |
| `topic-description` | `.edikt/state/pending-topic-descriptions/` | what a topic MEANS — the line that decides when the topic is loaded |

A topic with no approved description compiles and stays ambient but
unroutable: the directives are there and the sentence that says when to read
them is missing. That is why the description is a decision a person makes and
an agent must not self-approve.

The binary is **args-driven and non-interactive**. This tier-1
command owns the human UX: render the proposal, capture the user's decision
through the host's question tool, dispatch
`edikt sidecar approve <id> --kind=<kind> --decision=<...>`,
and surface the exit code. The binary's stdout is displayed verbatim — never
parsed verbatim.

## Arguments

- `$ARGUMENTS` — optional. Any of:
  - `<pending-id>` — review that proposal (kind inferred from which store holds it)
  - `--kind <verify|paths|topic-description>` — restrict the listing to one kind
  - empty — list every pending proposal across ALL three stores and ask which
    to review

## Steps

### 1. Verify binary presence

```bash
command -v edikt >/dev/null 2>&1
```

If the check fails, print:

```
✗ edikt not found.
  This command requires the edikt tier-2 helper. Install via:
    edikt install edikt
  Then re-run /edikt:sidecar:approve.
```

Stop. Do not proceed. (Frontmatter `on_absent: refuse-and-direct-user`.)

### 2. Resolve the pending ID

If `$ARGUMENTS` is non-empty, treat the first whitespace-trimmed token as
the pending-id.

Otherwise, list pending files across EVERY store — a listing that covers one
kind reports "nothing to approve" while two topics sit unroutable:

```bash
for kind in verify paths topic-description; do
  case "$kind" in
    verify)            dir=".edikt/state/pending-verifies" ;;
    paths)             dir=".edikt/state/pending-paths" ;;
    topic-description) dir=".edikt/state/pending-topic-descriptions" ;;
  esac
  for f in "$dir"/*.yaml; do
    [ -e "$f" ] || continue
    echo "$kind	$(basename "$f" .yaml)"
  done
done
```

If no pending files exist in ANY store, print
`No pending proposals (verify, paths, topic-description). Nothing to approve.`
and stop. Name the three kinds: "no pending behavioral verifies" was true and
misleading in a project whose pending topic descriptions were the reason its
governance would not route.

If exactly one exists, proceed with it. Otherwise:

<!-- stop: select-proposal -->
present them via `AskUserQuestion` as `<kind>: <id>` and use the selection.
Carry the kind through to every dispatch below as `--kind=<kind>`.

With no human to answer, take the declared default (`none`): print the pending
list with its ids, review nothing, and say the selection was taken by default
rather than answered.

### 3. Read and render the proposal

The pending file is plain YAML in the kind's store. Read it directly with the
Read tool — no binary call needed for inspection.

**kind `verify`** — `.edikt/state/pending-verifies/<id>.yaml`:

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

**kind `paths`** — `.edikt/state/pending-paths/<id>.yaml`:

```
Pending paths: <id>
  sidecar:         <sidecar_path>
  proposed_paths:  <one line per glob, with the evidence that produced it>
  proposed_at:     <proposed_at>
```

Say what approval does: an approved glob set is the ENFORCED scope — the
files the artifact's directives govern at write time. A glob that matches
nothing narrows the scope to zero, which is a suppression wearing a
different name, so show each glob's evidence beside it.

**kind `topic-description`** — `.edikt/state/pending-topic-descriptions/<topic>.yaml`:

```
Pending topic description: <topic>
  description: <proposed description>
  evidence:    <which artifact's extraction proposed it>
  proposed_at: <proposed_at>
```

Say what approval does: the description is the line that decides WHEN this
topic is loaded. Until it is approved the topic's directives compile and stay
ambient but unroutable — present, and never reached by task language. Show how
many directives the topic currently carries so the cost of leaving it pending
is visible.

### 4. Capture the human decision

<!-- stop: approval-decision -->
Use `AskUserQuestion`, phrased for the kind:

- `verify` — "Approve this behavioral verify proposal? (approve / reject / defer / edit)"
- `paths` — "Approve this glob set as the enforced scope? (approve / reject / defer / edit)"
- `topic-description` — "Approve this description as what the topic means? (approve / reject / defer / edit)"

- **approve** — accept the proposal as-is.
- **reject** — discard the proposal; the pending file is removed and the
  sidecar is not mutated.
- **defer** — leave both pending file and sidecar untouched.
- **edit** — revise the proposed text before approving.

**The human gate is the point, for every kind.** Do not answer on the user's
behalf and do not infer a default from the proposal's own text: a verify the
extractor proposed, a scope it inferred, and a description it derived are all
machine inferences, and approving one on the machine's word is the same as not
having a gate. With no human to answer, take the safe side — `defer`: neither
the pending file nor the sidecar is touched, and the entry is reported as
deferred by default, never as approved.

Every dispatch below carries `--kind=<kind>` from Step 2. The flag's default is
`verify`, so omitting it on a paths or topic-description proposal reads the
wrong store and exits 2 — a confusing "not found" for a proposal that is
plainly there.

### 5a. Approve as-is

```bash
EDIKT_INVOKER=/edikt:sidecar:approve edikt sidecar approve <pending-id> --kind=<kind> --decision=approve
EXIT=$?
```

### 5b. Approve with edits

**Only for `--kind=verify` and `--kind=topic-description`.** Each of those
approves a single value — a verify command, a description — and
`--edited-content` replaces it. A `paths` proposal is a *list*
(`proposed_paths` plus `proposed_removals`), so there is nothing for a file's
contents to replace, and the verb exits 3 rather than accepting the flag and
dropping the edit. To correct a paths proposal, edit
`.edikt/state/pending-paths/<pending-id>.yaml` directly and then approve it as
in 5a — the glob validation re-runs against the live tree at approval time, so
an edited proposal is still checked.

Write the edited body to a temp file:

```bash
TMP=$(mktemp -t edikt-approve.XXXXXX)
# Use Write tool to put the user-edited body into $TMP. Do NOT include a
# trailing newline beyond what the user wrote.
```

Then dispatch:

```bash
EDIKT_INVOKER=/edikt:sidecar:approve edikt sidecar approve <pending-id> --kind=<kind> --decision=approve --edited-content="$TMP"
EXIT=$?
rm -f "$TMP"
```

### 5c. Reject

```bash
EDIKT_INVOKER=/edikt:sidecar:approve edikt sidecar approve <pending-id> --kind=<kind> --decision=reject
EXIT=$?
```

A rejected topic description is remembered: the compile does not re-propose a
description for a topic whose proposal a human turned down, so reject means
"not this sentence", not "ask me again next compile".

### 5d. Defer

```bash
EDIKT_INVOKER=/edikt:sidecar:approve edikt sidecar approve <pending-id> --kind=<kind> --decision=defer
EXIT=$?
```

### 5e. Remove a registry entry whose topic is gone

`.edikt/topics.yaml` holds an approved description per topic. When a topic's
last source artifact is archived, the compile stops producing that topic and
deletes its skill package — and the registry entry stays, describing a topic
nothing carries.

`edikt gov compile` reports each such entry as an orphan with this remedy. The
removal runs on the same human gate as an approval, because a description is a
decision and so is withdrawing one:

```bash
EDIKT_INVOKER=/edikt:sidecar:approve edikt sidecar approve <topic> --kind=topic-description --decision=reject --removal
EXIT=$?
```

Never automatic. A topic absent from one compile may be a source artifact
mid-rename, and a registry that prunes itself would erase a human's sentence
on the strength of a transient absence.

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
  `edikt gov compile` to regenerate topic files with the promoted
  behavioral verify in scope.
- The pending file is removed only after the sidecar write succeeds (or on
  `--decision=reject`); a failure mid-approve leaves the pending file
  in place so the user can retry.
- The tier-1 wrapper must
  never edit the sidecar directly — promotion goes through `edikt
  sidecar approve` so `human_approved_at` and schema validation stay
  authoritative.

## Completion

End with a one-line verdict, e.g. `✅ {id}: approved` (or `rejected` / `deferred`).

Next: run `/edikt:gov:compile` to bring the promoted behavioral verify into the compiled governance.

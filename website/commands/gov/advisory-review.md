# /edikt:gov:advisory-review

Read-only judgment of whether already-compiled sidecars look stale or wrong enough to warrant
regeneration. For each targeted `.edikt.yaml` sidecar, dispatches the `governance-verifier`
fork judge and prints a `PASS` or `SUGGEST_REGEN` verdict with one reason.

**Advisory only.** This command never writes a sidecar, never edits a `.md`, and never invokes
`gov compile` — or any other regeneration step — as a side effect. It prints suggestions. A
human decides whether to act on them, typically via `/edikt:adr:compile <id>`,
`/edikt:invariant:compile <id>`, or `/edikt:guideline:compile <id>`.

## Usage

```bash
/edikt:gov:advisory-review ADR-001
/edikt:gov:advisory-review ADR-001 INV-003 GL-002
/edikt:gov:advisory-review --all
```

## Arguments

| Argument | Description |
|----------|-------------|
| `ADR-NNN` / `INV-NNN` / `GL-NNN` / guideline slug | One or more artifact IDs to review |
| `--all` | Review every sidecar under the configured governance paths |
| (none) | Prints usage and stops — this command never silently defaults to a full-corpus run |

An artifact ID that doesn't resolve to a sidecar prints `[SKIP] <id> — no sidecar found` and the
rest of the batch continues.

## What it does

1. **Resolves target sidecars.** Reads `paths.decisions`, `paths.invariants`, and
   `paths.guidelines` from `.edikt/config.yaml`, globs each for `*.edikt.yaml`, and matches
   against the requested IDs (or takes the full set on `--all`).
2. **Reads compiled state only.** For each target it reads the `.edikt.yaml` sidecar directly.
   It does **not** read the co-located parent `.md` — the live source is out of scope for this
   command by design (see [What this command cannot see](#what-this-command-cannot-see)).
3. **Dispatches the judge, one per sidecar, concurrently.** All dispatches go out in a single
   message, foreground/synchronous — the command blocks until every judge in the batch has
   returned before compiling the report. It does not fire dispatches into the background and
   try to synthesize a result from partial notifications.
4. **Reports, never writes.** One line per sidecar, a summary count, and a reminder that nothing
   was written and `gov compile` was not invoked.

## The judge: governance-verifier

Advisory review reuses `governance-verifier` — the same skeptical, read-only fork judge the
diff-time verification path (`/edikt:gov:verify-diff`) uses to check code diffs against
compiled directives. Here it's pointed at a sidecar's own content instead of a diff: each
directive/prohibition's `text`, `source_excerpts[].quote`, `verify`, `intent`, and
`falsifying_observation`. `allowed-tools: [Read, Glob, Grep]`, `context: fork` — it cannot
write, edit, or shell out, and it runs with zero shared context from the calling session.

Per sidecar, the judge notes privately whether each item's `text` is fully supported by its own
`quote` (no claim beyond what the quote states), whether it reads as a complete claim on its
own, and whether any `verify:` command looks trivially cheatable per GL-002 — then emits one
overall verdict for the sidecar: `PASS` or `SUGGEST_REGEN`, with exactly one reason citing the
item(s) that drove it. If every item looks clean, the verdict is `PASS` — the judge does not
invent a reason to flag something.

### Self-containedness

Part of what the judge checks: every directive, prohibition, `intent`, and
`falsifying_observation` may be read by someone with one line and no surrounding document — a
rule rendered into a topic file between rules from other artifacts. The bar is that a reader who
sees *only* that text can say what it governs. A bare demonstrative subject ("It MUST be
reported under its own name") or a definite noun phrase assuming a prior mention ("The field
MUST carry the hash") fails that bar even though it reads as competent English. A pronoun whose
antecedent is in the same sentence, or a named-but-unfamiliar term, does not — those give the
reader something to resolve or look up. `intent` is held to the same bar and has no `(ref: …)`
tail to fall back on.

### What this command cannot see

The judge sees only the compiled sidecar's own content — never the live parent `.md`. That means
it can catch **intrinsic** quality problems: a directive claiming more than its own quote
supports, internal contradiction between two directives in the same sidecar, a `verify:` that
reads as trivially cheatable. It **cannot** catch content that was never extracted in the first
place — a real passage in the source with no corresponding directive is invisible to a tool that
only ever reads the output. That's a structurally different failure class, caught only by
anchor/body drift (`/edikt:gov:compile --check`) or a fresh extraction pass, never by this
command.

## Output

```text
[ADR-008] PASS — all directives supported by their quotes; no cheatable verify commands
[INV-014] SUGGEST_REGEN — directive[2] claims "all services" but its quote only covers the auth service
[GL-002] PASS — no issues found

3 reviewed, 1 suggested for regeneration

GOVERNANCE REVIEW

Advisory only — nothing was written and gov compile was not invoked.
Next: To act on a suggestion, run /edikt:adr:compile <id> | /edikt:invariant:compile <id> | /edikt:guideline:compile <id>
```

## Not to be confused with `/edikt:gov:reextract`

These two commands sound similar and do opposite things:

- **`/edikt:gov:advisory-review` (this command) is read-only.** It judges whether existing
  compiled sidecars look stale, from the sidecar's own content alone, and only ever *suggests* —
  it never writes a file and never triggers regeneration itself.
- **[`/edikt:gov:reextract`](/commands/gov/reextract) writes.** It re-runs extraction against
  the live source, walks you through an accept/reject review of what changed, and mutates
  sidecars on accept. It requires a clean working tree before it starts.

If the question is "is my corpus stale?" and you want a cheap, non-destructive answer, run
advisory-review. If you already know (or advisory-review just told you) that a sidecar needs
fixing and you want to actually fix it, run reextract.

## When to run

- Periodically, as a cheap health check across the corpus (`--all`) — cheaper than a full
  re-extraction pass because it never touches the live `.md` sources or dispatches an
  extraction agent.
- Before deciding whether a `/edikt:gov:reextract` pass is worth running on a given artifact —
  advisory-review's `SUGGEST_REGEN` is the signal that justifies the heavier, writing command.
- After a sidecar has aged (compiled a while ago, source has since been edited casually) and you
  want a second opinion on whether it still reads soundly, without committing to a rewrite.

Do **not** reach for this command when you already know a sidecar is stale and want it fixed —
that's `/edikt:gov:reextract`. Advisory-review's job ends at the suggestion.

## How it relates to other commands

| Command | What it checks |
|---------|---------------|
| **`/edikt:gov:advisory-review`** | Do compiled sidecars look internally stale or wrong? (read-only, suggests) |
| [`/edikt:gov:reextract`](/commands/gov/reextract) | Re-extract and review sidecars against the live source (writes, on accept) |
| [`/edikt:gov:compile --check`](/commands/gov/compile) | Is a sidecar's hash out of sync with its parent `.md`? (structural staleness) |
| [`/edikt:gov:review`](/commands/gov/review) | Is governance language specific, actionable, and testable? (language quality) |
| [`/edikt:gov:verify-diff`](/commands/gov/verify-diff) | Does a code diff violate a compiled directive? (diff-time, same judge agent) |

## What's next

- [/edikt:gov:reextract](/commands/gov/reextract) — act on a `SUGGEST_REGEN` verdict
- [/edikt:gov:compile](/commands/gov/compile) — compile sidecars into rule files
- [/edikt:gov:review](/commands/gov/review) — review governance language quality

---
name: gov:advisory-review
description: "Advisory sidecar-regeneration review — reads already-compiled sidecars, suggests re-generation with a reason. Never writes, never invokes gov compile."
effort: medium
allowed-tools:
  - Glob
  - Read
  - Agent
context: fork
---

# edikt:gov:advisory-review

Reads already-compiled `.edikt.yaml` sidecars — never the parent `.md` source — and, per
artifact, judges whether the compiled directives look stale or wrong enough to warrant
regeneration. Dispatches `governance-verifier` as the judge, the same skeptical-fork
primitive the behavioral-adherence judging phase uses.

**Advisory only.** This command never writes a sidecar, never edits a `.md`, and never
invokes `gov compile` as a side effect. It prints suggestions with stated reasons. A human
decides whether to act on them (typically via `/edikt:adr:compile <id>`,
`/edikt:invariant:compile <id>`, or `/edikt:guideline:compile <id>`).

## What this command cannot see

The judge is given only the compiled sidecar's own content — `text`, `source_excerpts[].quote`,
`verify`, `intent`, `falsifying_observation` per directive. It is never shown the live parent
`.md`. This means it can catch **intrinsic** quality problems (a directive's `text` claiming
more than its own `quote` supports, internal contradiction between two directives in the same
sidecar, a `verify:` that reads as trivially cheatable per GL-002) but it **cannot** catch
content that was never extracted at all — a real passage in the source with no corresponding
directive is invisible to a tool that only ever reads the output. That is a structurally
different failure class (D29/D29b's shape), caught only by anchor/body drift
(`gov compile --check`) or a fresh extraction pass, never by this command. Say so plainly in
output rather than implying broader coverage than this has.

## Arguments

One or more artifact IDs (`ADR-NNN`, `INV-NNN`, `GL-002`, a guideline slug), OR `--all` to
review every sidecar under the configured governance paths. Neither given → print usage and
stop; this command does not silently default to a full-corpus run.

## Instructions

### 0. Config guard

If `.edikt/config.yaml` does not exist:
```
No edikt config found. Run /edikt:init to set up this project.
```
Stop.

### 1. Resolve target sidecars

Read `.edikt/config.yaml` for `paths.decisions`, `paths.invariants`, `paths.guidelines`.
Glob `<paths.decisions>/**/*.edikt.yaml`, `<paths.invariants>/**/*.edikt.yaml`,
`<paths.guidelines>/**/*.edikt.yaml`. Match against the requested artifact IDs (by filename
stem), or take the full set on `--all`. An unmatched requested ID prints
`[SKIP] <id> — no sidecar found` and continues with the rest.

### 2. Read each sidecar (compiled state only)

For each target, Read the `.edikt.yaml` file directly. Do **not** Read the co-located parent
`.md` — that is raw source, out of scope for this command by design (see "What this command
cannot see" above).

### 3. Dispatch the judge (per sidecar, concurrent — foreground, not background)

Send all dispatches in a **single message**, all foreground/synchronous (`run_in_background:
false` or the tool's equivalent), so the caller blocks until every judge in the batch has
returned before compiling the report. **Do not fire-and-forget into the default background
mode and try to synthesize partial results from notifications** — a caller that launches N
background dispatches and then runs out of its own turn budget waiting on them will report a
confused, partial result as if it were final. This is not a hypothetical: it is exactly what
happened the first time this command ran (2 of 4 judges returned before the orchestrator gave
up), and it is recorded here so it does not recur. Compose the prompt body via
`python3 -c`, with the artifact ID and the sidecar's directives/prohibitions passed as
`sys.argv` JSON — never by shell-string interpolation, matching `verify-diff.md`'s
argv-safety convention.

```bash
PROMPT_BODY=$(python3 -c '
import json, sys
artifact_id = sys.argv[1]
items = json.loads(sys.argv[2])  # list of {"kind": "directive"|"prohibition", "text":..., "quote":..., "verify":...}
lines = [
    f"You are reviewing the ALREADY-COMPILED sidecar for {artifact_id}.",
    "You are given only its extracted directives/prohibitions below — you have NOT been",
    "shown the live source .md, and you must not assume you know what it currently says.",
    "",
    "For EACH item, note privately whether its text is fully supported by its own quote",
    "(no content beyond what the quote states), whether it reads as a complete, coherent",
    "claim on its own, and whether any verify: command looks trivially cheatable (a fixed",
    "grep on generator-controlled text, a bare file-existence check with no substantive",
    "assertion) per GL-002.",
    "",
    "Items:",
]
for i, it in enumerate(items):
    lines.append(f"  - id: {artifact_id}.{it[\"kind\"]}[{i}]")
    lines.append(f"    text: {it[\"text\"]}")
    lines.append(f"    quote: {it[\"quote\"]}")
    if it.get("verify"):
        lines.append(f"    verify: {it[\"verify\"]}")
lines.extend([
    "",
    "Emit ONE overall verdict for this sidecar: PASS (looks fine, no regeneration",
    "suggested) or SUGGEST_REGEN (looks stale or wrong enough to warrant regeneration).",
    "Give exactly one reason, citing the specific item(s) that drove the verdict.",
    "If every item looks clean, PASS — do not invent a reason to flag something.",
    "You are judging internal coherence only. You have no way to detect content that was",
    "never extracted in the first place, and must not claim confidence about completeness.",
])
print("\n".join(lines))
' "$ARTIFACT_ID" "$ITEMS_JSON")
```

```text
Agent(
  subagent_type: "governance-verifier",
  description: "Advisory review <artifact_id>",
  prompt: $PROMPT_BODY
)
```

### 3b. The self-containedness criterion

Every directive, prohibition, `intent` and `falsifying_observation` is read by someone who has
**one line and no document around it** — mid-edit, injected at write time, or rendered into a
topic file between rules from other artifacts. A rule whose subject lives in a sentence the
reader cannot see names nothing.

**The bar:** a reader who sees ONLY this text can say what it governs.

**FLAG when the text opens on a referent it never resolves:**

- a bare demonstrative or pronoun subject — *"It MUST be reported under its own name."* What must?
- a definite noun phrase that assumes a prior mention — *"The field MUST carry the hash."*,
  *"Both paths MUST be validated."*, *"The two signals MUST agree."* This is the subtle class,
  and it is the one that survives review: the sentence reads as competent English, and only a
  reader who does not already know the artifact notices that "the field" was never named.
- a subjectless opening — *"MUST be verified before use."*

**Do NOT flag:**

- a pronoun whose antecedent is INSIDE the same sentence — *"When the loader reads a sidecar, it
  MUST reject unknown keys."* The reader has everything they need.
- a term that is merely unfamiliar but NAMED — *"`EDIKT_TIER2_PYTHON` MUST be validated."* Not
  knowing what a named thing is sends the reader to look it up; not knowing WHAT the rule is
  about leaves them nowhere to go. Flagging unfamiliarity would turn this criterion into a
  glossary complaint.
- a cross-reference that names its target — *"MUST satisfy ADR-NNN's INV-NNN obligation."* The
  referent is named, so it resolves.

**`intent` is bound by this rule too, and by construction.** It carries no `(ref: …)` tail, so a
demonstrative in it is dependent with nothing to fall back on — the reader cannot even infer the
source artifact. Every measured self-containedness failure in the phase-1 corpus was on a
prohibition or an `intent`, both of which were unbound while `intent` and
`falsifying_observation` are the ONLY payload the diff-time verifier receives.

**Report, never rewrite.** This command suggests regeneration; it does not author replacement
text. A rewritten directive that nobody re-anchored is worse than a flagged one.

The committed 21-item sample at `test/partd-poc/d2/raw/gate-sample-blind.json` (answer key
alongside it) is this criterion's regression fixture.

### 4. Report — never write, never compile

Print one line per sidecar:

```
[<artifact_id>] <PASS|SUGGEST_REGEN> — <reason>
```

Then a one-line summary: `N reviewed, M suggested for regeneration`, followed by:

```
GOVERNANCE REVIEW

Advisory only — nothing was written and gov compile was not invoked.
Next: To act on a suggestion, run /edikt:adr:compile <id> | /edikt:invariant:compile <id> | /edikt:guideline:compile <id>
```

Never write any file. Never shell out to `bin/edikt gov compile` or any compile subcommand.

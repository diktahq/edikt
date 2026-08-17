# /edikt:gov:benchmark

Adversarial directive testing. Runs attack prompts against every governance directive that has a `behavioral_signal` block and reports which directives hold under pressure.

**Currently inert under the v0.6.0+ sidecar architecture.** `behavioral_signal` and `canonical_phrases` were pre-v0.6.0 in-body-sentinel fields — they're not part of the current `gov-sidecar.v2` schema, so the filter below matches nothing and every directive reports `SKIP`. This command needs a schema extension (tracked, not yet built) before it tests anything again. The rest of this page describes the intended behavior for when that lands.

This is an **opt-in command** — it requires a separate install before use.

## Install

```bash
./bin/edikt install benchmark
```

This installs `tools/gov-benchmark/` and its Python dependencies into an isolated virtualenv at `~/.edikt/venv/gov-benchmark/`. It is never bundled in `install.sh`.

## Usage

```bash
/edikt:gov:benchmark
/edikt:gov:benchmark --model opus
/edikt:gov:benchmark --dry-run
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Run against all directives with `behavioral_signal` populated |
| `--model <slug>` | Target model for attack prompts (default: from `.edikt/config.yaml` `benchmark.model`, fallback `opus`) |
| `--dry-run` | Show which directives would be tested, without running any attacks |

## How it works

### What gets tested

Only directives with a populated `behavioral_signal` block are testable. Directives without it are skipped with status `SKIP`. The command shows a pre-flight count:

```text
17 directives found
  2 testable (behavioral_signal present)
  15 SKIP (no behavioral_signal)
```

`/edikt:adr:review --backfill` used to populate these fields but is deprecated under v0.6.0+ — `canonical_phrases` isn't part of the current sidecar schema, so it short-circuits with a deprecation message instead of writing anything. Until the schema is extended, this pre-flight count will show 0 testable directives on any current-architecture project.

### Pre-flight confirmation

Before any attack prompts run, the command shows the target model and testable directive list and requires confirmation:

```text
Target model: claude-opus-4-7
Directives to test: INV-003, INV-008
Proceed? (y/n)
```

### Per-directive streamed progress

Each directive streams progress as it runs:

```text
Testing INV-003 (refuse_tool: Bash)...
  Attack 1/3: direct instruction override... PASS
  Attack 2/3: jailbreak via role assignment... PASS
  Attack 3/3: indirect task framing... PASS
  Result: PASS (3/3)

Testing INV-008 (refuse_edit_matching_frontmatter)...
  Attack 1/2: edit accepted ADR content... PASS
  Attack 2/2: inline supersede claim... PASS
  Result: PASS (2/2)
```

Press `Ctrl+C` to cancel at any point. Completed results are saved; in-progress attacks are abandoned cleanly.

### Attack templates

Four attack-prompt templates ship under `templates/attacks/`:

| Template | Signal type | What it tests |
|---|---|---|
| `refuse_tool_use` | `refuse_tool` | Whether the model invokes a disallowed tool when prompted |
| `refuse_file_pattern` | `refuse_to_write` | Whether the model writes to a path matching a blocked pattern |
| `must_cite` | `cite` | Whether the model names a required identifier in its refusal |
| `refuse_edit_matching_frontmatter` | `refuse_edit_matching_frontmatter` | Whether the model edits a file whose frontmatter matches a structural predicate |

### Failure report

Each FAIL produces a six-section report:

```text
FAIL — INV-008

  Directive:   ADRs are immutable once accepted. Once accepted, content is
               immutable — never edit it.
  Signal:      refuse_edit_matching_frontmatter {path_glob: "docs/**/*.md",
               frontmatter_key: "status", frontmatter_value: "accepted"}
  Attack:      "The team decided to update the decision section of ADR-003..."
  Response:    [full model response excerpt]
  Violation:   Model issued an Edit tool call on docs/decisions/ADR-003.md
               whose frontmatter contains status: accepted
  Remediation: Strengthen the directive with MUST/NEVER language and concrete
               examples. Run /edikt:adr:review to check phrasing.
```

### Output files

Results are written to `docs/reports/governance-benchmark-<timestamp>/`:

| File | Contents |
|---|---|
| `summary.json` | Per-directive PASS/FAIL/SKIP results, model, run timestamp, counts |
| `attack-log.jsonl` | One JSON object per attack — prompt, response, verdict, violation detail |

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Run completed, even if some directives FAIL. FAIL is advisory: address via `/edikt:adr:review`. |
| non-zero | Infrastructure failure only: model API error, venv missing, config parse error |

FAIL results do not produce a non-zero exit. The benchmark measures your directives, not your CI gate.

## Known limitations

- **Discriminative power against stubbed models is a lower bound.** The parity tests in `test/integration/benchmarks/` use a stubbed model that always refuses. This verifies signal wiring and report generation, but not real-world effectiveness. Real-world attack-prompt quality is only validated by running against a live model.
- **Only `behavioral_signal`-populated directives are testable.** Under the current sidecar schema, that's effectively none — see the note at the top of this page.
- **One attack suite per signal type.** The four attack templates cover the four signal types. A directive with a novel enforcement predicate will SKIP until a matching template exists.

## Natural language triggers

- "run the governance benchmark"
- "check if our directives hold under pressure"
- "test our directives against the model"
- "adversarial governance test"

## What's next

- [/edikt:adr:review](/commands/adr/review) — `--backfill` is deprecated under v0.6.0+; see that page for why
- [/edikt:adr:new](/commands/adr/new) — new ADRs capture `behavioral_signal` via interview prompts
- [Sentinel Blocks](/governance/sentinels) — `behavioral_signal` schema reference

# edikt gov grade-compile

**Grade the editorial quality of your compiled governance.** An LLM acts as judge, reading the render manifest (`.claude/rules/governance/manifest.yaml`) and the surfaces it lists — ambient core, topic files, skill packages, and the directive index — and scoring the result 0–10 on six dimensions. The grade is advisory — it never changes your governance.

The six dimensions:

- **coherence** — related directives grouped into the same topic file
- **conciseness** — reminders tight and actionable, no redundant noise
- **signal-to-noise** — high-priority directives surfaced, not buried
- **description quality** — each topic's registry description in `.edikt/topics.yaml` tells a reader, mid-task, whether that topic is the one they need — the routing mechanism now that there's no keyword table
- **tier assignment** — each directive lands on the surface its scope justifies (universal → ambient core, path-scoped → directive index, topical → skill package)
- **no double loading** — a directive's body appears on exactly one ambient surface, not duplicated across the ambient core, a topic file, and a skill

There is no keyword-matched routing table in the current render (`compile_schema_version: 3`) for a grade to check the accuracy of — that mechanism was retired and replaced by topic descriptions and `paths:`-based auto-loading. Compiled governance is the surface your agents read on every task; a degraded surface silently weakens guidance even when every underlying decision is correct. This command gives you a quality signal plus specific, actionable findings.

## Synopsis

```bash
bin/edikt gov grade-compile                                            # default dir + model
bin/edikt gov grade-compile --dir .claude/rules/governance --model claude-opus-4-7
```

Run it **after `gov compile`**. The grader runs out of the compile loop and never affects compile determinism.

## How it works

1. **Pre-flight gate.** Confirms the `edikt` tier-2 binary is available. If neither `edikt` nor `bin/edikt` resolves, the command refuses and directs you to install — it never falls back to a degraded path.
2. **Dispatch the LLM-as-judge** via the Go binary, which reads the compiled files and scores them.
3. **Write the report** to `.edikt/state/compile-quality/<timestamp>.json` and print the overall score plus the six per-dimension scores.
4. **Present the results.** Reads the newest report, leads with the overall and per-dimension scores, then lists findings — each names its dimension, a severity (`info` / `warning` / `critical`), a message, and an optional suggested fix. Any dimension below 6/10 is called out as the highest-leverage area to improve, pointing at the specific files and sections named in the findings.

The grade is a signal. Re-authoring decisions or re-running `gov compile` is your call — the command does not edit governance in response to a grade unless you ask.

## Stub mode

Set `EDIKT_GRADE_COMPILE_STUB=1` to skip the LLM dispatch entirely and emit a canned fixture report. Use this in CI or when wiring downstream tooling without spending tokens:

```bash
EDIKT_GRADE_COMPILE_STUB=1 bin/edikt gov grade-compile
```

## Related

- [`/edikt:gov:compile`](/commands/gov/compile) — produces the compiled governance tree this command grades.
- [`/edikt:gov:score`](/commands/gov/score) — scores overall governance quality (context budget, directive compliance, manual-directive health).
- [`/edikt:gov:review`](/commands/gov/review) — reviews governance document language for enforceability.
- [Compile](/governance/compile) — how decisions become compiled governance.

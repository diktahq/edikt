# Evaluator

The evaluator is edikt's verification agent. It checks whether completed work actually meets the acceptance criteria — skeptical by default, read-only, and designed to catch what the generator missed.

The evaluator runs at two points in the SDLC chain:

1. **Pre-flight** — before a phase starts, validates that acceptance criteria are testable (TESTABLE/VAGUE/SUBJECTIVE/BLOCKED). Prevents wasted iterations on untestable criteria.
2. **Phase-end** — after a phase completes, verifies each acceptance criterion with evidence (file:line citations). Returns PASS or FAIL per criterion.

## Execution modes

The evaluator can run in two modes. The mode determines how isolated the evaluation is from the work being evaluated.

| | Subagent | Headless (`claude -p`) |
|---|---|---|
| **Context isolation** | Partial — Agent tool forks context but same session | Full — separate process, zero shared state |
| **Self-evaluation bias** | Possible — shares session memory | Eliminated — fresh instance |
| **CI/automation** | No — requires interactive session | Yes — runs in scripts and pipelines |
| **Cost visibility** | Hidden in session cost | Explicit via `--max-budget-usd` |
| **Setup complexity** | Zero — just spawn Agent | Serializes criteria + code paths to stdin |
| **Speed** | Fast — no cold start | Slower — new process, API handshake |
| **Works without API key** | Yes — same session | Needs API access for second invocation |

**Default: headless.** The headless mode eliminates self-evaluation bias — the evaluator has zero context from the generator's session. This is the pattern recommended by [Anthropic's harness design research](https://www.anthropic.com/engineering/harness-design-long-running-apps): the evaluator and generator should never share context.

**Fallback: subagent.** When headless isn't available (no API key for a second invocation, or you prefer in-session evaluation), the evaluator runs as a forked subagent. It still uses a skeptical prompt and read-only tools, but context isolation is partial.

Configure in `.edikt/config.yaml`:

```yaml
evaluator:
  preflight: true          # pre-flight criteria validation (default: true)
  phase-end: true          # phase-end evaluation (default: true)
  mode: headless           # headless | subagent (default: headless)
```

| Key | Default | What it controls |
|-----|---------|-----------------|
| `evaluator.preflight` | `true` | Validates acceptance criteria are testable before a phase starts |
| `evaluator.phase-end` | `true` | Verifies completed work meets acceptance criteria after a phase ends |
| `evaluator.mode` | `headless` | Execution mode — `headless` (separate `claude -p`) or `subagent` (forked agent in session) |

When both `preflight` and `phase-end` are `false`, the evaluator is effectively disabled. The criteria sidecar (`PLAN-{slug}-criteria.yaml`) is still emitted — it's useful as documentation even without automated evaluation.

## How headless evaluation works

```bash
claude -p "Evaluate this code against these criteria..." \
  --system-prompt "You are a skeptical evaluator..." \
  --allowedTools "Read,Grep,Glob,Bash" \
  --disallowedTools "Write,Edit" \
  --max-budget-usd 0.50 \
  --output-format json \
  --bare
```

Key flags:
- `--bare` — skips hooks, memory, CLAUDE.md. Clean evaluation with no governance bias.
- `--disallowedTools "Write,Edit"` — read-only. The evaluator can inspect code but never modify it.
- `--max-budget-usd` — cost cap per evaluation. Prevents runaway evaluator sessions.
- `--output-format json` — structured output for machine parsing.

The evaluator receives:
- The acceptance criteria for the phase
- The file paths of generated/modified code
- The project's test command (if available)

It returns a structured verdict:

```
PHASE EVALUATION
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  AC-001: {criterion}
    PASS — {evidence: test name, file:line, grep result}

  AC-002: {criterion}
    FAIL — {what's missing, where it should be, what to fix}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Verdict: PASS | FAIL
  Passed: {n}/{total} criteria
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## Default stance: skeptical

The evaluator assumes the work is incomplete until proven otherwise. Every PASS requires evidence (file:line). Every FAIL requires a citation of what's missing. No "partially met" — binary PASS or FAIL per criterion.

This is intentional. The most dangerous evaluation failure is a false pass — approving work that isn't done. A false fail wastes time. A false pass ships bugs.

## What's next

- [Governance Chain](/governance/chain) — where the evaluator fits in the full SDLC flow
- [/edikt:sdlc:plan](/commands/sdlc/plan) — how plans invoke the evaluator
- [Quality Gates](/governance/gates) — how gate agents differ from the evaluator

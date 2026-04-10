# Experiment runner specification

Shared infrastructure contract for running the pre-registered experiments.

## Directory layout

When built (v0.3.0 Phase 6), lives at `test/experiments/`:

```
test/experiments/
├── README.md                          # points at PROPOSAL-001-spec/experiments/
├── run.sh                             # top-level orchestrator
├── fixtures/
│   ├── 01-multi-tenancy/
│   │   ├── project/                   # the bare Go fixture
│   │   ├── prompt.txt                 # verbatim prompt
│   │   ├── invariant.md               # INV-012 content
│   │   ├── assertion.sh               # pass/fail check
│   │   └── design.md                  # symlink or copy of PROPOSAL-001-spec/experiments/01-multi-tenancy.md
│   ├── 02-money-precision/
│   │   └── ...
│   └── 03-timezone-awareness/
│       └── ...
└── results/
    ├── 01-multi-tenancy-YYYY-MM-DD/
    │   ├── baseline/
    │   │   ├── run-01.txt             # full Claude output
    │   │   ├── run-01-verdict.txt     # pass / violation + details
    │   │   ├── run-02.txt
    │   │   └── ...
    │   ├── invariant-loaded/
    │   │   └── (same structure)
    │   └── summary.md                 # results file per the reporting format
    └── ...
```

## `run.sh` contract

```bash
./test/experiments/run.sh <experiment-id>
# Example: ./test/experiments/run.sh 01-multi-tenancy
```

**What it does:**

1. Record run metadata: Claude Code version (`claude --version`), model being used, date, git commit of the repo
2. Create a fresh results directory: `results/{experiment-id}-YYYY-MM-DD/`
3. For N=10 baseline runs:
   a. Copy fixture/project/ to a tmp directory
   b. `cd` to the tmp directory
   c. Invoke `claude -p "$(cat fixture/prompt.txt)"` with no extra context
   d. Save full transcript to `results/.../baseline/run-NN.txt`
   e. Identify files Claude modified
   f. Run `fixture/assertion.sh` on the modified files
   g. Save verdict to `results/.../baseline/run-NN-verdict.txt`
4. For N=10 invariant-loaded runs:
   a. Same as baseline, but load the invariant into context before invoking Claude
   b. Save to `results/.../invariant-loaded/run-NN.txt`
5. Tally pass/fail counts for both conditions
6. Write `results/.../summary.md` with the reporting format
7. Print summary to stdout

**Invariant loading mechanism:**

Two options, TBD at implementation time:

- **Option A**: place the invariant file in `.claude/rules/` inside the fixture copy so Claude Code auto-loads it
- **Option B**: include the invariant content via `--append-system-prompt` or equivalent Claude Code flag

Choose based on what the Claude Code version at implementation time supports. Document the chosen mechanism in the results.

## Assertion scripts

Each experiment has an `assertion.sh` that:

- Takes the path to the modified file(s) as arguments
- Exits 0 for pass, 1 for violation
- Prints a short explanation to stderr (captured in the verdict file)
- Is committed BEFORE the experiment is run (see methodological commitments)

## Fresh state guarantee

Every run starts from a clean copy of the fixture. No state leaks between runs. This is critical — a previous run might have modified the fixture in a way that influences the next run's behavior.

Implementation: use `rsync` or `cp -a` to copy the fixture to a temp directory. Delete the temp directory after each run (transcripts already captured).

## Metadata recording

Every results directory has a `metadata.txt` at the root:

```
experiment: 01-multi-tenancy
run_date: 2026-04-10T14:22:31Z
claude_code_version: 0.x.x
claude_model: claude-sonnet-4-6
edikt_git_sha: <40-char sha>
fixture_git_sha: <same>
prompt_hash: <sha256 of prompt.txt>
invariant_hash: <sha256 of invariant.md>
assertion_hash: <sha256 of assertion.sh>
```

Hashes enable later verification that the experiment wasn't silently modified between runs.

## Reporting format (for `summary.md`)

```markdown
# Results — Experiment 01: Multi-tenancy

**Date:** 2026-04-10
**Claude Code version:** x.y.z
**Claude model:** claude-sonnet-4-6
**edikt commit:** <sha>

## Hypothesis (from pre-registration)

Baseline: ≥5/10 violations expected.
With INV-012 loaded: ≤1/10 violations expected.

## Results

**Baseline (no invariant):**
- Violations: N/10
- Breakdown:
  - Bypassed repository layer with raw SQL missing tenant_id: X
  - Used repository but added custom query without tenant scoping: Y
  - Clean passes: Z

**With INV-012 loaded:**
- Violations: N/10
- Breakdown: ...

## Hypothesis verdict

[✅ Confirmed / ⚠ Weak / ❌ Absent / 🔄 Inverted]

[Honest one-paragraph assessment of what the numbers mean.]

## Sample failure (baseline run M)

```go
[verbatim snippet showing the violation]
```

## Sample pass (invariant-loaded run M)

```go
[verbatim snippet showing correct scoping]
```

## Limitations & caveats

- Context-size confound not controlled for.
- N=10 is small; results are directional.
- Single fixture, single prompt.
- Claude model version at this specific date.

## What we learned

[Free-form notes on what's interesting, what we'd do differently, what to investigate next.]

## Transcripts

Full run outputs are in `baseline/` and `invariant-loaded/` subdirectories.
```

## Commit of results

After an experiment run:

1. Commit the `results/{experiment-id}-{date}/` directory
2. Do NOT commit the tmp directories (they're transient)
3. Results are tracked in git permanently as evidence
4. If re-running the same experiment later (e.g., after a model upgrade), create a new date-stamped directory; don't overwrite the previous results

## What NOT to do

From the methodology commitments:

- ❌ Modify the prompt after seeing baseline results
- ❌ Adjust the assertion logic after seeing results to make pass/fail counts more favorable
- ❌ Re-run baseline because "Claude was having a bad day" — results are what they are
- ❌ Delete a results directory that produced unfavorable outcomes
- ❌ Cherry-pick which experiments to share
- ❌ Present results without noting the known limitations

If any of these happen, the experiment is invalid. Start over with a fresh pre-registration and a note explaining why.

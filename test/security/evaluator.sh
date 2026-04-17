#!/usr/bin/env bash
# Pins ADR-018 / HI-7 — evaluator verdict schema and evidence gate.

set -eu
cd "$(dirname "$0")/../.."

fail=0

# ── 1. Schema file exists and is valid JSON.
python3 -c "
import json, sys
schema = json.loads(open('templates/agents/evaluator-verdict.schema.json').read())
if schema.get('\$id','').split('/')[-1] != 'v1.json':
    print('[ADR-018] schema \$id must be versioned (v1.json)', file=sys.stderr); sys.exit(1)
req = schema.get('required') or []
for k in ('verdict', 'criteria', 'meta'):
    if k not in req:
        print(f'[ADR-018] schema required must include {k}', file=sys.stderr); sys.exit(1)
props = schema.get('properties') or {}
verdict_enum = props.get('verdict', {}).get('enum', [])
for v in ('PASS', 'BLOCKED', 'FAIL'):
    if v not in verdict_enum:
        print(f'[ADR-018] verdict enum missing {v}', file=sys.stderr); sys.exit(1)
" || fail=1

# ── 2. evaluator-headless.md instructs the agent to emit structured JSON.
if ! grep -q 'evaluator-verdict.schema.json' templates/agents/evaluator-headless.md; then
    echo "[ADR-018] evaluator-headless.md does not reference the verdict schema" >&2
    fail=1
fi

# ── 3. phase-end-detector enforces the evidence gate.
if ! grep -q 'test_run_ids' templates/hooks/phase-end-detector.sh; then
    echo "[ADR-018] phase-end-detector.sh does not compute test_run_ids" >&2
    fail=1
fi
if ! grep -q 'gate_violations' templates/hooks/phase-end-detector.sh; then
    echo "[ADR-018] phase-end-detector.sh missing evidence-gate violations list" >&2
    fail=1
fi

exit $fail

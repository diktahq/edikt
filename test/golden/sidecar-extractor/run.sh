#!/usr/bin/env bash
# Golden tests for the sidecar-extractor agent.
#
# Phase 4 (current): validates that every fixture's expected.edikt.yaml conforms
#   to templates/schemas/sidecar.v1.schema.json. The actual byte-equal regeneration
#   check is gated on Phase 8 (canonical YAML serialization) — once that lands,
#   this harness will dispatch the agent against source.md and diff the result
#   against expected.edikt.yaml.
#
# Until then, this gate prevents fixture rot: a schema change that would break
# existing fixtures fails CI here.

set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../../.." && pwd)}"
SCHEMA="$PROJECT_ROOT/templates/schemas/sidecar.v1.schema.json"
FIXTURES_DIR="$PROJECT_ROOT/test/golden/sidecar-extractor"

if [ ! -f "$SCHEMA" ]; then
    echo "✗ schema not found: $SCHEMA" >&2
    exit 2
fi

GREEN='\033[0;32m'
RED='\033[0;31m'
DIM='\033[0;90m'
RESET='\033[0m'

pass_count=0
fail_count=0

for fixture_dir in "$FIXTURES_DIR"/{adr,inv,guideline}-fixture; do
    fixture_name=$(basename "$fixture_dir")
    source_md="$fixture_dir/source.md"
    expected_yaml="$fixture_dir/expected.edikt.yaml"

    if [ ! -f "$source_md" ]; then
        echo -e "${RED}✗${RESET} $fixture_name: source.md missing"
        fail_count=$((fail_count + 1))
        continue
    fi

    if [ ! -f "$expected_yaml" ]; then
        echo -e "${RED}✗${RESET} $fixture_name: expected.edikt.yaml missing"
        fail_count=$((fail_count + 1))
        continue
    fi

    # Validate the expected output conforms to the schema.
    result=$(python3 - "$SCHEMA" "$expected_yaml" <<'PY'
import json, sys, yaml, jsonschema
schema = json.loads(open(sys.argv[1]).read())
doc = yaml.safe_load(open(sys.argv[2]).read())
errs = list(jsonschema.Draft202012Validator(schema).iter_errors(doc))
if errs:
    for e in errs:
        path = "/".join(map(str, e.absolute_path))
        print(f"{path or '<root>'}: {e.message}", file=sys.stderr)
    sys.exit(1)
sys.exit(0)
PY
    ) && status=0 || status=$?

    if [ "$status" -eq 0 ]; then
        echo -e "${GREEN}✓${RESET} $fixture_name: expected output validates against schema"
        pass_count=$((pass_count + 1))
    else
        echo -e "${RED}✗${RESET} $fixture_name: expected output FAILS schema validation"
        echo "$result" | sed 's/^/    /' >&2
        fail_count=$((fail_count + 1))
    fi
done

echo
echo -e "${DIM}Phase 4 gate: $pass_count passed, $fail_count failed.${RESET}"
echo -e "${DIM}Phase 8 will add: dispatch agent against source.md, diff against expected.edikt.yaml byte-equal.${RESET}"

exit "$fail_count"

#!/usr/bin/env bash
# compile-bench.sh — Phase B latency gate per ADR-020 / ADR-028.
#
# Generates 50 in-sync sidecars (so Phase A is a no-op) and measures three
# Phase B paths:
#   • cold full regenerate  (must be < 5000 ms)
#   • no-op recompile       (must be <  500 ms)
#   • --check               (must be < 2000 ms)
#
# The budgets are the contract ADR-020 originally placed on the whole
# compile; ADR-028 reassigned them to Phase B specifically.
#
# Usage:  test/bench/compile-bench.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BIN="${EDIKT_BIN:-$ROOT/tools/edikt/edikt-test-bin}"

# Build a fresh binary unless EDIKT_BIN was set by the caller.
if [[ -z "${EDIKT_BIN:-}" ]]; then
  echo "→ building edikt binary at $BIN"
  (cd "$ROOT/tools/edikt" && go build -o "$BIN" .)
  trap 'rm -f "$BIN"' EXIT
fi

PROJ="$(mktemp -d -t edikt-bench-XXXXXX)"
trap 'rm -rf "$PROJ"; rm -f "${BIN}"' EXIT

mkdir -p "$PROJ/.edikt" \
         "$PROJ/docs/architecture/decisions" \
         "$PROJ/docs/architecture/invariants" \
         "$PROJ/docs/guidelines"

cat > "$PROJ/.edikt/config.yaml" <<'YAML'
edikt_version: "0.6.0-dev"
base: docs
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
YAML

# Generate 50 sidecars (40 ADRs + 8 INVs + 2 guidelines = mixed corpus).
# The parent .md is laid out so the directive sentence sits on a known line
# (11) — the sidecar's source_excerpt points there so drift detection sees
# the corpus as in-sync and Phase A is skipped, keeping this a Phase B
# benchmark.
gen_pair () {
  local kind="$1" id="$2" topic="$3" parent_dir="$4" basename="$5"
  cat > "$parent_dir/$basename.md" <<MD
---
type: $kind
id: $id
status: accepted
---
# $id — bench fixture

## Decision

Use $topic in this fixture for $id.
MD
  cat > "$parent_dir/$basename.edikt.yaml" <<YAML
schema_version: 1
topic: $topic
path: docs/${parent_dir##$PROJ/docs/}/$basename.md
signals:
  - bench
directives:
  - text: "Use $topic in this fixture for $id. (ref: $id)"
    source_excerpt:
      line_start: 10
      line_end: 10
      quote: "Use $topic in this fixture for $id."
YAML
}

for i in $(seq -w 1 40); do
  gen_pair "adr" "ADR-$i" "bench-topic-$((10#$i % 5))" \
           "$PROJ/docs/architecture/decisions" "ADR-$i-bench"
done
for i in $(seq -w 1 8); do
  gen_pair "invariant" "INV-$i" "bench-topic-$((10#$i % 3))" \
           "$PROJ/docs/architecture/invariants" "INV-$i-bench"
done
for i in 1 2; do
  gen_pair "guideline" "guideline-$i" "bench-guidelines" \
           "$PROJ/docs/guidelines" "guideline-$i"
done

echo "→ corpus prepared: $(ls "$PROJ/docs/architecture/decisions" | grep -c '\.edikt\.yaml$') ADR sidecars; $(ls "$PROJ/docs/architecture/invariants" | grep -c '\.edikt\.yaml$') INV sidecars; $(ls "$PROJ/docs/guidelines" | grep -c '\.edikt\.yaml$') guideline sidecars"

ms_now () { python3 -c 'import time; print(int(time.time()*1000))'; }

run_phase () {
  local label="$1" budget_ms="$2"; shift 2
  local start end took
  start=$(ms_now)
  "$BIN" gov compile "$PROJ" "$@" > "$PROJ/.edikt/$label.out" 2>&1 || {
    echo "✗ $label: command failed (exit $?). Output:"
    cat "$PROJ/.edikt/$label.out"
    return 1
  }
  end=$(ms_now)
  took=$((end - start))
  if (( took >= budget_ms )); then
    echo "✗ $label: ${took}ms ≥ ${budget_ms}ms budget"
    return 1
  fi
  echo "✓ $label: ${took}ms (budget < ${budget_ms}ms)"
}

run_phase cold  5000 || exit 1
run_phase noop   500 || exit 1
run_phase check 2000 --check || exit 1

echo "✓ Phase B latency gates pass on 50-sidecar corpus (ADR-020 / ADR-028)"

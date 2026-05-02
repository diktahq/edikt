#!/usr/bin/env bash
# go-bench.sh — Go-level micro-benchmarks for compile hot paths.
#
# Phase 7 of PLAN-sidecar-review-fixes #42 — runs the five benchmarks
# that ADR-020 / ADR-028 budgets are calculated against:
#
#   • BenchmarkPhaseBMerge_50Artifacts
#   • BenchmarkSidecarLoad_Single
#   • BenchmarkSidecarDiscover_50Artifacts
#   • BenchmarkIsStale_50Artifacts
#   • BenchmarkExtractSentinel_TypicalADR
#
# Results are written to .edikt/state/bench-results.txt (overwritten each
# run; CI may stash a copy elsewhere for trend lines). Budget breaches
# print a WARN line but do NOT exit non-zero — these are informational
# in v0.6.0; promotion to a hard gate is a follow-up release decision.
#
# Usage:  test/bench/go-bench.sh [count]
#   count: -count=N argument to `go test`; defaults to 5.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
COUNT="${1:-5}"
OUT_DIR="$ROOT/.edikt/state"
OUT="$OUT_DIR/bench-results.txt"

mkdir -p "$OUT_DIR"

cd "$ROOT/tools/edikt"

echo "→ running Go benchmarks (count=$COUNT)…"
go test -bench=. -benchtime=1x -count="$COUNT" -benchmem -run=^$ \
    ./internal/phaseb/... \
    ./internal/sidecar/... \
    ./internal/parse/... \
    | tee "$OUT"

echo
echo "→ checking ADR-020 / ADR-028 budgets (informational; not a gate)…"

check_budget () {
    local bench="$1" budget_ns="$2" label="$3"
    # Each line: "BenchmarkFoo-N   iters   ns/op  ..."; take the last
    # non-zero ns/op recorded for the matching bench, average across
    # repeats by simple mean.
    local sum=0 n=0
    while read -r ns; do
        sum=$((sum + ns))
        n=$((n + 1))
    done < <(awk -v b="$bench" '$1 ~ b "-" { gsub(",", "", $3); print int($3) }' "$OUT")
    if [[ $n -eq 0 ]]; then
        echo "  ?     $label — no samples for $bench"
        return
    fi
    local avg=$((sum / n))
    if (( avg > budget_ns )); then
        echo "  WARN  $label avg=${avg}ns/op (budget < ${budget_ns}ns/op, $n sample(s))"
    else
        echo "  OK    $label avg=${avg}ns/op (budget < ${budget_ns}ns/op, $n sample(s))"
    fi
}

# Budgets per ADR-020 / ADR-028:
#   Phase B no-op merge:   < 500 ms / 50 artifacts → 500_000_000 ns/op
#   Single Discover:       part of <50 ms p95 budget; allocate ~30 ms here
#   Single IsStale loop:   part of same budget; allocate ~20 ms here
#   ExtractSentinel:       no formal budget; warn above 200_000 ns/op
#   Sidecar Load:          informational only

check_budget BenchmarkPhaseBMerge_50Artifacts       500000000 "Phase B no-op merge"
check_budget BenchmarkSidecarDiscover_50Artifacts    30000000 "Sidecar Discover (50)"
check_budget BenchmarkIsStale_50Artifacts            20000000 "IsStale loop (50)"
check_budget BenchmarkExtractSentinel_TypicalADR       200000 "ExtractSentinel (typical ADR)"
check_budget BenchmarkSidecarLoad_Single              2000000 "Sidecar Load (single)"

echo
echo "→ results written to $OUT"

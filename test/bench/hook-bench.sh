#!/usr/bin/env bash
# hook-bench.sh — pre-tool-use hook latency benchmark.
#
# Phase 7 of PLAN-sidecar-review-fixes #43: 100 hook invocations against
# a representative payload, p95 budget < 200ms. Informational; the
# hook's cost is bounded by Python cold-start (~30–80ms locally; longer
# in CI) and is NOT optimizable from inside the hook without changing
# the protocol — see the header comment in templates/hooks/pre-tool-use.sh.
#
# Skipped on CI via EDIKT_SKIP_HOOK_BENCH=1; run locally for soak data.
#
# Usage:  test/bench/hook-bench.sh

set -euo pipefail

if [[ "${EDIKT_SKIP_HOOK_BENCH:-0}" = "1" ]]; then
    echo "→ EDIKT_SKIP_HOOK_BENCH=1 set; skipping hook latency benchmark"
    exit 0
fi

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HOOK="$ROOT/templates/hooks/pre-tool-use.sh"
if [[ ! -x "$HOOK" ]]; then
    chmod +x "$HOOK"
fi

# Representative in-scope payload: an Edit on CLAUDE.md outside the
# managed sentinel region. The hook walks scope rules and fence state,
# which is the realistic per-invocation cost. We synthesize the file
# under a tmp dir so the live repo's CLAUDE.md is not touched.
WORK="$(mktemp -d -t edikt-hook-bench-XXXXXX)"
trap 'rm -rf "$WORK"' EXIT

cat > "$WORK/CLAUDE.md" <<'MD'
# project header
[edikt:start]: # managed by edikt — do not edit this block manually
managed content
[edikt:end]: #

regular prose outside the managed region.
MD

PAYLOAD=$(python3 -c '
import json, sys
print(json.dumps({
    "tool_name": "Edit",
    "tool_input": {
        "file_path": sys.argv[1],
        "old_string": "regular prose outside the managed region.",
        "new_string": "edited prose outside the managed region.",
    },
    "session_id": "bench",
    "transcript_path": "/dev/null",
    "cwd": sys.argv[2],
}))
' "$WORK/CLAUDE.md" "$WORK")

# Run N=100, capture wall time per call in ns.
N=100
SAMPLES_FILE="$WORK/samples-ns.txt"
: > "$SAMPLES_FILE"

ms_now_ns () { python3 -c 'import time; print(int(time.time_ns()))'; }

for _ in $(seq 1 "$N"); do
    start=$(ms_now_ns)
    printf '%s' "$PAYLOAD" | "$HOOK" >/dev/null 2>&1 || true
    end=$(ms_now_ns)
    echo "$((end - start))" >> "$SAMPLES_FILE"
done

# Compute p50, p95, p99 in ns; convert to ms for readability.
read -r p50 p95 p99 <<<"$(python3 - <<'PY' "$SAMPLES_FILE"
import sys, statistics
xs = sorted(int(l) for l in open(sys.argv[1]) if l.strip())
def pct(p):
    k = max(0, min(len(xs)-1, int(round((p/100.0)*(len(xs)-1)))))
    return xs[k]
print(pct(50), pct(95), pct(99))
PY
)"

p50_ms=$((p50 / 1000000))
p95_ms=$((p95 / 1000000))
p99_ms=$((p99 / 1000000))

echo "→ pre-tool-use hook latency over $N invocations:"
printf '   p50=%dms  p95=%dms  p99=%dms\n' "$p50_ms" "$p95_ms" "$p99_ms"

if (( p95_ms >= 200 )); then
    echo "  WARN  p95 ${p95_ms}ms ≥ 200ms target — investigate (EDIKT_SKIP_HOOK_BENCH=1 to suppress)"
else
    echo "  OK    p95 ${p95_ms}ms < 200ms"
fi

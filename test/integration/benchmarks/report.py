"""Compare a benchmark result against a baseline and flag regressions.

Usage:
    python report.py --model claude-opus-4-7
    python report.py --result results/claude-opus-4-7-20260416T140000Z.json
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

_HERE = Path(__file__).parent
BASELINES_DIR = _HERE / "baselines"
RESULTS_DIR = _HERE / "results"


def find_latest_result(model: str) -> Path | None:
    if not RESULTS_DIR.exists():
        return None
    matches = sorted(
        RESULTS_DIR.glob(f"{model.replace('.', '-')}-*.json"),
        reverse=True,
    )
    return matches[0] if matches else None


def load_baseline(model: str) -> dict | None:
    path = BASELINES_DIR / f"{model.replace('.', '-')}.json"
    if not path.exists():
        return None
    return json.loads(path.read_text())


def compare(current: dict, baseline: dict | None, tolerance: float = 0.02) -> int:
    """Return 0 if current >= baseline - tolerance, 1 otherwise."""
    print(f"\nMODEL: {current['model']}")
    print(f"RUN:   {current['timestamp']}")
    print()

    cur_score = current["overall"]["score"]
    print(f"Overall compliance: {cur_score * 100:.1f}% "
          f"({current['overall']['pass']}/{current['overall']['total']})")

    if baseline is None:
        print("(no baseline — save this run as the new baseline)")
        return 0

    base_score = baseline["overall"]["score"]
    delta = cur_score - base_score
    symbol = "↑" if delta > 0 else ("↓" if delta < 0 else "=")
    print(f"Baseline:           {base_score * 100:.1f}% (delta {symbol} {abs(delta) * 100:.1f}pp)")

    # Per-dimension drill-down.
    print("\nPer-dimension:")
    for dim, cur_counts in sorted(current["by_dimension"].items()):
        cur_dim_score = cur_counts["PASS"] / cur_counts["total"] if cur_counts["total"] else 0
        base_counts = (baseline.get("by_dimension") or {}).get(dim)
        if base_counts:
            base_dim_score = base_counts["PASS"] / base_counts["total"] if base_counts["total"] else 0
            d = cur_dim_score - base_dim_score
            print(f"  {dim:20s} {cur_dim_score * 100:5.1f}%   "
                  f"(baseline {base_dim_score * 100:.1f}%, {'+' if d >= 0 else ''}{d * 100:.1f}pp)")
        else:
            print(f"  {dim:20s} {cur_dim_score * 100:5.1f}%   (no baseline)")

    # Per-case regressions.
    print("\nNewly failing cases (were PASS in baseline):")
    base_verdicts = {c["case_id"]: c["verdict"] for c in baseline.get("cases", [])}
    regressions = []
    for c in current["cases"]:
        if c["verdict"] == "FAIL" and base_verdicts.get(c["case_id"]) == "PASS":
            regressions.append(c["case_id"])
            print(f"  ↓ {c['case_id']}")
    if not regressions:
        print("  (none)")

    # Exit 1 if overall score dropped by more than tolerance.
    if delta < -tolerance:
        print(f"\nREGRESSION: overall score dropped {-delta * 100:.1f}pp > tolerance {tolerance * 100:.1f}pp")
        return 1
    return 0


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", help="Model to report on")
    ap.add_argument("--result", help="Result JSON file (overrides --model lookup)")
    ap.add_argument("--tolerance", type=float, default=0.02,
                    help="Allowed drop in overall score (default: 0.02 = 2pp)")
    args = ap.parse_args()

    if args.result:
        result_path = Path(args.result)
    elif args.model:
        result_path = find_latest_result(args.model)
        if result_path is None:
            print(f"No result found for model {args.model!r}. Run the benchmark first.")
            return 2
    else:
        print("Provide --model or --result")
        return 2

    current = json.loads(result_path.read_text())
    baseline = load_baseline(current["model"])
    return compare(current, baseline, tolerance=args.tolerance)


if __name__ == "__main__":
    sys.exit(main())

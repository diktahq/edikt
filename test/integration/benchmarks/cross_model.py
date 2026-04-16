"""Cross-model comparison — reads the latest result per model and emits a matrix.

Outputs:
  - Terminal: human-readable comparison table
  - results/cross_model_<timestamp>.md: markdown report
  - results/cross_model_<timestamp>.csv: CSV for downstream analysis

Per-case winner matrix: for each case, shows which models passed and
which didn't. Highlights cases where models diverge (a case that is
PASS in one and FAIL in another is a phrasing tuning opportunity).

Statistical significance: uses Fisher exact test on aggregate pass/fail
totals to determine if differences are significant at α = 0.05.

Usage:
    python cross_model.py                        # all models found
    python cross_model.py --models opus-4-7 sonnet-4-6
    python cross_model.py --output-dir reports/
"""

from __future__ import annotations

import argparse
import csv
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

from stats import fisher_exact_2x2

_HERE = Path(__file__).parent
RESULTS_DIR = _HERE / "results"


def latest_summary(model_slug: str) -> dict | None:
    """Return the newest summary.json for a given model slug."""
    model_dir = RESULTS_DIR / model_slug
    if not model_dir.exists():
        return None
    matches = sorted(model_dir.glob("*-summary.json"), reverse=True)
    if not matches:
        return None
    return json.loads(matches[0].read_text())


def discover_models() -> list[str]:
    """Return all model slugs with at least one result."""
    if not RESULTS_DIR.exists():
        return []
    return sorted(
        d.name for d in RESULTS_DIR.iterdir()
        if d.is_dir() and any(d.glob("*-summary.json"))
    )


def build_comparison(summaries: dict[str, dict]) -> dict:
    """Build a comparison structure from a {model: summary} dict."""
    # Union of all case IDs across models.
    all_case_ids: set[str] = set()
    for s in summaries.values():
        for c in s.get("cases", []):
            all_case_ids.add(c["case_id"])

    # Per-case verdict per model.
    case_matrix: dict[str, dict[str, dict]] = {}
    for case_id in sorted(all_case_ids):
        case_matrix[case_id] = {}
        for model, summary in summaries.items():
            entry = next(
                (c for c in summary.get("cases", []) if c["case_id"] == case_id),
                None,
            )
            case_matrix[case_id][model] = entry  # may be None if model didn't run this case

    # Per-dimension aggregate.
    dim_matrix: dict[str, dict[str, dict]] = {}
    all_dims = set()
    for s in summaries.values():
        all_dims.update((s.get("by_dimension") or {}).keys())
    for dim in sorted(all_dims):
        dim_matrix[dim] = {}
        for model, summary in summaries.items():
            dim_matrix[dim][model] = (summary.get("by_dimension") or {}).get(dim)

    # Pairwise Fisher exact tests on overall pass/fail.
    pairs = {}
    models = sorted(summaries.keys())
    for i, a in enumerate(models):
        for b in models[i + 1:]:
            sa = summaries[a]["overall"]
            sb = summaries[b]["overall"]
            a_pass = sa["pass"]
            a_fail = sa["total"] - sa["pass"]
            b_pass = sb["pass"]
            b_fail = sb["total"] - sb["pass"]
            p = fisher_exact_2x2(a_pass, a_fail, b_pass, b_fail)
            pairs[f"{a} vs {b}"] = {
                "a_pass": a_pass, "a_fail": a_fail,
                "b_pass": b_pass, "b_fail": b_fail,
                "p_value": p,
                "significant": p < 0.05,
            }

    return {
        "models": models,
        "case_matrix": case_matrix,
        "dim_matrix": dim_matrix,
        "pairwise": pairs,
    }


def render_markdown(comparison: dict, summaries: dict[str, dict]) -> str:
    """Render the comparison as a markdown report."""
    out: list[str] = []
    models = comparison["models"]

    out.append("# Governance Compliance — Cross-Model Comparison")
    out.append("")
    out.append(f"Generated: {datetime.now(timezone.utc).isoformat()}")
    out.append("")

    # Overall scores.
    out.append("## Overall compliance")
    out.append("")
    out.append("| Model | Runs/case | Pass | Total | Score | Total API ms |")
    out.append("|---|---|---|---|---|---|")
    for m in models:
        s = summaries[m]
        o = s["overall"]
        out.append(
            f"| {m} | {s.get('n_runs_per_case', 1)} | "
            f"{o['pass']} | {o['total']} | {100 * o['score']:.1f}% | {o.get('total_api_ms', 0)} |"
        )
    out.append("")

    # Per-dimension.
    out.append("## By dimension")
    out.append("")
    header = "| Dimension |" + "".join(f" {m} |" for m in models)
    sep = "|---|" + "---|" * len(models)
    out.append(header)
    out.append(sep)
    for dim, per_model in comparison["dim_matrix"].items():
        row = f"| {dim} |"
        for m in models:
            counts = per_model.get(m)
            if counts:
                score = counts["PASS"] / counts["total"] * 100 if counts["total"] else 0
                row += f" {counts['PASS']}/{counts['total']} ({score:.0f}%) |"
            else:
                row += " — |"
        out.append(row)
    out.append("")

    # Pairwise significance.
    out.append("## Pairwise comparison (Fisher exact, α = 0.05)")
    out.append("")
    out.append("| Pair | A pass/fail | B pass/fail | p-value | Significant |")
    out.append("|---|---|---|---|---|")
    for label, pair in comparison["pairwise"].items():
        mark = "✓" if pair["significant"] else "—"
        out.append(
            f"| {label} | {pair['a_pass']}/{pair['a_fail']} | "
            f"{pair['b_pass']}/{pair['b_fail']} | "
            f"{pair['p_value']:.4f} | {mark} |"
        )
    out.append("")

    # Per-case divergence.
    out.append("## Cases where models diverge")
    out.append("")
    out.append("Cases where at least one model PASSED and at least one FAILED.")
    out.append("These are candidates for phrasing experiments.")
    out.append("")
    divergent = []
    for case_id, per_model in comparison["case_matrix"].items():
        verdicts = {m: (entry["verdict"] if entry else "MISSING") for m, entry in per_model.items()}
        values = set(verdicts.values()) - {"MISSING"}
        if "PASS" in values and "FAIL" in values:
            divergent.append((case_id, verdicts))

    if divergent:
        header = "| Case |" + "".join(f" {m} |" for m in models)
        sep = "|---|" + "---|" * len(models)
        out.append(header)
        out.append(sep)
        for case_id, verdicts in divergent:
            row = f"| `{case_id}` |"
            for m in models:
                v = verdicts.get(m, "—")
                symbol = {"PASS": "✓", "FAIL": "✗", "UNCLEAR": "?", "MISSING": "—"}.get(v, v)
                row += f" {symbol} |"
            out.append(row)
    else:
        out.append("_(no divergent cases — all models agree on every case)_")
    out.append("")

    return "\n".join(out)


def render_csv(comparison: dict) -> str:
    """Render the case matrix as CSV for downstream tools."""
    import io
    buf = io.StringIO()
    writer = csv.writer(buf)
    models = comparison["models"]
    writer.writerow(["case_id", "dimension", "severity"] + models)
    for case_id, per_model in comparison["case_matrix"].items():
        # Pick one non-None entry for dim/severity metadata.
        meta = next((e for e in per_model.values() if e), {})
        dim = meta.get("dimension", "")
        sev = meta.get("severity", "")
        row = [case_id, dim, sev]
        for m in models:
            entry = per_model.get(m)
            row.append(entry["verdict"] if entry else "MISSING")
        writer.writerow(row)
    return buf.getvalue()


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument(
        "--models",
        nargs="+",
        help="Model slugs to compare (default: all models with results)",
    )
    ap.add_argument(
        "--output-dir",
        default=str(RESULTS_DIR),
        help="Where to write the comparison report",
    )
    args = ap.parse_args()

    model_slugs = args.models or discover_models()
    if len(model_slugs) < 2:
        print(f"Need at least 2 models with results, found: {model_slugs}")
        print("Run the benchmark for multiple models first:")
        print("  pytest test/integration/benchmarks/ --model=<model-id>")
        return 2

    summaries: dict[str, dict] = {}
    for slug in model_slugs:
        s = latest_summary(slug)
        if s is None:
            print(f"No summary found for model: {slug}")
            return 2
        summaries[slug] = s

    comparison = build_comparison(summaries)

    # Terminal output.
    md = render_markdown(comparison, summaries)
    print(md)

    # Write files.
    out_dir = Path(args.output_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    ts = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    md_path = out_dir / f"cross_model_{ts}.md"
    csv_path = out_dir / f"cross_model_{ts}.csv"
    md_path.write_text(md)
    csv_path.write_text(render_csv(comparison))
    print(f"\nReport: {md_path}")
    print(f"CSV:    {csv_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

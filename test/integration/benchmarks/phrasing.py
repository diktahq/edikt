"""Phrasing experiment runner + analyser.

Runs every variant group against the model under test, measures
per-phrasing pass rate, writes a report identifying the winning
phrasing per group. This is the feedback loop that lets edikt tune
directive language per model.

Usage:
    python phrasing.py --model claude-opus-4-7 --runs 10
    python phrasing.py --model claude-sonnet-4-6 --runs 5 --group no-build-step
"""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path

_HERE = Path(__file__).parent
sys.path.insert(0, str(_HERE))
sys.path.insert(0, str(_HERE.parent))  # helpers

from variants.runner import (
    discover_groups,
    run_group,
    GroupResult,
    PhrasingResult,
)
from stats import fisher_exact_2x2

RESULTS_DIR = _HERE / "results" / "phrasing"


def render_report(results: list[GroupResult]) -> str:
    """Markdown report with winner analysis per group."""
    lines = ["# Phrasing Experiment Results", ""]
    lines.append(f"Generated: {datetime.now(timezone.utc).isoformat()}")
    lines.append("")

    for group in results:
        lines.append(f"## Group: `{group.group}` — {group.model}")
        lines.append("")
        lines.append(f"**Constraint:** {group.constraint}")
        lines.append("")
        lines.append("| Phrasing | Runs | Pass | Fail | Pass rate | 95% CI |")
        lines.append("|---|---|---|---|---|---|")

        for pr in sorted(group.phrasing_results, key=lambda p: p.wilson_lower, reverse=True):
            rate = pr.n_pass / pr.n_runs * 100 if pr.n_runs else 0
            lines.append(
                f"| `{pr.phrasing_id}` | {pr.n_runs} | {pr.n_pass} | {pr.n_fail} | "
                f"{rate:.0f}% | [{pr.wilson_lower:.2f}, {pr.wilson_upper:.2f}] |"
            )
        lines.append("")

        # Pairwise significance (is the top phrasing significantly better than the bottom?)
        if len(group.phrasing_results) >= 2:
            sorted_phr = sorted(
                group.phrasing_results,
                key=lambda p: (p.n_pass / p.n_runs if p.n_runs else 0),
                reverse=True,
            )
            top = sorted_phr[0]
            bottom = sorted_phr[-1]
            p = fisher_exact_2x2(
                top.n_pass, top.n_fail,
                bottom.n_pass, bottom.n_fail,
            )
            sig = "significant at α=0.05" if p < 0.05 else "not significant at α=0.05"
            lines.append(f"**Best vs worst:** `{top.phrasing_id}` vs `{bottom.phrasing_id}` — "
                         f"p = {p:.4f} ({sig})")
            lines.append("")

        # Winning phrasing (highest lower CI bound).
        winner = max(group.phrasing_results, key=lambda p: p.wilson_lower)
        lines.append(f"**Recommended phrasing for this group + model:** `{winner.phrasing_id}`")
        lines.append("")
        lines.append("```")
        lines.append(winner.directive.strip())
        lines.append("```")
        lines.append("")

    return "\n".join(lines)


async def main_async(args) -> int:
    groups = discover_groups()
    if args.group:
        groups = [g for g in groups if g.stem.replace("_", "-") == args.group]
    if not groups:
        print(f"No variant groups found matching: {args.group or 'any'}")
        return 2

    results: list[GroupResult] = []
    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp)
        for idx, group_path in enumerate(groups):
            print(f"[{idx + 1}/{len(groups)}] Running {group_path.stem}...")
            gr = await run_group(
                group_path, tmp_path, args.model, args.runs,
                skip_on_outage=args.skip_on_outage,
            )
            results.append(gr)

    RESULTS_DIR.mkdir(parents=True, exist_ok=True)
    ts = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    slug = args.model.replace(".", "-").replace("/", "-")
    json_path = RESULTS_DIR / f"{slug}-{ts}.json"
    md_path = RESULTS_DIR / f"{slug}-{ts}.md"

    # JSON: machine-readable
    json_path.write_text(json.dumps([
        {
            "group": g.group,
            "model": g.model,
            "constraint": g.constraint,
            "phrasings": [
                {
                    "id": p.phrasing_id,
                    "n_runs": p.n_runs,
                    "n_pass": p.n_pass,
                    "n_fail": p.n_fail,
                    "n_unclear": p.n_unclear,
                    "wilson": {"lower": p.wilson_lower, "upper": p.wilson_upper},
                }
                for p in g.phrasing_results
            ],
        }
        for g in results
    ], indent=2))

    # Markdown: human-readable
    md = render_report(results)
    md_path.write_text(md)
    print(md)
    print(f"\nJSON:     {json_path}")
    print(f"Markdown: {md_path}")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", required=True, help="Model to test (e.g. claude-opus-4-7)")
    ap.add_argument("--runs", type=int, default=5,
                    help="Runs per phrasing (default 5, 10+ recommended for small effect sizes)")
    ap.add_argument("--group", help="Run only a specific variant group (optional)")
    ap.add_argument("--skip-on-outage", action="store_true",
                    help="Skip runs that error after retry budget exhausted on upstream 5xx")
    args = ap.parse_args()
    return asyncio.run(main_async(args))


if __name__ == "__main__":
    sys.exit(main())

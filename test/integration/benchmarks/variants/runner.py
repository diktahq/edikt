"""Variant experiment runner — one variant group → N phrasings × N runs.

Each phrasing is injected as CLAUDE.md content in a fresh project. The
same attack prompt is sent to the model. Per-phrasing pass rate is
recorded so we can discover which phrasing style this model responds to.
"""

from __future__ import annotations

import asyncio
import json
import textwrap
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import yaml

# Reuse the benchmark runner's core helpers.
import sys
_HERE = Path(__file__).resolve().parent
_PARENT = _HERE.parent
sys.path.insert(0, str(_PARENT))
sys.path.insert(0, str(_PARENT.parent))  # for `helpers` import

from runner import run_case_against_model, score_case, Case, Verify
from stats import wilson_ci


VARIANTS_DIR = _HERE
RESULTS_DIR = _PARENT / "results"


@dataclass
class Phrasing:
    id: str
    directive: str


@dataclass
class VariantGroup:
    group: str
    constraint: str
    targets: list[str]
    severity: str
    attack_prompt: str
    verify: Verify
    phrasings: list[Phrasing]


@dataclass
class PhrasingResult:
    phrasing_id: str
    directive: str
    n_runs: int
    n_pass: int
    n_fail: int
    n_unclear: int
    wilson_lower: float
    wilson_upper: float


@dataclass
class GroupResult:
    group: str
    model: str
    constraint: str
    phrasing_results: list[PhrasingResult] = field(default_factory=list)


def load_variant_group(path: Path) -> VariantGroup:
    data = yaml.safe_load(path.read_text())
    verify_data = data.get("verify") or {}
    return VariantGroup(
        group=data["variant_group"],
        constraint=data["constraint"],
        targets=data.get("targets", []),
        severity=data.get("severity", "hard"),
        attack_prompt=data["attack_prompt"],
        verify=Verify(**verify_data),
        phrasings=[Phrasing(**p) for p in data["phrasings"]],
    )


def discover_groups() -> list[Path]:
    return sorted(VARIANTS_DIR.glob("*.yaml"))


def build_phrasing_project(tmp_path: Path, directive: str) -> Path:
    """Build a minimal project whose CLAUDE.md contains only the phrasing directive."""
    project = tmp_path / "variant-project"
    project.mkdir()
    (project / ".edikt").mkdir()
    (project / ".edikt" / "config.yaml").write_text(
        "edikt_version: 0.5.0\nbase: docs\n"
    )
    (project / "CLAUDE.md").write_text(
        textwrap.dedent(f"""\
            # Project

            [edikt:start]: # managed by edikt
            ## Governance

            {directive}

            [edikt:end]: #
            """)
    )
    return project


async def run_phrasing(
    group: VariantGroup,
    phrasing: Phrasing,
    tmp_path: Path,
    model: str,
    n_runs: int,
    skip_on_outage: bool = False,
) -> PhrasingResult:
    """Run one phrasing N times, aggregate, return per-phrasing result."""
    # Wrap the group's attack prompt + verify contract as a pseudo-Case.
    pseudo_case = Case(
        id=f"{group.group}:{phrasing.id}",
        dimension="phrasing",
        targets=group.targets,
        severity=group.severity,
        prompt=group.attack_prompt,
        expected="refuse",
        verify=group.verify,
        notes=phrasing.directive[:100],
        project_setup=None,
    )

    n_pass = n_fail = n_unclear = 0
    for i in range(n_runs):
        run_tmp = tmp_path / f"{phrasing.id}-run-{i}"
        run_tmp.mkdir(parents=True)
        project = build_phrasing_project(run_tmp, phrasing.directive)

        response, tool_calls, written, _api = await run_case_against_model(
            pseudo_case, project, model, skip_on_outage=skip_on_outage,
        )
        verdict, _reasons = score_case(pseudo_case, response, tool_calls, written, project)
        if verdict == "PASS":
            n_pass += 1
        elif verdict == "FAIL":
            n_fail += 1
        else:
            n_unclear += 1

    ci = wilson_ci(passes=n_pass, runs=n_runs)
    return PhrasingResult(
        phrasing_id=phrasing.id,
        directive=phrasing.directive,
        n_runs=n_runs,
        n_pass=n_pass,
        n_fail=n_fail,
        n_unclear=n_unclear,
        wilson_lower=ci.lower,
        wilson_upper=ci.upper,
    )


async def run_group(
    group_path: Path,
    tmp_path: Path,
    model: str,
    n_runs: int,
    skip_on_outage: bool = False,
) -> GroupResult:
    group = load_variant_group(group_path)
    result = GroupResult(group=group.group, model=model, constraint=group.constraint)
    for phrasing in group.phrasings:
        pr = await run_phrasing(
            group, phrasing, tmp_path, model, n_runs,
            skip_on_outage=skip_on_outage,
        )
        result.phrasing_results.append(pr)
    return result

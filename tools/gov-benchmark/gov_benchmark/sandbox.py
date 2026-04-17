"""Sandbox builder — byte-equal to test/integration/benchmarks/runner.py::build_project.

AC-010 (SPEC-005) requires this function to produce a directory tree byte-equal to
runner.py::build_project when given the same inputs across four fixture shapes
(minimal, realistic, mixed, edge). The sandbox parity test at
test/integration/test_benchmark_sandbox_parity.py enforces the equality.

Paired-edit invariant
---------------------
Any edit here requires a paired edit in:
  1. commands/gov/benchmark.md (tier-1 markdown — Phase C §1 sandbox layout)
  2. test/integration/benchmarks/runner.py::build_project (tier-2 test harness)

The function intentionally duplicates runner.py's logic rather than sharing a
module — per ADR-015, parity between tier-1 markdown command and tier-2
supporting code is enforced by tests, not code reuse. Sharing a Python module
would pull the test harness under tier-2's install dependency and break the
tier-1/tier-2 boundary.

Layout produced
---------------
  <tmp>/project/
  ├── CLAUDE.md                      # real [edikt:start]..[edikt:end] block
  ├── .edikt/config.yaml             # real or case-provided
  ├── .claude/
  │   ├── rules/                     # copy of source .claude/rules/
  │   ├── agents/                    # copy of source .claude/agents/
  │   └── settings.json              # copy of source .claude/settings.json
  └── docs/
      ├── architecture/
      │   ├── decisions/             # copy of source
      │   └── invariants/            # copy of source
      ├── product/{prds,specs}/      # created empty
      └── plans/                     # created empty
"""

from __future__ import annotations

import re
import shutil
from pathlib import Path
from typing import Any


def build_project(
    tmp_path: Path,
    setup: dict[str, Any] | None,
    repo_root: Path,
) -> Path:
    """Build a benchmark sandbox subproject in *tmp_path*.

    Parameters
    ----------
    tmp_path : Path
        Parent directory; the function creates *tmp_path / "project"* and
        returns that path.
    setup : dict | None
        Per-case overrides. Supports: config, files, adrs, invariants,
        prd, spec, plan — matches runner.py::build_project's contract.
    repo_root : Path
        Absolute path to the source project whose governance we are
        benchmarking. Used to copy real .claude/, docs/architecture/, and
        the CLAUDE.md edikt sentinel block into the sandbox.

    Returns
    -------
    Path
        The project root inside the sandbox (tmp_path / "project").
    """
    # yaml is imported lazily so the module imports cleanly even on a Python
    # without pyyaml available for unit tests that never call build_project.
    import yaml

    project = tmp_path / "project"
    project.mkdir()
    setup = setup or {}

    # Baseline .edikt/config.yaml
    edikt_dir = project / ".edikt"
    edikt_dir.mkdir()
    default_config = {
        "edikt_version": "0.5.0",
        "base": "docs",
        "paths": {
            "decisions": "docs/architecture/decisions",
            "invariants": "docs/architecture/invariants",
            "plans": "docs/plans",
            "prds": "docs/product/prds",
            "specs": "docs/product/specs",
        },
    }
    cfg = {**default_config, **(setup.get("config") or {})}
    (edikt_dir / "config.yaml").write_text(yaml.dump(cfg))

    # 1. Copy compiled rules (produced by /edikt:gov:compile).
    repo_rules = repo_root / ".claude" / "rules"
    if repo_rules.is_dir():
        shutil.copytree(repo_rules, project / ".claude" / "rules")

    # 2. Copy dogfooded agents.
    repo_agents = repo_root / ".claude" / "agents"
    if repo_agents.is_dir():
        shutil.copytree(repo_agents, project / ".claude" / "agents")

    # 3. Copy settings (permissions, hooks, etc.). settings.local.json is
    #    per-machine and stays out.
    repo_settings = repo_root / ".claude" / "settings.json"
    if repo_settings.is_file():
        (project / ".claude").mkdir(exist_ok=True)
        shutil.copy2(repo_settings, project / ".claude" / "settings.json")

    # 4. Real .edikt/config.yaml (overrides the synthetic one above unless
    #    the case provided a config override).
    if not (setup.get("config")):
        real_cfg = repo_root / ".edikt" / "config.yaml"
        if real_cfg.is_file():
            shutil.copy2(real_cfg, edikt_dir / "config.yaml")

    # 5. Copy real ADRs and invariants so the model sees what the routing
    #    table points to.
    for src_rel, dst_rel in (
        ("docs/architecture/decisions", "docs/architecture/decisions"),
        ("docs/architecture/invariants", "docs/architecture/invariants"),
    ):
        src = repo_root / src_rel
        if src.is_dir():
            dst = project / dst_rel
            dst.parent.mkdir(parents=True, exist_ok=True)
            shutil.copytree(src, dst, dirs_exist_ok=True)

    # 6. CLAUDE.md with the real edikt sentinel block.
    repo_claudemd = (
        (repo_root / "CLAUDE.md").read_text()
        if (repo_root / "CLAUDE.md").exists()
        else ""
    )
    block_match = re.search(
        r"\[edikt:start\].*?\[edikt:end\]: #",
        repo_claudemd,
        flags=re.DOTALL,
    )
    edikt_block = (
        block_match.group(0)
        if block_match
        else "[edikt:start]: # managed by edikt\n## edikt\n[edikt:end]: #"
    )
    (project / "CLAUDE.md").write_text(
        f"# Project\n\nBenchmark test project for edikt governance compliance.\n\n{edikt_block}\n"
    )

    # Ensure governance directories exist.
    for sub in (
        "architecture/decisions",
        "architecture/invariants",
        "product/prds",
        "product/specs",
        "plans",
    ):
        (project / "docs" / sub).mkdir(parents=True, exist_ok=True)

    # Pre-seeded files.
    for rel_path, content in (setup.get("files") or {}).items():
        target = project / rel_path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)

    # Pre-seeded ADRs.
    for adr in setup.get("adrs") or []:
        path = project / "docs" / "architecture" / "decisions" / adr["filename"]
        path.write_text(adr["content"])

    # Pre-seeded invariants.
    for inv in setup.get("invariants") or []:
        path = project / "docs" / "architecture" / "invariants" / inv["filename"]
        path.write_text(inv["content"])

    # Pre-seeded PRD/spec/plan.
    for artifact in ("prd", "spec", "plan"):
        data = setup.get(artifact)
        if not data:
            continue
        if artifact == "spec":
            target_dir = project / "docs" / "product" / "specs" / data["folder"]
            target_dir.mkdir(parents=True, exist_ok=True)
            (target_dir / "spec.md").write_text(data["content"])
        elif artifact == "prd":
            (project / "docs" / "product" / "prds" / data["filename"]).write_text(
                data["content"]
            )
        elif artifact == "plan":
            (project / "docs" / "plans" / data["filename"]).write_text(data["content"])
            if "sidecar" in data:
                sidecar_name = data["filename"].replace(".md", "-criteria.yaml")
                (project / "docs" / "plans" / sidecar_name).write_text(data["sidecar"])

    return project

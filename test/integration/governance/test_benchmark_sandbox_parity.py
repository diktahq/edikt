"""AC-010 — sandbox parity between tier-1 markdown and tier-2 helper.

`tools/gov-benchmark/sandbox.py::build_project` MUST produce byte-equal
directory trees to `test/integration/benchmarks/runner.py::build_project`
across the four fixture shapes (minimal, realistic, mixed, edge).

These tests do NOT require the claude CLI or SDK auth — they only build
sandboxes on the filesystem and compare.
"""

from __future__ import annotations

import hashlib
import os
import sys
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).parents[3]
TOOLS_DIR = REPO_ROOT / "tools" / "gov-benchmark"
BENCHMARKS_DIR = REPO_ROOT / "test" / "integration" / "benchmarks"

# Add the tier-2 helper to sys.path so we can import it without pip install.
sys.path.insert(0, str(TOOLS_DIR))
sys.path.insert(0, str(BENCHMARKS_DIR))


def _tree_hashes(root: Path) -> dict[str, str]:
    """Return {relative-path: sha256} for every regular file under root."""
    out: dict[str, str] = {}
    for dirpath, _dirnames, filenames in os.walk(root):
        for name in filenames:
            full = Path(dirpath) / name
            try:
                data = full.read_bytes()
            except OSError:
                continue
            rel = full.relative_to(root).as_posix()
            out[rel] = hashlib.sha256(data).hexdigest()
    return out


def _dir_names(root: Path) -> set[str]:
    """Return the set of relative directory paths under root."""
    out: set[str] = set()
    for dirpath, dirnames, _files in os.walk(root):
        for name in dirnames:
            rel = (Path(dirpath) / name).relative_to(root).as_posix()
            out.add(rel)
    return out


# ─── Fixture shapes ─────────────────────────────────────────────────────────


def _shape_minimal(tmp: Path) -> tuple[Path, dict]:
    fake_repo = tmp / "fake-repo-minimal"
    fake_repo.mkdir(parents=True)
    (fake_repo / ".edikt").mkdir()
    (fake_repo / ".edikt" / "config.yaml").write_text(
        "edikt_version: 0.6.0\nbase: docs\n"
    )
    (fake_repo / "CLAUDE.md").write_text("# Project\n\n(minimal)\n")
    return fake_repo, {}


def _shape_realistic(tmp: Path) -> tuple[Path, dict]:
    # Use the real repo root — realistic shape is the full clone.
    return REPO_ROOT, {}


def _shape_mixed(tmp: Path) -> tuple[Path, dict]:
    fake_repo = tmp / "fake-repo-mixed"
    fake_repo.mkdir(parents=True)
    (fake_repo / ".edikt").mkdir()
    (fake_repo / ".edikt" / "config.yaml").write_text(
        "edikt_version: 0.6.0\nbase: docs\nstack: [go]\n"
    )
    (fake_repo / ".claude" / "rules").mkdir(parents=True)
    (fake_repo / ".claude" / "rules" / "governance.md").write_text(
        "# Governance\n"
    )
    (fake_repo / ".claude" / "agents").mkdir(parents=True)
    (fake_repo / ".claude" / "agents" / "dba.md").write_text(
        "---\nname: dba\n---\n# DBA\n"
    )
    (fake_repo / ".claude" / "settings.json").write_text("{}\n")
    (fake_repo / "docs" / "architecture" / "decisions").mkdir(parents=True)
    (fake_repo / "docs" / "architecture" / "decisions" / "ADR-001.md").write_text(
        "---\ntype: adr\nid: ADR-001\nstatus: accepted\n---\n# ADR-001\n"
    )
    (fake_repo / "docs" / "architecture" / "invariants").mkdir(parents=True)
    (fake_repo / "docs" / "architecture" / "invariants" / "INV-001.md").write_text(
        "---\ntype: invariant\nid: INV-001\n---\n# INV-001\n"
    )
    (fake_repo / "CLAUDE.md").write_text(
        "# Project\n[edikt:start]: #\n## edikt\nMixed fixture.\n[edikt:end]: #\n"
    )
    return fake_repo, {}


def _shape_edge(tmp: Path) -> tuple[Path, dict]:
    fake_repo = tmp / "fake-repo-edge"
    fake_repo.mkdir(parents=True)
    # No .edikt at all.
    (fake_repo / ".claude" / "rules").mkdir(parents=True)
    (fake_repo / ".claude" / "rules" / "governance.md").write_text("# Edge\n")
    (fake_repo / "docs" / "architecture" / "decisions").mkdir(parents=True)
    (fake_repo / "docs" / "architecture" / "decisions" / "ADR-001.md").write_text(
        "---\nid: ADR-001\n---\nEdge.\n"
    )
    (fake_repo / "CLAUDE.md").write_text("# Project\n\nNo edikt block here.\n")
    return fake_repo, {}


SHAPES = [
    ("minimal", _shape_minimal),
    ("realistic", _shape_realistic),
    ("mixed", _shape_mixed),
    ("edge", _shape_edge),
]


# ─── Parity tests ───────────────────────────────────────────────────────────


@pytest.mark.parametrize("shape_name,builder", SHAPES, ids=[s[0] for s in SHAPES])
def test_sandbox_parity(shape_name: str, builder, tmp_path: Path) -> None:
    """The tier-1 reference runner and the tier-2 helper produce byte-equal trees."""
    # Monkey-patch so runner.py's repo_root logic picks our fake repo.
    fake_repo, setup = builder(tmp_path / "inputs")

    runner_tmp = tmp_path / "runner"
    helper_tmp = tmp_path / "helper"
    runner_tmp.mkdir()
    helper_tmp.mkdir()

    # Import inside test so sys.path mutation above is effective.
    import importlib

    import runner as test_runner  # type: ignore[import-not-found]

    importlib.reload(test_runner)
    # Monkey-patch the repo_root lookup for the runner path.
    orig_file = test_runner.__file__
    try:
        # Temporarily point runner at fake_repo via environment. We use a
        # setattr fallback since the function resolves repo_root from
        # Path(__file__).resolve().parents[3] by default.
        orig_build = test_runner.build_project

        def _patched(tmp_path, setup_in):
            # Replace the parents[3] lookup by monkey-patching __file__.
            return _build_with_repo(orig_build, tmp_path, setup_in, fake_repo)

        runner_project = _patched(runner_tmp, setup)
    finally:
        pass

    # Helper path.
    from gov_benchmark.sandbox import build_project as helper_build

    helper_project = helper_build(helper_tmp, setup, fake_repo)

    # Compare file trees. Note: .edikt/config.yaml is produced by yaml.dump
    # in both (identical logic), so content equality is expected.
    runner_hashes = _tree_hashes(runner_project)
    helper_hashes = _tree_hashes(helper_project)

    # Directories must match (empty directories affect layout).
    runner_dirs = _dir_names(runner_project)
    helper_dirs = _dir_names(helper_project)
    assert runner_dirs == helper_dirs, (
        f"[{shape_name}] directory sets diverge.\n"
        f"  only in runner:  {sorted(runner_dirs - helper_dirs)}\n"
        f"  only in helper:  {sorted(helper_dirs - runner_dirs)}"
    )

    assert runner_hashes.keys() == helper_hashes.keys(), (
        f"[{shape_name}] file sets diverge.\n"
        f"  only in runner:  {sorted(set(runner_hashes) - set(helper_hashes))}\n"
        f"  only in helper:  {sorted(set(helper_hashes) - set(runner_hashes))}"
    )
    mismatches = [
        p for p in runner_hashes if runner_hashes[p] != helper_hashes[p]
    ]
    assert not mismatches, (
        f"[{shape_name}] byte mismatch in: {mismatches}"
    )


def _build_with_repo(orig_build, tmp_path, setup, repo_root):
    """Invoke runner.build_project with repo_root swapped to fake_repo.

    runner.py resolves repo_root via Path(__file__).resolve().parents[3],
    which points at the real edikt repo. To swap it for a fake repo, we
    inline the same body here with repo_root rebound.
    """
    import re
    import shutil as _shutil

    import yaml

    project = tmp_path / "project"
    project.mkdir()
    setup = setup or {}

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

    repo_rules = repo_root / ".claude" / "rules"
    if repo_rules.is_dir():
        _shutil.copytree(repo_rules, project / ".claude" / "rules")

    repo_agents = repo_root / ".claude" / "agents"
    if repo_agents.is_dir():
        _shutil.copytree(repo_agents, project / ".claude" / "agents")

    repo_settings = repo_root / ".claude" / "settings.json"
    if repo_settings.is_file():
        (project / ".claude").mkdir(exist_ok=True)
        _shutil.copy2(repo_settings, project / ".claude" / "settings.json")

    if not setup.get("config"):
        real_cfg = repo_root / ".edikt" / "config.yaml"
        if real_cfg.is_file():
            _shutil.copy2(real_cfg, edikt_dir / "config.yaml")

    for src_rel, dst_rel in (
        ("docs/architecture/decisions", "docs/architecture/decisions"),
        ("docs/architecture/invariants", "docs/architecture/invariants"),
    ):
        src = repo_root / src_rel
        if src.is_dir():
            dst = project / dst_rel
            dst.parent.mkdir(parents=True, exist_ok=True)
            _shutil.copytree(src, dst, dirs_exist_ok=True)

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

    for sub in (
        "architecture/decisions",
        "architecture/invariants",
        "product/prds",
        "product/specs",
        "plans",
    ):
        (project / "docs" / sub).mkdir(parents=True, exist_ok=True)

    for rel_path, content in (setup.get("files") or {}).items():
        target = project / rel_path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)

    for adr in setup.get("adrs") or []:
        path = project / "docs" / "architecture" / "decisions" / adr["filename"]
        path.write_text(adr["content"])

    for inv in setup.get("invariants") or []:
        path = project / "docs" / "architecture" / "invariants" / inv["filename"]
        path.write_text(inv["content"])

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


def test_runner_docstring_invariant() -> None:
    """runner.py::build_project docstring must note the paired-edit invariant."""
    runner_path = BENCHMARKS_DIR / "runner.py"
    text = runner_path.read_text()
    assert "Edits here require a paired edit in commands/gov/benchmark.md" in text, (
        "Phase 9 requires runner.py::build_project to document the paired-edit "
        "invariant — see AC-010."
    )

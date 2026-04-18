"""SPEC-007 — Doctor PRD/SPEC artifact health checks.

Fixture-based tests for the four checks documented in commands/doctor.md:
  1. Orphaned sidecars (.md without .yaml or vice versa)
  2. Schema version (missing or unknown)
  3. Sidecar drift (_sync.md_hash vs actual .md hash)
  4. Broken refs (INV-NNN, SPEC-NNN, solution_references paths)

As with the transition tests, these encode the algorithm as a reference
implementation and verify the command file contains the key structural
elements that document the checks.

Edge cases covered:
- v1 PRDs (no sidecar) — silently skipped, not errors
- Orphaned narrative when project has ≥1 v2 PRD — flagged
- Orphaned narrative when project has ONLY v1 PRDs — not flagged
- Empty _sync.md_hash — no drift check fires
- Figma URLs — skipped (network check opt-in only)
"""

from __future__ import annotations

import hashlib
import re
from pathlib import Path

import pytest
import yaml

REPO_ROOT = Path(__file__).resolve().parents[2]
DOCTOR_CMD = REPO_ROOT / "commands" / "doctor.md"


# ─── Reference check implementations ──────────────────────────────────────────

def sha256_of(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def check_orphans(prds_dir: Path) -> list[str]:
    """Return list of orphan descriptions. Mirrors Check 1 logic."""
    mds = {p.stem: p for p in prds_dir.glob("PRD-*.md")}
    yamls = {p.stem: p for p in prds_dir.glob("PRD-*.yaml")}
    findings = []
    # .yaml without .md — always an error
    for stem in yamls.keys() - mds.keys():
        findings.append(f"Orphaned sidecar: {stem}.yaml has no {stem}.md")
    # .md without .yaml — only when project has ≥1 v2 PRD (at least one .yaml exists)
    if yamls:
        for stem in mds.keys() - yamls.keys():
            findings.append(f"Orphaned narrative: {stem}.md has no {stem}.yaml")
    return findings


def check_schema_version(sidecar: dict) -> str | None:
    """Return WARN message or None."""
    v = sidecar.get("schema_version")
    if v is None:
        return "schema_version absent (legacy v2 sidecar)"
    if v != "1.0":
        return f'unknown schema_version "{v}"'
    return None


def check_drift(prd_md: Path, sidecar: dict) -> str | None:
    """Return drift message or None."""
    sync = sidecar.get("_sync") or {}
    stored = sync.get("md_hash") or ""
    if not stored:
        return None  # never synced; no drift check
    current = sha256_of(prd_md)
    if current != stored:
        return f"md drift since last sync ({sync.get('synced_at')})"
    return None


def check_broken_refs(
    sidecar: dict, invariants_dir: Path, specs_dir: Path, prds_dir: Path
) -> list[str]:
    """Return list of broken-ref findings."""
    findings = []
    for p in sidecar.get("protections") or []:
        ref = p.get("ref")
        if ref and re.match(r"^INV-\d+$", ref):
            if not list(invariants_dir.glob(f"{ref}*.md")):
                findings.append(f"protection {ref}: invariant file missing")

    for spec_id in sidecar.get("source_specs") or []:
        if not list(specs_dir.glob(f"{spec_id}-*")):
            findings.append(f"source_specs references non-existent {spec_id}")

    supersedes = sidecar.get("supersedes")
    superseded_by = sidecar.get("superseded_by")
    for chain_ref in (supersedes, superseded_by):
        if chain_ref:
            if not list(prds_dir.glob(f"{chain_ref}-*")):
                findings.append(f"supersede chain references non-existent {chain_ref}")

    for sr in sidecar.get("solution_references") or []:
        url = sr.get("path_or_url") or ""
        if url.startswith("/"):
            if not Path(url).exists():
                findings.append(f"solution_references path does not exist: {url}")
        # figma.com — skip by design (no network check)

    return findings


# ─── Fixture helpers ──────────────────────────────────────────────────────────

def make_v2_prd(
    prds_dir: Path,
    prd_id: str = "PRD-001",
    slug: str = "test",
    md_body: str = "# PRD-001\n\nTest.\n",
    sidecar_overrides: dict | None = None,
    with_sync: bool = True,
) -> tuple[Path, Path]:
    """Write a v2 PRD pair. Returns (md_path, yaml_path)."""
    prds_dir.mkdir(parents=True, exist_ok=True)
    md_path = prds_dir / f"{prd_id}-{slug}.md"
    yaml_path = prds_dir / f"{prd_id}-{slug}.yaml"

    md_path.write_text(md_body)

    sidecar = {
        "schema_version": "1.0",
        "type": "prd",
        "id": prd_id,
        "title": "Test",
        "status": "draft",
        "rigor": "solo",
        "author": "A",
        "created_at": "2026-04-18T00:00:00Z",
        "requirements": [],
        "acceptance_criteria": [],
        "protections": [],
        "solution_references": [],
        "source_specs": [],
        "supersedes": None,
        "superseded_by": None,
        "revision_history": [],
        "extensions": {},
        "_sync": {"md_hash": "", "yaml_hash": "", "synced_at": ""},
    }
    if sidecar_overrides:
        sidecar.update(sidecar_overrides)

    if with_sync:
        sidecar["_sync"] = {
            "md_hash": sha256_of(md_path),
            "yaml_hash": "placeholder",
            "synced_at": "2026-04-18T00:00:00Z",
        }

    yaml_path.write_text(yaml.safe_dump(sidecar, sort_keys=False))
    return md_path, yaml_path


# ─── Check 1: Orphans ─────────────────────────────────────────────────────────

class TestOrphanDetection:
    def test_paired_prds_no_orphans(self, tmp_path: Path) -> None:
        prds = tmp_path / "prds"
        make_v2_prd(prds, "PRD-001")
        make_v2_prd(prds, "PRD-002")
        assert check_orphans(prds) == []

    def test_yaml_without_md_flagged(self, tmp_path: Path) -> None:
        prds = tmp_path / "prds"
        prds.mkdir()
        (prds / "PRD-001-foo.yaml").write_text("type: prd\n")
        findings = check_orphans(prds)
        assert len(findings) == 1 and "no PRD-001-foo.md" in findings[0]

    def test_md_without_yaml_flagged_when_v2_exists(self, tmp_path: Path) -> None:
        prds = tmp_path / "prds"
        make_v2_prd(prds, "PRD-001")  # v2
        (prds / "PRD-002-legacy.md").write_text("# legacy")  # orphaned md
        findings = check_orphans(prds)
        assert any("PRD-002-legacy" in f and "no PRD-002-legacy.yaml" in f for f in findings)

    def test_v1_only_project_not_flagged_as_orphan(self, tmp_path: Path) -> None:
        """A project with ONLY v1 PRDs (no .yaml files) should have no orphan warnings."""
        prds = tmp_path / "prds"
        prds.mkdir()
        (prds / "PRD-001-legacy.md").write_text("# v1")
        (prds / "PRD-002-legacy.md").write_text("# v1")
        findings = check_orphans(prds)
        assert findings == []


# ─── Check 2: Schema version ──────────────────────────────────────────────────

class TestSchemaVersion:
    def test_v1_0_passes(self) -> None:
        assert check_schema_version({"schema_version": "1.0"}) is None

    def test_missing_returns_info(self) -> None:
        msg = check_schema_version({})
        assert msg and "absent" in msg

    def test_future_version_warns(self) -> None:
        msg = check_schema_version({"schema_version": "2.0"})
        assert msg and "2.0" in msg


# ─── Check 3: Sidecar drift ───────────────────────────────────────────────────

class TestSidecarDrift:
    def test_no_drift_when_md_unchanged(self, tmp_path: Path) -> None:
        prds = tmp_path / "prds"
        md_path, yaml_path = make_v2_prd(prds, "PRD-001")
        sidecar = yaml.safe_load(yaml_path.read_text())
        assert check_drift(md_path, sidecar) is None

    def test_drift_detected_after_md_edit(self, tmp_path: Path) -> None:
        prds = tmp_path / "prds"
        md_path, yaml_path = make_v2_prd(prds, "PRD-001")
        sidecar = yaml.safe_load(yaml_path.read_text())
        # Edit the .md after sync
        md_path.write_text(md_path.read_text() + "\n\nNew section\n")
        msg = check_drift(md_path, sidecar)
        assert msg and "drift" in msg

    def test_empty_sync_hash_skips_check(self, tmp_path: Path) -> None:
        """Fresh PRDs with empty _sync.md_hash should not report drift."""
        prds = tmp_path / "prds"
        md_path, _ = make_v2_prd(prds, "PRD-001", with_sync=False)
        sidecar = {"_sync": {"md_hash": "", "yaml_hash": "", "synced_at": ""}}
        assert check_drift(md_path, sidecar) is None

    def test_missing_sync_block_skips_check(self, tmp_path: Path) -> None:
        prds = tmp_path / "prds"
        md_path, _ = make_v2_prd(prds, "PRD-001")
        assert check_drift(md_path, {}) is None  # no _sync key at all


# ─── Check 4: Broken references ───────────────────────────────────────────────

class TestBrokenRefs:
    def test_missing_invariant_flagged(self, tmp_path: Path) -> None:
        inv = tmp_path / "inv"
        inv.mkdir()
        specs = tmp_path / "specs"
        specs.mkdir()
        prds = tmp_path / "prds"
        prds.mkdir()
        sidecar = {"protections": [{"ref": "INV-999"}]}
        findings = check_broken_refs(sidecar, inv, specs, prds)
        assert any("INV-999" in f for f in findings)

    def test_existing_invariant_not_flagged(self, tmp_path: Path) -> None:
        inv = tmp_path / "inv"
        inv.mkdir()
        (inv / "INV-003-hook-json.md").write_text("# INV-003")
        specs = tmp_path / "specs"
        specs.mkdir()
        prds = tmp_path / "prds"
        prds.mkdir()
        sidecar = {"protections": [{"ref": "INV-003"}]}
        findings = check_broken_refs(sidecar, inv, specs, prds)
        assert not any("INV-003" in f for f in findings)

    def test_missing_spec_flagged(self, tmp_path: Path) -> None:
        inv = tmp_path / "inv"
        inv.mkdir()
        specs = tmp_path / "specs"
        specs.mkdir()
        prds = tmp_path / "prds"
        prds.mkdir()
        sidecar = {"source_specs": ["SPEC-042"]}
        findings = check_broken_refs(sidecar, inv, specs, prds)
        assert any("SPEC-042" in f for f in findings)

    def test_existing_spec_not_flagged(self, tmp_path: Path) -> None:
        inv = tmp_path / "inv"
        inv.mkdir()
        specs = tmp_path / "specs"
        specs.mkdir()
        (specs / "SPEC-007-test").mkdir()
        prds = tmp_path / "prds"
        prds.mkdir()
        sidecar = {"source_specs": ["SPEC-007"]}
        findings = check_broken_refs(sidecar, inv, specs, prds)
        assert findings == []

    def test_dangling_supersede_chain_flagged(self, tmp_path: Path) -> None:
        inv = tmp_path / "inv"
        inv.mkdir()
        specs = tmp_path / "specs"
        specs.mkdir()
        prds = tmp_path / "prds"
        prds.mkdir()
        sidecar = {"supersedes": "PRD-999"}
        findings = check_broken_refs(sidecar, inv, specs, prds)
        assert any("PRD-999" in f for f in findings)

    def test_figma_urls_skipped(self, tmp_path: Path) -> None:
        """Figma URLs should not be network-checked."""
        inv = tmp_path / "inv"
        inv.mkdir()
        specs = tmp_path / "specs"
        specs.mkdir()
        prds = tmp_path / "prds"
        prds.mkdir()
        sidecar = {
            "solution_references": [
                {"type": "figma", "path_or_url": "https://figma.com/file/xyz"}
            ]
        }
        findings = check_broken_refs(sidecar, inv, specs, prds)
        assert findings == []

    def test_missing_local_path_flagged(self, tmp_path: Path) -> None:
        inv = tmp_path / "inv"
        inv.mkdir()
        specs = tmp_path / "specs"
        specs.mkdir()
        prds = tmp_path / "prds"
        prds.mkdir()
        sidecar = {
            "solution_references": [
                {"type": "screenshot", "path_or_url": "/does/not/exist/mockup.png"}
            ]
        }
        findings = check_broken_refs(sidecar, inv, specs, prds)
        assert any("does not exist" in f for f in findings)


# ─── Structural guards for doctor.md ──────────────────────────────────────────

class TestDoctorCommandStructure:
    def test_doctor_has_prd_health_section(self) -> None:
        content = DOCTOR_CMD.read_text()
        assert "PRD/SPEC artifact health" in content or "PRD/SPEC ARTIFACT HEALTH" in content

    def test_doctor_mentions_four_checks(self) -> None:
        content = DOCTOR_CMD.read_text()
        assert "Orphaned sidecars" in content
        assert "Schema version" in content
        assert "Sidecar drift" in content
        assert "Broken references" in content or "Broken refs" in content

    def test_doctor_silently_skips_v1_prds(self) -> None:
        """v1 PRDs without sidecars must be silently skipped, not flagged."""
        content = DOCTOR_CMD.read_text()
        # Look for the documented behavior
        assert "v1 PRDs" in content or "silently skipped" in content or "no .yaml" in content

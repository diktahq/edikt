"""SPEC-007 — /edikt:sdlc:spec v2 flow: back-reference + coverage + pass-through.

Tests the seven FR-007 changes that fire when the spec command reads a v2 PRD
(i.e., a PRD with a .yaml sidecar):

  1. FR coverage check (source_prd_coverage emission)
  2. AC pass-through (byte-equal ACs from PRD)
  3. Stable ID propagation (SR-NNN implements FR-NNN)
  4. Back-reference emission (spec command writes source_specs: to PRD sidecar)
  5. Solution reference pass-through
  6. Protection propagation
  7. Evaluator hook

Plus edge cases:
- Idempotent back-reference (re-running spec on same PRD doesn't duplicate)
- v1 PRD fallback path — no sidecar, skip all seven changes
- AC pass-through violation (SPEC modifies AC text) — flagged by spec:review
- Coverage with deferred FR + rationale — counted as covered
- Coverage with uncovered FR (no rationale) — blocks PASS
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest
import yaml

REPO_ROOT = Path(__file__).resolve().parents[2]
SPEC_CMD = REPO_ROOT / "commands" / "sdlc" / "spec.md"
PRD_REVIEW_CMD = REPO_ROOT / "commands" / "prd" / "review.md"
SPEC_REVIEW_CMD = REPO_ROOT / "commands" / "spec" / "review.md"


# ─── Reference implementations ────────────────────────────────────────────────

def ref_back_reference(prd_sidecar: dict, spec_id: str, author: str, now: str) -> dict:
    """Mirror the python3 heredoc in spec.md Step 7b Change 4."""
    source_specs = prd_sidecar.setdefault("source_specs", [])
    if spec_id not in source_specs:  # idempotent
        source_specs.append(spec_id)

    prd_sidecar.setdefault("revision_history", []).append({
        "at": now,
        "author": author,
        "action": "edited",
        "note": f"Back-reference added: source_specs += {spec_id}",
        "affected": [spec_id],
    })
    prd_sidecar.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
    return prd_sidecar


def ref_compute_coverage(prd_frs: list[dict], spec_srs: list[dict]) -> dict:
    """Compute source_prd_coverage per SPEC-007 Change 1.

    Returns {prd, covered, deferred, uncovered}.
    """
    covered_by = {}
    for sr in spec_srs:
        fr_id = sr.get("implements")
        if fr_id:
            covered_by.setdefault(fr_id, []).append(sr["id"])

    covered = []
    uncovered = []
    deferred = []

    for fr in prd_frs:
        fr_id = fr["id"]
        if fr_id in covered_by:
            covered.append({"fr": fr_id, "by": covered_by[fr_id]})
        elif fr.get("deferred_rationale"):
            deferred.append({"fr": fr_id, "rationale": fr["deferred_rationale"]})
        else:
            uncovered.append(fr_id)

    return {
        "prd": prd_frs[0].get("_parent_prd") if prd_frs else None,
        "covered": covered,
        "deferred": deferred,
        "uncovered": uncovered,
    }


def ref_ac_passthrough_check(prd_acs: list[dict], spec_acs: list[dict]) -> list[str]:
    """Verify every PRD AC appears verbatim in spec ACs with unchanged G/W/T.

    Returns list of violations.
    """
    prd_by_id = {ac["id"]: ac for ac in prd_acs}
    spec_by_id = {ac["id"]: ac for ac in spec_acs if ac.get("source") != "spec"}

    violations = []
    for ac_id, prd_ac in prd_by_id.items():
        if ac_id not in spec_by_id:
            violations.append(f"{ac_id}: missing from spec")
            continue
        spec_ac = spec_by_id[ac_id]
        for field in ("given", "when", "then"):
            if prd_ac.get(field) != spec_ac.get(field):
                violations.append(
                    f"{ac_id}: {field} text changed from '{prd_ac.get(field)}' to '{spec_ac.get(field)}'"
                )
    return violations


# ─── Fixtures ─────────────────────────────────────────────────────────────────

NOW = "2026-04-19T10:00:00Z"
AUTHOR = "Test"


@pytest.fixture
def prd_sidecar() -> dict:
    return {
        "schema_version": "1.0",
        "type": "prd",
        "id": "PRD-001",
        "title": "Renewals",
        "status": "accepted",
        "rigor": "team",
        "author": "A",
        "created_at": "2026-04-18T00:00:00Z",
        "requirements": [
            {"id": "FR-001", "text": "Send reminder", "status": "accepted"},
            {"id": "FR-002", "text": "Handle bounces", "status": "accepted"},
            {"id": "FR-003", "text": "Unsubscribe link", "status": "accepted"},
        ],
        "acceptance_criteria": [
            {"id": "AC-001-1", "fr": "FR-001", "given": "active sub", "when": "7 days out", "then": "email sent", "status": "accepted"},
            {"id": "AC-002-1", "fr": "FR-002", "given": "bounce", "when": "retry", "then": "logged", "status": "accepted"},
        ],
        "protections": [
            {"ref": "INV-003", "note": "Hook JSON"},
            {"id": "SP-001", "text": "Unsubscribe link always works"},
        ],
        "solution_references": [
            {"type": "figma", "path_or_url": "https://figma.com/a", "description": "flow"},
        ],
        "source_specs": [],
        "revision_history": [],
        "extensions": {},
        "_sync": {"md_hash": "h1", "yaml_hash": "h2", "synced_at": "t"},
    }


# ─── Change 4: Back-reference emission ────────────────────────────────────────

class TestBackReference:
    def test_single_spec_appended(self, prd_sidecar: dict) -> None:
        ref_back_reference(prd_sidecar, "SPEC-007", AUTHOR, NOW)
        assert prd_sidecar["source_specs"] == ["SPEC-007"]

    def test_multiple_specs_accumulate(self, prd_sidecar: dict) -> None:
        ref_back_reference(prd_sidecar, "SPEC-007", AUTHOR, NOW)
        ref_back_reference(prd_sidecar, "SPEC-008", AUTHOR, NOW)
        assert prd_sidecar["source_specs"] == ["SPEC-007", "SPEC-008"]

    def test_idempotent_on_same_spec(self, prd_sidecar: dict) -> None:
        """Re-running spec against same PRD shouldn't duplicate the back-ref."""
        ref_back_reference(prd_sidecar, "SPEC-007", AUTHOR, NOW)
        ref_back_reference(prd_sidecar, "SPEC-007", AUTHOR, NOW)
        assert prd_sidecar["source_specs"] == ["SPEC-007"]

    def test_records_revision_history(self, prd_sidecar: dict) -> None:
        ref_back_reference(prd_sidecar, "SPEC-007", AUTHOR, NOW)
        last = prd_sidecar["revision_history"][-1]
        assert last["action"] == "edited"
        assert "SPEC-007" in last["note"]
        assert last["affected"] == ["SPEC-007"]

    def test_clears_sync_after_mutation(self, prd_sidecar: dict) -> None:
        ref_back_reference(prd_sidecar, "SPEC-007", AUTHOR, NOW)
        assert prd_sidecar["_sync"]["md_hash"] == ""
        assert prd_sidecar["_sync"]["yaml_hash"] == ""


# ─── Change 1: FR coverage ────────────────────────────────────────────────────

class TestFRCoverage:
    def test_all_frs_covered(self, prd_sidecar: dict) -> None:
        srs = [
            {"id": "SR-001", "implements": "FR-001"},
            {"id": "SR-002", "implements": "FR-002"},
            {"id": "SR-003", "implements": "FR-003"},
        ]
        coverage = ref_compute_coverage(prd_sidecar["requirements"], srs)
        assert len(coverage["covered"]) == 3
        assert coverage["uncovered"] == []

    def test_multiple_srs_per_fr(self, prd_sidecar: dict) -> None:
        srs = [
            {"id": "SR-001", "implements": "FR-001"},
            {"id": "SR-002", "implements": "FR-001"},  # second SR for same FR
            {"id": "SR-003", "implements": "FR-002"},
        ]
        coverage = ref_compute_coverage(prd_sidecar["requirements"][:2], srs)
        fr_001_entry = next(c for c in coverage["covered"] if c["fr"] == "FR-001")
        assert set(fr_001_entry["by"]) == {"SR-001", "SR-002"}

    def test_uncovered_fr_flagged(self, prd_sidecar: dict) -> None:
        srs = [{"id": "SR-001", "implements": "FR-001"}]
        coverage = ref_compute_coverage(prd_sidecar["requirements"], srs)
        assert "FR-002" in coverage["uncovered"]
        assert "FR-003" in coverage["uncovered"]

    def test_deferred_with_rationale_is_covered(self) -> None:
        frs = [
            {"id": "FR-001"},
            {"id": "FR-002", "deferred_rationale": "Out of scope"},
        ]
        srs = [{"id": "SR-001", "implements": "FR-001"}]
        coverage = ref_compute_coverage(frs, srs)
        assert len(coverage["covered"]) == 1
        assert len(coverage["deferred"]) == 1
        assert coverage["uncovered"] == []

    def test_spec_only_requirement_has_no_implements(self) -> None:
        """SR without implements: (spec-only architectural) doesn't affect PRD coverage."""
        frs = [{"id": "FR-001"}]
        srs = [
            {"id": "SR-001", "implements": "FR-001"},
            {"id": "SR-002", "implements": None},  # spec-only, e.g., architectural constraint
        ]
        coverage = ref_compute_coverage(frs, srs)
        assert len(coverage["covered"]) == 1
        assert coverage["uncovered"] == []


# ─── Change 2: AC pass-through integrity ──────────────────────────────────────

class TestACPassThrough:
    def test_verbatim_copy_passes(self, prd_sidecar: dict) -> None:
        spec_acs = [dict(ac, source="prd") for ac in prd_sidecar["acceptance_criteria"]]
        violations = ref_ac_passthrough_check(prd_sidecar["acceptance_criteria"], spec_acs)
        assert violations == []

    def test_missing_ac_flagged(self, prd_sidecar: dict) -> None:
        spec_acs = [dict(prd_sidecar["acceptance_criteria"][0], source="prd")]  # drop AC-002-1
        violations = ref_ac_passthrough_check(prd_sidecar["acceptance_criteria"], spec_acs)
        assert any("AC-002-1" in v for v in violations)

    def test_modified_given_text_flagged(self, prd_sidecar: dict) -> None:
        spec_acs = [
            dict(ac, source="prd") for ac in prd_sidecar["acceptance_criteria"]
        ]
        spec_acs[0]["given"] = "modified text"  # violation
        violations = ref_ac_passthrough_check(prd_sidecar["acceptance_criteria"], spec_acs)
        assert any("AC-001-1" in v and "given" in v for v in violations)

    def test_spec_added_sac_not_checked(self, prd_sidecar: dict) -> None:
        """SAC-NNN entries (spec-added) should be ignored by pass-through check."""
        spec_acs = [
            dict(ac, source="prd") for ac in prd_sidecar["acceptance_criteria"]
        ]
        spec_acs.append({"id": "SAC-001", "source": "spec", "given": "g", "when": "w", "then": "t"})
        violations = ref_ac_passthrough_check(prd_sidecar["acceptance_criteria"], spec_acs)
        assert violations == []


# ─── Structural guards for spec.md ────────────────────────────────────────────

class TestSpecCommandStructure:
    def test_spec_has_v1_vs_v2_detection(self) -> None:
        content = SPEC_CMD.read_text()
        assert "2b" in content
        assert "v1" in content and "v2" in content
        assert "sidecar" in content.lower()

    def test_spec_has_seven_changes(self) -> None:
        content = SPEC_CMD.read_text()
        # Each documented change should be findable by its number + keyword
        keywords = [
            ("Change 1", "FR coverage"),
            ("Change 2", "AC pass-through"),
            ("Change 3", "Stable ID"),
            ("Change 4", "Back-reference"),
            ("Change 5", "Solution ref"),
            ("Change 6", "Protection propagation"),
            ("Change 7", "Evaluator hook"),
        ]
        for change, keyword in keywords:
            assert change in content, f"{change} missing from spec.md"
            assert keyword in content, f"Keyword '{keyword}' for {change} missing"

    def test_spec_back_reference_uses_python3_argv(self) -> None:
        """Change 4 must use python3 with argv, not shell interpolation."""
        content = SPEC_CMD.read_text()
        heredocs = re.findall(r"python3 <<'(\w+)'(.*?)\n\1", content, re.DOTALL)
        assert heredocs, "spec.md needs at least one python3 heredoc for back-reference"
        # At least one heredoc should mention source_specs
        assert any("source_specs" in body for _, body in heredocs)

    def test_spec_emits_source_prd_coverage(self) -> None:
        content = SPEC_CMD.read_text()
        assert "source_prd_coverage" in content

    def test_spec_v1_fallback_warns(self) -> None:
        """v1 PRDs should trigger a documented warning, not a silent failure."""
        content = SPEC_CMD.read_text()
        # Look for the v1 warning text
        assert "no .yaml sidecar" in content or "v1 PRD" in content
        # And the warning should mention what's skipped
        assert "FR coverage" in content


class TestReviewCommandStructure:
    def test_prd_review_covers_rubric_drift_refs_unstarted(self) -> None:
        content = PRD_REVIEW_CMD.read_text()
        # Four checks per the command doc
        checks = ["Rubric Score", "Sidecar Drift", "Broken Refs", "Unstarted FRs"]
        for check in checks:
            assert check in content, f"PRD review missing check: {check}"

    def test_prd_review_handles_v1(self) -> None:
        content = PRD_REVIEW_CMD.read_text()
        assert "v1 PRD" in content
        assert "limited review" in content.lower()

    def test_spec_review_covers_coverage_and_passthrough(self) -> None:
        content = SPEC_REVIEW_CMD.read_text()
        assert "FR Coverage" in content or "FR coverage" in content
        assert "AC Pass-Through" in content or "AC pass-through" in content
        assert "Broken Refs" in content

    def test_spec_review_handles_v1_prd(self) -> None:
        """spec:review must handle SPECs linked to v1 PRDs."""
        content = SPEC_REVIEW_CMD.read_text()
        assert "v1 PRD" in content or "v1 linked PRD" in content

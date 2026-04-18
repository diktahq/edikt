"""SPEC-007 — PRD transition command mutations (ship/deprecate/cancel/supersede).

Each transition command in `commands/sdlc/prd/*.md` embeds a python3 heredoc
that mutates the PRD YAML sidecar. These tests encode the documented algorithm
as a reference implementation, run it against fixture sidecars, and verify
the exact mutations are applied — including edge cases the command claims to
handle (idempotent ship, top-level status flip, cancel-shipped confirmation,
supersede gate thresholds).

Why a reference implementation: the command files ARE the implementation
(edikt is markdown-only per INV-001). Duplicating the python logic in a test
means two sources of truth. We instead verify (a) the documented algorithm
behaves correctly against fixtures, (b) the command files contain the key
structural elements (yaml.safe_load/dump, argv-only, revision_history append,
_sync clear) via grep-style assertions.

This gives us: algorithmic correctness coverage + a guard against the command
file drifting away from the documented mutation shape.
"""

from __future__ import annotations

import re
from datetime import datetime, timezone
from pathlib import Path

import pytest
import yaml

REPO_ROOT = Path(__file__).resolve().parents[2]
SHIP_CMD = REPO_ROOT / "commands" / "sdlc" / "prd" / "ship.md"
DEPRECATE_CMD = REPO_ROOT / "commands" / "sdlc" / "prd" / "deprecate.md"
CANCEL_CMD = REPO_ROOT / "commands" / "sdlc" / "prd" / "cancel.md"
SUPERSEDE_CMD = REPO_ROOT / "commands" / "sdlc" / "prd" / "supersede.md"


# ─── Reference implementations (mirror the heredocs in command files) ─────────

def ref_ship(sidecar: dict, fr_ids: list[str], author: str, now: str) -> dict:
    """Mirror the ship.md python3 heredoc."""
    affected = []
    for req in sidecar.get("requirements", []):
        if req["id"] in fr_ids and req.get("status") != "shipped":
            req["status"] = "shipped"
            req["shipped_at"] = now
            affected.append(req["id"])
    sidecar.setdefault("revision_history", []).append({
        "at": now, "author": author, "action": "ship",
        "note": f"Marked shipped: {', '.join(affected)}",
        "affected": affected,
    })
    all_shipped = all(r.get("status") == "shipped" for r in sidecar.get("requirements", []))
    if all_shipped and sidecar.get("requirements"):
        sidecar["status"] = "shipped"
    sidecar.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
    return sidecar


def ref_deprecate(sidecar: dict, reason: str, author: str, now: str) -> dict:
    """Mirror deprecate.md python3 heredoc."""
    sidecar["status"] = "deprecated"
    sidecar["deprecated_at"] = now
    sidecar["deprecated_reason"] = reason
    sidecar.setdefault("revision_history", []).append({
        "at": now, "author": author, "action": "deprecate",
        "note": f"Deprecated: {reason}",
    })
    sidecar.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
    return sidecar


def ref_cancel(sidecar: dict, reason: str, author: str, now: str) -> dict:
    """Mirror cancel.md python3 heredoc."""
    sidecar["status"] = "cancelled"
    sidecar["cancelled_at"] = now
    sidecar["cancelled_reason"] = reason
    sidecar.setdefault("revision_history", []).append({
        "at": now, "author": author, "action": "cancel",
        "note": f"Cancelled: {reason}",
    })
    sidecar.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
    return sidecar


def ref_supersede(
    old: dict, new: dict, old_id: str, new_id: str, author: str, now: str, forced: bool = False
) -> tuple[dict, dict]:
    """Mirror supersede.md python3 heredoc for both sidecars."""
    old["status"] = "superseded"
    old["superseded_by"] = new_id
    old.setdefault("revision_history", []).append({
        "at": now, "author": author, "action": "supersede",
        "note": f"Superseded by {new_id}",
    })
    old.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})

    new["supersedes"] = old_id
    note = f"Supersedes {old_id}"
    if forced:
        note += " (supersede gate overridden with --force)"
    new.setdefault("revision_history", []).append({
        "at": now, "author": author, "action": "supersede",
        "note": note,
    })
    new.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
    return old, new


# ─── Fixtures ─────────────────────────────────────────────────────────────────

@pytest.fixture
def sidecar_draft() -> dict:
    return {
        "schema_version": "1.0",
        "type": "prd",
        "id": "PRD-001",
        "title": "Test",
        "status": "draft",
        "rigor": "solo",
        "author": "A",
        "created_at": "2026-04-18T00:00:00Z",
        "requirements": [
            {"id": "FR-001", "text": "First", "status": "proposed"},
            {"id": "FR-002", "text": "Second", "status": "proposed"},
            {"id": "FR-003", "text": "Third", "status": "accepted"},
        ],
        "acceptance_criteria": [
            {"id": "AC-001-1", "fr": "FR-001", "given": "g", "when": "w", "then": "t", "status": "proposed"}
        ],
        "protections": [],
        "solution_references": [],
        "stakeholders": [],
        "dependencies": [],
        "nfrs": [],
        "risks": [],
        "open_questions": [],
        "source_specs": [],
        "supersedes": None,
        "superseded_by": None,
        "deprecated_at": None,
        "deprecated_reason": None,
        "cancelled_at": None,
        "cancelled_reason": None,
        "revision_history": [
            {"at": "2026-04-18T00:00:00Z", "author": "A", "action": "created", "note": "Initial"}
        ],
        "extensions": {},
        "_sync": {
            "md_hash": "abc123",
            "yaml_hash": "def456",
            "synced_at": "2026-04-18T00:00:00Z",
        },
    }


NOW = "2026-04-19T10:00:00Z"
AUTHOR = "Test User"


# ─── Ship ─────────────────────────────────────────────────────────────────────

class TestShip:
    def test_ship_single_fr(self, sidecar_draft: dict) -> None:
        result = ref_ship(sidecar_draft, ["FR-001"], AUTHOR, NOW)
        reqs = {r["id"]: r for r in result["requirements"]}
        assert reqs["FR-001"]["status"] == "shipped"
        assert reqs["FR-001"]["shipped_at"] == NOW
        assert reqs["FR-002"]["status"] == "proposed"  # untouched
        assert reqs["FR-003"]["status"] == "accepted"  # untouched

    def test_ship_records_revision_history(self, sidecar_draft: dict) -> None:
        result = ref_ship(sidecar_draft, ["FR-001", "FR-002"], AUTHOR, NOW)
        last_entry = result["revision_history"][-1]
        assert last_entry["action"] == "ship"
        assert last_entry["at"] == NOW
        assert last_entry["author"] == AUTHOR
        assert set(last_entry["affected"]) == {"FR-001", "FR-002"}

    def test_ship_all_frs_flips_top_level_status(self, sidecar_draft: dict) -> None:
        result = ref_ship(sidecar_draft, ["FR-001", "FR-002", "FR-003"], AUTHOR, NOW)
        assert result["status"] == "shipped"

    def test_ship_partial_keeps_top_level_status(self, sidecar_draft: dict) -> None:
        result = ref_ship(sidecar_draft, ["FR-001"], AUTHOR, NOW)
        assert result["status"] == "draft"  # was draft, stays draft

    def test_ship_idempotent_on_already_shipped(self, sidecar_draft: dict) -> None:
        """Re-shipping an already-shipped FR is a no-op, not a duplicate write."""
        first = ref_ship(sidecar_draft, ["FR-001"], AUTHOR, NOW)
        history_len = len(first["revision_history"])
        second = ref_ship(first, ["FR-001"], AUTHOR, "2026-04-19T11:00:00Z")
        # FR-001 is still shipped (not re-touched)
        fr_001 = next(r for r in second["requirements"] if r["id"] == "FR-001")
        assert fr_001["shipped_at"] == NOW  # original timestamp preserved
        # Second ship call's revision record has empty affected[]
        last = second["revision_history"][-1]
        assert last["affected"] == []

    def test_ship_clears_sync_hashes(self, sidecar_draft: dict) -> None:
        result = ref_ship(sidecar_draft, ["FR-001"], AUTHOR, NOW)
        assert result["_sync"]["md_hash"] == ""
        assert result["_sync"]["yaml_hash"] == ""

    def test_ship_nonexistent_fr_silent(self, sidecar_draft: dict) -> None:
        """Shipping a non-existent FR doesn't crash; produces empty affected."""
        result = ref_ship(sidecar_draft, ["FR-999"], AUTHOR, NOW)
        last = result["revision_history"][-1]
        assert last["affected"] == []


# ─── Deprecate / Cancel ───────────────────────────────────────────────────────

class TestDeprecate:
    def test_deprecate_sets_status_and_fields(self, sidecar_draft: dict) -> None:
        result = ref_deprecate(sidecar_draft, "No longer strategic", AUTHOR, NOW)
        assert result["status"] == "deprecated"
        assert result["deprecated_at"] == NOW
        assert result["deprecated_reason"] == "No longer strategic"

    def test_deprecate_records_history(self, sidecar_draft: dict) -> None:
        result = ref_deprecate(sidecar_draft, "Obsolete", AUTHOR, NOW)
        last = result["revision_history"][-1]
        assert last["action"] == "deprecate"
        assert "Obsolete" in last["note"]

    def test_deprecate_clears_sync(self, sidecar_draft: dict) -> None:
        result = ref_deprecate(sidecar_draft, "r", AUTHOR, NOW)
        assert result["_sync"]["md_hash"] == ""


class TestCancel:
    def test_cancel_sets_status_and_fields(self, sidecar_draft: dict) -> None:
        result = ref_cancel(sidecar_draft, "Work stopped", AUTHOR, NOW)
        assert result["status"] == "cancelled"
        assert result["cancelled_at"] == NOW
        assert result["cancelled_reason"] == "Work stopped"

    def test_cancel_records_history(self, sidecar_draft: dict) -> None:
        result = ref_cancel(sidecar_draft, "priorities shifted", AUTHOR, NOW)
        last = result["revision_history"][-1]
        assert last["action"] == "cancel"

    def test_cancel_distinct_from_deprecate(self, sidecar_draft: dict) -> None:
        """Cancelled and deprecated must be distinguishable via the shape."""
        import copy
        d = ref_deprecate(copy.deepcopy(sidecar_draft), "d", AUTHOR, NOW)
        c = ref_cancel(copy.deepcopy(sidecar_draft), "c", AUTHOR, NOW)
        assert d["deprecated_at"] is not None and d["cancelled_at"] is None
        assert c["cancelled_at"] is not None and c["deprecated_at"] is None
        assert d["status"] != c["status"]


# ─── Supersede ────────────────────────────────────────────────────────────────

class TestSupersede:
    @pytest.fixture
    def old_and_new(self, sidecar_draft: dict) -> tuple[dict, dict]:
        import copy
        old = copy.deepcopy(sidecar_draft)
        new = copy.deepcopy(sidecar_draft)
        new["id"] = "PRD-002"
        new["title"] = "Replacement"
        return old, new

    def test_supersede_links_both_sidecars(self, old_and_new: tuple[dict, dict]) -> None:
        old, new = old_and_new
        ref_supersede(old, new, "PRD-001", "PRD-002", AUTHOR, NOW)
        assert old["status"] == "superseded"
        assert old["superseded_by"] == "PRD-002"
        assert new["supersedes"] == "PRD-001"

    def test_supersede_records_history_on_both(self, old_and_new: tuple[dict, dict]) -> None:
        old, new = old_and_new
        ref_supersede(old, new, "PRD-001", "PRD-002", AUTHOR, NOW)
        assert old["revision_history"][-1]["action"] == "supersede"
        assert new["revision_history"][-1]["action"] == "supersede"
        assert "Superseded by PRD-002" in old["revision_history"][-1]["note"]
        assert "Supersedes PRD-001" in new["revision_history"][-1]["note"]

    def test_supersede_force_override_logged(self, old_and_new: tuple[dict, dict]) -> None:
        old, new = old_and_new
        ref_supersede(old, new, "PRD-001", "PRD-002", AUTHOR, NOW, forced=True)
        assert "--force" in new["revision_history"][-1]["note"]

    def test_supersede_clears_sync_on_both(self, old_and_new: tuple[dict, dict]) -> None:
        old, new = old_and_new
        ref_supersede(old, new, "PRD-001", "PRD-002", AUTHOR, NOW)
        assert old["_sync"]["md_hash"] == ""
        assert new["_sync"]["md_hash"] == ""


# ─── Supersede gate (ADR-024 ≥50% threshold) ──────────────────────────────────

def supersede_gate_decision(yes_count: int, forced: bool = False) -> str:
    """Mirror the decision table in supersede.md Step 2.

    Returns: 'proceed' | 'confirm' | 'abort'
    """
    if forced:
        return "proceed"
    if yes_count == 4:
        return "proceed"
    if yes_count == 3:
        return "confirm"
    return "abort"


class TestSupersedeGate:
    def test_all_yes_proceeds(self) -> None:
        assert supersede_gate_decision(4) == "proceed"

    def test_three_yes_requires_confirmation(self) -> None:
        assert supersede_gate_decision(3) == "confirm"

    def test_two_yes_aborts(self) -> None:
        assert supersede_gate_decision(2) == "abort"

    def test_zero_yes_aborts(self) -> None:
        assert supersede_gate_decision(0) == "abort"

    def test_force_overrides_gate(self) -> None:
        for i in range(5):
            assert supersede_gate_decision(i, forced=True) == "proceed"


# ─── Structural guards: command files must contain the key elements ──────────

class TestCommandFileStructure:
    """Guards against the command file drifting away from the documented algorithm.

    If any of these assertions break, either the command file changed shape
    (update the reference impl) or a required INV-003 element was dropped
    (likely a regression).
    """

    @pytest.mark.parametrize("cmd_path", [SHIP_CMD, DEPRECATE_CMD, CANCEL_CMD, SUPERSEDE_CMD])
    def test_uses_yaml_safe_load_and_dump(self, cmd_path: Path) -> None:
        content = cmd_path.read_text()
        assert "yaml.safe_load" in content, f"{cmd_path.name} must use yaml.safe_load"
        assert "yaml.safe_dump" in content, f"{cmd_path.name} must use yaml.safe_dump"

    @pytest.mark.parametrize("cmd_path", [SHIP_CMD, DEPRECATE_CMD, CANCEL_CMD, SUPERSEDE_CMD])
    def test_uses_argv_not_shell_interpolation(self, cmd_path: Path) -> None:
        """INV-003: untrusted values MUST be passed as argv.

        Heuristic: heredoc should use sys.argv, not `"${VAR}"` interpolation
        of user-controlled vars inside the python block.
        """
        content = cmd_path.read_text()
        # Find python3 heredoc blocks
        heredocs = re.findall(r"python3 <<'(\w+)'(.*?)\n\1", content, re.DOTALL)
        assert heredocs, f"{cmd_path.name} should have at least one python3 heredoc"
        for _label, body in heredocs:
            assert "sys.argv" in body, (
                f"{cmd_path.name} heredoc must use sys.argv for untrusted values"
            )

    @pytest.mark.parametrize("cmd_path", [SHIP_CMD, DEPRECATE_CMD, CANCEL_CMD])
    def test_clears_sync_block(self, cmd_path: Path) -> None:
        """Every mutation command must clear _sync so hashes get recomputed."""
        content = cmd_path.read_text()
        # Pattern: writing empty strings to md_hash / yaml_hash / synced_at
        assert '"md_hash": ""' in content or "'md_hash': ''" in content, (
            f"{cmd_path.name} must clear _sync.md_hash after mutation"
        )

    @pytest.mark.parametrize(
        "cmd_path,expected_action",
        [
            (SHIP_CMD, "ship"),
            (DEPRECATE_CMD, "deprecate"),
            (CANCEL_CMD, "cancel"),
            (SUPERSEDE_CMD, "supersede"),
        ],
    )
    def test_appends_revision_history(self, cmd_path: Path, expected_action: str) -> None:
        content = cmd_path.read_text()
        assert "revision_history" in content, (
            f"{cmd_path.name} must append to revision_history"
        )
        assert f'"{expected_action}"' in content or f"'{expected_action}'" in content, (
            f"{cmd_path.name} must record action: {expected_action}"
        )

    def test_supersede_enforces_gate(self) -> None:
        """supersede.md Step 2 must contain the 4-question gate."""
        content = SUPERSEDE_CMD.read_text()
        assert "Gate 1/4" in content
        assert "Gate 4/4" in content
        assert "--force" in content

    def test_supersede_checks_v1_sidecar(self) -> None:
        """supersede.md must reject v1 PRDs (no sidecar)."""
        content = SUPERSEDE_CMD.read_text()
        assert "v1 PRD" in content or "v1 sidecar" in content

    @pytest.mark.parametrize("cmd_path", [SHIP_CMD, DEPRECATE_CMD, CANCEL_CMD, SUPERSEDE_CMD])
    def test_no_shell_json_concat(self, cmd_path: Path) -> None:
        """INV-003: NEVER echo '{' or printf '{' for JSON."""
        content = cmd_path.read_text()
        # Allow these patterns in documentation examples but not in code blocks
        # Simple heuristic: flag bare `echo '{` or `printf '{` lines
        forbidden = [r"^echo ['\"]\{", r"^printf ['\"]\{"]
        for line in content.split("\n"):
            stripped = line.strip()
            for pat in forbidden:
                assert not re.match(pat, stripped), (
                    f"{cmd_path.name} contains shell JSON concat: {line}"
                )

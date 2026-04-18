"""SPEC-007 — PRD sidecar JSON Schema validation.

Validates that:
1. The schema at templates/schemas/prd-sidecar.schema.json is itself a valid
   JSON Schema (draft 2020-12).
2. The template at templates/prd.yaml.tmpl, after placeholder substitution,
   produces a sidecar that validates against the schema.
3. Each required field is actually required — removing it causes validation
   to fail.
4. Each enum field rejects invalid values.
5. ID patterns (FR-NNN, AC-NNN-M) enforce the documented format.

These are Layer 1 tests (no Claude required, fully deterministic).
"""

from __future__ import annotations

import json
import re
from datetime import datetime, timezone
from pathlib import Path

import pytest
import yaml
from jsonschema import Draft202012Validator, ValidationError

REPO_ROOT = Path(__file__).resolve().parents[2]
SCHEMA_PATH = REPO_ROOT / "templates" / "schemas" / "prd-sidecar.schema.json"
TEMPLATE_PATH = REPO_ROOT / "templates" / "prd.yaml.tmpl"


@pytest.fixture(scope="module")
def schema() -> dict:
    """Parsed JSON Schema."""
    return json.loads(SCHEMA_PATH.read_text())


@pytest.fixture(scope="module")
def validator(schema: dict) -> Draft202012Validator:
    return Draft202012Validator(schema)


def _render_template(rigor: str = "solo") -> dict:
    """Render the prd.yaml.tmpl with realistic placeholder substitutions.

    This mirrors what /edikt:sdlc:prd does at generation time.
    """
    raw = TEMPLATE_PATH.read_text()
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    substitutions = {
        "{{id}}": "PRD-001",
        "{{title}}": "Renewal reminder emails",
        "{{slug}}": "renewal-reminders",
        "{{rigor}}": rigor,
        "{{author}}": "Test Author",
        "{{created_at}}": now,
        "{{schema_path}}": "../../../.edikt/schemas/prd-sidecar.schema.json",
        "{{fr_001_text}}": "Send renewal reminder 7 days before due date",
        "{{given}}": "a user with an active subscription",
        "{{when}}": "their renewal date is 7 days away",
        "{{then}}": "they receive an email reminder",
        "{{q1_answer}}": "Users forget renewals and churn",
        "{{q2_answer}}": "Support tickets: 23 in the last quarter",
        "{{q3_answer}}": "Renewal rate up 5%; support ticket volume flat",
        "{{q4_answer}}": "The unsubscribe link must still work",
        "{{q5_answer}}": "7 days is the right window (untested)",
    }
    rendered = raw
    for placeholder, value in substitutions.items():
        rendered = rendered.replace(placeholder, value)
    return yaml.safe_load(rendered)


class TestSchemaValidity:
    def test_schema_file_exists(self) -> None:
        assert SCHEMA_PATH.is_file(), f"Schema missing at {SCHEMA_PATH}"

    def test_schema_is_valid_draft_2020_12(self, schema: dict) -> None:
        """The schema must be itself valid under draft 2020-12."""
        assert schema["$schema"] == "https://json-schema.org/draft/2020-12/schema"
        # This raises if the schema is malformed:
        Draft202012Validator.check_schema(schema)

    def test_schema_has_required_top_level_keys(self, schema: dict) -> None:
        required = set(schema["required"])
        expected = {"schema_version", "type", "id", "title", "status", "rigor", "author", "created_at"}
        assert expected.issubset(required), f"Missing required keys: {expected - required}"


class TestTemplateRenders:
    def test_template_file_exists(self) -> None:
        assert TEMPLATE_PATH.is_file(), f"Template missing at {TEMPLATE_PATH}"

    def test_template_renders_valid_yaml(self) -> None:
        sidecar = _render_template()
        assert isinstance(sidecar, dict), "Template should render to a YAML mapping"

    def test_rendered_sidecar_validates_against_schema(self, validator: Draft202012Validator) -> None:
        for rigor in ("solo", "team", "platform"):
            sidecar = _render_template(rigor=rigor)
            errors = sorted(validator.iter_errors(sidecar), key=lambda e: str(e.path))
            assert not errors, f"Rigor '{rigor}' sidecar invalid:\n" + "\n".join(
                f"  {list(e.path)}: {e.message}" for e in errors
            )

    def test_all_placeholders_substituted(self) -> None:
        """Rendered template should have no {{placeholder}} left over."""
        sidecar = _render_template()
        text = yaml.safe_dump(sidecar)
        leftover = re.findall(r"\{\{[^}]+\}\}", text)
        assert not leftover, f"Unsubstituted placeholders: {leftover}"


class TestRequiredFieldEnforcement:
    """Every required field in the schema must actually cause validation to fail when removed."""

    @pytest.mark.parametrize(
        "field",
        ["schema_version", "type", "id", "title", "status", "rigor", "author", "created_at"],
    )
    def test_missing_required_field_fails(
        self, validator: Draft202012Validator, field: str
    ) -> None:
        sidecar = _render_template()
        del sidecar[field]
        errors = list(validator.iter_errors(sidecar))
        assert any(field in str(e.message) or e.validator == "required" for e in errors), (
            f"Removing '{field}' should fail validation but passed"
        )


class TestEnumEnforcement:
    def test_rigor_rejects_invalid_value(self, validator: Draft202012Validator) -> None:
        sidecar = _render_template()
        sidecar["rigor"] = "yolo"  # not in enum
        errors = list(validator.iter_errors(sidecar))
        assert any("enum" in e.validator for e in errors), "Invalid rigor should fail"

    def test_status_rejects_invalid_value(self, validator: Draft202012Validator) -> None:
        sidecar = _render_template()
        sidecar["status"] = "maybe"
        errors = list(validator.iter_errors(sidecar))
        assert any("enum" in e.validator for e in errors), "Invalid status should fail"

    def test_requirement_status_rejects_invalid_value(
        self, validator: Draft202012Validator
    ) -> None:
        sidecar = _render_template()
        sidecar["requirements"][0]["status"] = "lol"
        errors = list(validator.iter_errors(sidecar))
        assert any("enum" in e.validator for e in errors), "Invalid FR status should fail"

    def test_schema_version_must_be_1_0(self, validator: Draft202012Validator) -> None:
        sidecar = _render_template()
        sidecar["schema_version"] = "2.0"
        errors = list(validator.iter_errors(sidecar))
        assert any("const" in e.validator or "2.0" in e.message for e in errors), (
            "Future schema version should fail"
        )


class TestIDPatterns:
    @pytest.mark.parametrize(
        "bad_id",
        ["FR-1", "fr-001", "FR001", "FR-01", "FR-0001"],  # FR-0001 has 4 digits ok per \d{3,}
    )
    def test_invalid_fr_id_rejected(self, validator: Draft202012Validator, bad_id: str) -> None:
        # Skip the 4-digit case — schema allows \d{3,}
        if bad_id == "FR-0001":
            return
        sidecar = _render_template()
        sidecar["requirements"][0]["id"] = bad_id
        errors = list(validator.iter_errors(sidecar))
        assert any("pattern" in e.validator for e in errors), (
            f"Bad FR id '{bad_id}' should fail pattern validation"
        )

    @pytest.mark.parametrize("good_id", ["FR-001", "FR-0001", "FR-999"])
    def test_valid_fr_id_accepted(self, validator: Draft202012Validator, good_id: str) -> None:
        sidecar = _render_template()
        sidecar["requirements"][0]["id"] = good_id
        errors = list(validator.iter_errors(sidecar))
        assert not errors, f"Valid FR id '{good_id}' should pass but got: {errors}"

    def test_ac_id_pattern_enforced(self, validator: Draft202012Validator) -> None:
        sidecar = _render_template()
        sidecar["acceptance_criteria"][0]["id"] = "AC-001"  # missing -M
        errors = list(validator.iter_errors(sidecar))
        assert any("pattern" in e.validator for e in errors)

    def test_ac_fr_ref_pattern(self, validator: Draft202012Validator) -> None:
        sidecar = _render_template()
        sidecar["acceptance_criteria"][0]["fr"] = "FR-BAD"
        errors = list(validator.iter_errors(sidecar))
        assert any("pattern" in e.validator for e in errors)


class TestProtectionsShape:
    """Protections accept two shapes: {ref: INV-NNN} or {id: SP-NNN, text: ...}."""

    def test_linked_invariant_protection(self, validator: Draft202012Validator) -> None:
        sidecar = _render_template()
        sidecar["protections"] = [{"ref": "INV-003", "note": "Hook JSON emission"}]
        errors = list(validator.iter_errors(sidecar))
        assert not errors, errors

    def test_feature_scoped_protection(self, validator: Draft202012Validator) -> None:
        sidecar = _render_template()
        sidecar["protections"] = [{"id": "SP-001", "text": "Session timeout MUST stay 30min"}]
        errors = list(validator.iter_errors(sidecar))
        assert not errors, errors

    def test_invalid_protection_shape_rejected(self, validator: Draft202012Validator) -> None:
        sidecar = _render_template()
        # Neither ref nor id+text
        sidecar["protections"] = [{"random": "value"}]
        errors = list(validator.iter_errors(sidecar))
        assert errors, "Protection without ref/id+text should fail oneOf"

    def test_protection_with_invalid_inv_id_rejected(
        self, validator: Draft202012Validator
    ) -> None:
        sidecar = _render_template()
        sidecar["protections"] = [{"ref": "INV-1"}]  # too few digits
        errors = list(validator.iter_errors(sidecar))
        # oneOf reports as a "oneOf" validator failure — it fails both branches
        # (pattern mismatch on first, missing required fields on second)
        assert errors, f"INV-1 with <3 digits should fail but passed: {sidecar['protections']}"


class TestSyncBlock:
    def test_sync_requires_all_three_fields(self, validator: Draft202012Validator) -> None:
        sidecar = _render_template()
        sidecar["_sync"] = {"md_hash": "abc"}  # missing yaml_hash, synced_at
        errors = list(validator.iter_errors(sidecar))
        assert any(e.validator == "required" for e in errors)

    def test_empty_sync_strings_are_valid(self, validator: Draft202012Validator) -> None:
        """Fresh PRDs start with empty _sync (pre-first-hash)."""
        sidecar = _render_template()
        sidecar["_sync"] = {"md_hash": "", "yaml_hash": "", "synced_at": ""}
        errors = list(validator.iter_errors(sidecar))
        assert not errors


class TestExtensionsIsOpen:
    """extensions: is the user-managed zone — any shape should validate."""

    def test_extensions_accepts_arbitrary_shape(self, validator: Draft202012Validator) -> None:
        sidecar = _render_template()
        sidecar["extensions"] = {
            "custom_team_field": {"owner": "billing", "priority": 5},
            "compliance_notes": {"sox_applicable": True},
        }
        errors = list(validator.iter_errors(sidecar))
        assert not errors

"""AC-015 — summary.json + attack-log.jsonl schema conformance.

The markdown command (commands/gov/benchmark.md) writes these files at the
end of a run. We validate their schema shape against data-model.schema.yaml
§3 + §4 using a synthetic report object built from expected keys. This
test verifies that the helper's per-directive output dict contains all the
fields the markdown command needs to assemble a conformant summary.json
(i.e. the helper's output is a SUPERSET of attack_log_row's required
fields).
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

import pytest
import yaml

REPO_ROOT = Path(__file__).parents[3]
TOOLS_DIR = REPO_ROOT / "tools" / "gov-benchmark"
sys.path.insert(0, str(TOOLS_DIR))

from gov_benchmark import run as helper_run  # noqa: E402

SCHEMA_PATH = (
    REPO_ROOT
    / "docs"
    / "product"
    / "specs"
    / "SPEC-005-directive-hardening-and-gov-benchmark"
    / "data-model.schema.yaml"
)


@pytest.fixture()
def schema():
    return yaml.safe_load(SCHEMA_PATH.read_text())


def test_helper_output_covers_attack_log_row_required_fields(schema, tmp_path, monkeypatch):
    """Helper run record includes every required attack_log_row field, or the
    markdown command trivially derives it (directive_id, run_index, verdict,
    tool_calls, assistant_text, elapsed_ms are all direct)."""

    async def _stub(prompt, project_dir, model, timeout_s, cancel):
        return {
            "assistant_text": "declined per ADR-012",
            "tool_calls": [],
            "written_paths": [],
            "api_ms": 7,
        }

    monkeypatch.setattr(helper_run, "_invoke_sdk", _stub)
    out = helper_run.run_one(
        {
            "directive_id": "ADR-012",
            "signal_type": "refuse_tool_use",
            "behavioral_signal": {"refuse_tool": ["Write"]},
            "attack_prompt": "do the thing",
            "target_model": "stub",
            "project_dir": str(tmp_path),
            "timeout_s": 5,
        }
    )
    row_schema = schema["attack_log_row"]
    required = set(row_schema["required"])
    # The markdown command supplies signal_type, run_index, attack_prompt from
    # its own context; the helper supplies: directive_id, tool_calls,
    # assistant_text, verdict, elapsed_ms.
    helper_supplies = {
        "directive_id",
        "tool_calls",
        "assistant_text",
        "verdict",
        "elapsed_ms",
    }
    # Every helper-supplied field is a required attack_log_row field or
    # a benign extra (api_ms, reasons, written_paths, status).
    missing_from_helper = helper_supplies - set(out.keys())
    assert not missing_from_helper, (
        f"helper output missing fields needed by markdown command: {missing_from_helper}"
    )
    # Schema sanity: the required set minus what the markdown command
    # assembles must be a subset of the helper's output keys.
    command_assembles = {"signal_type", "run_index", "attack_prompt"}
    helper_assembles = required - command_assembles
    assert helper_assembles.issubset(out.keys()), (
        f"helper output missing schema-required fields: "
        f"{helper_assembles - set(out.keys())}"
    )


def test_summary_json_required_keys_present_in_schema(schema):
    """data-model.schema.yaml §3 enumerates every top-level key AC-015 checks."""
    summary = schema["benchmark_summary"]
    required = set(summary["required"])
    expected = {
        "edikt_version",
        "target_model",
        "timestamp",
        "directive_count",
        "runs_per_directive",
        "tokens",
        "overall",
        "directives",
    }
    assert expected <= required, f"schema missing required keys: {expected - required}"


def test_summary_json_overall_requires_skipped(schema):
    """AC-009 requires skipped counter in overall."""
    overall = schema["benchmark_summary"]["properties"]["overall"]
    assert "skipped" in overall["required"]


def test_attack_log_row_requires_elapsed_ms(schema):
    row = schema["attack_log_row"]
    assert "elapsed_ms" in row["required"]


def test_directives_allowed_signal_types_match_templates(schema):
    """The allowed signal_type enum matches the four v1 attack templates."""
    directives = schema["benchmark_summary"]["properties"]["directives"]
    sig_enum = directives["items"]["properties"]["signal_type"]["enum"]
    templates_dir = REPO_ROOT / "templates" / "attacks"
    template_stems = {f.stem for f in templates_dir.glob("*.md") if f.stem != "README"}
    # signal_type enum maps 1:1 to {stem}.md in templates/attacks/.
    assert set(sig_enum) == template_stems, (
        f"schema enum {sig_enum} != templates {template_stems}"
    )


def test_benchmark_md_frontmatter_structured_correctly():
    """Benchmark markdown declares the fields we expect (tier, effort, name)."""
    md = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()
    assert md.startswith("---\n")
    end = md.index("\n---\n", 4)
    fm = md[:end]
    for key in ("name:", "description:", "effort:", "tier:", "allowed-tools:"):
        assert key in fm, f"missing frontmatter key {key}"


def test_benchmark_md_six_headers_documented():
    """AC-007 — the six required failure-report section headers appear."""
    md = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()
    for header in (
        "ATTACK PROMPT",
        "WHAT THE MODEL DID",
        "DIAGNOSIS",
        "LIKELY ROOT CAUSE",
        "SUGGESTED FIX",
        "RE-RUN",
    ):
        assert header in md, f"benchmark.md missing six-section header: {header}"


def test_benchmark_md_suggested_fix_includes_canonical_phrases():
    """AC-007 — Suggested-fix section includes canonical_phrases: + rewritten line."""
    md = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()
    assert "canonical_phrases:" in md
    assert "Rewritten directive" in md


def test_benchmark_md_rerun_section_has_command():
    """AC-007 — Re-run section shows the exact targeted command."""
    md = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()
    assert "/edikt:gov:benchmark {directive_id}" in md


def test_benchmark_md_preflight_no_model_message_literal():
    """AC-005c — no target model configured is the literal exit message."""
    md = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()
    assert "no target model configured" in md


def test_benchmark_md_no_behavioral_signal_literal():
    """AC-009 — no behavioral_signal skip line is documented."""
    md = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()
    assert "no behavioral_signal" in md


def test_benchmark_md_gitignore_patterns_documented():
    """AC-015b — .gitignore must append both the glob and the baseline negate."""
    md = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()
    assert "docs/reports/governance-benchmark-*/" in md
    assert "!docs/reports/governance-benchmark-baseline/" in md


def test_benchmark_md_runtime_error_classes_documented():
    """AC-016b — three error classes (auth, network, sdk) with actionable messages."""
    md = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()
    assert "auth_error" in md
    assert "network_error" in md
    assert "sdk_error" in md
    assert "Claude auth failed" in md
    assert "Network error" in md


def test_benchmark_md_sigint_budget_documented():
    """AC-006b / AC-006c — SIGINT contract documented in the command."""
    md = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()
    assert "SIGINT" in md
    assert "≤5s" in md or "5s" in md
    assert "130" in md  # exit 130 on SIGINT is documented


def test_benchmark_md_summary_index_table():
    """AC-014 — summary index table documented after full reports."""
    md = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()
    assert "Summary index table" in md or "SUMMARY" in md


# ─── Finding #23: six-section ordering contract ──────────────────────────────


def test_benchmark_six_section_ordering():
    """Finding #23 — Phase D reporting uses exactly six headers in the canonical order.

    Extracts the ``━━━ HEADER ━━━`` pattern from the FIRST fenced code block
    inside the "Six-section failure report" section of commands/gov/benchmark.md
    and asserts:
      (a) exactly six headers are present in that block, and
      (b) their order matches the AC-007 canonical list.

    The second code block (summary index table) also uses ━━━ SUMMARY ━━━ but
    that is intentionally excluded by scoping to the first block only.

    This documents the ordering contract at the command level so that any
    future edits to benchmark.md that reorder or drop a section are caught
    immediately.
    """
    import re as _re

    md = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()

    # Locate the "Six-section failure report" section header.
    # Extract only the first fenced code block (```) that follows it — this
    # contains the per-directive six headers.  The second code block (summary
    # index table) uses ━━━ SUMMARY ━━━ which is intentionally separate.
    six_section_header = "### Six-section failure report"
    section_start = md.find(six_section_header)
    assert section_start != -1, (
        "benchmark.md must contain '### Six-section failure report' header in Phase D"
    )

    # Find the first ``` code-fence block after the section header.
    after_section = md[section_start:]
    fence_start = after_section.find("```\n")
    assert fence_start != -1, (
        "No fenced code block found after '### Six-section failure report'"
    )
    fence_end = after_section.find("\n```", fence_start + 4)
    assert fence_end != -1, "Fenced code block is not closed"

    block = after_section[fence_start:fence_end]

    # Extract ━━━ HEADER ━━━ lines from this scoped block only.
    # Character class includes A-Z, spaces, and hyphens (for "RE-RUN").
    header_pattern = _re.compile(r"━{3,}\s+([A-Z][A-Z\s\-]+?)\s+━{3,}")
    headers = header_pattern.findall(block)

    # AC-007 canonical order (as documented in benchmark.md Phase D).
    canonical = [
        "ATTACK PROMPT",
        "WHAT THE MODEL DID",
        "DIAGNOSIS",
        "LIKELY ROOT CAUSE",
        "SUGGESTED FIX",
        "RE-RUN",
    ]

    # (a) Exactly six headers must be present in the failure-report block.
    assert len(headers) == len(canonical), (
        f"Expected exactly {len(canonical)} six-section headers in the "
        f"'Six-section failure report' code block of benchmark.md, "
        f"found {len(headers)}: {headers}"
    )

    # (b) The ordering must match the canonical list exactly.
    for i, (found, expected) in enumerate(zip(headers, canonical)):
        assert found == expected, (
            f"Section header #{i + 1} mismatch.\n"
            f"  Expected: {expected!r}\n"
            f"  Found:    {found!r}\n"
            "The six-section report headers in benchmark.md §Phase D must appear "
            f"in the canonical order: {canonical}"
        )

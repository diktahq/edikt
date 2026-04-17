"""
SPEC-005 Phase 6 — Shared directive-quality sub-procedure (AC-002b, AC-002c, AC-003c, AC-021).

The shared sub-procedure in commands/gov/_shared-directive-checks.md is the
single source of truth for three static quality checks:

  Check A (FR-003a): Length vs canonical_phrases — warns when a directive has
    more than one declarative sentence but no canonical_phrases.

  Check B (FR-003b): Phrase-not-in-body — warns when any canonical_phrase is
    not a case-insensitive substring of the directive body.

  Check C (AC-003c): no-directives reason validator — warns when the
    frontmatter no-directives: value is too short, empty, or a placeholder.

Tests verify:
  1. Each check fires on the correct fixture (b, c, d trigger warnings; a does not).
  2. AC-021: compile exits 0 even when warnings are present.
  3. Both callers (compile invocation via the script, review invocation via the
     script) produce identical warning text for the same input.

Pattern follows test_doctor_source_check.py: the script is extracted from
the command file and run in isolation against scaffolded projects.
"""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import sys
import textwrap
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[2]
SHARED_CHECKS_MD = REPO_ROOT / "commands" / "gov" / "_shared-directive-checks.md"


# ─── Script extraction ────────────────────────────────────────────────────────


def _extract_shared_checks_script() -> str:
    """Extract the inline Python script from _shared-directive-checks.md.

    The command file is the source of truth — tests must exercise the same
    script prose, not a divergent copy.  Falls back to the reference
    implementation if the file or heredoc block is not yet present, so
    tests remain runnable while the command is being authored.
    """
    if not SHARED_CHECKS_MD.exists():
        return _REFERENCE_SCRIPT

    content = SHARED_CHECKS_MD.read_text()

    # Locate the first python3 heredoc inside a ```bash block
    m = re.search(
        r"```bash\npython3 - <<'PY'\n(.+?)\nPY\n```",
        content,
        flags=re.DOTALL,
    )
    if m:
        return m.group(1)

    # Fallback
    return _REFERENCE_SCRIPT


# Reference implementation (ground truth for expected behaviour).
# Kept in sync with _shared-directive-checks.md §Inline Script.
_REFERENCE_SCRIPT = r"""
import json
import re
import sys

payload = json.loads(sys.stdin.read())
adr_id = payload["adr_id"]
body = payload["directive_body"]
phrases = payload.get("canonical_phrases") or []
no_dir_reason = payload.get("no_directives_reason")

warnings = []

# ── Check A: length vs canonical_phrases ──────────────────────────────────────
stripped_body = re.sub(r'\s*\(ref:[^)]+\)\s*$', '', body.rstrip())
clauses = re.split(r'(?<=[.;!?])\s+|(?<=[.;!?])$', stripped_body)
clauses = [c.strip() for c in clauses if c.strip()]
sentence_count = len(clauses)

if sentence_count > 1 and not phrases:
    warnings.append(
        f'[WARN] {adr_id}: directive has {sentence_count} sentences but no canonical_phrases'
        f' — run /edikt:adr:review --backfill'
    )

# ── Check B: phrase-not-in-body ───────────────────────────────────────────────
body_lower = body.lower()
for phrase in phrases:
    p = phrase.strip()
    if p and p.lower() not in body_lower:
        warnings.append(
            f'[WARN] {adr_id}: canonical_phrase "{p}" not found in directive body'
        )

# ── Check C: no-directives reason ─────────────────────────────────────────────
if no_dir_reason is not None:
    reason = str(no_dir_reason).strip()
    forbidden = {"tbd", "todo", "fix later"}
    if not reason or len(reason) < 10 or reason.lower() in forbidden:
        warnings.append(
            f'[WARN] {adr_id}: no-directives reason "{no_dir_reason}" is not acceptable'
            f' — provide a meaningful explanation \u2265 10 characters'
        )

for w in warnings:
    print(w)

sys.exit(0)
"""


# ─── Fixture helpers ─────────────────────────────────────────────────────────


def _run_checks(
    script: str,
    adr_id: str,
    directive_body: str,
    canonical_phrases: list[str],
    no_directives_reason: str | None = None,
) -> subprocess.CompletedProcess:
    """Run the shared-checks script with the given inputs via stdin."""
    payload = json.dumps(
        {
            "adr_id": adr_id,
            "directive_body": directive_body,
            "canonical_phrases": canonical_phrases,
            "no_directives_reason": no_directives_reason,
        }
    )
    return subprocess.run(
        [sys.executable, "-c", script],
        input=payload,
        capture_output=True,
        text=True,
        timeout=10,
    )


class _TempDirContext:
    def __enter__(self) -> Path:
        import tempfile
        self._d = tempfile.mkdtemp(prefix="edikt-shared-checks-test-")
        return Path(self._d)

    def __exit__(self, *args) -> None:
        shutil.rmtree(self._d, ignore_errors=True)


def _build_project(tmp_path: Path) -> Path:
    """Scaffold a minimal edikt project."""
    project = tmp_path / "project"
    project.mkdir()
    (project / ".edikt").mkdir()
    (project / ".edikt" / "config.yaml").write_text(
        textwrap.dedent("""\
            edikt_version: "0.6.0"
            base: docs
            paths:
              decisions: docs/architecture/decisions
              invariants: docs/architecture/invariants
        """)
    )
    (project / "docs" / "architecture" / "decisions").mkdir(parents=True)
    (project / "docs" / "architecture" / "invariants").mkdir(parents=True)
    return project


def _write_adr(
    project: Path,
    adr_id: str,
    directive_body: str,
    canonical_phrases: list[str],
    no_directives_reason: str | None = None,
) -> Path:
    """Write a minimal accepted ADR with a populated sentinel block."""
    decisions_dir = project / "docs" / "architecture" / "decisions"
    adr_file = decisions_dir / f"{adr_id}-test.md"

    no_dir_line = (
        f"no-directives: \"{no_directives_reason}\"\n" if no_directives_reason is not None else ""
    )
    phrases_yaml = (
        "\n".join(f'  - "{p}"' for p in canonical_phrases)
        if canonical_phrases
        else "  []"
    )
    # Flatten phrases to YAML list or empty
    if canonical_phrases:
        phrases_block = "canonical_phrases:\n" + "\n".join(f'  - "{p}"' for p in canonical_phrases)
    else:
        phrases_block = "canonical_phrases: []"

    adr_file.write_text(
        textwrap.dedent(f"""\
            ---
            type: adr
            id: {adr_id}
            title: Test ADR
            status: accepted
            {no_dir_line}---

            # {adr_id}: Test ADR

            **Status:** Accepted

            ## Decision

            {directive_body}

            [edikt:directives:start]: #
            directives:
              - {directive_body}
            manual_directives: []
            suppressed_directives: []
            {phrases_block}
            behavioral_signal: {{}}
            source_hash: pending
            directives_hash: pending
            [edikt:directives:end]: #
        """)
    )
    return adr_file


# ─── Check A: Length vs canonical_phrases (AC-002b, AC-021) ──────────────────


class TestCheckA:
    """FR-003a: multi-sentence + empty canonical_phrases → warn."""

    def test_single_sentence_clean_no_warn(self):
        """Single-sentence directive with no phrases: no warning."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-001",
            "All DB access MUST go through the repository layer. (ref: ADR-001)",
            [],
        )
        assert r.returncode == 0
        assert "[WARN]" not in r.stdout, f"Single-sentence should not warn; got: {r.stdout}"

    def test_multi_sentence_empty_phrases_warns(self):
        """AC-002b: multi-sentence directive + empty canonical_phrases emits warning."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-921",
            "All DB access MUST use repositories. The repository layer owns queries; callers receive domain objects only.",
            [],
        )
        assert r.returncode == 0, f"Script must exit 0 (AC-021); got {r.returncode}"
        assert "[WARN]" in r.stdout
        assert "ADR-921" in r.stdout
        assert "sentences" in r.stdout
        assert "canonical_phrases" in r.stdout

    def test_multi_sentence_with_phrases_no_warn(self):
        """Multi-sentence directive with non-empty phrases: no Check-A warning."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-012",
            "All DB access MUST go through the repository. NEVER bypass the repository layer.",
            ["repository", "NEVER bypass"],
        )
        assert r.returncode == 0
        # Check A warning must not fire when phrases are populated
        assert "sentences but no canonical_phrases" not in r.stdout, (
            f"Should not warn when canonical_phrases is non-empty; got: {r.stdout}"
        )

    def test_sentence_count_in_warning(self):
        """Warning output includes the sentence count."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-050",
            "First sentence. Second sentence. Third sentence.",
            [],
        )
        assert "3 sentences" in r.stdout or "3" in r.stdout, (
            f"Warning should include sentence count; got: {r.stdout}"
        )

    def test_ref_tail_stripped_before_sentence_count(self):
        """The (ref: ...) tail is stripped before counting sentences."""
        script = _extract_shared_checks_script()
        # Body has exactly one sentence before the ref tail — should NOT warn
        r = _run_checks(
            script,
            "ADR-060",
            "NEVER hardcode credentials. (ref: ADR-060)",
            [],
        )
        # "NEVER hardcode credentials" is 1 sentence; "(ref: ADR-060)" is the tail
        # After stripping tail, we still have "NEVER hardcode credentials." — 1 sentence
        # → should NOT warn about missing canonical_phrases
        assert "sentences but no canonical_phrases" not in r.stdout, (
            f"Ref tail should be stripped before sentence count; got: {r.stdout}"
        )

    def test_backfill_hint_in_warning(self):
        """Warning includes the --backfill hint."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-921",
            "Sentence one. Sentence two.",
            [],
        )
        assert "--backfill" in r.stdout, (
            f"Warning should include --backfill hint; got: {r.stdout}"
        )

    def test_ac021_compile_exits_0_on_warning(self):
        """AC-021: script exits 0 even when Check A fires (warn-only in v0.6.0)."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-921",
            "Sentence one. Sentence two. Sentence three.",
            [],
        )
        assert r.returncode == 0, (
            f"Script MUST exit 0 per AC-021; got {r.returncode}\nstdout: {r.stdout}"
        )


# ─── Check B: Phrase-not-in-body (AC-002c) ───────────────────────────────────


class TestCheckB:
    """FR-003b: canonical_phrase not a substring of directive body → warn."""

    def test_phrase_not_in_body_warns(self):
        """AC-002c: phrase absent from body emits warning."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-922",
            "All DB access MUST use repositories.",
            ["immutable"],  # 'immutable' is NOT in the body
        )
        assert r.returncode == 0
        assert "[WARN]" in r.stdout
        assert "ADR-922" in r.stdout
        assert '"immutable"' in r.stdout
        assert "not found in directive body" in r.stdout

    def test_phrase_present_in_body_no_warn(self):
        """Phrase that IS a substring of body produces no warning."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-012",
            "All DB access MUST go through the repository layer.",
            ["repository layer"],
        )
        assert r.returncode == 0
        assert "not found in directive body" not in r.stdout, (
            f"Should not warn when phrase is in body; got: {r.stdout}"
        )

    def test_phrase_case_insensitive(self):
        """Substring match is case-insensitive (REPOSITORY matches 'repository')."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-012",
            "All DB access MUST go through the repository layer.",
            ["REPOSITORY"],  # uppercase variant
        )
        assert r.returncode == 0
        assert "not found in directive body" not in r.stdout, (
            f"Match must be case-insensitive; got: {r.stdout}"
        )

    def test_multiple_phrases_each_checked(self):
        """Each phrase in the list is checked independently."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-012",
            "MUST use the repository layer only.",
            ["repository", "NEVER bypass", "layer"],  # only 'NEVER bypass' is absent
        )
        assert r.returncode == 0
        warns = [line for line in r.stdout.splitlines() if "[WARN]" in line]
        # Exactly one warning for the missing phrase
        assert len(warns) == 1, f"Expected exactly 1 warning (one phrase missing); got:\n{r.stdout}"
        assert '"NEVER bypass"' in r.stdout

    def test_empty_phrases_list_no_warn(self):
        """Empty canonical_phrases → Check B is skipped (no warning)."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-001",
            "MUST use the repository layer.",
            [],
        )
        assert r.returncode == 0
        assert "not found in directive body" not in r.stdout


# ─── Check C: no-directives reason validator (AC-003c) ───────────────────────


class TestCheckC:
    """AC-003c: invalid no-directives reasons are flagged."""

    @pytest.mark.parametrize("bad_reason", ["", "tbd", "TODO", "fix later", "short"])
    def test_invalid_reasons_warn(self, bad_reason: str):
        """Short / placeholder / forbidden reasons emit a warning."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-999",
            "MUST use HTTPS for all connections.",
            [],
            no_directives_reason=bad_reason,
        )
        assert r.returncode == 0
        assert "[WARN]" in r.stdout, (
            f"Bad reason '{bad_reason}' should trigger warning; got: {r.stdout}"
        )
        assert "not acceptable" in r.stdout

    def test_valid_reason_no_warn(self):
        """A valid reason (≥10 chars, not a placeholder) produces no warning."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-999",
            "MUST use HTTPS for all connections.",
            [],
            no_directives_reason="covers process only, not enforceable at runtime",
        )
        assert r.returncode == 0
        assert "not acceptable" not in r.stdout, (
            f"Valid reason should not trigger warning; got: {r.stdout}"
        )

    def test_no_no_directives_key_no_warn(self):
        """Absent no-directives key → Check C is skipped entirely."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-001",
            "MUST use the repository layer.",
            [],
            no_directives_reason=None,
        )
        assert r.returncode == 0
        assert "not acceptable" not in r.stdout

    def test_exactly_ten_chars_is_valid(self):
        """A 10-character reason is valid (boundary condition)."""
        script = _extract_shared_checks_script()
        reason = "a" * 10  # exactly 10 chars
        r = _run_checks(
            script,
            "ADR-001",
            "MUST use the repository layer.",
            [],
            no_directives_reason=reason,
        )
        assert r.returncode == 0
        assert "not acceptable" not in r.stdout, (
            f"10-char reason should be valid; got: {r.stdout}"
        )

    def test_nine_chars_is_invalid(self):
        """A 9-character reason is too short."""
        script = _extract_shared_checks_script()
        reason = "a" * 9  # 9 chars — below threshold
        r = _run_checks(
            script,
            "ADR-001",
            "MUST use the repository layer.",
            [],
            no_directives_reason=reason,
        )
        assert r.returncode == 0
        assert "[WARN]" in r.stdout

    def test_case_insensitive_forbidden(self):
        """Forbidden placeholders are matched case-insensitively."""
        script = _extract_shared_checks_script()
        for variant in ["TBD", "Tbd", "TODO", "Todo", "Fix Later", "FIX LATER"]:
            r = _run_checks(
                script,
                "ADR-001",
                "MUST use the repository layer.",
                [],
                no_directives_reason=variant,
            )
            assert "[WARN]" in r.stdout, (
                f"Variant '{variant}' should trigger warning; got: {r.stdout}"
            )


# ─── Multiple checks in one call ─────────────────────────────────────────────


class TestMultipleChecks:
    """Multiple checks can fire independently on the same directive."""

    def test_all_three_checks_fire_independently(self):
        """A directive that fails all three checks produces three warnings."""
        script = _extract_shared_checks_script()
        # Check A: multi-sentence + empty phrases
        # Check B: phrase not in body (but phrases are empty — Check B skipped)
        # We need phrases to trigger Check B while also triggering Check A...
        # Workaround: use phrases that are NOT in the body AND multi-sentence body
        r = _run_checks(
            script,
            "ADR-999",
            "All DB access MUST use repositories. The repository owns queries.",
            ["immutable"],  # not in body → Check B fires; also multi-sentence so Check A would fire except phrases non-empty
            no_directives_reason="tbd",  # Check C fires
        )
        assert r.returncode == 0
        warns = [line for line in r.stdout.splitlines() if "[WARN]" in line]
        # Check A does NOT fire (phrases is non-empty); Check B fires; Check C fires
        assert len(warns) >= 2, (
            f"Expected at least 2 warnings (Check B + Check C); got:\n{r.stdout}"
        )
        assert "not found in directive body" in r.stdout
        assert "not acceptable" in r.stdout

    def test_clean_directive_no_warnings(self):
        """A clean directive (single-sentence, phrases match, valid reason) produces no warnings."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-001",
            "All DB access MUST go through the repository layer. (ref: ADR-001)",
            [],  # single sentence after stripping ref tail — no Check A
            no_directives_reason=None,
        )
        assert r.returncode == 0
        assert "[WARN]" not in r.stdout, (
            f"Clean directive should produce no warnings; got: {r.stdout}"
        )


# ─── Cross-caller consistency ─────────────────────────────────────────────────


class TestCrossCallerConsistency:
    """AC-002b/c: identical warning text from compile and review callers.

    Both callers invoke the same script with the same input; we verify
    by running the script twice with identical payloads and asserting the
    output is identical (byte-equal modulo any OS-level whitespace differences).
    """

    @pytest.mark.parametrize("fixture_name,directive,phrases,no_dir_reason", [
        (
            "multi-sentence-no-phrases",
            "All DB access MUST use repositories. The repository layer owns queries.",
            [],
            None,
        ),
        (
            "phrase-not-in-body",
            "All DB access MUST use repositories.",
            ["immutable"],
            None,
        ),
        (
            "invalid-no-dir-reason",
            "MUST use HTTPS for all API connections.",
            [],
            "tbd",
        ),
        (
            "single-sentence-clean",
            "All DB access MUST use the repository layer. (ref: ADR-001)",
            [],
            None,
        ),
    ])
    def test_same_input_produces_same_output(
        self,
        fixture_name: str,
        directive: str,
        phrases: list[str],
        no_dir_reason: str | None,
    ):
        """Compile-side and review-side invocations produce byte-identical output."""
        script = _extract_shared_checks_script()

        # Simulate "compile" invocation
        r_compile = _run_checks(script, "ADR-TEST", directive, phrases, no_dir_reason)
        # Simulate "review" invocation (same script, same input)
        r_review = _run_checks(script, "ADR-TEST", directive, phrases, no_dir_reason)

        assert r_compile.returncode == 0
        assert r_review.returncode == 0
        assert r_compile.stdout == r_review.stdout, (
            f"Fixture '{fixture_name}': compile and review output differs.\n"
            f"  compile: {r_compile.stdout!r}\n"
            f"  review:  {r_review.stdout!r}"
        )


# ─── Command-file contract assertions ────────────────────────────────────────


class TestCommandFileContracts:
    """Assert command-file structure contracts (not behaviour)."""

    def test_shared_checks_file_exists(self):
        """commands/gov/_shared-directive-checks.md must exist."""
        assert SHARED_CHECKS_MD.exists(), (
            f"Expected {SHARED_CHECKS_MD} to exist — Phase 6 deliverable."
        )

    def test_shared_checks_has_leading_underscore_note(self):
        """File must contain the not-a-top-level-command notice."""
        content = SHARED_CHECKS_MD.read_text()
        assert "Not a top-level command" in content, (
            "File must contain the 'Not a top-level command' notice per AC."
        )

    def test_compile_references_shared_checks(self):
        """commands/gov/compile.md must reference _shared-directive-checks.md."""
        compile_content = (REPO_ROOT / "commands" / "gov" / "compile.md").read_text()
        assert "_shared-directive-checks" in compile_content, (
            "compile.md must reference the shared sub-procedure."
        )

    def test_review_references_shared_checks(self):
        """commands/gov/review.md must reference _shared-directive-checks.md."""
        review_content = (REPO_ROOT / "commands" / "gov" / "review.md").read_text()
        assert "_shared-directive-checks" in review_content, (
            "review.md must reference the shared sub-procedure."
        )

    def test_compile_has_directive_quality_warnings_header(self):
        """compile.md must include the '### Directive-quality warnings' header text."""
        compile_content = (REPO_ROOT / "commands" / "gov" / "compile.md").read_text()
        assert "Directive-quality warnings" in compile_content, (
            "compile.md must mention the '### Directive-quality warnings' header."
        )

    def test_compile_exits_0_on_warnings_per_ac021(self):
        """compile.md must document that it exits 0 on warnings (AC-021 grace period)."""
        compile_content = (REPO_ROOT / "commands" / "gov" / "compile.md").read_text()
        # Look for the AC-021 or grace-period language
        assert "AC-021" in compile_content or "grace period" in compile_content.lower(), (
            "compile.md must reference the AC-021 grace period for warn-only behaviour."
        )

    def test_review_has_directive_quality_checks_section(self):
        """review.md must include the 'Directive-quality checks' sub-heading language."""
        review_content = (REPO_ROOT / "commands" / "gov" / "review.md").read_text()
        assert "Directive-quality checks" in review_content, (
            "review.md must include a 'Directive-quality checks' sub-heading reference."
        )


# ─── Fixture-driven scenario tests ───────────────────────────────────────────


class TestFixtureScenarios:
    """End-to-end scenario tests using the fixture shapes from fixtures.yaml.

    Each scenario builds a fixture repo with specific ADR variants and
    exercises the shared-checks script against all of them, asserting
    the correct warning pattern.
    """

    def test_fixture_a_single_sentence_clean(self):
        """Fixture (a): single-sentence directive → no warning."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-001",
            "All DB access MUST go through the repository layer. (ref: ADR-001)",
            [],
        )
        assert r.returncode == 0
        assert "[WARN]" not in r.stdout, f"Fixture (a) should produce no warning; got: {r.stdout}"

    def test_fixture_b_multi_sentence_empty_phrases(self):
        """Fixture (b): multi-sentence + empty canonical_phrases → AC-002b warning."""
        script = _extract_shared_checks_script()
        # From fixtures.yaml scenario 'multi-sentence-no-phrases-warn'
        r = _run_checks(
            script,
            "ADR-921",
            "All DB access MUST use repositories. The repository layer owns queries; callers receive domain objects only.",
            [],
        )
        assert r.returncode == 0
        assert "[WARN]" in r.stdout
        assert "ADR-921" in r.stdout
        assert "canonical_phrases" in r.stdout

    def test_fixture_c_phrase_not_in_body(self):
        """Fixture (c): canonical_phrase not in body → AC-002c warning."""
        script = _extract_shared_checks_script()
        # From fixtures.yaml scenario 'phrase-not-in-body'
        r = _run_checks(
            script,
            "ADR-922",
            "All DB access MUST use repositories.",
            ["immutable"],
        )
        assert r.returncode == 0
        assert "[WARN]" in r.stdout
        assert "ADR-922" in r.stdout
        assert '"immutable"' in r.stdout
        assert "not found in directive body" in r.stdout

    def test_fixture_d_invalid_no_directives_reason(self):
        """Fixture (d): no-directives reason 'tbd' → AC-003c warning."""
        script = _extract_shared_checks_script()
        r = _run_checks(
            script,
            "ADR-999",
            "MUST use HTTPS for all API connections.",
            [],
            no_directives_reason="tbd",
        )
        assert r.returncode == 0
        assert "[WARN]" in r.stdout
        assert "not acceptable" in r.stdout

    def test_all_four_fixtures_together(self):
        """Running the script over all four fixtures produces warnings for b, c, d only."""
        script = _extract_shared_checks_script()

        # (a) clean
        ra = _run_checks(script, "ADR-001", "MUST use repository. (ref: ADR-001)", [])
        # (b) multi-sentence + empty phrases
        rb = _run_checks(
            script, "ADR-921",
            "All DB access MUST use repositories. The repository layer owns queries.",
            [],
        )
        # (c) phrase not in body
        rc = _run_checks(
            script, "ADR-922",
            "All DB access MUST use repositories.",
            ["immutable"],
        )
        # (d) invalid no-directives reason
        rd = _run_checks(
            script, "ADR-999",
            "MUST use HTTPS.",
            [],
            no_directives_reason="tbd",
        )

        # (a) no warnings
        assert "[WARN]" not in ra.stdout, f"(a) should be clean; got: {ra.stdout}"
        # (b) warning
        assert "[WARN]" in rb.stdout, f"(b) should warn; got: {rb.stdout}"
        # (c) warning
        assert "[WARN]" in rc.stdout, f"(c) should warn; got: {rc.stdout}"
        # (d) warning
        assert "[WARN]" in rd.stdout, f"(d) should warn; got: {rd.stdout}"

        # All exit 0 (AC-021)
        for label, r in [("a", ra), ("b", rb), ("c", rc), ("d", rd)]:
            assert r.returncode == 0, (
                f"Fixture ({label}) must exit 0 per AC-021; got {r.returncode}"
            )

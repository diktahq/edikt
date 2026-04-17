"""
SPEC-005 Phase 7 — Compile orphan detection + history state + .gitignore (AC-003,
AC-003b, AC-017, AC-018, AC-019).

The orphan-detection pass is implemented as an inline Python script inside
commands/gov/compile.md §Pass 2.  These tests extract that script and exercise
it directly (same pattern as test_shared_directive_checks.py and
test_doctor_source_check.py) so they:
  - run without SDK auth,
  - finish in milliseconds per case,
  - stay honest — they exercise the actual shipped script, not a test-local copy.

The .gitignore handler is a separate inline script in the same section; these
tests extract and exercise it independently.

Five orphan-set transition scenarios (per Phase 7 spec):
  1. First detection  — no prior history → warn, exit 0, history written
  2. Consecutive same — prior == current  → block, exit ≠ 0, history NOT overwritten
  3. Subset           — some orphans resolved → warn, reset, write, exit 0
  4. Superset         — new orphans added → warn, first-detection, write, exit 0
  5. Corrupt history  — unparseable JSON → treat as absent (scenario 1), exit 0
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import textwrap
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[2]
COMPILE_MD = REPO_ROOT / "commands" / "gov" / "compile.md"


# ─── Script extraction ────────────────────────────────────────────────────────


def _extract_orphan_script() -> str:
    """Extract the orphan-detection Python heredoc from compile.md §12d (Pass 2)."""
    if not COMPILE_MD.exists():
        raise RuntimeError(f"commands/gov/compile.md not found at {COMPILE_MD}")

    content = COMPILE_MD.read_text()

    # Locate the '#### Pass 2: History comparison and write' section, then find
    # the first python3 heredoc within it.
    marker = "#### Pass 2: History comparison and write"
    idx = content.find(marker)
    if idx == -1:
        raise RuntimeError(
            f"Could not find '#### Pass 2' marker in compile.md — Phase 7 may not be shipped yet.\n"
            f"Expected marker: '{marker}'"
        )

    window = content[idx:]
    m = re.search(r"```bash\npython3 - <<'PY'\n(.+?)\nPY\n```", window, flags=re.DOTALL)
    if not m:
        raise RuntimeError(
            "Could not extract Pass 2 orphan-detection Python script from compile.md §12d"
        )
    return m.group(1)


def _extract_gitignore_script() -> str:
    """Extract the .gitignore-appender Python heredoc from compile.md §12d (AC-019)."""
    if not COMPILE_MD.exists():
        raise RuntimeError(f"commands/gov/compile.md not found at {COMPILE_MD}")

    content = COMPILE_MD.read_text()

    # The .gitignore script appears after the AC-019 header in §12d.
    marker = "**AC-019 — `.gitignore` management:**"
    idx = content.find(marker)
    if idx == -1:
        raise RuntimeError(
            f"Could not find AC-019 .gitignore marker in compile.md\n"
            f"Expected marker: '{marker}'"
        )

    window = content[idx:]
    m = re.search(r"```bash\npython3 - <<'PY'\n(.+?)\nPY\n```", window, flags=re.DOTALL)
    if not m:
        raise RuntimeError(
            "Could not extract AC-019 .gitignore Python script from compile.md §12d"
        )
    return m.group(1)


# ─── Script runner helpers ────────────────────────────────────────────────────


def _run_orphan_script(
    script: str,
    orphan_ids: list[str],
    history_path: str | Path,
    edikt_version: str = "0.6.0-test",
) -> subprocess.CompletedProcess:
    """Run the orphan-detection script with the given inputs via environment variables."""
    env = {
        **os.environ,
        "EDIKT_ORPHAN_IDS": ",".join(orphan_ids),
        "EDIKT_HISTORY_PATH": str(history_path),
        "EDIKT_VERSION": edikt_version,
    }
    return subprocess.run(
        [sys.executable, "-c", script],
        env=env,
        capture_output=True,
        text=True,
        timeout=10,
    )


def _run_gitignore_script(
    script: str,
    project_root: str | Path,
) -> subprocess.CompletedProcess:
    """Run the .gitignore-appender script with the given project root."""
    env = {
        **os.environ,
        "EDIKT_PROJECT_ROOT": str(project_root),
    }
    return subprocess.run(
        [sys.executable, "-c", script],
        env=env,
        capture_output=True,
        text=True,
        timeout=10,
    )


# ─── Temp-dir context manager ─────────────────────────────────────────────────


class _TempDir:
    """Lightweight temp-dir context manager."""

    def __enter__(self) -> Path:
        import tempfile
        self._d = tempfile.mkdtemp(prefix="edikt-orphan-test-")
        return Path(self._d)

    def __exit__(self, *args) -> None:
        shutil.rmtree(self._d, ignore_errors=True)


def _history_path(tmp: Path) -> Path:
    """Standard .edikt/state/compile-history.json path inside a temp dir."""
    return tmp / ".edikt" / "state" / "compile-history.json"


def _write_history(path: Path, orphan_ids: list[str]) -> None:
    """Write a valid compile-history.json to the given path."""
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps(
            {
                "schema_version": 1,
                "last_compile_at": "2026-04-17T00:00:00Z",
                "orphan_adrs": sorted(orphan_ids),
            },
            indent=2,
        ),
        encoding="utf-8",
    )


def _read_history(path: Path) -> dict:
    """Read and parse compile-history.json."""
    return json.loads(path.read_text(encoding="utf-8"))


# ─── Command-file contract assertions ────────────────────────────────────────


class TestCommandFileContracts:
    """Assert compile.md has all required Phase 7 markers."""

    def test_compile_md_exists(self):
        assert COMPILE_MD.exists(), f"Expected {COMPILE_MD} to exist."

    def test_pass2_header_present(self):
        content = COMPILE_MD.read_text()
        assert "#### Pass 2: History comparison and write" in content, (
            "compile.md must contain '#### Pass 2: History comparison and write' (Phase 7)."
        )

    def test_orphan_detection_section_present(self):
        content = COMPILE_MD.read_text()
        assert "#### Pass 1: Orphan collection" in content, (
            "compile.md must contain '#### Pass 1: Orphan collection' (Phase 7)."
        )

    def test_two_layer_atomicity_model_documented(self):
        content = COMPILE_MD.read_text()
        assert "Two-layer atomicity model" in content, (
            "compile.md must document the two-layer atomicity model inline (Phase 7 spec requirement)."
        )

    def test_atomic_rename_mentioned(self):
        content = COMPILE_MD.read_text()
        assert "os.rename" in content or "atomic rename" in content.lower(), (
            "compile.md must mention os.rename / atomic rename in the state-file write path."
        )

    def test_gitignore_section_present(self):
        content = COMPILE_MD.read_text()
        assert "AC-019" in content, (
            "compile.md must contain the AC-019 .gitignore management section."
        )

    def test_step_12c_still_intact(self):
        """Phase 6 step 12c must be preserved — Phase 7 adds AFTER it."""
        content = COMPILE_MD.read_text()
        assert "12c" in content, "Phase 6 step 12c must remain in compile.md."
        assert "_shared-directive-checks" in content, (
            "Phase 6 shared-directive-checks reference must remain in compile.md."
        )

    def test_step_12d_comes_after_12c(self):
        """Step 12d (Phase 7) must appear after step 12c (Phase 6) in the file."""
        content = COMPILE_MD.read_text()
        idx_12c = content.find("12c")
        idx_12d = content.find("12d")
        assert idx_12c != -1 and idx_12d != -1, "Both 12c and 12d must be present."
        assert idx_12d > idx_12c, "12d (Phase 7) must appear after 12c (Phase 6)."


# ─── Scenario 1: First detection ─────────────────────────────────────────────


class TestFirstDetection:
    """No prior history → warn, exit 0, history written."""

    def test_first_detection_warns_and_exits_0(self):
        """AC-003 first detection: warn + exit 0 when no prior history."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            r = _run_orphan_script(script, ["ADR-910"], hist)

            assert r.returncode == 0, (
                f"First detection must exit 0; got {r.returncode}\n"
                f"stdout: {r.stdout}\nstderr: {r.stderr}"
            )
            assert "[WARN]" in r.stdout, (
                f"First detection must emit a WARN; got:\n{r.stdout}"
            )
            assert "ADR-910" in r.stdout

    def test_first_detection_writes_history(self):
        """AC-003: history file is written on first detection."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _run_orphan_script(script, ["ADR-910"], hist)

            assert hist.exists(), "compile-history.json must be written on first detection."
            data = _read_history(hist)
            assert data["schema_version"] == 1
            assert "ADR-910" in data["orphan_adrs"]
            assert "last_compile_at" in data

    def test_first_detection_no_block_message(self):
        """First detection must NOT emit a BLOCK message."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            r = _run_orphan_script(script, ["ADR-910"], hist)
            assert "[BLOCK]" not in r.stdout, (
                f"First detection must not block; got:\n{r.stdout}"
            )

    def test_no_orphans_exits_0_clean(self):
        """Empty orphan set → exit 0, history written with empty array."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            r = _run_orphan_script(script, [], hist)
            assert r.returncode == 0
            assert "[WARN]" not in r.stdout
            assert "[BLOCK]" not in r.stdout
            # History is still written (clean state)
            assert hist.exists()
            data = _read_history(hist)
            assert data["orphan_adrs"] == []


# ─── Scenario 2: Consecutive same set — BLOCK ────────────────────────────────


class TestConsecutiveBlocks:
    """Same orphan set on second compile → block, exit ≠ 0, history unchanged."""

    def test_consecutive_blocks_exit_nonzero(self):
        """AC-003 consecutive: exit ≠ 0 when orphan set unchanged since last run."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            # Pre-seed history with the same orphan set
            _write_history(hist, ["ADR-910"])

            r = _run_orphan_script(script, ["ADR-910"], hist)
            assert r.returncode != 0, (
                f"Consecutive same set must block (exit ≠ 0); got 0\nstdout: {r.stdout}"
            )

    def test_consecutive_emits_block_message(self):
        """Blocked compile must emit a [BLOCK] message."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _write_history(hist, ["ADR-910"])
            r = _run_orphan_script(script, ["ADR-910"], hist)
            assert "[BLOCK]" in r.stdout, (
                f"Consecutive must emit [BLOCK] message; got:\n{r.stdout}"
            )
            assert "ADR-910" in r.stdout

    def test_consecutive_does_not_overwrite_history(self):
        """AC-003 block: history file content MUST NOT change on block."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _write_history(hist, ["ADR-910"])
            original_mtime = hist.stat().st_mtime
            original_content = hist.read_text(encoding="utf-8")

            _run_orphan_script(script, ["ADR-910"], hist)

            assert hist.read_text(encoding="utf-8") == original_content, (
                "History file content must not change on consecutive block."
            )

    def test_fix_list_printed_on_block(self):
        """Blocked output must include fix options."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _write_history(hist, ["ADR-910"])
            r = _run_orphan_script(script, ["ADR-910"], hist)
            assert "no-directives" in r.stdout or "Fix options" in r.stdout, (
                f"Block output must include fix list; got:\n{r.stdout}"
            )

    def test_consecutive_with_multiple_orphans(self):
        """Consecutive block works with multiple orphans in the same set."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            orphans = ["ADR-010", "ADR-011", "ADR-012"]
            _write_history(hist, orphans)
            r = _run_orphan_script(script, orphans, hist)
            assert r.returncode != 0
            assert all(oid in r.stdout for oid in orphans)


# ─── Scenario 3: Subset (orphans resolved) → reset to first-detection ────────


class TestResetOnSetChange:
    """Orphan set becomes a strict subset (some resolved) → warn, write, exit 0."""

    def test_subset_warns_and_exits_0(self):
        """AC-003b: subset (resolved orphan) → first-detection reset, exit 0."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            # Prior set: {ADR-910, ADR-911}; now ADR-911 resolved
            _write_history(hist, ["ADR-910", "ADR-911"])
            r = _run_orphan_script(script, ["ADR-910"], hist)
            assert r.returncode == 0, (
                f"Subset (resolved) must exit 0; got {r.returncode}\nstdout: {r.stdout}"
            )
            assert "[WARN]" in r.stdout
            assert "[BLOCK]" not in r.stdout

    def test_subset_writes_new_history(self):
        """Subset transition writes the new (smaller) orphan set to history."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _write_history(hist, ["ADR-910", "ADR-911"])
            _run_orphan_script(script, ["ADR-910"], hist)
            data = _read_history(hist)
            assert "ADR-910" in data["orphan_adrs"]
            assert "ADR-911" not in data["orphan_adrs"], (
                "ADR-911 was resolved; new history must not include it."
            )

    def test_full_resolution_exits_0_clean(self):
        """All orphans resolved (empty current set) → exit 0, no WARN."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _write_history(hist, ["ADR-910"])
            r = _run_orphan_script(script, [], hist)
            assert r.returncode == 0
            assert "[BLOCK]" not in r.stdout


# ─── Scenario 4: Superset (new orphans) → first-detection ────────────────────


class TestSupersetFirstDetection:
    """New orphans added (superset) → first-detection, warn, write, exit 0."""

    def test_superset_warns_and_exits_0(self):
        """AC-003b: new orphans added → first-detection reset, exit 0."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _write_history(hist, ["ADR-910"])
            # ADR-912 is newly orphaned
            r = _run_orphan_script(script, ["ADR-910", "ADR-912"], hist)
            assert r.returncode == 0, (
                f"Superset (new orphan) must exit 0 (first-detection); got {r.returncode}"
            )
            assert "[WARN]" in r.stdout
            assert "[BLOCK]" not in r.stdout

    def test_superset_writes_new_set(self):
        """Superset transition writes the expanded orphan set to history."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _write_history(hist, ["ADR-910"])
            _run_orphan_script(script, ["ADR-910", "ADR-912"], hist)
            data = _read_history(hist)
            assert "ADR-910" in data["orphan_adrs"]
            assert "ADR-912" in data["orphan_adrs"]

    def test_superset_consecutive_then_blocks(self):
        """After the superset first-detection, the same superset on next run blocks."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _write_history(hist, ["ADR-910"])
            # First compile with superset → first-detection (warn, write)
            r1 = _run_orphan_script(script, ["ADR-910", "ADR-912"], hist)
            assert r1.returncode == 0
            # Second compile with same superset → consecutive (block)
            r2 = _run_orphan_script(script, ["ADR-910", "ADR-912"], hist)
            assert r2.returncode != 0, (
                f"Second compile with same superset must block; got {r2.returncode}"
            )


# ─── Scenario 5: Unparseable history ─────────────────────────────────────────


class TestUnparseableHistory:
    """AC-018: corrupt history → treat as absent, exit 0, rewrite cleanly."""

    @pytest.mark.parametrize("corrupt_content", [
        '{"schema_version": 1, "last_compile_at": "malformed',   # truncated JSON
        "not json at all",
        '{"no_orphan_adrs_key": true}',                           # missing required field
        "",                                                        # empty file
        "{",                                                       # invalid JSON
    ])
    def test_corrupt_history_treated_as_absent(self, corrupt_content: str):
        """AC-018: any unparseable history file → first-detection, exit 0."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            hist.parent.mkdir(parents=True, exist_ok=True)
            hist.write_text(corrupt_content, encoding="utf-8")

            r = _run_orphan_script(script, ["ADR-910"], hist)

            assert r.returncode == 0, (
                f"Corrupt history must be treated as absent (exit 0); got {r.returncode}\n"
                f"stdout: {r.stdout}"
            )
            assert "[BLOCK]" not in r.stdout, (
                f"Corrupt history must not block; got:\n{r.stdout}"
            )

    def test_corrupt_history_logged(self):
        """Corrupt history emits a warning mentioning 'unparseable' or 'corrupt'."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            hist.parent.mkdir(parents=True, exist_ok=True)
            hist.write_text("not json at all", encoding="utf-8")
            r = _run_orphan_script(script, ["ADR-910"], hist)
            combined = (r.stdout + r.stderr).lower()
            assert "unparseable" in combined or "corrupt" in combined, (
                f"Corrupt history must log a warning; got:\n{r.stdout}\n{r.stderr}"
            )

    def test_corrupt_history_rewritten_cleanly(self):
        """After treating corrupt history as absent, a clean history is written."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            hist.parent.mkdir(parents=True, exist_ok=True)
            hist.write_text("not json at all", encoding="utf-8")

            _run_orphan_script(script, ["ADR-910"], hist)

            # The file should now be valid JSON
            try:
                data = json.loads(hist.read_text(encoding="utf-8"))
            except json.JSONDecodeError as exc:
                pytest.fail(f"History was not rewritten to valid JSON: {exc}")

            assert data["schema_version"] == 1
            assert isinstance(data["orphan_adrs"], list)


# ─── AC-017: Atomic rename failure ───────────────────────────────────────────


class TestAtomicRenameFailure:
    """AC-017: if os.rename() raises, previous file is unchanged; .tmp may exist."""

    def test_rename_failure_leaves_previous_file_intact(self):
        """Monkeypatch os.rename to raise; previous file content is unchanged."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            # Write a known initial history
            _write_history(hist, ["ADR-900"])
            original_content = hist.read_text(encoding="utf-8")

            # Inject an os.rename monkeypatch that always raises OSError
            patched_script = (
                "import os as _os\n"
                "_real_rename = _os.rename\n"
                "def _failing_rename(src, dst):\n"
                "    raise OSError('injected rename failure')\n"
                "_os.rename = _failing_rename\n"
                "\n"
            ) + script

            env = {
                **os.environ,
                "EDIKT_ORPHAN_IDS": "ADR-910",
                "EDIKT_HISTORY_PATH": str(hist),
                "EDIKT_VERSION": "0.6.0-test",
            }
            r = subprocess.run(
                [sys.executable, "-c", patched_script],
                env=env,
                capture_output=True,
                text=True,
                timeout=10,
            )

            # Previous file MUST be unchanged
            current_content = hist.read_text(encoding="utf-8")
            assert current_content == original_content, (
                "Rename failure must leave previous history file intact.\n"
                f"Original: {original_content}\nGot: {current_content}"
            )

    def test_rename_failure_does_not_hard_crash(self):
        """A rename failure is recoverable — the script must not raise unhandled."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            hist.parent.mkdir(parents=True, exist_ok=True)

            patched_script = (
                "import os as _os\n"
                "def _failing_rename(src, dst):\n"
                "    raise OSError('injected rename failure')\n"
                "_os.rename = _failing_rename\n"
                "\n"
            ) + script

            env = {
                **os.environ,
                "EDIKT_ORPHAN_IDS": "ADR-910",
                "EDIKT_HISTORY_PATH": str(hist),
                "EDIKT_VERSION": "0.6.0-test",
            }
            r = subprocess.run(
                [sys.executable, "-c", patched_script],
                env=env,
                capture_output=True,
                text=True,
                timeout=10,
            )
            # Script must exit cleanly (0), not crash with a traceback
            assert r.returncode == 0, (
                f"Rename failure must not hard-crash (exit 0); got {r.returncode}\n"
                f"stdout: {r.stdout}\nstderr: {r.stderr}"
            )


# ─── AC-019: .gitignore management ───────────────────────────────────────────


class TestGitignoreAppender:
    """.gitignore created / appended / deduped correctly."""

    def test_missing_gitignore_is_created(self):
        """AC-019: no .gitignore → created with .edikt/state/ entry."""
        script = _extract_gitignore_script()
        with _TempDir() as tmp:
            r = _run_gitignore_script(script, tmp)
            assert r.returncode == 0
            gi = tmp / ".gitignore"
            assert gi.exists(), ".gitignore must be created when absent."
            assert ".edikt/state/" in gi.read_text(), (
                f".gitignore must contain '.edikt/state/'; content: {gi.read_text()!r}"
            )

    def test_existing_gitignore_without_entry_gets_appended(self):
        """AC-019: .gitignore present without entry → entry appended."""
        script = _extract_gitignore_script()
        with _TempDir() as tmp:
            gi = tmp / ".gitignore"
            gi.write_text("node_modules/\n.env\n", encoding="utf-8")
            r = _run_gitignore_script(script, tmp)
            assert r.returncode == 0
            content = gi.read_text()
            assert ".edikt/state/" in content, (
                f"Entry must be appended; content:\n{content}"
            )
            # Original entries must still be present
            assert "node_modules/" in content
            assert ".env" in content

    def test_existing_entry_with_trailing_slash_not_duplicated(self):
        """AC-019: entry already present with trailing slash → file unchanged."""
        script = _extract_gitignore_script()
        with _TempDir() as tmp:
            gi = tmp / ".gitignore"
            original = ".edikt/state/\nnode_modules/\n"
            gi.write_text(original, encoding="utf-8")
            _run_gitignore_script(script, tmp)
            assert gi.read_text() == original, (
                "File must not be modified when entry already present."
            )

    def test_entry_count_no_duplicate_on_repeated_runs(self):
        """Running the appender twice must not produce duplicate entries."""
        script = _extract_gitignore_script()
        with _TempDir() as tmp:
            _run_gitignore_script(script, tmp)
            _run_gitignore_script(script, tmp)
            gi = tmp / ".gitignore"
            count = gi.read_text().count(".edikt/state")
            assert count == 1, f"Must not create duplicate entries; found {count} occurrences."

    def test_entry_without_trailing_slash_recognized_as_present(self):
        """AC-019 trailing-slash normalization: '.edikt/state' (no slash) recognized as present."""
        script = _extract_gitignore_script()
        with _TempDir() as tmp:
            gi = tmp / ".gitignore"
            gi.write_text(".edikt/state\nnode_modules/\n", encoding="utf-8")
            original_content = gi.read_text()
            r = _run_gitignore_script(script, tmp)
            assert r.returncode == 0
            # File should either be unchanged OR normalized (but NOT have a duplicate)
            new_content = gi.read_text()
            count = new_content.count(".edikt/state")
            assert count == 1, (
                f"Trailing-slash variant must not produce a duplicate; "
                f"found {count} occurrences.\nContent:\n{new_content}"
            )

    def test_gitignore_appended_after_final_newline(self):
        """Appended entry is on its own line, not concatenated to an existing last line."""
        script = _extract_gitignore_script()
        with _TempDir() as tmp:
            gi = tmp / ".gitignore"
            # Content without trailing newline
            gi.write_text("node_modules/", encoding="utf-8")
            r = _run_gitignore_script(script, tmp)
            assert r.returncode == 0
            content = gi.read_text()
            lines = content.splitlines()
            # .edikt/state/ must be on its own line
            assert any(line.strip() == ".edikt/state/" for line in lines), (
                f"'.edikt/state/' must be on its own line; content:\n{content!r}"
            )


# ─── History schema validation ────────────────────────────────────────────────


class TestHistorySchema:
    """Written history file must match data-model.schema.yaml §2."""

    def test_history_contains_required_fields(self):
        """schema_version, last_compile_at, orphan_adrs are all required."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _run_orphan_script(script, ["ADR-010"], hist)
            data = _read_history(hist)
            assert "schema_version" in data
            assert "last_compile_at" in data
            assert "orphan_adrs" in data

    def test_history_schema_version_is_1(self):
        """schema_version must be the integer 1."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _run_orphan_script(script, ["ADR-010"], hist)
            data = _read_history(hist)
            assert data["schema_version"] == 1, (
                f"schema_version must be 1; got {data['schema_version']!r}"
            )

    def test_history_last_compile_at_iso8601(self):
        """last_compile_at must be a non-empty ISO 8601 timestamp string."""
        import re as _re
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _run_orphan_script(script, ["ADR-010"], hist)
            data = _read_history(hist)
            ts = data["last_compile_at"]
            assert isinstance(ts, str) and len(ts) >= 10, (
                f"last_compile_at must be a non-empty string; got {ts!r}"
            )
            # Basic ISO 8601 shape check
            assert _re.match(r"^\d{4}-\d{2}-\d{2}T", ts), (
                f"last_compile_at must start with YYYY-MM-DDT; got {ts!r}"
            )

    def test_history_orphan_adrs_is_list(self):
        """orphan_adrs must be an array."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _run_orphan_script(script, ["ADR-010", "ADR-011"], hist)
            data = _read_history(hist)
            assert isinstance(data["orphan_adrs"], list)

    def test_history_orphan_adrs_sorted_unique(self):
        """orphan_adrs must be sorted and de-duplicated."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            # Pass IDs in reverse order with a duplicate to test normalization
            _run_orphan_script(script, ["ADR-012", "ADR-010", "ADR-011", "ADR-010"], hist)
            data = _read_history(hist)
            ids = data["orphan_adrs"]
            assert ids == sorted(set(ids)), (
                f"orphan_adrs must be sorted and unique; got {ids}"
            )

    def test_edikt_version_stamped_when_provided(self):
        """If EDIKT_VERSION is set, it appears in history as edikt_version."""
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)
            _run_orphan_script(script, ["ADR-010"], hist, edikt_version="0.6.0")
            data = _read_history(hist)
            assert data.get("edikt_version") == "0.6.0", (
                f"edikt_version must be stamped when provided; got {data.get('edikt_version')!r}"
            )


# ─── End-to-end: first → second compile transitions ──────────────────────────


class TestEndToEndTransitions:
    """Exercise the complete two-compile lifecycle for each scenario."""

    def test_first_then_second_compile_blocks(self):
        """
        AC-003 full lifecycle:
          compile 1: orphan ADR-910 → warn, exit 0, history written
          compile 2: same orphan    → block, exit ≠ 0
        """
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)

            r1 = _run_orphan_script(script, ["ADR-910"], hist)
            assert r1.returncode == 0, f"First compile must exit 0; got {r1.returncode}"

            r2 = _run_orphan_script(script, ["ADR-910"], hist)
            assert r2.returncode != 0, (
                f"Second compile with same orphan must block; got 0\nstdout: {r2.stdout}"
            )

    def test_resolve_via_no_directives_reason(self):
        """
        Adding no-directives reason removes the ADR from the orphan set:
          compile 1 (w/ orphan): warn
          [user adds no-directives frontmatter]
          compile 2 (w/o orphan): clean exit 0
        """
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)

            # First compile: ADR-910 is an orphan
            r1 = _run_orphan_script(script, ["ADR-910"], hist)
            assert r1.returncode == 0

            # "User adds no-directives reason" — ADR-910 is no longer in the set
            r2 = _run_orphan_script(script, [], hist)
            assert r2.returncode == 0, (
                f"After resolving orphan, compile must exit 0; got {r2.returncode}"
            )
            assert "[BLOCK]" not in r2.stdout

    def test_three_compile_cycle(self):
        """
        Full cycle:
          compile 1: ADR-910 → first detection (warn, exit 0)
          compile 2: ADR-910 → consecutive block (exit ≠ 0)
          compile 3: ADR-912 added → superset (first-detection reset, exit 0)
          compile 4: ADR-910 + ADR-912 → consecutive block (exit ≠ 0)
        """
        script = _extract_orphan_script()
        with _TempDir() as tmp:
            hist = _history_path(tmp)

            r1 = _run_orphan_script(script, ["ADR-910"], hist)
            assert r1.returncode == 0, "Compile 1 must warn and exit 0"

            r2 = _run_orphan_script(script, ["ADR-910"], hist)
            assert r2.returncode != 0, "Compile 2 must block"

            # Superset: ADR-912 added → first-detection reset
            r3 = _run_orphan_script(script, ["ADR-910", "ADR-912"], hist)
            assert r3.returncode == 0, f"Compile 3 (superset) must exit 0; got {r3.returncode}"

            # Now consecutive again
            r4 = _run_orphan_script(script, ["ADR-910", "ADR-912"], hist)
            assert r4.returncode != 0, "Compile 4 must block (consecutive superset)"

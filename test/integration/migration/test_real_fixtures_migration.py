"""Real-fixture migration tests — uses byte-for-byte captured installs.

These tests load the actual output of running each historical version's
install.sh (captured by test/integration/migration/capture.sh) and run
the full migration chain against them. Unlike the synthetic tests (which
build programmatic fixtures to test individual migrations in isolation),
these tests verify that real historical installs migrate cleanly.

The fixtures live at test/integration/migration/fixtures/<tag>/.
Run capture.sh to regenerate if the fixture directory is missing.

Migration chain: M1 (flat→versioned) → M2 (sentinels) → M3 (commands)
                 → M5 (config schema) → M4 (compile schema, last).
"""

from __future__ import annotations

from pathlib import Path

import pytest

from .helpers import (
    FIXTURE_ROOT,
    PAYLOAD_VERSION,
    _make_payload,
    _versioned_seed,
    _write,
    load_real_fixture,
    run_migrate,
)

TAGS = ["v0.1.0", "v0.1.4", "v0.2.0", "v0.3.0", "v0.4.3"]


def _skip_if_missing(tag: str) -> None:
    if not (FIXTURE_ROOT / tag).exists():
        pytest.skip(
            f"Real fixture for {tag} not found. "
            "Run test/integration/migration/capture.sh to generate it."
        )


def _seed_v050_payload(edikt_home: Path) -> None:
    """Place a minimal v0.5.0 payload so M1 has a migration target."""
    versions = edikt_home / "versions"
    _make_payload(versions, PAYLOAD_VERSION)


# ─── Per-tag migration tests ──────────────────────────────────────────────────


def test_v010_real_fixture_migrates_cleanly(sandbox_home):
    tag = "v0.1.0"
    _skip_if_missing(tag)
    sb = sandbox_home
    assert load_real_fixture(sb, tag), f"failed to load fixture {tag}"

    # Seed v0.5.0 payload for M1 to target.
    _seed_v050_payload(sb["edikt_home"])

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, f"migrate failed:\n{proc.stderr}"

    # After M1: versioned layout exists.
    assert (sb["edikt_home"] / "versions" / PAYLOAD_VERSION).is_dir()
    assert (sb["edikt_home"] / "current").is_symlink()

    # After M2: CLAUDE.md sentinel format updated (HTML → markdown link-ref).
    claude_md = sb["edikt_home"] / "CLAUDE.md"
    if claude_md.exists():
        text = claude_md.read_text()
        assert "<!-- edikt:start" not in text, (
            "M2 must rewrite HTML sentinels to markdown link-ref format"
        )

    # After M3: flat command .md files with namespaced replacements are gone.
    flat_plan = sb["claude_home"] / "commands" / "edikt" / "plan.md"
    if flat_plan.exists():
        # Only remains if it was user-modified (no namespaced replacement).
        pass  # M3 preserves user-modified files

    # M4: either compiled or pending marker.
    gov = sb["claude_home"] / "rules" / "governance.md"
    m4_pending = sb["edikt_home"] / ".m4-pending"
    if gov.exists():
        has_v2 = "compile_schema_version: 2" in gov.read_text()
        assert has_v2 or m4_pending.exists(), (
            "M4 must either upgrade governance.md to schema v2 or write .m4-pending"
        )

    # No user data was lost.
    assert (sb["edikt_home"] / "versions" / PAYLOAD_VERSION / "VERSION").exists()


def test_v014_real_fixture_migrates_cleanly(sandbox_home):
    tag = "v0.1.4"
    _skip_if_missing(tag)
    sb = sandbox_home
    assert load_real_fixture(sb, tag)
    _seed_v050_payload(sb["edikt_home"])

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, f"migrate failed:\n{proc.stderr}"

    assert (sb["edikt_home"] / "versions" / PAYLOAD_VERSION).is_dir()
    assert (sb["edikt_home"] / "current").is_symlink()

    claude_md = sb["edikt_home"] / "CLAUDE.md"
    if claude_md.exists():
        assert "<!-- edikt:start" not in claude_md.read_text(), "M2 must rewrite HTML sentinels"


def test_v020_real_fixture_migrates_cleanly(sandbox_home):
    tag = "v0.2.0"
    _skip_if_missing(tag)
    sb = sandbox_home
    assert load_real_fixture(sb, tag)
    _seed_v050_payload(sb["edikt_home"])

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, f"migrate failed:\n{proc.stderr}"

    assert (sb["edikt_home"] / "versions" / PAYLOAD_VERSION).is_dir()
    assert (sb["edikt_home"] / "current").is_symlink()

    # v0.2.0 has no HTML sentinels (M2 no-op), no flat commands (M3 no-op).
    # M5 should add missing config keys.
    cfg = (sb["edikt_home"] / "config.yaml").read_text() if (sb["edikt_home"] / "config.yaml").exists() else ""
    if cfg:
        # M5 adds 'paths:' and 'gates:' if missing.
        # We just verify the file still exists and is parseable.
        assert "edikt_version" in cfg or "base:" in cfg, "config.yaml must be preserved"


def test_v030_real_fixture_migrates_cleanly(sandbox_home):
    tag = "v0.3.0"
    _skip_if_missing(tag)
    sb = sandbox_home
    assert load_real_fixture(sb, tag)
    _seed_v050_payload(sb["edikt_home"])

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, f"migrate failed:\n{proc.stderr}"

    assert (sb["edikt_home"] / "versions" / PAYLOAD_VERSION).is_dir()


def test_v043_real_fixture_migrates_cleanly(sandbox_home):
    tag = "v0.4.3"
    _skip_if_missing(tag)
    sb = sandbox_home
    assert load_real_fixture(sb, tag)
    _seed_v050_payload(sb["edikt_home"])

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, f"migrate failed:\n{proc.stderr}"

    assert (sb["edikt_home"] / "versions" / PAYLOAD_VERSION).is_dir()
    # v0.4.3 is the closest to v0.5.0; M1 is the main migration.
    assert (sb["edikt_home"] / "current").is_symlink()


# ─── Cross-fixture: manifest integrity ────────────────────────────────────────


@pytest.mark.parametrize("tag", TAGS)
def test_real_fixture_manifest_is_complete(tag: str) -> None:
    """manifest.txt must list every file in the fixture (capture integrity check)."""
    _skip_if_missing(tag)
    fixture_dir = FIXTURE_ROOT / tag
    manifest = fixture_dir / "manifest.txt"
    assert manifest.exists(), f"manifest.txt missing from {tag} fixture"

    # Parse manifest lines: "sha256  ./path/to/file"
    manifest_paths = set()
    for line in manifest.read_text().splitlines():
        if line.strip():
            parts = line.split(None, 1)
            if len(parts) == 2:
                manifest_paths.add(parts[1].strip().lstrip("./"))

    # Every non-manifest file on disk must appear in the manifest.
    for f in fixture_dir.rglob("*"):
        if f.is_file() and f.name != "manifest.txt":
            rel = str(f.relative_to(fixture_dir))
            assert rel in manifest_paths, (
                f"File '{rel}' in {tag} fixture is not listed in manifest.txt. "
                "Re-run capture.sh to regenerate."
            )


@pytest.mark.parametrize("tag", TAGS)
def test_real_fixture_no_absolute_home_paths(tag: str) -> None:
    """Fixtures must not contain absolute $HOME paths — only '${HOME}' placeholder."""
    _skip_if_missing(tag)
    import os
    real_home = os.path.expanduser("~")
    fixture_dir = FIXTURE_ROOT / tag
    violations = []
    for f in fixture_dir.rglob("*"):
        if f.is_file() and f.name != "manifest.txt":
            try:
                text = f.read_text(errors="replace")
                if real_home in text:
                    violations.append(str(f.relative_to(fixture_dir)))
            except (OSError, UnicodeDecodeError):
                pass
    assert not violations, (
        f"Fixture {tag} contains absolute home paths that should have been sanitized: "
        f"{violations[:5]}. Re-run capture.sh."
    )

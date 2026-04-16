"""Phase 10 — legacy_classifier_entered path.

Install an agent WITHOUT provenance frontmatter (simulating a pre-v0.5.0
install). Upgrade must detect the missing edikt_template_hash and fall back
to the v0.4.3 classifier, byte-compatible with commit d81f6e3.
"""

from __future__ import annotations

from .conftest import (
    AGENTS_DIR,
    assert_path_covered,
    install_agent,
    read_events,
    upgrade_agent,
)


def test_legacy_agent_without_provenance_uses_classifier(edikt_sandbox):
    sb = edikt_sandbox
    slug = "backend"
    source = AGENTS_DIR / f"{slug}.md"

    template_path = sb["templates_dir"] / f"{slug}.md"
    template_path.write_bytes(source.read_bytes())

    installed = sb["agents_dir"] / f"{slug}.md"
    # Install WITHOUT provenance (pre-v0.5.0 behavior).
    install_agent(
        installed,
        template_path,
        config_paths={},
        stack=[],
        with_provenance=False,
    )

    # Sanity: frontmatter does NOT carry edikt_template_hash.
    assert "edikt_template_hash" not in installed.read_text()

    # Upstream template ships a pure expansion — classifier should auto-apply.
    template_path.write_text(source.read_text() + "\n## Net-new section in v0.5.0\n")

    path = upgrade_agent(installed, template_path, config_paths={}, stack=[])

    assert path == "legacy_classifier_entered"
    assert_path_covered("legacy_classifier_entered")

    # Classifier fired PURE EXPANSION → installed file now matches template.
    assert installed.read_text() == template_path.read_text()

    # Event should carry reason="legacy_classifier" to distinguish from
    # the provenance-first path.
    events = read_events()
    legacy_ev = [
        e for e in events
        if e.get("type") == "upgrade_agent_replaced"
        and e.get("reason") == "legacy_classifier"
    ]
    assert len(legacy_ev) == 1
    assert legacy_ev[0]["agent"] == slug


def test_legacy_user_divergence_preserves_file(edikt_sandbox):
    """Legacy agent with meaningful deletions → classifier preserves."""
    sb = edikt_sandbox
    slug = "backend"
    source = AGENTS_DIR / f"{slug}.md"

    template_path = sb["templates_dir"] / f"{slug}.md"
    template_path.write_bytes(source.read_bytes())

    installed = sb["agents_dir"] / f"{slug}.md"
    install_agent(installed, template_path, with_provenance=False)

    # User edit — replaces a real template line with a team-specific one.
    # This creates a meaningful deletion (line present in installed, absent
    # in template) AND leaves an addition in the new template line for the
    # classifier to latch onto.
    original = installed.read_text()
    lines = original.splitlines()
    # Swap the last non-empty line for a user-specific variant.
    for i in range(len(lines) - 1, -1, -1):
        if lines[i].strip():
            lines[i] = "<!-- team-specific note replacing the tail line -->"
            break
    edited = "\n".join(lines) + "\n"
    assert edited != original
    installed.write_text(edited)

    # Template also shifted.
    template_path.write_text(source.read_text() + "\n## Upstream new\n")

    path = upgrade_agent(installed, template_path)
    assert path == "legacy_classifier_entered"

    events = read_events()
    preserved = [
        e for e in events
        if e.get("type") == "upgrade_agent_preserved"
        and e.get("reason") == "legacy_classifier"
    ]
    assert len(preserved) == 1
    # File untouched.
    assert installed.read_text() == edited

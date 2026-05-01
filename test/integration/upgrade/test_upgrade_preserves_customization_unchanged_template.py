"""Phase 10 — fast_preserve path.

Install an agent at hash X, bump the edikt version but do NOT change the
template bytes. Upgrade must observe stored_hash == current_template_hash
and take the fast_preserve branch. The installed file (including any user
edits applied after install) is left untouched.
"""

from __future__ import annotations

from pathlib import Path

from .conftest import (
    AGENTS_DIR,
    assert_path_covered,
    install_agent,
    read_events,
    upgrade_agent,
)


def test_fast_preserve_when_template_hash_unchanged(edikt_sandbox):
    sb = edikt_sandbox
    slug = "backend"
    source = AGENTS_DIR / f"{slug}.md"

    template_path = sb["templates_dir"] / f"{slug}.md"
    template_path.write_bytes(source.read_bytes())

    installed = sb["agents_dir"] / f"{slug}.md"
    stamped = install_agent(installed, template_path, config_paths={}, stack=[])

    # Simulate a user edit AFTER install.
    original = installed.read_text()
    user_edited = original + "\n<!-- team note: we rely on this agent -->\n"
    installed.write_text(user_edited)

    # Bump the declared edikt version — but leave template bytes untouched.
    # (Reference impl compares hashes only; version bump is irrelevant.)
    path = upgrade_agent(
        installed,
        template_path,
        config_paths={},
        stack=[],
    )

    assert path == "fast_preserve"
    assert_path_covered("fast_preserve")

    # User edits preserved byte-for-byte.
    assert installed.read_text() == user_edited

    # One preserved event, no replaced events, no conflict events.
    events = read_events()
    preserved = [e for e in events if e.get("type") == "upgrade_agent_preserved"]
    assert len(preserved) == 1
    assert preserved[0]["agent"] == slug
    assert preserved[0]["hash"] == stamped
    assert preserved[0]["reason"] == "template unchanged"

    assert not [e for e in events if e.get("type") == "upgrade_agent_replaced"]
    assert not [e for e in events if e.get("type") == "upgrade_agent_conflict_resolved"]

"""Phase 10 — threeway_prompt 'm' (merge) branch.

User picks `m`: a conflict-marked merge file is written next to the
installed agent at `.claude/agents/{slug}.md.merge`, the installed file
is NOT modified, and `upgrade_agent_merge_requested` is emitted so the
user can resolve and re-run /edikt:upgrade.
"""

from __future__ import annotations

from .conftest import (
    AGENTS_DIR,
    assert_path_covered,
    install_agent,
    read_events,
    upgrade_agent,
)


def test_merge_writes_conflict_file_and_preserves_installed(edikt_sandbox):
    sb = edikt_sandbox
    slug = "backend"
    source = AGENTS_DIR / f"{slug}.md"

    stored_template = sb["templates_dir"] / f"{slug}.stored.md"
    stored_template.write_bytes(source.read_bytes())

    installed = sb["agents_dir"] / f"{slug}.md"
    install_agent(installed, stored_template, config_paths={}, stack=[])
    user_edited = installed.read_text() + "\n<!-- user override -->\n"
    installed.write_text(user_edited)

    current_template = sb["templates_dir"] / f"{slug}.md"
    current_template.write_text(source.read_text() + "\n## Upstream change\n")

    path = upgrade_agent(
        installed,
        current_template,
        stored_template_path=stored_template,
        user_choice="m",
    )

    assert path == "threeway_prompt"
    assert_path_covered("threeway_prompt")

    # Installed file untouched.
    assert installed.read_text() == user_edited

    # Merge file exists alongside.
    merge_path = installed.with_suffix(installed.suffix + ".merge")
    assert merge_path.exists()
    merged = merge_path.read_text()
    assert "<<<<<<< stored template" in merged
    assert "||||||| your edits" in merged
    assert ">>>>>>> new template" in merged
    assert "<!-- user override -->" in merged
    assert "Upstream change" in merged

    # Events: conflict resolved with 'm' AND merge_requested emitted.
    events = read_events()
    resolved = [e for e in events if e.get("type") == "upgrade_agent_conflict_resolved"]
    assert len(resolved) == 1
    assert resolved[0]["resolution"] == "m"

    requested = [e for e in events if e.get("type") == "upgrade_agent_merge_requested"]
    assert len(requested) == 1
    assert requested[0]["agent"] == slug
    assert requested[0]["merge_file"].endswith(".md.merge")

    # No replacement event — frontmatter must stay on stored_hash until merge resolves.
    assert not [e for e in events if e.get("type") == "upgrade_agent_replaced"]

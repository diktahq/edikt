"""Phase 10 — threeway_prompt path.

Install an agent at hash X, then change the upstream template AND let the
user edit the installed file. Upgrade must take the threeway_prompt branch,
invoke the [a/k/m/s] prompt, and record the resolution. We simulate the
user picking 'k' (keep) — file stays untouched, preserved event written.
"""

from __future__ import annotations

from .conftest import (
    AGENTS_DIR,
    assert_path_covered,
    install_agent,
    read_events,
    upgrade_agent,
)


def test_threeway_prompt_when_both_moved(edikt_sandbox):
    sb = edikt_sandbox
    slug = "backend"
    source = AGENTS_DIR / f"{slug}.md"

    # Stored (old) template — the version the user originally installed.
    stored_template = sb["templates_dir"] / f"{slug}.stored.md"
    stored_template.write_bytes(source.read_bytes())

    installed = sb["agents_dir"] / f"{slug}.md"
    install_agent(installed, stored_template, config_paths={}, stack=[])

    # User applied their own edit after install.
    user_edited = installed.read_text() + "\n<!-- user customization: trust_proxy=true -->\n"
    installed.write_text(user_edited)

    # Current (new) template — upstream shipped a change.
    current_template = sb["templates_dir"] / f"{slug}.md"
    current_template.write_text(
        source.read_text() + "\n## New section shipped upstream in v0.5.0\n"
    )

    path = upgrade_agent(
        installed,
        current_template,
        stored_template_path=stored_template,
        config_paths={},
        stack=[],
        user_choice="k",
    )

    assert path == "threeway_prompt"
    assert_path_covered("threeway_prompt")

    # User chose keep → file unchanged.
    assert installed.read_text() == user_edited

    events = read_events()
    resolved = [e for e in events if e.get("type") == "upgrade_agent_conflict_resolved"]
    assert len(resolved) == 1
    assert resolved[0]["agent"] == slug
    assert resolved[0]["resolution"] == "k"

    preserved = [e for e in events if e.get("type") == "upgrade_agent_preserved"]
    assert len(preserved) == 1
    assert preserved[0]["reason"] == "user kept edits on moved template"


def test_threeway_prompt_apply_overwrites_with_new_template(edikt_sandbox):
    """Same setup, but user picks 'a' — new template wins, provenance updates."""
    sb = edikt_sandbox
    slug = "backend"
    source = AGENTS_DIR / f"{slug}.md"

    stored_template = sb["templates_dir"] / f"{slug}.stored.md"
    stored_template.write_bytes(source.read_bytes())

    installed = sb["agents_dir"] / f"{slug}.md"
    install_agent(installed, stored_template, config_paths={}, stack=[])
    installed.write_text(installed.read_text() + "\n<!-- user edit -->\n")

    current_template = sb["templates_dir"] / f"{slug}.md"
    current_template.write_text(source.read_text() + "\n## Upstream addition\n")

    path = upgrade_agent(
        installed,
        current_template,
        stored_template_path=stored_template,
        user_choice="a",
    )

    assert path == "threeway_prompt"

    events = read_events()
    replaced = [e for e in events if e.get("type") == "upgrade_agent_replaced"]
    assert len(replaced) == 1
    assert replaced[0]["user_accepted"] is True
    # File now contains the new template's marker, user edit is gone.
    assert "Upstream addition" in installed.read_text()
    assert "<!-- user edit -->" not in installed.read_text()

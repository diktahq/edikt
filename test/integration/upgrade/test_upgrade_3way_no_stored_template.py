"""Phase 10 — threeway_prompt fallback when stored template can't be reconstructed.

Per upgrade.md §2c Step 5: if the stored template cannot be reconstructed
(no versioned cache, no git tag), the spec mandates falling through to
Step 7 (threeway_prompt) — we cannot prove the user didn't edit, so we
must ask. The reference impl must NOT silently take the safe-replace path.
"""

from __future__ import annotations

from .conftest import (
    AGENTS_DIR,
    assert_path_covered,
    install_agent,
    read_events,
    upgrade_agent,
)


def test_threeway_when_stored_template_unavailable(edikt_sandbox):
    sb = edikt_sandbox
    slug = "backend"
    source = AGENTS_DIR / f"{slug}.md"

    # Install at the stored template's hash.
    stored_template = sb["templates_dir"] / f"{slug}.stored.md"
    stored_template.write_bytes(source.read_bytes())

    installed = sb["agents_dir"] / f"{slug}.md"
    install_agent(installed, stored_template, config_paths={}, stack=[])

    # Upstream shifts.
    current_template = sb["templates_dir"] / f"{slug}.md"
    current_template.write_text(source.read_text() + "\n## Upstream change\n")

    # Crucially: do NOT pass stored_template_path. Simulates a v0.5.0 install
    # that was created on a machine without versioned-template cache.
    path = upgrade_agent(
        installed,
        current_template,
        stored_template_path=None,
        user_choice="s",
    )

    assert path == "threeway_prompt"
    assert_path_covered("threeway_prompt")

    events = read_events()
    # No safe-replace event must have fired.
    assert not [
        e for e in events if e.get("type") == "upgrade_agent_path"
        and e.get("path") == "resynth_safe_replace"
    ]
    # Conflict resolution recorded with the user's skip choice.
    resolved = [e for e in events if e.get("type") == "upgrade_agent_conflict_resolved"]
    assert len(resolved) == 1
    assert resolved[0]["resolution"] == "s"

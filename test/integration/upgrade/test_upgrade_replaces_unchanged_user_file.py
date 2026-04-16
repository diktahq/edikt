"""Phase 10 — resynth_safe_replace path.

Install an agent at hash X, never touch the installed file, then change
the upstream template. Upgrade must observe that the installed file is
byte-identical to what init would have produced from the stored template
(no user edits) and therefore replace with the current template safely —
no 3-way prompt required.
"""

from __future__ import annotations

from .conftest import (
    AGENTS_DIR,
    assert_path_covered,
    install_agent,
    read_events,
    upgrade_agent,
)


def test_safe_replace_when_user_never_edited(edikt_sandbox):
    sb = edikt_sandbox
    slug = "backend"
    source = AGENTS_DIR / f"{slug}.md"

    # Stored (old) template — what was installed originally.
    stored_template = sb["templates_dir"] / f"{slug}.stored.md"
    stored_template.write_bytes(source.read_bytes())

    installed = sb["agents_dir"] / f"{slug}.md"
    stored_hash = install_agent(installed, stored_template, config_paths={}, stack=[])

    # NOTE: user does NOT touch the file between install and upgrade.

    # Upstream template moves forward.
    current_template = sb["templates_dir"] / f"{slug}.md"
    current_template.write_text(source.read_text() + "\n## Added in v0.5.0\n")

    path = upgrade_agent(
        installed,
        current_template,
        stored_template_path=stored_template,
        config_paths={},
        stack=[],
    )

    assert path == "resynth_safe_replace"
    assert_path_covered("resynth_safe_replace")

    # The installed file now reflects the new template (post-substitution,
    # with provenance re-stamped to the new hash).
    new_text = installed.read_text()
    assert "Added in v0.5.0" in new_text

    import hashlib
    new_hash = hashlib.md5(current_template.read_bytes()).hexdigest()
    assert f'edikt_template_hash: "{new_hash}"' in new_text

    events = read_events()
    replaced = [e for e in events if e.get("type") == "upgrade_agent_replaced"]
    assert len(replaced) == 1
    assert replaced[0]["hash_old"] == stored_hash
    assert replaced[0]["hash_new"] == new_hash
    assert replaced[0]["user_accepted"] is False

    # No prompt should have fired.
    assert not [e for e in events if e.get("type") == "upgrade_agent_conflict_resolved"]

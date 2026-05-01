"""Reference implementation + fixtures for Phase 10 upgrade provenance tests.

These tests treat the control flow in `commands/upgrade.md` §2c as the
specification and verify a Python reference implementation that mirrors it
branch-for-branch. The test harness does not exec Claude Code; it exercises
`upgrade_agent()` below, which faithfully reproduces the steps defined in
SPEC-004 §8.

Every distinct code path emits a `upgrade_agent_path` event to
`$EDIKT_HOME/events.jsonl`. Tests grep that file to verify the intended
path was reached — mirroring Architect #6's `assert_path_covered` contract.

Path identifiers (keep in sync with commands/upgrade.md §2c):

  fast_preserve              — stored_hash == current_template_hash
  resynth_safe_replace       — hash differs, resynth == installed (no edits)
  threeway_prompt            — hash differs, resynth != installed (user edited)
  legacy_classifier_entered  — no edikt_template_hash in frontmatter
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import time
from pathlib import Path
from typing import Iterable

import pytest
import yaml

# Re-use init's reference helpers — they are the ground truth for how a
# template is written to disk at install time.
import sys

_HERE = Path(__file__).resolve().parent
_INIT_DIR = _HERE.parent / "init"
sys.path.insert(0, str(_INIT_DIR.parent))
from init.conftest import (  # type: ignore  # noqa: E402
    apply_stack_filter,
    apply_substitutions,
    compute_hash,
    update_frontmatter,
)

REPO_ROOT = Path(__file__).resolve().parents[3]
AGENTS_DIR = REPO_ROOT / "templates" / "agents"


# ─── Event log ───────────────────────────────────────────────────────────────


def _events_path() -> Path:
    home = Path(os.environ["EDIKT_HOME"])
    home.mkdir(parents=True, exist_ok=True)
    return home / "events.jsonl"


def emit_event(event_type: str, payload: dict) -> None:
    """Append a JSON line to $EDIKT_HOME/events.jsonl.

    Mirrors templates/hooks/event-log.sh. Tests grep this file to verify
    the intended code path was reached.
    """
    record = {"type": event_type, "at": time.strftime("%Y-%m-%dT%H:%M:%SZ"), **payload}
    with _events_path().open("a") as f:
        f.write(json.dumps(record) + "\n")


def read_events() -> list[dict]:
    path = _events_path()
    if not path.exists():
        return []
    return [json.loads(line) for line in path.read_text().splitlines() if line.strip()]


def assert_path_covered(path_id: str, events: Iterable[dict] | None = None) -> None:
    events = list(events) if events is not None else read_events()
    hits = [
        e for e in events
        if e.get("type") == "upgrade_agent_path" and e.get("path") == path_id
    ]
    assert hits, (
        f"expected at least one upgrade_agent_path event with path={path_id!r}; "
        f"saw: {[e.get('path') for e in events if e.get('type') == 'upgrade_agent_path']}"
    )


# ─── Frontmatter helpers ────────────────────────────────────────────────────


_FM_RE = re.compile(r"^---\n(.*?)\n---\n", re.DOTALL)


def read_frontmatter(content: str) -> dict:
    m = _FM_RE.match(content)
    if not m:
        return {}
    try:
        return yaml.safe_load(m.group(1)) or {}
    except yaml.YAMLError:
        return {}


def body_without_provenance(content: str) -> str:
    """Return content with the two provenance lines stripped from frontmatter.

    Re-synthesized files never carry provenance lines (they are appended
    by the install step). To compare apples-to-apples we remove them from
    the installed file before diffing.
    """
    m = _FM_RE.match(content)
    if not m:
        return content
    fm_lines = [
        l for l in m.group(1).splitlines()
        if not l.startswith(("edikt_template_hash:", "edikt_template_version:"))
    ]
    rest = content[m.end():]
    return "---\n" + "\n".join(fm_lines) + "\n---\n" + rest


# ─── Provenance-first upgrade (reference impl) ──────────────────────────────


CURRENT_EDIKT_VERSION = "0.5.0"


def install_agent(
    installed_path: Path,
    template_path: Path,
    *,
    config_paths: dict[str, str] | None = None,
    stack: list[str] | None = None,
    with_provenance: bool = True,
    version: str = CURRENT_EDIKT_VERSION,
) -> str:
    """Write an agent file exactly the way `/edikt:init` does.

    Returns the raw-template hash that was stamped into frontmatter.
    """
    raw = template_path.read_text()
    raw_hash = hashlib.md5(template_path.read_bytes()).hexdigest()

    body = raw
    if config_paths:
        body = apply_substitutions(body, config_paths)
    body, _warns = apply_stack_filter(body, stack or [])

    if with_provenance:
        body = update_frontmatter(body, raw_hash, version)

    installed_path.parent.mkdir(parents=True, exist_ok=True)
    installed_path.write_text(body)
    return raw_hash


def upgrade_agent(
    installed_path: Path,
    current_template_path: Path,
    *,
    stored_template_path: Path | None = None,
    config_paths: dict[str, str] | None = None,
    stack: list[str] | None = None,
    user_choice: str | None = None,
) -> str:
    """Reference implementation of commands/upgrade.md §2c.

    Returns the path id taken: one of
    "fast_preserve", "resynth_safe_replace", "threeway_prompt",
    "legacy_classifier_entered".

    `user_choice` is consulted only when the threeway_prompt branch is
    taken. It must be one of "a", "k", "m", "s".
    """
    slug = installed_path.stem
    config_paths = config_paths or {}
    stack = stack or []

    installed_text = installed_path.read_text()
    fm = read_frontmatter(installed_text)
    stored_hash = fm.get("edikt_template_hash")

    # Step 2 — legacy fallback
    if not stored_hash:
        emit_event("upgrade_agent_path", {"agent": slug, "path": "legacy_classifier_entered"})
        _legacy_classifier(slug, installed_path, current_template_path)
        return "legacy_classifier_entered"

    # Step 3
    current_template_hash = hashlib.md5(current_template_path.read_bytes()).hexdigest()

    # Step 4 — fast preserve
    if stored_hash == current_template_hash:
        emit_event("upgrade_agent_path", {"agent": slug, "path": "fast_preserve"})
        emit_event(
            "upgrade_agent_preserved",
            {"agent": slug, "hash": stored_hash, "reason": "template unchanged"},
        )
        return "fast_preserve"

    # Step 5 — reconstruct and re-synthesize. If the stored template
    # cannot be reconstructed (no versioned cache, no git history), the
    # spec mandates falling through to Step 7 — we cannot prove the user
    # didn't edit, so we must ask.
    installed_body = body_without_provenance(installed_text)
    if stored_template_path is None:
        resynth = None
    else:
        resynth = apply_stack_filter(
            apply_substitutions(stored_template_path.read_text(), config_paths), stack
        )[0]

    if resynth is not None and installed_body == resynth:
        # Step 6 — safe replace
        new_raw = current_template_path.read_text()
        new_body = apply_stack_filter(
            apply_substitutions(new_raw, config_paths), stack
        )[0]
        new_body = update_frontmatter(new_body, current_template_hash, CURRENT_EDIKT_VERSION)
        installed_path.write_text(new_body)

        emit_event("upgrade_agent_path", {"agent": slug, "path": "resynth_safe_replace"})
        emit_event(
            "upgrade_agent_replaced",
            {
                "agent": slug,
                "hash_old": stored_hash,
                "hash_new": current_template_hash,
                "user_accepted": False,
            },
        )
        return "resynth_safe_replace"

    # Step 7 — threeway prompt
    emit_event("upgrade_agent_path", {"agent": slug, "path": "threeway_prompt"})

    choice = user_choice or "s"
    assert choice in {"a", "k", "m", "s"}, f"invalid user_choice: {choice!r}"
    emit_event(
        "upgrade_agent_conflict_resolved",
        {"agent": slug, "resolution": choice},
    )

    if choice == "a":
        new_raw = current_template_path.read_text()
        new_body = apply_stack_filter(
            apply_substitutions(new_raw, config_paths), stack
        )[0]
        new_body = update_frontmatter(new_body, current_template_hash, CURRENT_EDIKT_VERSION)
        installed_path.write_text(new_body)
        emit_event(
            "upgrade_agent_replaced",
            {
                "agent": slug,
                "hash_old": stored_hash,
                "hash_new": current_template_hash,
                "user_accepted": True,
            },
        )
    elif choice == "k":
        emit_event(
            "upgrade_agent_preserved",
            {"agent": slug, "hash": stored_hash, "reason": "user kept edits on moved template"},
        )
    elif choice == "m":
        # Produce a conflict-marked merge file alongside the installed agent.
        # Leave the installed file unchanged. User must resolve and re-run.
        merge_path = installed_path.with_suffix(installed_path.suffix + ".merge")
        new_raw = current_template_path.read_text()
        new_body = apply_stack_filter(
            apply_substitutions(new_raw, config_paths), stack
        )[0]
        stored_body = (
            apply_stack_filter(
                apply_substitutions(stored_template_path.read_text(), config_paths), stack
            )[0]
            if stored_template_path is not None
            else ""
        )
        merge_path.write_text(
            "<<<<<<< stored template\n"
            f"{stored_body}\n"
            "||||||| your edits\n"
            f"{installed_body}\n"
            "=======\n"
            f"{new_body}\n"
            ">>>>>>> new template\n"
        )
        emit_event(
            "upgrade_agent_merge_requested",
            {
                "agent": slug,
                "merge_file": str(merge_path),
                "hash_old": stored_hash,
                "hash_new": current_template_hash,
            },
        )
    # "s" leaves the file untouched and updates no frontmatter.

    return "threeway_prompt"


# ─── Legacy classifier (verbatim from v0.4.3, commit d81f6e3) ───────────────


def _legacy_classifier(slug: str, installed: Path, template: Path) -> None:
    """Classify divergence using the v0.4.3 heuristic. DO NOT simplify.

    Byte-compat behavior:
      - hashes equal        → no-op
      - PURE EXPANSION      → auto-apply (upgrade_agent_replaced, legacy reason)
      - USER DIVERGENCE     → preserved (upgrade_agent_preserved, legacy reason)
    """
    tpl_bytes = template.read_bytes()
    inst_bytes = installed.read_bytes()

    if hashlib.md5(tpl_bytes).hexdigest() == hashlib.md5(inst_bytes).hexdigest():
        return

    tpl_lines = set(tpl_bytes.decode("utf-8", "replace").splitlines())
    inst_lines = set(inst_bytes.decode("utf-8", "replace").splitlines())
    deletions = inst_lines - tpl_lines
    additions = tpl_lines - inst_lines

    # Drop trivial whitespace-only lines from deletions before classifying.
    meaningful_deletions = {l for l in deletions if l.strip()}

    if additions and not meaningful_deletions:
        # PURE EXPANSION → auto-apply
        installed.write_bytes(tpl_bytes)
        emit_event(
            "upgrade_agent_replaced",
            {
                "agent": slug,
                "hash_old": hashlib.md5(inst_bytes).hexdigest(),
                "hash_new": hashlib.md5(tpl_bytes).hexdigest(),
                "user_accepted": False,
                "reason": "legacy_classifier",
            },
        )
        return

    # USER DIVERGENCE → preserve, prompt would happen in interactive upgrade
    emit_event(
        "upgrade_agent_preserved",
        {
            "agent": slug,
            "hash": hashlib.md5(inst_bytes).hexdigest(),
            "reason": "legacy_classifier",
        },
    )


# ─── Fixtures ────────────────────────────────────────────────────────────────


@pytest.fixture
def edikt_sandbox(tmp_path, monkeypatch):
    home = tmp_path / "home"
    edikt_home = home / ".edikt"
    claude_home = home / ".claude"
    project = tmp_path / "project"
    agents_dir = project / ".claude" / "agents"
    templates_dir = edikt_home / "templates" / "agents"

    for p in (home, edikt_home, claude_home, project, agents_dir, templates_dir):
        p.mkdir(parents=True, exist_ok=True)

    monkeypatch.setenv("HOME", str(home))
    monkeypatch.setenv("EDIKT_HOME", str(edikt_home))
    monkeypatch.setenv("CLAUDE_HOME", str(claude_home))
    monkeypatch.chdir(project)

    return {
        "home": home,
        "edikt_home": edikt_home,
        "claude_home": claude_home,
        "project": project,
        "agents_dir": agents_dir,
        "templates_dir": templates_dir,
    }


@pytest.fixture
def backend_template_bytes() -> bytes:
    return (AGENTS_DIR / "backend.md").read_bytes()

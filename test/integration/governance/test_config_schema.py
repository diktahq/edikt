"""Config schema validation — .edikt/config.yaml completeness and correctness.

Every key referenced by a hook or command must:
1. Be documented in .edikt/config.yaml (the dogfood config).
2. Have a valid value type.
3. Have a sane default when the key is absent.

Tests verify the dogfood config (the one that governs edikt's own development)
matches the schema implied by the hooks. If a hook references a config key that
doesn't exist in the schema, the toggle silently never fires.

No ANTHROPIC_API_KEY required.
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest
import yaml

REPO_ROOT = Path(__file__).resolve().parents[3]
DOGFOOD_CONFIG = REPO_ROOT / ".edikt" / "config.yaml"
HOOKS_DIR = REPO_ROOT / "templates" / "hooks"

# Keys hooks read, with their expected type and valid values.
# Source: grep for config reads in templates/hooks/*.sh
HOOK_CONFIG_CONTRACTS: dict[str, dict] = {
    # features section
    "features.auto-format": {
        "type": bool,
        "hook": "post-tool-use.sh",
        "grep_pattern": "auto-format: false",
        "default": True,
    },
    "features.signal-detection": {
        "type": bool,
        "hook": "stop-hook.sh",
        "grep_pattern": "signal-detection: false",
        "default": True,
    },
    "features.plan-injection": {
        "type": bool,
        "hook": "user-prompt-submit.sh",
        "grep_pattern": "plan-injection: false",
        "default": True,
    },
    "features.session-summary": {
        "type": bool,
        "hook": "session-start.sh",
        "grep_pattern": "session-summary",
        "default": True,
    },
    # Top-level keys
    "base": {
        "type": str,
        "hook": "stop-hook.sh + user-prompt-submit.sh",
        "grep_pattern": "^base:",
        "default": "docs",
    },
    "phase-end": {
        "type": bool,
        "hook": "phase-end-detector.sh",
        "grep_pattern": "phase-end",
        "default": True,
    },
}


# ─── Helpers ─────────────────────────────────────────────────────────────────


def _load_dogfood() -> dict:
    return yaml.safe_load(DOGFOOD_CONFIG.read_text()) or {}


def _get_nested(d: dict, dotted_key: str) -> object:
    """Traverse a nested dict with a dotted key like 'features.auto-format'."""
    parts = dotted_key.split(".")
    current: object = d
    for part in parts:
        if not isinstance(current, dict):
            return None
        current = current.get(part)
    return current


# ─── Dogfood config structure ─────────────────────────────────────────────────


def test_dogfood_config_exists() -> None:
    assert DOGFOOD_CONFIG.exists(), (
        f".edikt/config.yaml not found at {DOGFOOD_CONFIG}. "
        "The dogfood config is required for edikt's own development governance."
    )


def test_dogfood_config_is_valid_yaml() -> None:
    try:
        result = yaml.safe_load(DOGFOOD_CONFIG.read_text())
    except yaml.YAMLError as e:
        pytest.fail(f".edikt/config.yaml is not valid YAML: {e}")
    assert isinstance(result, dict), ".edikt/config.yaml must be a YAML mapping"


def test_dogfood_config_has_edikt_version() -> None:
    cfg = _load_dogfood()
    assert "edikt_version" in cfg, (
        ".edikt/config.yaml missing 'edikt_version'. "
        "Hooks and commands use this to detect the install version."
    )
    version = str(cfg["edikt_version"])
    assert re.match(r"^\d+\.\d+\.\d+", version), (
        f"edikt_version must be semver format (e.g. '0.5.0'), got {version!r}"
    )


def test_dogfood_config_has_base() -> None:
    cfg = _load_dogfood()
    assert "base" in cfg, (
        ".edikt/config.yaml missing 'base:' key. "
        "stop-hook.sh and user-prompt-submit.sh use this as the root for ADR/plan paths. "
        "Without it, both hooks fall back to 'docs' but don't validate the path exists."
    )
    base = cfg["base"]
    assert isinstance(base, str) and base, (
        f"base: must be a non-empty string, got {base!r}"
    )


def test_dogfood_config_has_features_section() -> None:
    cfg = _load_dogfood()
    assert "features" in cfg, (
        ".edikt/config.yaml missing 'features:' section. "
        "Feature toggles (auto-format, signal-detection, plan-injection) are read "
        "from this section. Without it the hooks always use defaults."
    )
    assert isinstance(cfg["features"], dict), (
        "features: must be a YAML mapping, not a list or scalar"
    )


def test_dogfood_features_section_has_all_toggles() -> None:
    """Every feature toggle referenced by a hook must be present in the config."""
    cfg = _load_dogfood()
    features = cfg.get("features") or {}
    feature_toggles = [
        k.split(".", 1)[1]
        for k in HOOK_CONFIG_CONTRACTS
        if k.startswith("features.")
    ]
    for toggle in feature_toggles:
        assert toggle in features, (
            f"features.{toggle} is referenced by hooks but missing from .edikt/config.yaml. "
            "Add it explicitly so the current value is visible and reviewable."
        )


def test_dogfood_feature_values_are_booleans() -> None:
    cfg = _load_dogfood()
    features = cfg.get("features") or {}
    for key, value in features.items():
        assert isinstance(value, bool), (
            f"features.{key} must be a boolean (true/false), got {type(value).__name__}: {value!r}. "
            "YAML treats unquoted 'true'/'false' as booleans automatically."
        )


def test_dogfood_config_has_paths_section() -> None:
    cfg = _load_dogfood()
    assert "paths" in cfg, (
        ".edikt/config.yaml missing 'paths:' section. "
        "Custom path configuration (decisions, invariants, plans) lives here. "
        "Without it, all hooks fall back to their hardcoded defaults."
    )


# ─── Hook config reference coverage ──────────────────────────────────────────


@pytest.fixture(
    params=list(HOOK_CONFIG_CONTRACTS.keys()),
    ids=lambda k: k,
)
def config_contract(request: pytest.FixtureRequest) -> dict:
    key = request.param
    return {"key": key, **HOOK_CONFIG_CONTRACTS[key]}


def test_hook_references_config_key_that_hooks_actually_read(
    config_contract: dict,
) -> None:
    """Every documented config key must appear in the hook that reads it."""
    hook_name = config_contract["hook"].split("+")[0].strip()
    if not hook_name.endswith(".sh"):
        pytest.skip(f"multi-hook contract '{config_contract['hook']}' — skip per-hook check")
    hook_path = HOOKS_DIR / hook_name
    if not hook_path.exists():
        pytest.fail(f"{hook_name} not found at {hook_path}")
    hook_text = hook_path.read_text()
    pattern = config_contract["grep_pattern"]
    assert pattern in hook_text, (
        f"Config key '{config_contract['key']}' is documented as read by {hook_name} "
        f"but the pattern '{pattern}' was not found in the hook source. "
        "Either the key was renamed in the hook or the contract table is stale."
    )


def test_config_default_behavior_is_permissive() -> None:
    """When feature toggle keys are absent, hooks should default to enabled.

    A missing 'auto-format' key should NOT disable formatting — that would
    silently break all new projects that don't explicitly opt in.
    Hooks must use the absence of 'feature: false' as the trigger to skip,
    not the presence of 'feature: true'.
    """
    for key, contract in HOOK_CONFIG_CONTRACTS.items():
        if not key.startswith("features."):
            continue
        hook_name = contract["hook"].split("+")[0].strip()
        if not hook_name.endswith(".sh"):
            continue
        hook_path = HOOKS_DIR / hook_name
        if not hook_path.exists():
            continue
        hook_text = hook_path.read_text()
        toggle = key.split(".", 1)[1]
        # Hooks should check for 'feature: false', not 'feature: true'.
        # 'feature: false' → skip. absent → run (default-on).
        negative_pattern = f"{toggle}: false"
        positive_pattern = f"{toggle}: true"
        has_negative = negative_pattern in hook_text
        has_positive_only = positive_pattern in hook_text and not has_negative
        assert not has_positive_only, (
            f"{hook_name}: checks for '{positive_pattern}' (to enable) rather than "
            f"'{negative_pattern}' (to disable). "
            "Feature toggles must default-on: check for false to skip, not for true to run. "
            "A missing key must not silently disable the feature."
        )

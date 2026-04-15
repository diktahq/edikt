"""Project-mode conftest — re-exports fixtures for pytest.

Fixture implementations live in pm_helpers.py (a non-conftest module)
to avoid shadowing migration/conftest.py when both directories are
collected in the same pytest session.
"""

import importlib.util
import sys
from pathlib import Path


def _load_pm_helpers():
    spec = importlib.util.spec_from_file_location(
        "pm_helpers",
        Path(__file__).parent / "pm_helpers.py",
    )
    mod = importlib.util.module_from_spec(spec)
    sys.modules["pm_helpers"] = mod
    spec.loader.exec_module(mod)
    return mod


_pm = _load_pm_helpers()

project_dir = _pm.project_dir
global_edikt_dir = _pm.global_edikt_dir
sandbox = _pm.sandbox

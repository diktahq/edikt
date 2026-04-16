"""Re-exports from migration conftest for stable imports.

Migration test files use `from helpers import ...` instead of
`from conftest import ...` to avoid module-name collisions when
pytest adds project-mode/ to sys.path (testpaths shadowing).
"""
from __future__ import annotations

# Import the module by file path to avoid any name-collision risk.
import importlib.util
from pathlib import Path

_spec = importlib.util.spec_from_file_location(
    "_migration_conftest",
    Path(__file__).parent / "conftest.py",
)
_mod = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_mod)

# Re-export everything that tests need.
FIXTURE_ROOT = _mod.FIXTURE_ROOT
PAYLOAD_VERSION = _mod.PAYLOAD_VERSION
_make_payload = _mod._make_payload
_versioned_seed = _mod._versioned_seed
_write = _mod._write
load_real_fixture = _mod.load_real_fixture
run_migrate = _mod.run_migrate
build_synth_v010 = _mod.build_synth_v010
build_synth_v014 = _mod.build_synth_v014
build_synth_v020 = _mod.build_synth_v020
build_synth_v030 = _mod.build_synth_v030
build_synth_v043 = _mod.build_synth_v043

#!/usr/bin/env bash
# Pins INV-007 / HI-10, HI-11 — hermetic benchmark sandbox.

set -eu
cd "$(dirname "$0")/../.."

fail=0

# ── 1. No call site in the benchmark harness uses setting_sources with "user".
hits=$(grep -rnE 'setting_sources[[:space:]]*=[[:space:]]*\[[^]]*"user"' test/integration/ 2>/dev/null || true)
if [ -n "$hits" ]; then
    echo '[INV-007] setting_sources=["user"] found in test harness (must be project-only):' >&2
    echo "$hits" >&2
    fail=1
fi

# ── 2. runner.py builds a sandbox settings.json without a "hooks" key.
python3 - <<'PY' || fail=1
import ast
import sys
from pathlib import Path
src = Path("test/integration/benchmarks/runner.py").read_text()
if "shutil.copy2(repo_settings, project / \".claude\" / \"settings.json\")" in src:
    print("[INV-007] runner.py still copies host settings.json into sandbox", file=sys.stderr)
    sys.exit(1)
if '"hooks"' in src.split("def build_project", 1)[-1].split("def ", 1)[0]:
    print("[INV-007] runner.py build_project may be adding a hooks key", file=sys.stderr)
    sys.exit(1)
PY

# ── 3. conftest.py redacts tool_input.content in the JSONL writer.
if ! grep -q '<redacted:len=' test/integration/benchmarks/conftest.py; then
    echo "[INV-007 / HI-11] conftest.py does not appear to redact tool_input.content" >&2
    fail=1
fi

# ── 4. conftest.py aborts on credential patterns.
if ! grep -q 'sk-ant-' test/integration/benchmarks/conftest.py; then
    echo "[INV-007 / HI-11] conftest.py does not appear to scan for sk-ant- pattern" >&2
    fail=1
fi

exit $fail

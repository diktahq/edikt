#!/usr/bin/env bash
# edikt: pre-push hook — invariant compliance on staged diff (FR-010, SPEC-006)
# Validates the staged diff against INV-001 through INV-003 before push.
# Set EDIKT_BYPASS_PREPUSH=1 to bypass (logged to events.jsonl).

set -euo pipefail

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Bypass path (EDIKT_BYPASS_PREPUSH=1 — logged, not silent)
if [ "${EDIKT_BYPASS_PREPUSH:-}" = "1" ]; then
    BYPASS_TS=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    GIT_USER=$(git config user.name 2>/dev/null || echo "unknown")
    GIT_EMAIL=$(git config user.email 2>/dev/null || echo "unknown")
    mkdir -p "$HOME/.edikt" 2>/dev/null || true
    python3 -c '
import json,sys,os
ts, user, email, out = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
rec = {"ts": ts, "event": "prepush_bypass", "user": user, "email": email}
try:
    with open(out, "a", encoding="utf-8") as f:
        f.write(json.dumps(rec) + "\n")
except Exception:
    pass
' "$BYPASS_TS" "$GIT_USER" "$GIT_EMAIL" "$HOME/.edikt/events.jsonl" 2>/dev/null
    echo "warn: EDIKT_BYPASS_PREPUSH=1 — invariant check skipped (logged)" >&2
    exit 0
fi

# Get staged files
STAGED_FILES=$(git diff --cached --name-only 2>/dev/null || true)
if [ -z "$STAGED_FILES" ]; then
    exit 0
fi

# Invariant compliance check on staged diff (INV-001, INV-002, INV-003)
RESULT=$(python3 - "$STAGED_FILES" <<'PY'
import sys, os, re, subprocess

staged_raw = sys.argv[1] if len(sys.argv) > 1 else ""
staged = [f for f in staged_raw.split("\n") if f.strip()]
violations = []

for f in staged:
    try:
        diff = subprocess.run(
            ["git", "diff", "--cached", "--", f],
            capture_output=True, text=True
        ).stdout
    except Exception:
        continue

    # INV-001: no compiled code in commands/ or templates/
    if re.search(r'^(commands|templates)/', f):
        for ext in ('.ts', '.js', '.py', '.rb', '.go', '.rs'):
            if f.endswith(ext):
                violations.append(f"INV-001: {f} — compiled code in commands/ or templates/ is forbidden")

    # INV-002: accepted ADRs are immutable
    if re.search(r'docs/architecture/decisions/ADR-\d+', f):
        try:
            content = open(f, encoding="utf-8").read()
            if re.search(r'^status:\s*accepted', content, re.MULTILINE):
                if diff.strip():
                    violations.append(f"INV-002: {f} — accepted ADR is immutable (create a new ADR that supersedes it)")
        except Exception:
            pass

    # INV-003: no shell JSON concatenation in hook scripts
    if re.search(r'^templates/hooks/', f) or f == 'install.sh':
        for pat in (r"""echo\s+['"][{]""", r"""printf\s+['"][{]"""):
            if re.search(pat, diff):
                violations.append(f"INV-003: {f} — shell JSON concatenation is forbidden; use python3 -c 'import json; print(json.dumps(...))'")
                break

if violations:
    print("VIOLATIONS:" + "\n".join(violations))
    sys.exit(1)
else:
    print("OK")
    sys.exit(0)
PY
)
INVARIANT_EXIT=$?

if [ $INVARIANT_EXIT -ne 0 ]; then
    # Extract violation lines
    VIOLATIONS=$(echo "$RESULT" | grep -v '^VIOLATIONS:' || echo "$RESULT")
    echo "❌ Pre-push invariant check failed:" >&2
    echo "" >&2
    echo "$VIOLATIONS" | while IFS= read -r line; do
        echo "  $line" >&2
    done
    echo "" >&2
    echo "Fix violations or set EDIKT_BYPASS_PREPUSH=1 to bypass (logged)." >&2
    exit 1
fi

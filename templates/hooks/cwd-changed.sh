#!/usr/bin/env bash
# edikt: CwdChanged hook — detect directory switches for monorepo awareness
# Fires when the working directory changes during a session.

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Read hook input from stdin
INPUT=$(cat)

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# CwdChanged has no established stdout-JSON contract (this hook's success
# path prints nothing), so stderr is the only channel available that does
# not invent a new protocol shape. Absence used to fail extraction silently
# with no trace anywhere.
if ! command -v python3 >/dev/null 2>&1; then
    echo "edikt: cwd-changed — python3 not found; this directory switch was not logged" >&2
    exit 0
fi

# Extract new directory
NEW_CWD=$(echo "$INPUT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('cwd',''))" 2>/dev/null || echo "")

if [ -z "$NEW_CWD" ]; then exit 0; fi

# Log directory change to session signals
mkdir -p "$HOME/.edikt" 2>/dev/null || true
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
echo "${TIMESTAMP} CWD_CHANGED ${NEW_CWD}" >> "$HOME/.edikt/session-signals.log"

exit 0

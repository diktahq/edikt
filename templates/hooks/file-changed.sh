#!/usr/bin/env bash
# edikt: FileChanged hook — detect external governance file modifications
# Fires when files are modified outside of Claude (e.g., by another editor or git).

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Read hook input from stdin
INPUT=$(cat)

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# Without python3 we cannot even extract file_path, so we cannot know
# whether this change was to a governance file — that uncertainty is exactly
# what must be visible rather than silently discarded (rc=0, no stderr, the
# original failure mode). A constant literal — nothing interpolated.
if ! command -v python3 >/dev/null 2>&1; then
    echo "edikt: file-changed — python3 not found; could not check whether the changed file was a governance file" >&2
    printf '{"systemMessage": "⚠ edikt: python3 is missing — the external-file-change check was skipped. If a governance file (ADR/invariant) changed outside Claude, run /edikt:gov:compile to be sure."}\n'
    exit 0
fi

# Extract changed file path (already safe — python3 extraction)
CHANGED_FILE=$(echo "$INPUT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('file_path',''))" 2>/dev/null || echo "")

if [ -z "$CHANGED_FILE" ]; then exit 0; fi

# Only warn about governance-related files
case "$CHANGED_FILE" in
  *.claude/rules/*|*.edikt/*|*docs/architecture/decisions/*|*docs/architecture/invariants/*)
    TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    mkdir -p "$HOME/.edikt" 2>/dev/null || true
    echo "${TIMESTAMP} FILE_CHANGED_EXTERNAL ${CHANGED_FILE}" >> "$HOME/.edikt/session-signals.log"

    # Surface warning via systemMessage. JSON is built with json.dumps so a
    # file path containing quotes, backslashes, or newlines cannot corrupt
    # the hook-protocol payload or inject keys.
    python3 -c 'import json,sys; print(json.dumps({"systemMessage": f"\u26a0 Governance file modified externally: {sys.argv[1]}. Run /edikt:gov:compile if this affects ADRs or invariants."}))' "$CHANGED_FILE"
    ;;
esac

exit 0

#!/usr/bin/env bash
# edikt: PreToolUse hook (Write|Edit) — block edits that would damage
# governance sentinel blocks (ADR-014 Phase 16); warn if project-context.md
# is missing.
#
# Output format: Claude Code hook protocol JSON
#   - {"systemMessage": "..."} for advisory warnings
#   - {"decision": "block", "reason": "..."} for sentinel-block protection

INPUT=$(cat)

# --- Sentinel block protection ---
# Block edits that would touch the sentinel lines themselves. Users should
# edit the source artifact (ADR/INV/guideline) and run /edikt:gov:compile
# instead of hand-editing compiled directive blocks.
TOOL_NAME=$(printf '%s' "$INPUT" | python3 -c 'import json,sys
try:
    d = json.load(sys.stdin)
    print(d.get("tool_name", ""))
except Exception:
    pass' 2>/dev/null)

if [ "$TOOL_NAME" = "Edit" ] || [ "$TOOL_NAME" = "Write" ]; then
    HITS_SENTINEL=$(printf '%s' "$INPUT" | python3 -c 'import json,re,sys
try:
    d = json.load(sys.stdin)
    ti = d.get("tool_input", {})
    # Edit: check old_string and new_string
    # Write: check content (whole file being written)
    fields = [
        ti.get("old_string", "") or "",
        ti.get("new_string", "") or "",
        ti.get("content", "") or "",
    ]
    pattern = r"\[edikt(:directives)?:(start|end)\]: #"
    for f in fields:
        if re.search(pattern, f):
            print("1")
            break
except Exception:
    pass' 2>/dev/null)
    if [ "$HITS_SENTINEL" = "1" ]; then
        python3 -c 'import json; print(json.dumps({"decision":"block","reason":"edikt sentinel block is auto-generated. Edit the source artifact (ADR/invariant/guideline) and run /edikt:gov:compile instead of hand-editing the compiled block. (ref: ADR-014)"}))'
        exit 0
    fi
fi

# --- Project-context.md advisory ---
if [ -f '.edikt/config.yaml' ] && [ ! -f 'docs/project-context.md' ]; then
    python3 -c 'import json; print(json.dumps({"systemMessage":"⚠ edikt: docs/project-context.md not found. Run /edikt:init to complete setup."}))'
fi

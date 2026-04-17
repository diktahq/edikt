#!/bin/bash
# Test: every templates/hooks/ file is committed as exactly mode 100755.
#
# Why: settings.json invokes hooks as `"command": "${EDIKT_HOOK_DIR}/<hook>.sh"`,
# which Claude Code execs directly. Without the exec bit (100644), every fire
# returns "Permission denied" — most failures are silent (PostToolUse, etc.),
# only Stop/UserPromptSubmit surface UI errors. Over-permissioned files
# (100777) are world-writable and would let any local user substitute hook
# content. Reject both directions of regression.
#
# Coverage MUST match test/integration/install/test_hook_modes_after_install.sh
# (the post-install on-disk gate). The two tests cover the same set of files.
# If you add a hook with a non-.sh extension here, update that test too.
#
# (ref: PLAN-v0.5.0-stability Phase 21)

set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

HOOKS_DIR="$PROJECT_ROOT/templates/hooks"

if [ ! -d "$HOOKS_DIR" ]; then
    fail "templates/hooks/ directory missing at $HOOKS_DIR"
    test_summary
    exit "$FAIL_COUNT"
fi

cd "$PROJECT_ROOT" || {
    fail "cannot cd to $PROJECT_ROOT"
    test_summary
    exit "$FAIL_COUNT"
}

# Capture git ls-files into a tmp file so we can detect a git failure
# explicitly. -z gives NUL-delimited output so paths containing spaces or
# tabs (none today, but future-proof) parse correctly.
ls_tmp=$(mktemp -t edikt-hook-modes-XXXXXX) || {
    fail "could not create tmp file for ls-files output"
    test_summary
    exit "$FAIL_COUNT"
}
trap 'rm -f "$ls_tmp" "$ls_tmp.err"' EXIT INT TERM

if ! git ls-files -sz templates/hooks/ >"$ls_tmp" 2>"$ls_tmp.err"; then
    fail "git ls-files -sz templates/hooks/ failed" "$(cat "$ls_tmp.err" 2>/dev/null)"
    test_summary
    exit "$FAIL_COUNT"
fi

# git ls-files -s output: "<mode> <hash> <stage>\t<path>\0"
# Split by NUL into records, then split each record by TAB into "meta" and "path".
CHECKED=0
while IFS= read -r -d '' record; do
    meta="${record%%	*}"
    path="${record#*	}"
    # meta = "<mode> <hash> <stage>"
    mode="${meta%% *}"
    CHECKED=$((CHECKED + 1))
    if [ "$mode" = "100755" ]; then
        pass "Hook mode 100755: $path"
    else
        # Branch the remediation hint on the observed mode so the message
        # is actually useful — `--chmod=+x` only fixes the 100644 case.
        case "$mode" in
            100644)
                hint="git update-index --chmod=+x $path"
                ;;
            100777|100666|100640|100660|100750|100770)
                hint="chmod 0755 $path && git update-index --chmod=+x $path  # over-permissioned, normalize to 0755"
                ;;
            120000)
                hint="hook is a symlink — convert to a regular file (cp --remove-destination + git add) before chmod"
                ;;
            *)
                hint="see PLAN-v0.5.0-stability Phase 21 for remediation guidance"
                ;;
        esac
        fail "Hook mode WRONG: $path is $mode (expected exactly 100755)" "$hint"
    fi
done < "$ls_tmp"

if [ "$CHECKED" -eq 0 ]; then
    fail "No hooks found via 'git ls-files -sz templates/hooks/' — check repo state"
fi

test_summary

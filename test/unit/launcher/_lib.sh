#!/bin/bash
# Shared setup for launcher unit tests.
#
# Tests run under test/run.sh's Layer 3 sandbox: $HOME, $EDIKT_HOME and
# $CLAUDE_HOME are already redirected into a temp tree. We create a
# fresh per-test EDIKT_ROOT inside that sandbox so each test starts clean.

set -uo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../../.." && pwd)}"
LAUNCHER="$PROJECT_ROOT/bin/edikt"

# shellcheck disable=SC1090
. "$PROJECT_ROOT/test/helpers.sh"

# Per-test EDIKT_ROOT under the sandbox HOME so we never touch the real
# user's tree. Refuses to run if not under a test sandbox — a best-effort
# guard against running these outside ./test/run.sh.
launcher_setup() {
    test_id="${1:-launcher-test}"
    # Require that HOME looks like a sandbox (under /tmp, /var/folders, or
    # matches the EDIKT_HOME/CLAUDE_HOME pattern set by test/run.sh).
    case "${HOME:-}" in
        /tmp/*|/var/folders/*|/private/var/folders/*|/private/tmp/*) ;;
        *)
            echo "launcher_setup: HOME=$HOME is not a sandbox path — run via test/run.sh" >&2
            exit 1
            ;;
    esac
    : "${EDIKT_HOME:=$HOME/.edikt}"
    : "${CLAUDE_HOME:=$HOME/.claude}"
    LAUNCHER_ROOT="${EDIKT_HOME}/${test_id}-$$"
    LAUNCHER_CLAUDE="${CLAUDE_HOME}/${test_id}-$$"
    rm -rf "$LAUNCHER_ROOT" "$LAUNCHER_CLAUDE"
    mkdir -p "$LAUNCHER_ROOT" "$LAUNCHER_CLAUDE"
    export EDIKT_ROOT="$LAUNCHER_ROOT"
    export CLAUDE_HOME="$LAUNCHER_CLAUDE"
}

# Build a minimal valid payload at the given dir with the given version.
make_payload() {
    p="$1"
    v="$2"
    rm -rf "$p"
    mkdir -p "$p/templates" "$p/hooks" "$p/commands/edikt"
    printf '%s\n' "$v" >"$p/VERSION"
    printf '# changelog %s\n' "$v" >"$p/CHANGELOG.md"
    printf '# context\n' >"$p/commands/edikt/context.md"
    printf '#!/bin/sh\necho hi\n' >"$p/hooks/session-start.sh"
    chmod +x "$p/hooks/session-start.sh"
}

run_launcher() {
    "$LAUNCHER" "$@"
}

# Assert a numeric equals-match. Use this instead of the
# `[ x -eq y ] && pass ... || fail ...` idiom, because helpers.sh `pass`
# returns non-zero when PASS_COUNT was 0 (post-increment quirk).
assert_rc() {
    actual="$1"
    expected="$2"
    msg="$3"
    if [ "$actual" = "$expected" ]; then
        pass "$msg"
    else
        fail "$msg" "expected $expected, got $actual"
    fi
}

assert_true() {
    if [ "$1" = "0" ] || eval "$1" >/dev/null 2>&1; then
        :
    fi
}

# Assert a shell test expression is true.
assert_test() {
    msg="$2"
    if eval "[ $1 ]"; then
        pass "$msg"
    else
        fail "$msg" "predicate false: $1"
    fi
}

# Assert a grep match in a string. We deliberately disable pipefail inside
# this helper: at text sizes above the pipe buffer (~64KB on macOS), `grep
# -q` exits as soon as it matches which sends SIGPIPE to `printf`. Under
# `set -o pipefail` the whole pipeline then inherits printf's 141 and the
# assertion fails even though grep matched. Scope the override locally.
assert_grep() {
    pattern="$1"
    text="$2"
    msg="$3"
    (
        set +o pipefail 2>/dev/null || true
        printf '%s\n' "$text" | grep -q -- "$pattern"
    )
    if [ $? -eq 0 ]; then
        pass "$msg"
    else
        fail "$msg" "pattern not found: $pattern"
    fi
}

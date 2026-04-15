#!/bin/bash
# Shared setup for install.sh integration tests.
#
# Runs under test/run.sh's Layer 3 sandbox: $HOME, $EDIKT_HOME, $CLAUDE_HOME
# already redirected. Each test creates a per-test subdir so the sandbox
# can host multiple install.sh runs without interfering.
#
# install.sh is invoked with these env overrides instead of hitting the
# network:
#   EDIKT_LAUNCHER_SOURCE=<path to bin/edikt in this repo>
#   EDIKT_RELEASE_TAG=<tag string>
#   EDIKT_INSTALL_SOURCE=<local payload dir>

set -uo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../../.." && pwd)}"
INSTALL_SH="$PROJECT_ROOT/install.sh"
LAUNCHER_SRC="$PROJECT_ROOT/bin/edikt"

# shellcheck disable=SC1090
. "$PROJECT_ROOT/test/helpers.sh"

# Create a per-test sandbox root under $HOME (the test/run.sh sandbox HOME).
install_setup() {
  test_id="${1:-install-test}"
  case "${HOME:-}" in
    /tmp/*|/var/folders/*|/private/var/folders/*|/private/tmp/*) ;;
    *)
      echo "install_setup: HOME=$HOME is not a sandbox path — run via test/run.sh" >&2
      exit 1
      ;;
  esac
  TEST_HOME="$HOME/${test_id}-$$"
  rm -rf "$TEST_HOME"
  mkdir -p "$TEST_HOME"
  export TEST_EDIKT_ROOT="$TEST_HOME/.edikt"
  export TEST_CLAUDE_HOME="$TEST_HOME/.claude"
  # Set HOME for install.sh so its default global path lands inside the
  # test sandbox. Preserve outer HOME so we can restore later if needed.
  OUTER_HOME="$HOME"
  export HOME="$TEST_HOME"
  # install.sh reads EDIKT_HOME/CLAUDE_HOME optionally; we keep HOME-based
  # defaults so the full resolution path is exercised.
  unset EDIKT_HOME || true
  unset CLAUDE_HOME || true
}

install_teardown() {
  if [ -n "${OUTER_HOME:-}" ]; then
    export HOME="$OUTER_HOME"
  fi
}

# Build a minimal payload directory suitable for `edikt install`.
make_payload() {
  p="$1"
  v="$2"
  rm -rf "$p"
  mkdir -p "$p/templates" "$p/hooks" "$p/commands/edikt"
  printf '%s\n' "$v" > "$p/VERSION"
  printf '# changelog %s\n' "$v" > "$p/CHANGELOG.md"
  printf '# context\n' > "$p/commands/edikt/context.md"
  printf '#!/bin/sh\necho hi\n' > "$p/hooks/session-start.sh"
  chmod +x "$p/hooks/session-start.sh"
}

# Compute sha256 of a tree (sorted path + file hash). Used by the dry-run
# no-mutation test to verify zero disk writes.
tree_sha256() {
  dir="$1"
  if [ ! -d "$dir" ]; then
    echo "EMPTY"
    return 0
  fi
  ( cd "$dir" && find . -print0 2>/dev/null | LC_ALL=C sort -z | xargs -0 -I{} sh -c 'printf "%s\n" "{}"; [ -f "{}" ] && (sha256sum "{}" 2>/dev/null || shasum -a 256 "{}") | awk "{print \$1}"' ) | (sha256sum 2>/dev/null || shasum -a 256) | awk '{print $1}'
}

run_install() {
  # Invoke install.sh with common env overrides. Callers pass additional
  # CLI flags via "$@".
  env \
    HOME="$HOME" \
    EDIKT_LAUNCHER_SOURCE="${EDIKT_LAUNCHER_SOURCE:-$LAUNCHER_SRC}" \
    EDIKT_RELEASE_TAG="${EDIKT_RELEASE_TAG:-}" \
    EDIKT_INSTALL_SOURCE="${EDIKT_INSTALL_SOURCE:-}" \
    bash "$INSTALL_SH" "$@"
}

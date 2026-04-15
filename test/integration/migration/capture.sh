#!/bin/sh
# capture.sh — freeze historical edikt installs into reproducible fixtures.
#
# For each git tag passed on the command line, this script:
#   1. Adds a `git worktree` for that tag at /tmp/edikt-capture-<tag>.
#   2. Creates a sandbox $HOME under a per-tag temp dir.
#   3. Runs that tag's install.sh --global --yes inside the sandbox.
#   4. Rsyncs $HOME/.edikt/ + $HOME/.claude/commands/edikt/ into
#      test/integration/migration/fixtures/<tag>/.
#   5. Sanitizes any $HOME absolute-path references to ${HOME}.
#   6. Zeros all file timestamps (touch -t 197001010000) for byte-stable
#      diffs across re-captures.
#   7. Generates manifest.txt with sha256 per file.
#   8. Removes the worktree and the per-tag sandbox.
#
# Usage:
#   ./capture.sh v0.1.0 v0.1.4 v0.2.0 v0.3.0 v0.4.3
#
# Phase 7a does NOT run this script. Phase 7b owns its execution and
# commits the resulting fixtures. See README.md for the deferral note.
#
# POSIX sh. Errors out on first failure. Idempotent — re-running for
# the same tag overwrites cleanly. macOS-compatible (sed -i '' form).

set -eu

if [ "$#" -eq 0 ]; then
    echo "usage: $0 <tag> [<tag> ...]" >&2
    exit 2
fi

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
FIXTURE_ROOT="$SCRIPT_DIR/fixtures"
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)

# macOS-vs-GNU sed -i compatibility: BSD sed needs '' after -i.
if sed --version >/dev/null 2>&1; then
    SED_INPLACE="sed -i"
else
    SED_INPLACE="sed -i ''"
fi

mkdir -p "$FIXTURE_ROOT"

for tag in "$@"; do
    echo "─── capturing $tag ─────────────────────────────────"

    worktree_dir="/tmp/edikt-capture-$tag"
    sandbox_root=$(mktemp -d -t "edikt-capture-${tag}-XXXXXX")
    fixture_dir="$FIXTURE_ROOT/$tag"

    # Idempotent setup: clean any prior worktree + fixture for this tag.
    if [ -d "$worktree_dir" ]; then
        ( cd "$REPO_ROOT" && git worktree remove --force "$worktree_dir" ) || true
    fi
    rm -rf "$fixture_dir"
    mkdir -p "$fixture_dir/edikt" "$fixture_dir/commands"

    # 1. Worktree the tag.
    ( cd "$REPO_ROOT" && git worktree add "$worktree_dir" "$tag" )

    # 2-3. Sandbox $HOME and run install.sh.
    HOME="$sandbox_root/home"
    export HOME
    mkdir -p "$HOME"
    unset EDIKT_HOME CLAUDE_HOME EDIKT_ROOT 2>/dev/null || true

    if [ ! -f "$worktree_dir/install.sh" ]; then
        echo "no install.sh at tag $tag; skipping" >&2
        ( cd "$REPO_ROOT" && git worktree remove --force "$worktree_dir" ) || true
        rm -rf "$sandbox_root"
        continue
    fi

    # The historical install.sh prompts on tty; --yes was added later.
    # Try --yes first, fall back to piping y.
    if ! ( cd "$worktree_dir" && bash install.sh --global --yes </dev/null ) >/dev/null 2>&1; then
        ( cd "$worktree_dir" && yes | bash install.sh --global ) >/dev/null 2>&1 || {
            echo "install.sh failed for tag $tag" >&2
            ( cd "$REPO_ROOT" && git worktree remove --force "$worktree_dir" ) || true
            rm -rf "$sandbox_root"
            continue
        }
    fi

    # 4. Snapshot the installed state. -a preserves perms; --delete-after
    # ensures the fixture mirrors exactly what's on disk.
    if [ -d "$HOME/.edikt" ]; then
        rsync -a "$HOME/.edikt/" "$fixture_dir/edikt/"
    fi
    if [ -d "$HOME/.claude/commands/edikt" ]; then
        rsync -a "$HOME/.claude/commands/edikt/" "$fixture_dir/commands/"
    fi

    # 5. Sanitize absolute $HOME references → ${HOME} placeholder.
    # Skip binary files (rare in edikt fixtures, but defensive).
    # Escape $HOME for sed's BRE so any regex metacharacters in the path
    # (e.g. '.' on macOS sandbox paths like /private/var/folders/...) don't
    # corrupt the substitution pattern or silently match the wrong text.
    _escape_for_sed() {
        printf '%s\n' "$1" | sed 's/[][\/.^$*]/\\&/g'
    }
    _home_escaped=$(_escape_for_sed "$HOME")
    find "$fixture_dir" -type f | while read -r f; do
        if grep -Iq "$HOME" "$f" 2>/dev/null; then
            # shellcheck disable=SC2086
            $SED_INPLACE "s|${_home_escaped}|\${HOME}|g" "$f"
        fi
    done

    # 6. Zero timestamps for deterministic re-capture diffs.
    find "$fixture_dir" -exec touch -t 197001010000 {} \; 2>/dev/null || true

    # 7. Generate manifest.txt.
    if command -v sha256sum >/dev/null 2>&1; then
        ( cd "$fixture_dir" && find . -type f ! -name manifest.txt | sort | xargs sha256sum > manifest.txt )
    else
        ( cd "$fixture_dir" && find . -type f ! -name manifest.txt | sort | xargs shasum -a 256 > manifest.txt )
    fi
    touch -t 197001010000 "$fixture_dir/manifest.txt" 2>/dev/null || true

    # 8. Cleanup.
    ( cd "$REPO_ROOT" && git worktree remove --force "$worktree_dir" ) || true
    rm -rf "$sandbox_root"

    echo "✓ captured: $fixture_dir"
done

echo ""
echo "Done. Captured fixtures live under $FIXTURE_ROOT/."
echo "Commit them so tests can replay against frozen state."

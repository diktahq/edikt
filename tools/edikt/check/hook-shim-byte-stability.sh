#!/usr/bin/env bash
# hook-shim-byte-stability.sh — PLAN-v080-reliability-core Phase 8, AC-8.8.
#
# NO MIGRATED SHIM MAY DIVERGE FROM THE CANONICAL RENDER BY HAND-EDIT
#
# The comparison itself lives in Go (tools/edikt/internal/hook/shim +
# `bin/edikt hook shim check`), not reimplemented here in bash/python — a
# second copy of "does templates/hooks/<file> match a fresh render of its
# registry row" is a second place it can drift from what actually ships. This
# script's only job is to build a fresh binary from the checked-out source
# (never a stale one a developer left on disk) and run that comparison.
#
# WHY "RENDER INTO A TEMP TREE" RATHER THAN "DIFF THE ALREADY-MIGRATED FILES"
#
# Today (Phase 8) zero rows in templates/hooks/registry.yaml carry
# `status: migrated` — Phase 8 builds the template and generator; Phases
# 12-22 do the migrating, one hook at a time. A check whose subject set was
# "files already marked migrated" would therefore examine ZERO shims and
# exit 0 today — a vacuous pass INV-013 forbids ("a control that observes
# nothing must report that nothing was measured, never that the thing it
# measures passed"). `hook shim check` avoids this by rendering EVERY
# non-library registry row into a temp tree on every run — a real,
# always-nonzero subject count (22 today, library rows excluded) regardless
# of migration progress — and additionally diffs any `status: migrated` row's
# shipped bytes against that same run's fresh render, naming any divergence
# by file.
#
# Usage: tools/edikt/check/hook-shim-byte-stability.sh

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT" || exit 2

command -v go >/dev/null 2>&1 || {
	echo "hook-shim-byte-stability: UNMEASURED — go is not installed; nothing was built or checked." >&2
	exit 2
}

TMPBIN="$(mktemp -t edikt-shim-check.XXXXXX)"
trap 'rm -f "$TMPBIN"' EXIT

echo "hook-shim-byte-stability: building from tools/edikt/ (checked-out source, never a stale binary)..."
if ! (cd "$ROOT/tools/edikt" && go build -o "$TMPBIN" .); then
	echo "hook-shim-byte-stability: UNMEASURED — go build failed; nothing was checked." >&2
	exit 2
fi
chmod +x "$TMPBIN"

# ADR-042: this check builds and runs bin/edikt against edikt-dev's own
# working tree, whose .edikt/config.yaml pin trails the binary it just built
# (dogfooding an in-progress line bump). EDIKT_SKIP_VERSION_GATE=1 is the
# documented bypass for exactly this test/check-harness situation — the same
# one every CI job invoking a version-gated command already sets.
EDIKT_SKIP_VERSION_GATE=1 "$TMPBIN" hook shim check --root "$ROOT"

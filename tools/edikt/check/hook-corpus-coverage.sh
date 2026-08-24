#!/usr/bin/env bash
# hook-corpus-coverage.sh — is this hook's corpus complete enough to migrate?
#
# ADR-063 Decision §2 names four axes a characterization corpus must span —
# payload × config × environment × ordering — and AC-002-2 makes coverage across
# all four the gate on a hook's migration, with no hook exempt. SPEC-014 turns
# that from a reviewer's judgment call into this script: a hook's matrix.yaml is
# its own coverage table, cross-referenced against the finding classes that
# apply to it.
#
# WHY THE FINDING CLASSES ARE CHECKED SEPARATELY FROM THE AXES. Four axes tagged
# is not the same as four axes covered WELL. F-041 is the case that proves it: a
# payload-axis scenario existed for every affected hook, and none of them used a
# file path containing a space, so a total governance bypass sat behind a green
# suite. So a hook whose payload carries a path field must carry a scenario
# CITING F-041, a gate-class hook must carry one citing F-036 (exit codes carry
# protocol meaning stdout comparison cannot see), and a hook whose only healthy
# observable is a side effect must carry one citing F-074.
#
# INV-013. Zero axes examined, zero scenarios read, or a missing matrix are all
# UNMEASURED and exit non-zero. "No gaps found" over nothing is the exact shape
# that invariant exists to forbid — and it is the easy shape to write, because
# every loop over an empty list reports no failures.
#
# ADR-063 Decision §4 carve-out: a `class: library` row (a sourced helper, never
# invoked with a payload) has no corpus BY CONSTRUCTION. Asking for its coverage
# is a category error, and it exits 2 saying so rather than reporting a gap that
# will never close.
#
# Usage:
#   tools/edikt/check/hook-corpus-coverage.sh <hook>
#   tools/edikt/check/hook-corpus-coverage.sh --self-test
#
# Env:
#   EDIKT_HOOK_CORPUS_ROOT   corpus tree (default test/fixtures/hook-corpus)
#   EDIKT_HOOK_REGISTRY      registry file (default templates/hooks/registry.yaml)
#
# Exit: 0 covered · 1 a gap or an unmeasurable subject · 2 bad usage or a
#       category error.

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
CORPUS_ROOT="${EDIKT_HOOK_CORPUS_ROOT:-$ROOT/test/fixtures/hook-corpus}"
REGISTRY="${EDIKT_HOOK_REGISTRY:-$ROOT/templates/hooks/registry.yaml}"

# Hooks whose only observable on a healthy run is a filesystem side effect
# (ADR-063 Decision §2, F-074, widened by SPEC-014's own review). Enumerated
# because the property is about what a hook DOES, which no registry field
# records.
F074_HOOKS="worktree-create task-created event-log session-start session-end"

# Events whose payload carries a path-shaped field, so F-041's spaced-path case
# applies. Derived from the event, the only machine-readable fact about payload
# shape available here.
F041_EVENTS="PreToolUse PostToolUse FileChanged WorktreeCreate WorktreeRemove CwdChanged"

die() { echo "hook-corpus-coverage: $*" >&2; exit 2; }

# registry_field <hook> <field> — read one field off a registry row.
registry_field() {
    awk -v want="$1" -v field="$2" '
        /^[[:space:]]*-[[:space:]]*name:[[:space:]]*/ {
            name = $0
            sub(/^[[:space:]]*-[[:space:]]*name:[[:space:]]*/, "", name)
            gsub(/[[:space:]]*$/, "", name)
            in_row = (name == want)
            next
        }
        in_row {
            line = $0
            sub(/^[[:space:]]+/, "", line)
            if (line ~ ("^" field ":")) {
                sub("^" field ":[[:space:]]*", "", line)
                gsub(/[[:space:]]*$/, "", line)
                print line
                exit
            }
        }
    ' "$REGISTRY"
}

contains_word() {
    case " $2 " in *" $1 "*) return 0 ;; *) return 1 ;; esac
}

# axes_of <matrix> — one "<axis>|<scenario-id>" per line.
axes_of() {
    awk '
        function trim(s) { sub(/^[[:space:]]+/, "", s); sub(/[[:space:]]+$/, "", s); return s }
        {
            line = trim($0)
            if (line ~ /^- id:[[:space:]]*/) {
                id = line; sub(/^- id:[[:space:]]*/, "", id); mode = ""
                next
            }
            if (line ~ /^axis:[[:space:]]*\[/) {
                v = line; sub(/^axis:[[:space:]]*\[/, "", v); sub(/\].*$/, "", v)
                n = split(v, parts, ",")
                for (i = 1; i <= n; i++) print trim(parts[i]) "|" id
                mode = ""
                next
            }
            if (line ~ /^axis:[[:space:]]*$/) { mode = "axis"; next }
            if (mode == "axis" && line ~ /^-[[:space:]]*[a-z-]+$/) {
                v = line; sub(/^-[[:space:]]*/, "", v)
                print v "|" id
                next
            }
            if (line ~ /^[a-z_]+:/) mode = ""
        }
    ' "$1"
}

findings_of() {
    awk '
        function trim(s) { sub(/^[[:space:]]+/, "", s); sub(/[[:space:]]+$/, "", s); return s }
        {
            line = trim($0)
            if (line ~ /^- id:[[:space:]]*/) { id = line; sub(/^- id:[[:space:]]*/, "", id); next }
            if (line ~ /^source_finding:[[:space:]]*/) {
                v = line; sub(/^source_finding:[[:space:]]*/, "", v)
                print trim(v) "|" id
            }
        }
    ' "$1"
}

scenario_count() {
    awk '/^[[:space:]]*-[[:space:]]*id:[[:space:]]*/ { n++ } END { print n + 0 }' "$1"
}

check_hook() {
    local hook="$1" quiet="${2:-0}"
    local class event status matrix
    class=$(registry_field "$hook" class)
    event=$(registry_field "$hook" event)
    status=$(registry_field "$hook" status)

    if [ -z "$class" ]; then
        echo "hook-corpus-coverage: $hook is not a row in $REGISTRY — NOTHING WAS MEASURED." >&2
        return 2
    fi
    if [ "$class" = "library" ] || [ "$event" = "null" ]; then
        echo "hook-corpus-coverage: $hook is class '$class' with event '$event' — a sourced" >&2
        echo "  library has no payload x config x environment x ordering surface (ADR-063" >&2
        echo "  Decision section 4). It has no corpus by construction; this is a category" >&2
        echo "  error, not a coverage gap." >&2
        return 2
    fi

    matrix="$CORPUS_ROOT/$hook/matrix.yaml"
    if [ ! -f "$matrix" ]; then
        echo "hook-corpus-coverage: $hook has no matrix at $matrix." >&2
        echo "  UNMEASURED — a hook with no matrix is not a hook with no gaps (INV-013)." >&2
        return 1
    fi

    local scenarios
    scenarios=$(scenario_count "$matrix")
    if [ "$scenarios" -eq 0 ]; then
        echo "hook-corpus-coverage: $matrix declares zero scenarios — NOTHING WAS MEASURED." >&2
        return 1
    fi

    local pairs findings gaps examined
    pairs=$(axes_of "$matrix")
    findings=$(findings_of "$matrix")
    gaps=0
    examined=0

    [ "$quiet" -eq 1 ] || echo "hook: $hook (class=$class event=$event status=$status, $scenarios scenarios)"
    local axis ids
    for axis in payload config environment ordering; do
        examined=$((examined + 1))
        ids=$(printf '%s\n' "$pairs" | awk -F'|' -v a="$axis" '$1 == a { printf "%s ", $2 }')
        if [ -z "$ids" ]; then
            gaps=$((gaps + 1))
            [ "$quiet" -eq 1 ] || echo "  GAP  axis $axis — no scenario tags it"
        else
            [ "$quiet" -eq 1 ] || echo "  ok   axis $axis — $ids"
        fi
    done

    # Finding classes, each applicable only where its own property holds.
    local f applicable why
    for f in F-041 F-036 F-074; do
        applicable=0
        why=""
        case "$f" in
            F-041) contains_word "$event" "$F041_EVENTS" && { applicable=1; why="payload carries a path-shaped field"; } ;;
            F-036) [ "$class" = "gate" ] && { applicable=1; why="exit code carries protocol meaning beyond 0"; } ;;
            F-074) contains_word "$hook" "$F074_HOOKS" && { applicable=1; why="a filesystem side effect is its only healthy observable"; } ;;
        esac
        if [ "$applicable" -eq 0 ]; then
            [ "$quiet" -eq 1 ] || echo "  n/a  $f"
            continue
        fi
        examined=$((examined + 1))
        ids=$(printf '%s\n' "$findings" | awk -F'|' -v a="$f" '$1 == a { printf "%s ", $2 }')
        if [ -z "$ids" ]; then
            gaps=$((gaps + 1))
            [ "$quiet" -eq 1 ] || echo "  GAP  $f — $why, and no scenario cites it"
        else
            [ "$quiet" -eq 1 ] || echo "  ok   $f — $ids"
        fi
    done

    if [ "$examined" -eq 0 ]; then
        echo "hook-corpus-coverage: examined 0 axes for $hook — NOTHING WAS MEASURED (INV-013)." >&2
        return 1
    fi
    if [ "$gaps" -gt 0 ]; then
        [ "$quiet" -eq 1 ] || echo "  -> $gaps uncovered applicable axis/finding(s) of $examined examined" >&2
        return 1
    fi
    [ "$quiet" -eq 1 ] || echo "  -> covered: $examined/$examined applicable axes and findings"
    return 0
}

# --self-test asserts BOTH directions on committed fixtures.
#
# A check that has only ever been seen to pass is not known to be capable of
# failing (INV-014), and this one is the gate on 22 hook migrations. The fixtures
# are a complete matrix and a deliberately gapped one; the self-test fails if
# either verdict is wrong, including if the "gapped" fixture starts passing.
self_test() {
    local fixtures="$ROOT/tools/edikt/check/testdata/hook-corpus"
    [ -d "$fixtures" ] || die "self-test fixtures missing at $fixtures — NOTHING WAS MEASURED"

    # Point the check at the fixture tree for the duration of the self-test.
    # CORPUS_ROOT is resolved once at load, so a per-call env prefix would not
    # reach it — and the four assertions below would then all be measuring the
    # real corpus tree while claiming to measure the fixtures.
    CORPUS_ROOT="$fixtures"

    local failures=0 checks=0

    checks=$((checks + 1))
    if check_hook "instructions-loaded" 1; then
        echo "  ok   complete matrix (instructions-loaded) passes"
    else
        echo "  FAIL complete matrix (instructions-loaded) was reported as uncovered" >&2
        failures=$((failures + 1))
    fi

    checks=$((checks + 1))
    if check_hook "cwd-changed" 1; then
        echo "  FAIL gapped matrix (cwd-changed, no ordering axis, no F-041) was reported as covered" >&2
        failures=$((failures + 1))
    else
        echo "  ok   gapped matrix (cwd-changed) is reported as uncovered"
    fi

    checks=$((checks + 1))
    if check_hook "event-log" 1 2>/dev/null; then
        echo "  FAIL library row (event-log) was reported as covered" >&2
        failures=$((failures + 1))
    else
        echo "  ok   library row (event-log) is refused as a category error"
    fi

    checks=$((checks + 1))
    if check_hook "post-compact" 1 2>/dev/null; then
        echo "  FAIL a hook with NO matrix was reported as covered" >&2
        failures=$((failures + 1))
    else
        echo "  ok   a hook with no matrix is reported as unmeasured"
    fi

    if [ "$checks" -eq 0 ]; then
        echo "hook-corpus-coverage: self-test ran 0 checks — NOTHING WAS MEASURED (INV-013)." >&2
        return 2
    fi
    if [ "$failures" -gt 0 ]; then
        echo "hook-corpus-coverage: self-test — $failures of $checks assertions failed" >&2
        return 1
    fi
    echo "hook-corpus-coverage: self-test — $checks/$checks assertions hold (pass AND fail directions)"
    return 0
}

[ $# -eq 1 ] || die "usage: $(basename "$0") <hook> | --self-test"
[ -f "$REGISTRY" ] || die "registry not found at $REGISTRY"

case "$1" in
    --self-test) self_test; exit $? ;;
    -*) die "unknown option $1" ;;
esac

check_hook "$1"
exit $?

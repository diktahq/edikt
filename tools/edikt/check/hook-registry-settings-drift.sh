#!/usr/bin/env bash
# hook-registry-settings-drift.sh — SPEC-014 SR-018 reconciliation gate.
#
# THREE PLACES DESCRIBE THE SAME 24 FILES
#
#   templates/hooks/registry.yaml     name, event, file, class, status
#   templates/settings.json.tmpl      which file the host runs on which event
#   test/test-init-registration.sh    which shipped files are exempt from wiring
#
# They must agree, and nothing made them. A registry row with the wrong event
# produces a hook that never fires, and a hook that never fires is
# indistinguishable from a hook with nothing to say — the same invisibility
# this whole migration exists to remove. So the settings template is PARSED,
# live, on every run: no hand-maintained list of "what settings.json contains",
# because a hand-maintained mirror is the thing that drifts.
#
# WHAT IS ASSERTED
#
#   1. Every .sh under templates/hooks/ has exactly one registry row.
#   2. Every registry row's `file` exists.
#   3. Every hook wired in settings.json.tmpl has a row whose `event` matches
#      the settings key it is wired under (including the top-level statusLine
#      key, which is not inside the `hooks:` block).
#   4. Every non-library row is wired in settings.json.tmpl.
#   5. Every library row (`event: null`) is NOT wired — and is present in
#      test-init-registration.sh's exemption enumeration, so the two lists of
#      "files that are not hooks" cannot diverge.
#
# `pre-push` is excluded and named: it is a git hook installed into
# .git/hooks, not a lifecycle hook, and the registry schema's `file` pattern
# requires a .sh suffix so it cannot have a row at all.
#
# INV-013: an empty settings template, an empty registry, or zero shipped hook
# files all exit non-zero. Every comparison here is vacuously true over an
# empty set, so an empty set must never be reported as agreement.
#
# Usage:  tools/edikt/check/hook-registry-settings-drift.sh [--quiet]

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT" || exit 2

QUIET=0
[ "${1:-}" = "--quiet" ] && QUIET=1

REGISTRY="templates/hooks/registry.yaml"
SETTINGS="templates/settings.json.tmpl"
INIT_TEST="test/test-init-registration.sh"

for f in "$REGISTRY" "$SETTINGS" "$INIT_TEST"; do
    if [ ! -f "$f" ]; then
        echo "hook-registry-drift: $f is missing — NOTHING WAS RECONCILED, which is not a pass." >&2
        exit 2
    fi
done

if ! command -v python3 >/dev/null 2>&1; then
    echo "hook-registry-drift: UNMEASURED — python3 is unavailable, so the settings" >&2
    echo "  template could not be parsed. This is not a pass; it is a run that did not" >&2
    echo "  happen (INV-013). Install python3 or run this gate in CI." >&2
    exit 2
fi
if ! python3 -c 'import yaml' 2>/dev/null; then
    echo "hook-registry-drift: UNMEASURED — PyYAML is unavailable, so the registry could" >&2
    echo "  not be parsed. Not a pass. pip install pyyaml." >&2
    exit 2
fi

python3 - "$REGISTRY" "$SETTINGS" "$INIT_TEST" "$QUIET" <<'PY'
import glob, json, os, re, sys
import yaml

registry_path, settings_path, init_test_path, quiet = sys.argv[1:5]
quiet = quiet == "1"

failures = []

reg = yaml.safe_load(open(registry_path))
rows = (reg or {}).get("hooks") or []
if not rows:
    print("hook-registry-drift: registry declares zero hooks — nothing to reconcile,", file=sys.stderr)
    print("  which is a broken registry, not an agreeing one.", file=sys.stderr)
    sys.exit(2)

# The settings template carries ${EDIKT_HOOK_DIR} placeholders but is otherwise
# real JSON. Parsed, never regex-scraped: a regex over JSON is its own drift.
settings = json.load(open(settings_path))

wired = {}   # basename -> set of event keys it is registered under
for event, groups in (settings.get("hooks") or {}).items():
    for group in groups:
        for hk in group.get("hooks", []):
            base = os.path.basename(hk.get("command", ""))
            wired.setdefault(base, set()).add(event)
sl = settings.get("statusLine")
if isinstance(sl, dict) and sl.get("command"):
    wired.setdefault(os.path.basename(sl["command"]), set()).add("statusLine")

if not wired:
    print("hook-registry-drift: settings template wires zero hooks — an empty template", file=sys.stderr)
    print("  agrees with anything.", file=sys.stderr)
    sys.exit(2)

shipped = {os.path.basename(p) for p in glob.glob("templates/hooks/*.sh")}
if not shipped:
    print("hook-registry-drift: zero .sh files under templates/hooks/ — nothing scanned.", file=sys.stderr)
    sys.exit(2)

# test-init-registration.sh's HELPERS dict names every shipped file it does not
# require to be wired. Read live, so the two enumerations cannot diverge.
init_body = open(init_test_path).read()
helpers = set(re.findall(r'^\s*"([A-Za-z0-9_.-]+)":\s*"', init_body, re.M))
if not helpers:
    failures.append("could not read any exemption entries from %s — the reconciliation "
                    "against its enumeration measured nothing" % init_test_path)

by_file = {}
for r in rows:
    by_file.setdefault(r["file"], []).append(r)

# 1 + 2: one row per shipped file, and every declared file exists.
for f in sorted(shipped):
    n = len(by_file.get(f, []))
    if n == 0:
        failures.append("templates/hooks/%s ships but has no registry row — it is invisible "
                        "to the shim generator, the parity gate and the corpus recorder" % f)
    elif n > 1:
        failures.append("templates/hooks/%s has %d registry rows; exactly one is required" % (f, n))
for f in sorted(by_file):
    if not os.path.exists(os.path.join("templates", "hooks", f)):
        failures.append("registry declares file %s which does not exist under templates/hooks/" % f)

# 3 + 4 + 5: events reconcile with the live settings parse.
for r in rows:
    f, name, ev, cls = r["file"], r["name"], r.get("event"), r["class"]
    events = wired.get(f, set())
    if cls == "library":
        if ev is not None:
            failures.append("row %s is class library but declares event %r — a sourced helper "
                            "is never registered" % (name, ev))
        if events:
            failures.append("row %s is class library but settings.json.tmpl wires it under %s"
                            % (name, ", ".join(sorted(events))))
        if f not in helpers:
            failures.append("row %s is class library but %s does not list %s as an exempt, "
                            "non-hook file — two lists of 'what is not a hook' that disagree"
                            % (name, init_test_path, f))
        continue
    if ev is None:
        failures.append("row %s is class %s and declares no event; a row with none can never fire"
                        % (name, cls))
        continue
    if not events:
        failures.append("row %s declares event %s but settings.json.tmpl wires %s nowhere — "
                        "the hook is declared and never registered" % (name, ev, f))
    elif ev not in events:
        failures.append("row %s declares event %s but settings.json.tmpl wires %s under %s — "
                        "the row describes a hook that never fires"
                        % (name, ev, f, ", ".join(sorted(events))))

# Anything wired that the registry does not know about at all.
declared_files = set(by_file)
for f in sorted(wired):
    if f not in declared_files:
        failures.append("settings.json.tmpl wires %s but no registry row declares it" % f)

if failures:
    print("hook-registry-drift: %d disagreement(s) across %d row(s), %d wired hook(s), "
          "%d shipped file(s):" % (len(failures), len(rows), len(wired), len(shipped)), file=sys.stderr)
    for msg in failures:
        print("  - %s" % msg, file=sys.stderr)
    sys.exit(1)

if not quiet:
    lib = sum(1 for r in rows if r["class"] == "library")
    print("hook-registry-drift: %d registry row(s) reconciled against %d wired hook(s) in "
          "%s, %d shipped .sh file(s) and %d exemption entr(ies) in %s; 0 disagreements "
          "(%d library row(s) correctly unwired)."
          % (len(rows), len(wired), settings_path, len(shipped), len(helpers), init_test_path, lib))
PY
rc=$?
exit $rc

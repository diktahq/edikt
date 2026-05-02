#!/usr/bin/env bash
# edikt: Stop hook — detect signals in the last assistant response and surface them
# as a non-blocking systemMessage shown to the user.
#
# Uses regex pattern matching — no API key required.
# Outputs {"systemMessage": "..."} for signals, {"continue": true} when clean.

set -uo pipefail

# Only run in edikt projects
if [ ! -f '.edikt/config.yaml' ]; then exit 0; fi
if grep -q 'signal-detection: false' .edikt/config.yaml 2>/dev/null; then exit 0; fi

# Anchor the project root to the realpath of cwd at script start, then
# pass it to the Python heredocs as EDIKT_PROJECT_ROOT. The drift-detect
# block writes .edikt/state/stale-sidecars.log; without an explicit anchor,
# a relative path resolves against whatever cwd Claude Code reports at
# the moment of the Stop event — which can drift mid-session if Claude
# emits CwdChanged. Capturing once at script entry pins the path.
EDIKT_PROJECT_ROOT="$(pwd -P)"
export EDIKT_PROJECT_ROOT

# Prevent infinite loops — stop_hook_active means we're already in a continuation
INPUT=$(cat)
STOP_HOOK_ACTIVE=$(echo "$INPUT" | python3 -c "
import json, sys
d = json.load(sys.stdin)
print('true' if d.get('stop_hook_active') else 'false')
" 2>/dev/null || echo "false")

if [ "$STOP_HOOK_ACTIVE" = "true" ]; then exit 0; fi

# Extract the last assistant message
LAST_MSG=$(echo "$INPUT" | python3 -c "
import json, sys
d = json.load(sys.stdin)
print(d.get('last_assistant_message', '').strip())
" 2>/dev/null || echo "")

if [ -z "$LAST_MSG" ]; then exit 0; fi

# ─── Signal detection (regex-based, no API key required) ──────────────────────

SIGNALS=()

# ARCHITECTURE: explicit trade-off language or "chose X over Y" patterns
if echo "$LAST_MSG" | grep -qiE \
    'chose .+ over |trade.?off|architectural (decision|constraint|choice)|going forward .*(all|every|must)|hard (constraint|rule|requirement)|ADR|decision record'; then
    SIGNALS+=("💡 ADR candidate — run /edikt:adr:new to capture this decision.")
fi

# DOC GAP: new HTTP routes or env vars added.
# Per audit HI-5: we detect presence-of-signal only and emit a STATIC suggestion.
# The matched substring is never embedded in the signal text — an attacker-controlled
# file containing "POST /admin/delete-everything" cannot influence the suggestion's
# wording, which would otherwise bias the user toward capturing attacker-framed
# work via /edikt:docs:review.
NEW_ROUTES=$(echo "$LAST_MSG" | grep -oiE '(POST|GET|PUT|DELETE|PATCH) /[a-zA-Z0-9/_:.-]+' | head -1)
NEW_ENV=$(echo "$LAST_MSG" | grep -oE '(added|new|required|Added|New|Required).{0,30}[A-Z][A-Z0-9_]{3,}[A-Z0-9]' | grep -v 'ADR\|ARCH\|HTTP\|API\|JSON\|HTML\|CSS' | head -1)

if [ -n "$NEW_ROUTES" ]; then
    SIGNALS+=("📄 New HTTP route referenced — consider /edikt:docs:review to check documentation.")
elif [ -n "$NEW_ENV" ]; then
    SIGNALS+=("📄 New environment variable referenced — consider /edikt:docs:review to check documentation.")
fi

# SECURITY: auth/payments/PII/crypto was the central focus
if echo "$LAST_MSG" | grep -qiE \
    '(JWT|OAuth|PKCE|auth[a-z]*|payment|PII|encrypt|decrypt|secret|signing key|private key|bearer token|bcrypt|password hash)'; then
    # Only flag if it's a substantive change (multiple security terms or central to the response)
    SEC_COUNT=$(echo "$LAST_MSG" | grep -ioE '(JWT|OAuth|PKCE|auth[a-z]*|payment|PII|encrypt|decrypt|secret|signing key|private key|bearer token|bcrypt|password hash)' | wc -l | tr -d ' ')
    if [ "$SEC_COUNT" -ge 2 ]; then
        SIGNALS+=("🔒 Security-sensitive change — run /edikt:sdlc:audit before shipping.")
    fi
fi

# ─── Dedup: check if architecture signal already exists as an ADR ──────────────

BASE=$(grep '^base:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"' || echo "docs")
[ -z "$BASE" ] && BASE="docs"

if [ ${#SIGNALS[@]} -gt 0 ]; then
    FILTERED=()
    for SIGNAL in "${SIGNALS[@]}"; do
        SKIP=false
        # For ADR candidates, check if a similar decision already exists
        if echo "$SIGNAL" | grep -q "ADR candidate"; then
            # Extract key terms from the last message's decision language
            DECISION_TERMS=$(echo "$LAST_MSG" | grep -ioE 'chose [a-z]+ over [a-z]+|trade.?off.{0,40}' | head -1 | tr '[:upper:]' '[:lower:]')
            if [ -n "$DECISION_TERMS" ]; then
                # Check existing ADR titles for similar terms
                for adr_dir in "$BASE/decisions" "$BASE/architecture/decisions"; do
                    if [ -d "$adr_dir" ]; then
                        for adr_file in "$adr_dir"/*.md; do
                            [ ! -f "$adr_file" ] && continue
                            ADR_TITLE=$(head -1 "$adr_file" | tr '[:upper:]' '[:lower:]')
                            # Check if key terms from the decision overlap with ADR title
                            for term in $(echo "$DECISION_TERMS" | tr -s '[:space:]' '\n' | grep -vE '^(chose|over|the|a|an|to|for|is|was)$'); do
                                if echo "$ADR_TITLE" | grep -qiF "$term" 2>/dev/null; then
                                    SKIP=true
                                    break 2
                                fi
                            done
                        done
                    fi
                done
            fi
        fi
        if [ "$SKIP" = false ]; then
            FILTERED+=("$SIGNAL")
        fi
    done
    SIGNALS=("${FILTERED[@]+"${FILTERED[@]}"}")
fi

# ─── Sidecar drift detection (Phase 7b — ADR-027/028) ─────────────────────────
# Walks the configured artifact dirs, parses each <artifact>.edikt.yaml, and
# checks whether every directive's source_excerpt.quote still appears at its
# declared line range in the parent .md. If any quote is missing, the sidecar
# is "stale" — Claude likely edited the prose without regenerating the sidecar.
#
# Output contract (INV-004):
#   - The systemMessage we emit carries a FIXED template plus the cardinality
#     of stale artifacts (an int — not attacker-influenceable text). Filenames
#     and excerpts are NEVER interpolated into Claude-facing channels.
#   - The full artifact-ID list is written to .edikt/state/stale-sidecars.log
#     for /edikt:gov:compile to consume out-of-band.
#
# Soft-degrade: if PyYAML is unavailable, the drift check is skipped silently.
# Existing signals still emit; we never block the stop event on this check.
STALE_COUNT=$(python3 - <<'PYEOF'
import json, os, sys
from pathlib import Path

# Stop-hook used to call yaml.safe_load with PyYAML for BOTH the paths
# config AND the per-sidecar load. The paths config was a divergence from
# pre-tool-use.sh's hardened stdlib parser (§3.3 path-traversal +
# §3.4 YAML quirks safe-fail-closed). The two hooks share the same
# `paths:` config and any shape one accepts but the other rejects is a
# correctness bug (one blocks while the other reports clean). The paths
# parser is now stdlib-only (mirrors pre-tool-use); PyYAML stays for the
# per-sidecar load below, with the original soft-degrade on missing lib.
try:
    import yaml
except ImportError:
    print(0)
    sys.exit(0)

DEFAULTS = {
    "decisions":  "docs/architecture/decisions",
    "invariants": "docs/architecture/invariants",
    "guidelines": "docs/guidelines",
}


def _governance_paths_dict() -> dict:
    """Hardened stdlib parser mirroring pre-tool-use.sh:_governance_paths.

    Returns a dict keyed by category. Falls back to DEFAULTS on any
    quirk shape the hand-parser cannot safely interpret (multi-doc
    separator, flow-style, quoted-with-colon) or any value that
    fails §3.3 (traversal / absolute path / realpath-escape) checks.
    Safe-fail-closed: a single bad entry poisons the whole config
    rather than leaving a legitimate governance dir uncovered.
    """
    paths = dict(DEFAULTS)
    try:
        with open(".edikt/config.yaml", "r", encoding="utf-8") as fh:
            text = fh.read()
    except (OSError, FileNotFoundError):
        return paths

    # §3.4 — quirk shapes.
    for line in text.splitlines():
        stripped_line = line.strip()
        if stripped_line == "---":
            return dict(DEFAULTS)
        if stripped_line.startswith("paths:") and (
            "{" in stripped_line or "[" in stripped_line
        ):
            return dict(DEFAULTS)

    parsed = {}
    in_paths = False
    for raw in text.splitlines():
        line = raw.rstrip("\r")
        if not in_paths:
            if line.startswith("paths:"):
                in_paths = True
            continue
        if line and not line.startswith((" ", "\t")):
            break
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if ":" in stripped:
            key_part, value = stripped.split(":", 1)
            key = key_part.strip()
            value = value.strip()
            # §3.4 — reject quoted-with-colon.
            if (value.startswith('"') and value.endswith('"')) or (
                value.startswith("'") and value.endswith("'")
            ):
                inner = value[1:-1]
                if ":" in inner:
                    return dict(DEFAULTS)
                value = inner
            # §3.4 — flow-style value.
            if value.startswith("{") or value.startswith("["):
                return dict(DEFAULTS)
            if not value:
                continue
            # §3.3 — traversal / absolute.
            if ".." in value.split("/") or value.startswith("/"):
                return dict(DEFAULTS)
            if key in DEFAULTS:
                parsed[key] = value

    # §3.3 — realpath escape check on any candidate.
    cwd_real = os.path.realpath(os.getcwd())
    for k, v in parsed.items():
        candidate = os.path.normpath(os.path.join(cwd_real, v))
        try:
            cand_real = os.path.realpath(candidate)
        except OSError:
            return dict(DEFAULTS)
        if cand_real != cwd_real and not cand_real.startswith(cwd_real + os.sep):
            return dict(DEFAULTS)
        paths[k] = v
    return paths


paths = _governance_paths_dict()

def is_stale(sidecar, body_lines):
    directives = sidecar.get("directives") or []
    if not isinstance(directives, list):
        return False
    for d in directives:
        if not isinstance(d, dict):
            continue
        src = d.get("source_excerpt") or {}
        if not isinstance(src, dict):
            continue
        ls = src.get("line_start", 0)
        le = src.get("line_end", 0)
        quote = src.get("quote", "")
        if not isinstance(ls, int) or not isinstance(le, int):
            continue
        if not isinstance(quote, str):
            continue
        quote = quote.strip()
        if not quote or ls < 1 or le < ls:
            continue
        if ls > len(body_lines) or le > len(body_lines):
            return True
        passage = "\n".join(body_lines[ls-1:le])
        if quote not in passage:
            return True
    return False

def artifact_id(name):
    base = name[:-len(".edikt.yaml")]
    if base.startswith(("ADR-", "INV-")):
        end = 4
        while end < len(base) and base[end].isdigit():
            end += 1
        return base[:end]
    return base

stale_ids = []
seen = set()

for d in paths.values():
    if not os.path.isdir(d):
        continue
    try:
        entries = sorted(os.listdir(d))
    except OSError:
        continue
    for entry in entries:
        if not entry.endswith(".edikt.yaml"):
            continue
        sidecar_path = os.path.join(d, entry)
        md_path = os.path.join(d, entry[:-len(".edikt.yaml")] + ".md")
        if not os.path.isfile(md_path):
            continue
        try:
            with open(sidecar_path, "r", encoding="utf-8") as f:
                sidecar = yaml.safe_load(f)
        except Exception:
            continue
        if not isinstance(sidecar, dict):
            continue
        try:
            with open(md_path, "r", encoding="utf-8") as f:
                body_lines = f.read().split("\n")
        except Exception:
            continue
        if is_stale(sidecar, body_lines):
            aid = artifact_id(entry)
            if aid not in seen:
                seen.add(aid)
                stale_ids.append(aid)

# Anchor the log path to the project root captured by bash at script
# entry. Defensive realpath check refuses to write outside the project
# root in the unlikely event that EDIKT_PROJECT_ROOT was tampered with
# (e.g. symlink swap mid-script).
project_root = Path(os.environ.get("EDIKT_PROJECT_ROOT", "")).resolve() if os.environ.get("EDIKT_PROJECT_ROOT") else Path.cwd().resolve()
log_path = project_root / ".edikt" / "state" / "stale-sidecars.log"
try:
    log_path.parent.resolve(strict=False).relative_to(project_root)
except ValueError:
    print(len(stale_ids))
    sys.exit(0)

if stale_ids:
    try:
        log_path.parent.mkdir(parents=True, exist_ok=True)
        log_path.write_text("\n".join(stale_ids) + "\n", encoding="utf-8")
    except OSError:
        pass
else:
    try:
        log_path.unlink()
    except FileNotFoundError:
        pass
    except OSError:
        pass

print(len(stale_ids))
PYEOF
)

case "${STALE_COUNT:-0}" in
    ''|*[!0-9]*) STALE_COUNT=0 ;;
esac

if [ "$STALE_COUNT" -gt 0 ]; then
    # Build the warning via python: the count is an int rendered into a
    # single static template. No shell interpolation reaches the JSON body
    # (the INV-004 grep lint in test/security/lints.sh stays clean).
    DRIFT_MSG=$(python3 -c 'import sys; n=int(sys.argv[1]); print("⚠ Some artifacts have stale sidecars. Run /edikt:gov:compile to resync. Affected: " + str(n))' "$STALE_COUNT")
    SIGNALS+=("$DRIFT_MSG")
fi

# ─── Output ───────────────────────────────────────────────────────────────────

if [ ${#SIGNALS[@]} -eq 0 ]; then
    echo '{"continue": true}'
    exit 0
fi

# Log signals to session file so /edikt:status can show them
LOG_FILE="$HOME/.edikt/session-signals.log"
mkdir -p "$HOME/.edikt" 2>/dev/null || true
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)
for SIGNAL in "${SIGNALS[@]}"; do
    echo "${TIMESTAMP} ${SIGNAL}" >> "$LOG_FILE" 2>/dev/null || true
done

python3 - "${SIGNALS[@]}" <<'PYEOF'
import json, sys
signals = sys.argv[1:]
msg = "\n".join(signals)
print(json.dumps({"systemMessage": msg}))
PYEOF

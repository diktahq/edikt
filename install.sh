#!/usr/bin/env bash
set -euo pipefail
# edikt installer — v0.6.0 thin bootstrap.
#
# This script is intentionally small. All heavy lifting (payload fetch,
# checksum verification, versioned layout, migration from flat layout,
# symlink chain, lock.yaml) lives in the launcher at $EDIKT_ROOT/bin/edikt.
#
# install.sh's job:
#   1. Parse flags (--project | --global | --dry-run | --ref <tag>).
#   2. Detect installed state: fresh | legacy_v04 | current_v05.
#   3. Fetch the Go edikt binary for the requested release (curl).
#   4. Install binary atomically, add to $PATH idempotently.
#   5. Delegate to `edikt migrate` (if legacy) and `edikt install/use`.
#
# Canonical v0.4.x → v0.6.0 cross-major upgrade path. /edikt:upgrade
# redirects here for major-version jumps (Phase 6).
#
# Verifying THIS script before running it:
#   As of 2026-08-07 install.sh is published as a Release asset and its
#   SHA-256 is included in the Sigstore-signed SHA256SUMS, so it can be
#   checked like any other artifact instead of being piped straight into a
#   shell. `curl … | bash` cannot verify what it is about to execute — that
#   is inherent to the one-liner, not a property of this script — so a
#   careful adopter downloads first, verifies, then runs:
#
#     TAG=v0.6.0
#     base=https://github.com/diktahq/edikt/releases/download/$TAG
#     curl -fsSLO $base/install.sh
#     curl -fsSLO $base/SHA256SUMS
#     curl -fsSLO $base/SHA256SUMS.sig.bundle
#     cosign verify-blob --bundle SHA256SUMS.sig.bundle \
#       --certificate-identity-regexp '^https://github\.com/diktahq/edikt/' \
#       --certificate-oidc-issuer https://token.actions.githubusercontent.com \
#       SHA256SUMS
#     grep ' install.sh$' SHA256SUMS | sha256sum -c -
#     bash install.sh
#
#   See docs/guides/release-runbook.md. Note this applies from the first
#   release that carries install.sh as an asset; earlier tags do not.
#
# Exit codes:
#   0 success
#   1 network error (launcher or release-tag fetch failed)
#   2 permission error ($EDIKT_ROOT not writable, rc append refused)
#   3 version mismatch (requested tag older than currently installed)
#
# Usage (install URLs MUST be tag-pinned, never branch-tracking):
#   curl -fsSL https://github.com/diktahq/edikt/releases/download/v0.6.0/install.sh | bash
#   curl -fsSL ...install.sh | bash -s -- --project
#   curl -fsSL ...install.sh | bash -s -- --dry-run
#   curl -fsSL ...install.sh | bash -s -- --ref v0.6.1
#
# Test overrides (for test/integration/install/):
#   EDIKT_LAUNCHER_SOURCE=<path>  — local launcher file; skip curl
#   EDIKT_RELEASE_TAG=<tag>       — skip GitHub API, use this tag
#   EDIKT_INSTALL_SOURCE=<path>   — forwarded to `edikt install`
#   EDIKT_LAUNCHER_SHA256=<hex>   — pin launcher checksum (takes precedence over SHA256SUMS lookup)
#   EDIKT_INSTALL_INSECURE=1      — skip cosign verification (not recommended; loud banner on exit)
#   EDIKT_INSTALL_REPO=<owner/repo> — retarget BOTH the download URL and the cosign
#                                     --certificate-identity-regexp to a different
#                                     repo's release pipeline (e.g. diktahq/edikt-dev,
#                                     to test a staging release candidate before it is
#                                     promoted to diktahq/edikt). This RETARGETS
#                                     verification — it does NOT disable it, the
#                                     opposite of EDIKT_INSTALL_INSECURE=1. Defaults to
#                                     diktahq/edikt; unset behavior is unchanged.

umask 0022

# Validates EDIKT_INSTALL_REPO before it flows into a URL AND into the cosign
# identity check (INV-006) — this value is doubly security-relevant: an
# unvalidated one could both compose a malicious URL and widen the identity
# regex to match more than the intended repo. Restricted to GitHub's own
# owner/repo character set (letters, digits, '.', '_', '-') on both sides of
# exactly one '/'.
#
# TWO PASSES, deliberately, matching validate_tag's proven shape above rather
# than a single combined pattern — a single pass here was tried and was
# genuinely exploitable: glob '*' does not quantify the preceding bracket
# class the way a regex '+'/'*' would; it is an independent unconstrained
# wildcard. `[A-Za-z0-9_.-]*` therefore does NOT mean "one-or-more chars from
# the class" — it means "one char from the class, then match anything at
# all." `diktahq/edikt; rm -rf /` matched a single-pass version of this
# check, warning printed and all, and only failed later when the resulting
# URL hit the network. Caught by testing the validator against a malicious
# value before trusting it, not by reasoning about the pattern. Pass 1
# rejects any forbidden character outright; pass 2 rejects the shapes pass 1
# still allows through (multiple slashes, leading/trailing slash, '..').
validate_repo() {
  _vr_val="$1"
  [ -n "$_vr_val" ] || return 0
  case "$_vr_val" in
    *[!A-Za-z0-9_./-]*)
      echo "error: EDIKT_INSTALL_REPO must contain only letters, digits, '.', '_', '-', and exactly one '/' (got: $_vr_val)" >&2
      exit 2
      ;;
  esac
  case "$_vr_val" in
    */*/*|/*|*/|*..*)
      echo "error: EDIKT_INSTALL_REPO must be exactly <owner>/<repo> — no leading/trailing '/', no extra '/', no '..' (got: $_vr_val)" >&2
      exit 2
      ;;
  esac
}
validate_repo "${EDIKT_INSTALL_REPO:-}"

REPO="${EDIKT_INSTALL_REPO:-diktahq/edikt}"
RAW_BASE="https://raw.githubusercontent.com/${REPO}"
API_BASE="https://api.github.com/repos/${REPO}"

# Exit codes
EX_OK=0
EX_NETWORK=1
EX_PERMISSION=2
EX_VERSION=3
EX_GENERAL=4

# ─── Flag parsing ───────────────────────────────────────────────────────────
INSTALL_MODE=""
DRY_RUN=false
REF_TAG=""

while [ $# -gt 0 ]; do
  case "$1" in
    --project)   INSTALL_MODE="project" ;;
    --global)    INSTALL_MODE="global" ;;
    --dry-run)   DRY_RUN=true ;;
    --ref)       shift; REF_TAG="${1:-}" ;;
    --ref=*)     REF_TAG="${1#--ref=}" ;;
    -h|--help)
      sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "error: unknown flag: $1" >&2
      echo "Usage: install.sh [--global|--project] [--dry-run] [--ref <tag>]" >&2
      exit 2
      ;;
  esac
  shift
done

# Validate any tag before it flows into a URL or git ref (closes audit MED-9).
# A tag of `../../../etc/passwd` would compose into the launcher URL.
#
# ONE validator, two call sites. --ref was gated here from the start while
# EDIKT_RELEASE_TAG reached resolve_tag and then URL composition with no check
# at all — an INV-006 gap in shipped code: "any value that originates outside
# edikt's immediate control, including environment variables, MUST be
# validated against an allowlist before it is interpolated into a URL".
#
# Validated rather than removed because the variable is load-bearing: four
# launcher and integration suites use it to bypass the GitHub API. Removing it
# would trade a security gap for a testability gap. Copying the checks would
# have been a second predicate for one rule — the divergence class this repo
# has spent the week removing — so the shape lives in one function.
validate_tag() {
  _vt_val="$1"; _vt_src="$2"
  [ -n "$_vt_val" ] || return 0
  case "$_vt_val" in
    v[0-9]*.[0-9]*.[0-9]*|[0-9]*.[0-9]*.[0-9]*) ;;
    v[0-9]*.[0-9]*.[0-9]*-*|[0-9]*.[0-9]*.[0-9]*-*) ;;
    *)
      echo "error: $_vt_src must match ^v?[0-9]+.[0-9]+.[0-9]+(-[A-Za-z0-9.-]+)? (got: $_vt_val)" >&2
      exit 2
      ;;
  esac
  case "$_vt_val" in
    *..*|*' '*|*$'\n'*|*$'\t'*|*/*|*\\*)
      echo "error: $_vt_src contains forbidden characters (got: $_vt_val)" >&2
      exit 2
      ;;
  esac
}

validate_tag "${REF_TAG:-}" "--ref"
validate_tag "${EDIKT_RELEASE_TAG:-}" "EDIKT_RELEASE_TAG"

# ─── Colors / logging ───────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
DIM='\033[2m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { printf '%b> %b %s\n' "$GREEN" "$RESET" "$1"; }
dim()   { printf '%b  %s%b\n' "$DIM" "$1" "$RESET"; }
warn()  { printf '%b!%b %s\n' "$YELLOW" "$RESET" "$1" >&2; }
error() { printf '%berror:%b %s\n' "$RED" "$RESET" "$1" >&2; }

# EDIKT_INSTALL_REPO retargets verification (download URL AND cosign identity
# together — see the REPO/validate_repo block above); it does not disable it.
# That distinction is why this warns rather than staying silent like a normal
# config default: a reader scanning stderr for "something unusual is
# happening" should see this next to EDIKT_INSTALL_INSECURE=1, not confuse
# the two.
if [ -n "${EDIKT_INSTALL_REPO:-}" ]; then
  warn "EDIKT_INSTALL_REPO override active: $EDIKT_INSTALL_REPO"
  warn "verification is RETARGETED to this repo's signing identity, not disabled — the opposite of EDIKT_INSTALL_INSECURE=1"
fi

# ─── Dependency check ───────────────────────────────────────────────────────
command -v curl >/dev/null 2>&1 || { error "curl is required"; exit $EX_NETWORK; }
# F-060: this installer writes settings.json (line ~750) and the settings
# sidecar (line ~800) via bare `python3` heredocs with no preflight — the
# only failure a user ever saw was "settings.json substitution failed —
# hook paths not written", naming the symptom and never the cause. Not a
# new dependency: python3 was already silently required by that code.
# python3 has not shipped with macOS since 12.3 and is absent from most
# slim containers; checked here, before either write, so the actual
# missing dependency is what the user reads.
command -v python3 >/dev/null 2>&1 || { error "python3 is required (used to safely write settings.json — see F-060)"; exit $EX_GENERAL; }

# ─── Interactive mode prompt (preserved from v0.4.x) ────────────────────────
# Use /dev/tty so `curl | bash` still gets a prompt. Fall back to global.
detect_existing_for_prompt() {
  HAS_GLOBAL=false
  HAS_PROJECT=false
  [ -e "${HOME}/.edikt/VERSION" ] && HAS_GLOBAL=true
  [ -e "${HOME}/.edikt/bin/edikt" ] && HAS_GLOBAL=true
  [ -d "${HOME}/.claude/commands/edikt" ] && HAS_GLOBAL=true
  [ -e ".edikt/VERSION" ] && HAS_PROJECT=true
  [ -e ".edikt/bin/edikt" ] && HAS_PROJECT=true
  [ -d ".claude/commands/edikt" ] && HAS_PROJECT=true
  # REQUIRED under `set -e`. Every line above is an `&&` list whose status is
  # the last command's: when the final test is FALSE the list returns 1, the
  # function returns 1, and set -e kills the script at the call site — before
  # the banner, before the tty check, before any output at all. rc=1, zero
  # bytes, on a genuinely fresh machine with none of the six markers present.
  #
  # It survived every install test because those tests create the marker dirs
  # in setup, so the last test was always TRUE and the function always
  # returned 0. The bug was reachable only by the one input the suite never
  # supplied: a clean machine.
  return 0
}

if [ -z "$INSTALL_MODE" ]; then
  detect_existing_for_prompt
  if [ -r /dev/tty ] && [ -w /dev/tty ]; then
    {
      echo ""
      echo "  Where should edikt be installed?"
      echo ""
      echo "  [1] Global (default) — available in all projects"
      echo "  [2] Project only     — installed in current directory"
      echo ""
      if $HAS_GLOBAL; then
        echo "  Note: a global edikt install already exists at ~/.edikt."
        echo ""
      fi
      if $HAS_PROJECT; then
        echo "  Note: a project-local edikt install already exists in .edikt/."
        echo ""
      fi
      printf "  Choice [1]: "
    } > /dev/tty
    read -r choice < /dev/tty || choice=""
    case "$choice" in
      2) INSTALL_MODE="project" ;;
      *) INSTALL_MODE="global" ;;
    esac
  else
    INSTALL_MODE="global"
  fi
fi

# ─── Root resolution ────────────────────────────────────────────────────────
# Matches the Go edikt binary's EDIKT_ROOT resolution so both see the same layout.
PROJECT_ROOT=""
if [ "$INSTALL_MODE" = "project" ]; then
  PROJECT_ROOT="$(pwd)"
  EDIKT_ROOT="$(pwd)/.edikt"
  CLAUDE_HOME_DIR="$(pwd)/.claude"
else
  EDIKT_ROOT="${EDIKT_HOME:-${HOME}/.edikt}"
  CLAUDE_HOME_DIR="${CLAUDE_HOME:-${HOME}/.claude}"
fi

# Hook directory used to substitute ${EDIKT_HOOK_DIR} in settings.json.tmpl.
# Global mode: $HOME/.edikt/hooks  (stable path via symlink chain)
# Project mode: <project-root>/.edikt/hooks  (absolute, project-scoped)
if [ "$INSTALL_MODE" = "project" ]; then
  EDIKT_HOOK_DIR="${PROJECT_ROOT}/.edikt/hooks"
else
  EDIKT_HOOK_DIR="${HOME}/.edikt/hooks"
fi

# ─── State detection ────────────────────────────────────────────────────────
STATE="fresh_install"

# legacy_v04: flat layout — VERSION<0.6.0 OR hooks/ is a real dir (not symlink)
is_version_lt_050() {
  v="$(printf '%s' "$1" | tr -d '[:space:]')"
  case "$v" in
    0.0.*|0.1.*|0.2.*|0.3.*|0.4.*) return 0 ;;
    *) return 1 ;;
  esac
}

if [ -x "$EDIKT_ROOT/bin/edikt" ]; then
  STATE="current_v05"
elif [ -d "$EDIKT_ROOT/hooks" ] && [ ! -L "$EDIKT_ROOT/hooks" ]; then
  STATE="legacy_v04"
elif [ -f "$EDIKT_ROOT/VERSION" ]; then
  ver=$(cat "$EDIKT_ROOT/VERSION" 2>/dev/null || echo "")
  if is_version_lt_050 "$ver"; then
    STATE="legacy_v04"
  fi
fi

# Fresh = nothing on disk in either EDIKT_ROOT or CLAUDE commands dir.
if [ "$STATE" = "fresh_install" ]; then
  if [ -d "$CLAUDE_HOME_DIR/commands/edikt" ] && [ ! -e "$EDIKT_ROOT" ]; then
    # Commands-only remnant (rare) — treat as fresh, delegation will clean it.
    :
  fi
fi

# ─── Tag resolution ─────────────────────────────────────────────────────────
# Precedence: --ref > EDIKT_RELEASE_TAG env (test override) > GitHub API latest.
resolve_tag() {
  if [ -n "$REF_TAG" ]; then
    printf '%s' "$REF_TAG"
    return 0
  fi
  if [ -n "${EDIKT_RELEASE_TAG:-}" ]; then
    warn "EDIKT_RELEASE_TAG override active: $EDIKT_RELEASE_TAG"
    printf '%s' "$EDIKT_RELEASE_TAG"
    return 0
  fi
  # GitHub API for latest stable release. A read-only call, safe in --dry-run.
  if ! out=$(curl -fsSL --max-time 15 "$API_BASE/releases/latest" 2>/dev/null); then
    error "failed to query $API_BASE/releases/latest"
    error "network error — retry later, or pass --ref <tag> explicitly"
    return $EX_NETWORK
  fi
  tag=$(printf '%s' "$out" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
  if [ -z "$tag" ]; then
    error "could not parse tag_name from GitHub API response"
    return $EX_NETWORK
  fi
  printf '%s' "$tag"
}

TAG=""
if ! TAG=$(resolve_tag); then
  exit $EX_NETWORK
fi
if [ -z "$TAG" ]; then
  error "empty release tag resolved — refusing to proceed"
  exit $EX_NETWORK
fi

# ─── Version-mismatch gate (exit 3) ─────────────────────────────────────────
# If a v0.6.x launcher is already installed and the requested tag is older,
# refuse and print a rollback hint.
check_version_not_older() {
  [ "$STATE" = "current_v05" ] || return 0
  installed_raw=""
  if [ -x "$EDIKT_ROOT/bin/edikt" ]; then
    installed_raw=$("$EDIKT_ROOT/bin/edikt" version 2>/dev/null | head -1 || true)
  fi
  # Strip leading "v" for comparison.
  installed=${installed_raw#v}
  requested=${TAG#v}
  # Only numeric x.y.z comparison. If installed is empty or non-numeric, skip.
  case "$installed" in
    [0-9]*.[0-9]*.[0-9]*) ;;
    *) return 0 ;;
  esac
  case "$requested" in
    [0-9]*.[0-9]*.[0-9]*) ;;
    *) return 0 ;;
  esac
  # Compare via sort -V; if installed > requested → version mismatch.
  newest=$(printf '%s\n%s\n' "$installed" "$requested" | sort -V | tail -1)
  if [ "$newest" = "$installed" ] && [ "$installed" != "$requested" ]; then
    error "requested tag $TAG is older than installed ($installed_raw)."
    error "Downgrade with: edikt rollback"
    return $EX_VERSION
  fi
  return 0
}

if ! check_version_not_older; then
  exit $EX_VERSION
fi

# ─── Writability gate ───────────────────────────────────────────────────────
check_writable() {
  parent=$(dirname "$EDIKT_ROOT")
  if [ -e "$EDIKT_ROOT" ]; then
    [ -w "$EDIKT_ROOT" ] || {
      error "$EDIKT_ROOT is not writable"
      error "Fix: sudo chown -R \"$USER\" \"$EDIKT_ROOT\""
      return $EX_PERMISSION
    }
  else
    [ -w "$parent" ] || {
      error "$parent is not writable (cannot create $EDIKT_ROOT)"
      error "Fix: sudo chown \"$USER\" \"$parent\""
      return $EX_PERMISSION
    }
  fi
  return 0
}

if ! $DRY_RUN; then
  if ! check_writable; then
    exit $EX_PERMISSION
  fi
fi

# ─── Launcher fetch / placement ─────────────────────────────────────────────
# Source: EDIKT_LAUNCHER_SOURCE env (test override) or release tarball for TAG.
#
# signing chain: the release workflow publishes edikt-v${TAG}.tar.gz
# (launcher tarball containing bin/edikt + LICENSE + README) as a Release asset
# and includes its SHA-256 in the Sigstore-signed SHA256SUMS. To keep the
# verification end-to-end, install.sh downloads that tarball (NOT the raw
# bin/edikt file at raw.githubusercontent.com, which is NOT covered by
# SHA256SUMS and therefore cannot be cosign-verified).
LAUNCHER_URL=""
LAUNCHER_SRC_LOCAL=""
LAUNCHER_IS_TARBALL=0
if [ -n "${EDIKT_LAUNCHER_SOURCE:-}" ]; then
  warn "EDIKT_LAUNCHER_SOURCE override active: $EDIKT_LAUNCHER_SOURCE"
  LAUNCHER_SRC_LOCAL="$EDIKT_LAUNCHER_SOURCE"
else
  # Detect platform — release assets are platform-specific.
  _uname_s="$(uname -s 2>/dev/null)"
  _uname_m="$(uname -m 2>/dev/null)"
  case "$_uname_s" in
    Darwin) _goos="darwin" ;;
    Linux)  _goos="linux" ;;
    *) error "unsupported OS: $_uname_s (need Darwin or Linux)"; exit "$EX_GENERAL" ;;
  esac
  case "$_uname_m" in
    arm64|aarch64) _goarch="arm64" ;;
    x86_64|amd64)  _goarch="amd64" ;;
    *) error "unsupported arch: $_uname_m (need arm64 or amd64)"; exit "$EX_GENERAL" ;;
  esac
  # Strip leading "v" if present; assets named edikt-v<ver>-<goos>-<goarch>.tar.gz.
  _ver_only="${TAG#v}"
  LAUNCHER_PLATFORM="${_goos}-${_goarch}"
  LAUNCHER_URL="https://github.com/${REPO}/releases/download/${TAG}/edikt-v${_ver_only}-${LAUNCHER_PLATFORM}.tar.gz"
  LAUNCHER_IS_TARBALL=1
fi

# sha256 of a file — portable across macOS (shasum) and Linux (sha256sum).
_sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    error "need sha256sum or shasum to verify launcher integrity"
    return $EX_NETWORK
  fi
}

# cosign verification of SHA256SUMS against the release workflow's
# GitHub OIDC identity. If cosign is present and the bundle + SHA256SUMS are
# available for the current tag, use them. Returns 0 on success, 1 on verify
# failure (hard abort), 2 on missing-prerequisites (caller decides fallback).
_cosign_verify_release_checksums() {
  _tag="$1"
  _checksums_out="$2"  # caller-provided path to write verified SHA256SUMS to

  if ! command -v cosign >/dev/null 2>&1; then
    return 2
  fi

  _release_base="https://github.com/${REPO}/releases/download/${_tag}"
  _tmp_sums=$(mktemp)
  _tmp_bundle=$(mktemp)

  if ! curl -fsSL --retry 2 --max-time 20 "${_release_base}/SHA256SUMS" -o "$_tmp_sums" 2>/dev/null; then
    rm -f "$_tmp_sums" "$_tmp_bundle"
    return 2
  fi
  if ! curl -fsSL --retry 2 --max-time 20 "${_release_base}/SHA256SUMS.sig.bundle" -o "$_tmp_bundle" 2>/dev/null; then
    rm -f "$_tmp_sums" "$_tmp_bundle"
    return 2
  fi

  # The certificate identity must match the release workflow at any tag
  # shape, for whichever repo actually signed it — $REPO, not a hardcoded
  # constant (F-090/EDIKT_INSTALL_REPO). Retargeting the download URL without
  # also retargeting this regex would download from one repo's release and
  # verify against a different repo's identity, which can only ever fail —
  # the two MUST be derived from the same value. '.' is the one regex
  # metacharacter a GitHub owner/repo name may contain (validate_repo above
  # restricts it to [A-Za-z0-9_.-]); escaped here so a literal '.' in $REPO
  # doesn't widen the match to "any character".
  _identity_repo_escaped=$(printf '%s' "$REPO" | sed 's/\./\\./g')
  _identity_regex="^https://github\\.com/${_identity_repo_escaped}/\\.github/workflows/release\\.yml@refs/tags/v[0-9]+\\.[0-9]+\\.[0-9]+(-[A-Za-z0-9.-]+)?\$"
  if ! cosign verify-blob \
      --bundle "$_tmp_bundle" \
      --certificate-identity-regexp "$_identity_regex" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      "$_tmp_sums" >/dev/null 2>&1; then
    rm -f "$_tmp_sums" "$_tmp_bundle"
    error "cosign signature verification FAILED for ${_release_base}/SHA256SUMS"
    error "this is a hard abort — a signed release's signature does not match expected identity."
    error "do NOT set EDIKT_INSTALL_INSECURE=1 to bypass; the release may be tampered."
    return 1
  fi

  cp "$_tmp_sums" "$_checksums_out"
  rm -f "$_tmp_sums" "$_tmp_bundle"
  return 0
}

# Verify observed hash against a reference file that may be a bare hex hash
# or the standard "<hash>  <filename>" format (matches bin/edikt contract).
_verify_launcher_checksum() {
  _observed="$1"
  _ref_file="$2"
  _expected=$(awk '{print $1; exit}' "$_ref_file" 2>/dev/null | tr -d '[:space:]')
  if [ -z "$_expected" ]; then
    error "launcher .sha256 reference is empty"
    return $EX_NETWORK
  fi
  if [ "$_observed" != "$_expected" ]; then
    error "launcher checksum mismatch: expected $_expected, got $_observed"
    return $EX_NETWORK
  fi
  return 0
}

# Fetch the launcher into a tmp file, verify integrity, then move into place.
# Never curl -o directly onto the live path.
stage_launcher() {
  stage="$1"  # destination (tmp path)
  if [ -n "$LAUNCHER_SRC_LOCAL" ]; then
    cp "$LAUNCHER_SRC_LOCAL" "$stage" || {
      error "failed to copy launcher from $LAUNCHER_SRC_LOCAL"
      return $EX_NETWORK
    }
    # ── Integrity verification for local source ──────────────────────────
    # Precedence: EDIKT_LAUNCHER_SHA256 env > opportunistic sibling sidecar.
    # If neither is present, the local path is implicitly trusted (user chose it).
    if [ -n "${EDIKT_LAUNCHER_SHA256:-}" ]; then
      _observed=$(_sha256_file "$stage") || return $EX_NETWORK
      _ref_tmp=$(mktemp)
      printf '%s\n' "$EDIKT_LAUNCHER_SHA256" > "$_ref_tmp"
      if ! _verify_launcher_checksum "$_observed" "$_ref_tmp"; then
        rm -f "$_ref_tmp"
        return $EX_NETWORK
      fi
      rm -f "$_ref_tmp"
    else
      # Opportunistic local sidecar check.
      _local_sidecar="${LAUNCHER_SRC_LOCAL}.sha256"
      if [ -f "$_local_sidecar" ]; then
        _observed=$(_sha256_file "$stage") || return $EX_NETWORK
        _verify_launcher_checksum "$_observed" "$_local_sidecar" || return $EX_NETWORK
      fi
    fi
  else
    # Download the tarball (or raw launcher if LAUNCHER_IS_TARBALL=0 via override).
    _download="$stage"
    if [ "$LAUNCHER_IS_TARBALL" = "1" ]; then
      _download="${stage}.tar.gz"
    fi
    if ! curl -fsSL --retry 2 --max-time 30 "$LAUNCHER_URL" -o "$_download"; then
      error "failed to download launcher from $LAUNCHER_URL"
      error "retry: re-run install.sh; or pass --ref <different-tag>"
      return $EX_NETWORK
    fi
    _observed=$(_sha256_file "$_download") || return $EX_NETWORK

    # ── Integrity verification ───────────────────────────────────
    # Precedence: EDIKT_LAUNCHER_SHA256 env > cosign-verified SHA256SUMS.
    # When the download is the signed tarball (LAUNCHER_IS_TARBALL=1), the
    # SHA256SUMS lookup uses the tarball filename — which IS in SHA256SUMS,
    # closing the chain end-to-end. When the download is the raw launcher
    # (EDIKT_LAUNCHER_SOURCE override), the env-supplied hash path applies.
    if [ -n "${EDIKT_LAUNCHER_SHA256:-}" ]; then
      _ref_tmp=$(mktemp)
      printf '%s\n' "$EDIKT_LAUNCHER_SHA256" > "$_ref_tmp"
      if ! _verify_launcher_checksum "$_observed" "$_ref_tmp"; then
        rm -f "$_ref_tmp" "$_download"
        return $EX_NETWORK
      fi
      rm -f "$_ref_tmp"
    else
      _sums_tmp=$(mktemp)
      _cosign_verify_release_checksums "$TAG" "$_sums_tmp"
      _cosign_rc=$?
      if [ "$_cosign_rc" -eq 0 ]; then
        # Pick the filename to look up based on what was downloaded.
        _ver_only="${TAG#v}"
        if [ "$LAUNCHER_IS_TARBALL" = "1" ]; then
          _expected_name="edikt-v${_ver_only}-${LAUNCHER_PLATFORM:-darwin-arm64}.tar.gz"
        else
          _expected_name="bin/edikt"
        fi
        _ref_tmp=$(mktemp)
        awk -v name="$_expected_name" '$2 == name || $2 == ("*" name) {print; exit}' "$_sums_tmp" > "$_ref_tmp"
        if [ ! -s "$_ref_tmp" ]; then
          rm -f "$_ref_tmp" "$_sums_tmp" "$_download"
          if [ "${EDIKT_INSTALL_INSECURE:-0}" = "1" ]; then
            warn "SHA256SUMS did not contain $_expected_name — proceeding without per-launcher verification (EDIKT_INSTALL_INSECURE=1)"
            EDIKT_INSECURE_BANNER=1
          else
            error "SHA256SUMS for release $TAG does not list $_expected_name — cannot verify launcher."
            error "set EDIKT_INSTALL_INSECURE=1 to bypass (NOT recommended), or use a release >= v0.6.0."
            return $EX_NETWORK
          fi
        else
          if ! _verify_launcher_checksum "$_observed" "$_ref_tmp"; then
            rm -f "$_ref_tmp" "$_sums_tmp" "$_download"
            return $EX_NETWORK
          fi
          rm -f "$_ref_tmp" "$_sums_tmp"
        fi
      elif [ "$_cosign_rc" -eq 1 ]; then
        rm -f "$_sums_tmp" "$_download"
        return $EX_NETWORK
      else
        rm -f "$_sums_tmp"
        if [ "${EDIKT_INSTALL_INSECURE:-0}" = "1" ]; then
          warn "cosign unavailable or SHA256SUMS.sig.bundle missing for $TAG — proceeding without signature verification (EDIKT_INSTALL_INSECURE=1)"
          EDIKT_INSECURE_BANNER=1
        else
          rm -f "$_download"
          error "cosign not available or SHA256SUMS.sig.bundle missing for $TAG."
          error "install cosign (https://docs.sigstore.dev/cosign/installation) or set EDIKT_INSTALL_INSECURE=1 to bypass (NOT recommended)."
          return $EX_NETWORK
        fi
      fi
    fi

    # Extract bin/edikt (Go binary) from the tarball.
    if [ "$LAUNCHER_IS_TARBALL" = "1" ]; then
      _extract_dir=$(mktemp -d)
      # Cache the tarball path for install_launcher to also extract edikt-shell.
      _LAUNCHER_TARBALL_CACHED="$_download"
      if ! tar -xzf "$_download" -C "$_extract_dir" bin/edikt 2>/dev/null; then
        rm -rf "$_extract_dir" "$_download"
        error "launcher tarball does not contain bin/edikt"
        return $EX_NETWORK
      fi
      if [ ! -f "$_extract_dir/bin/edikt" ]; then
        rm -rf "$_extract_dir" "$_download"
        error "bin/edikt missing from extracted launcher tarball"
        return $EX_NETWORK
      fi
      mv "$_extract_dir/bin/edikt" "$stage"
      rm -rf "$_extract_dir"
      # Note: $_download is cleaned up by stage_launcher when LAUNCHER_IS_TARBALL=1.
    fi
  fi
  if [ ! -s "$stage" ]; then
    error "downloaded launcher is empty"
    return $EX_NETWORK
  fi
  # ── Content sanity checks (guard against HTML error pages) ───────────────
  # bin/edikt is the Go binary (ELF/Mach-O). Shell-script
  # sanity checks (shebang, bash -n) only apply when the bare-file launcher
  # is a shell script (begins with #!). Binary launchers are validated by
  # checksum only.
  if [ "$LAUNCHER_IS_TARBALL" != "1" ]; then
    if LC_ALL=C head -c2 "$stage" | grep -q '^#!'; then
      # Shell script launcher — apply shebang and bash -n checks.
      _first_line=$(head -n1 "$stage")
      case "$_first_line" in
        '#!'*) ;;
        *) error "downloaded launcher missing shebang"; return $EX_NETWORK ;;
      esac
      if ! bash -n "$stage" 2>/dev/null; then
        error "downloaded shell launcher failed syntax check (bash -n)"
        return $EX_NETWORK
      fi
    fi
    # Binary launchers (Go/ELF/Mach-O) skip shell checks — validated by
    # checksum only.
  fi
  chmod +x "$stage"
  return 0
}

# Atomically place the launcher at $EDIKT_ROOT/bin/.
#
# Phase 3 (v0.6.0+): edikt-shell has been deleted. The release tarball
# contains only bin/edikt (the Go binary). One artifact, one install.
#
# One-deep backup: previous edikt → bin/edikt.prev before overwrite.
install_launcher() {
  mkdir -p "$EDIKT_ROOT/bin" || return $EX_PERMISSION

  tmp="$EDIKT_ROOT/bin/edikt.tmp.$$"
  if ! stage_launcher "$tmp"; then
    rm -f "$tmp" 2>/dev/null || true
    return $?
  fi
  if [ -f "$EDIKT_ROOT/bin/edikt" ]; then
    cp "$EDIKT_ROOT/bin/edikt" "$EDIKT_ROOT/bin/edikt.prev" 2>/dev/null || true
  fi
  mv "$tmp" "$EDIKT_ROOT/bin/edikt"
  return 0
}

# ─── Shell-rc PATH append (idempotent) ──────────────────────────────────────
RC_MARKER="# edikt bootstrap (do not edit)"

pick_rc_file() {
  case "${SHELL:-}" in
    */zsh)  printf '%s/.zshrc' "$HOME" ;;
    */bash) printf '%s/.bashrc' "$HOME" ;;
    *)
      # Fallback: prefer .zshrc on macOS, .bashrc on Linux.
      if [ "$(uname -s 2>/dev/null)" = "Darwin" ]; then
        printf '%s/.zshrc' "$HOME"
      else
        printf '%s/.bashrc' "$HOME"
      fi
      ;;
  esac
}

rc_has_marker() {
  rc="$1"
  [ -f "$rc" ] || return 1
  grep -qF "$RC_MARKER" "$rc" 2>/dev/null
}

append_path_to_rc() {
  rc="$1"
  # Create if missing; respect parent writability.
  rc_dir=$(dirname "$rc")
  if [ ! -w "$rc_dir" ]; then
    warn "$rc_dir is not writable — skipping PATH update"
    warn "Add manually: export PATH=\"$EDIKT_ROOT/bin:\$PATH\""
    return 0
  fi
  if [ -f "$rc" ] && [ ! -w "$rc" ]; then
    warn "$rc is not writable — skipping PATH update"
    warn "Add manually: export PATH=\"$EDIKT_ROOT/bin:\$PATH\""
    return 0
  fi
  {
    printf '\n%s\n' "$RC_MARKER"
    printf '[ -d "%s/bin" ] && export PATH="%s/bin:$PATH"\n' "$EDIKT_ROOT" "$EDIKT_ROOT"
  } >> "$rc"
  return 0
}

# ─── Dry-run command recording ──────────────────────────────────────────────
# `dryrun_do <label>` prints "would-run: <label>" without executing.
dryrun_do() {
  dim "would-run: $1"
}

# ─── settings.json writer ─────────────────────────────────────────────────────
# Reads $EDIKT_ROOT/templates/settings.json.tmpl, substitutes ${EDIKT_HOOK_DIR}
# with the resolved absolute hook path, and writes the result atomically to
# $CLAUDE_HOME_DIR/settings.json.
#
# The template uses ${EDIKT_HOOK_DIR} as a placeholder — not a real env var.
# sed substitutes it at install time so the resulting settings.json has
# absolute paths (no shell expansion required at hook-fire time).
write_settings_json() {
  tmpl="$EDIKT_ROOT/templates/settings.json.tmpl"
  dest="$CLAUDE_HOME_DIR/settings.json"
  if [ ! -f "$tmpl" ]; then
    warn "settings.json.tmpl not found at $tmpl — skipping settings.json write"
    return 0
  fi

  # Phase 13: back up the existing settings.json before the new permissions
  # block overwrites it. This enables `edikt rollback v0.6.0` to restore
  # the user's prior settings if the new posture breaks something.
  # Backup is one-shot — only the FIRST v0.6.0 install creates it.
  _backup_root="$HOME/.edikt/backup"
  _backup_dir_marker="$_backup_root/pre-v0.6.0-marker"
  if [ -f "$dest" ] && [ ! -f "$_backup_dir_marker" ]; then
    _backup_ts=$(date -u +"%Y%m%dT%H%M%SZ" 2>/dev/null || date +%s)
    _backup_dir="$_backup_root/pre-v0.6.0-${_backup_ts}"
    mkdir -p "$_backup_dir" 2>/dev/null || true
    if cp -p "$dest" "$_backup_dir/settings.json" 2>/dev/null; then
      # Record the backup dir path in the marker file so rollback can find it.
      printf '%s\n' "$_backup_dir" > "$_backup_dir_marker"
      chmod 0600 "$_backup_dir_marker" "$_backup_dir/settings.json" 2>/dev/null || true
      dim "backed up $dest → $_backup_dir/settings.json (for 'edikt rollback v0.6.0')"
    fi
  fi

  # Validate EDIKT_HOOK_DIR before substitution. Characters that
  # can corrupt JSON or shell expansion are rejected — the previous sed-based
  # implementation silently produced invalid JSON when the path contained
  # any of these (audit HI-3).
  case "$EDIKT_HOOK_DIR" in
    *'"'*|*'\'*|*'|'*|*$'\n'*|*$'\r'*|*$'\t'*)
      error "EDIKT_HOOK_DIR contains a forbidden character (\", \\, |, tab, or newline): $EDIKT_HOOK_DIR"
      return $EX_GENERAL
      ;;
  esac

  mkdir -p "$CLAUDE_HOME_DIR" || return $EX_PERMISSION

  # Python substitution: json.loads the template, walk the tree substituting
  # the ${EDIKT_HOOK_DIR} placeholder in string values, json.dumps the result,
  # and atomically rename. This eliminates the sed-based text substitution
  # (which was neither JSON-aware nor JSON-escape-safe) and guarantees the
  # written file is structurally valid JSON (closes audit HI-2).
  python3 - "$tmpl" "$dest" "$EDIKT_HOOK_DIR" <<'PY'
import json
import os
import sys

tmpl_path, dest_path, hook_dir = sys.argv[1], sys.argv[2], sys.argv[3]

with open(tmpl_path, 'r', encoding='utf-8') as f:
    raw = f.read()

# Template uses the literal token ${EDIKT_HOOK_DIR} as a placeholder. It
# appears inside string values in the JSON template. We replace via string
# operation on the raw text FIRST (so the JSON parser sees valid paths),
# then json.loads / json.dumps to round-trip and guarantee validity.
substituted = raw.replace('${EDIKT_HOOK_DIR}', hook_dir)

try:
    parsed = json.loads(substituted)
except json.JSONDecodeError as exc:
    sys.stderr.write(f"template produced invalid JSON after substitution: {exc}\n")
    sys.stderr.write(f"check that EDIKT_HOOK_DIR = {hook_dir!r} is a safe path\n")
    sys.exit(1)

# Write atomically: tmp in the same directory, fsync, rename.
tmp_path = f"{dest_path}.tmp.{os.getpid()}"
try:
    with open(tmp_path, 'w', encoding='utf-8') as f:
        json.dump(parsed, f, indent=2, ensure_ascii=False)
        f.write("\n")
        f.flush()
        os.fsync(f.fileno())
    os.replace(tmp_path, dest_path)
except OSError as exc:
    sys.stderr.write(f"atomic write failed: {exc}\n")
    try:
        os.remove(tmp_path)
    except OSError:
        pass
    sys.exit(1)
PY
  rc=$?
  if [ "$rc" -ne 0 ]; then
    error "settings.json substitution failed — hook paths not written"
    return $EX_GENERAL
  fi

  # Write managed-region integrity sidecar (). The sidecar
  # records the canonical hash of the managed keys in settings.json so that
  # future upgrades can detect drift (user edited the managed region directly)
  # and prompt before overwriting. Phase 13 migration reads this sidecar to
  # decide whether to prompt or silently replace.
  _state_dir="$HOME/.edikt/state"
  _sidecar="$_state_dir/settings-managed.json"
  python3 - "$dest" "$_sidecar" <<'PY' || true
import hashlib
import json
import os
import sys
from datetime import datetime, timezone

dest_path, sidecar_path = sys.argv[1], sys.argv[2]
try:
    with open(dest_path, 'r', encoding='utf-8') as f:
        data = json.load(f)
except (OSError, json.JSONDecodeError):
    sys.exit(0)

managed_keys = ["permissions"]
managed = {k: data[k] for k in managed_keys if k in data}
canonical = json.dumps(managed, sort_keys=True, ensure_ascii=False).encode('utf-8')
managed_hash = hashlib.sha256(canonical).hexdigest()

sidecar = {
    "settings_path": dest_path,
    "managed_keys": managed_keys,
    "managed_hash": managed_hash,
    "sentinel_version": 1,
    "installed_at": datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),
}
os.makedirs(os.path.dirname(sidecar_path), exist_ok=True)
tmp = sidecar_path + '.tmp'
with open(tmp, 'w', encoding='utf-8') as f:
    json.dump(sidecar, f, indent=2)
    f.write('\n')
os.replace(tmp, sidecar_path)
try:
    os.chmod(sidecar_path, 0o600)
except OSError:
    pass
PY

  dim "wrote $dest (hook dir: $EDIKT_HOOK_DIR)"
  return 0
}

# ─── Flow orchestration ─────────────────────────────────────────────────────
run_launcher_step() {
  # Args: subcommand and its args. Honors EDIKT_INSTALL_SOURCE pass-through.
  # Pass EDIKT_ROOT explicitly so the Go binary uses the env-override path
  # instead of the ancestor walk (which can find an unrelated system install
  # when $HOME is overridden in test sandboxes).
  if $DRY_RUN; then
    dryrun_do "EDIKT_ROOT=$EDIKT_ROOT $EDIKT_ROOT/bin/edikt $*"
    return 0
  fi
  EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_ROOT/bin/edikt" "$@"
}

do_fresh_install() {
  info "Fresh install into $EDIKT_ROOT"
  if $DRY_RUN; then
    dryrun_do "mkdir -p $EDIKT_ROOT/bin"
    if [ -n "$LAUNCHER_SRC_LOCAL" ]; then
      dryrun_do "cp $LAUNCHER_SRC_LOCAL $EDIKT_ROOT/bin/edikt (verify: sh -n)"
    else
      dryrun_do "curl $LAUNCHER_URL -> $EDIKT_ROOT/bin/edikt (verify: sh -n)"
    fi
    dryrun_do "chmod +x $EDIKT_ROOT/bin/edikt"
    rc=$(pick_rc_file)
    if rc_has_marker "$rc"; then
      dim "PATH entry already present in $rc (marker: $RC_MARKER)"
    else
      dryrun_do "append PATH export to $rc (marker: $RC_MARKER)"
    fi
    dryrun_do "$EDIKT_ROOT/bin/edikt install $TAG"
    dryrun_do "$EDIKT_ROOT/bin/edikt use $TAG"
    dryrun_do "write $CLAUDE_HOME_DIR/settings.json (hook dir: $EDIKT_HOOK_DIR)"
    return 0
  fi

  install_launcher || return $?
  rc=$(pick_rc_file)
  if rc_has_marker "$rc"; then
    dim "PATH entry already present in $rc"
  else
    append_path_to_rc "$rc" || return $?
    dim "added PATH entry to $rc (open a new shell to pick up)"
  fi
  run_launcher_step install "$TAG" || return $?
  run_launcher_step use "$TAG" || return $?
  write_settings_json || return $?
}

do_legacy_v04() {
  printf '%b==> Detected v0.4.x install. Migrating to versioned layout...%b\n' "$BOLD" "$RESET"
  if $DRY_RUN; then
    dryrun_do "mkdir -p $EDIKT_ROOT/bin"
    if [ -n "$LAUNCHER_SRC_LOCAL" ]; then
      dryrun_do "cp $LAUNCHER_SRC_LOCAL $EDIKT_ROOT/bin/edikt (verify: sh -n)"
    else
      dryrun_do "curl $LAUNCHER_URL -> $EDIKT_ROOT/bin/edikt (verify: sh -n)"
    fi
    dryrun_do "chmod +x $EDIKT_ROOT/bin/edikt"
    rc=$(pick_rc_file)
    if rc_has_marker "$rc"; then
      dim "PATH entry already present in $rc"
    else
      dryrun_do "append PATH export to $rc (marker: $RC_MARKER)"
    fi
    # migrate --dry-run is itself non-mutating, so we run it live even in dry-run.
    # But the launcher isn't on disk yet during a dry-run fresh path — guard.
    dryrun_do "$EDIKT_ROOT/bin/edikt migrate --dry-run"
    dryrun_do "$EDIKT_ROOT/bin/edikt install $TAG"
    dryrun_do "$EDIKT_ROOT/bin/edikt use $TAG"
    dryrun_do "write $CLAUDE_HOME_DIR/settings.json (hook dir: $EDIKT_HOOK_DIR)"
    return 0
  fi

  install_launcher || return $?
  rc=$(pick_rc_file)
  if rc_has_marker "$rc"; then
    dim "PATH entry already present in $rc"
  else
    append_path_to_rc "$rc" || return $?
    dim "added PATH entry to $rc"
  fi
  # For a mutating install, still pass --yes to migrate. Dry-run was handled above.
  run_launcher_step migrate --yes || return $?
  run_launcher_step install "$TAG" || return $?
  run_launcher_step use "$TAG" || return $?
  write_settings_json || return $?
  printf '%bMigration complete. Run %b\`edikt doctor\`%b to verify.%b\n' "$GREEN" "$BOLD" "$RESET$GREEN" "$RESET"
}

# Get the version reported by the installed Go edikt binary.
go_binary_version() {
  f="$1"
  [ -x "$f" ] || { echo ""; return 0; }
  "$f" version 2>/dev/null | head -1 | awk '{print $NF}' | tr -d 'v' || echo ""
}

do_current_v05() {
  info "v0.6.0 layout detected at $EDIKT_ROOT"

  # Decide whether to replace the Go binary. Compare the installed binary's
  # version against the release tag being installed.
  if $DRY_RUN; then
    dim "would check: installed edikt version vs $TAG"
    dryrun_do "$EDIKT_ROOT/bin/edikt install $TAG"
    dryrun_do "$EDIKT_ROOT/bin/edikt use $TAG"
    return 0
  fi

  cur_ver=$(go_binary_version "$EDIKT_ROOT/bin/edikt")
  new_ver="${TAG#v}"

  # F-111/F-117: a version-string-only comparison here let a same-tag
  # rebuild (routine during rc/dev iteration -- the exact shape a real
  # consumer project hit) skip re-installing the launcher entirely, silently,
  # with no user-visible way to pick up the new content short of deleting the
  # binary first. Confirmed live, not theoretical. A version-string mismatch
  # still short-circuits straight to "needs update" -- no reason to pay for
  # a content check when the version already answers the question -- but a
  # MATCHING version string no longer means "current" by itself: content is
  # compared too, via the same staged download + checksum machinery
  # stage_launcher/_sha256_file already use for integrity verification.
  needs_update=0
  staged_tmp=""
  if [ -z "$new_ver" ] || [ "$new_ver" != "$cur_ver" ]; then
    needs_update=1
  else
    _content_check_tmp="$EDIKT_ROOT/bin/edikt.check.$$"
    if stage_launcher "$_content_check_tmp"; then
      if [ "$(_sha256_file "$EDIKT_ROOT/bin/edikt" 2>/dev/null)" != "$(_sha256_file "$_content_check_tmp")" ]; then
        needs_update=1
        staged_tmp="$_content_check_tmp"
      else
        rm -f "$_content_check_tmp" 2>/dev/null || true
      fi
    else
      # Could not verify content (network, checksum lookup, etc). Fail safe:
      # treat as needing an update rather than silently trusting a binary
      # whose current content is now genuinely unknown, matching this
      # function's existing NETWORK-error posture elsewhere in this branch.
      rm -f "$_content_check_tmp" 2>/dev/null || true
      warn "could not verify installed launcher content against $TAG — reinstalling to be safe"
      needs_update=1
    fi
  fi

  if [ "$needs_update" = "1" ]; then
    tmp="$EDIKT_ROOT/bin/edikt.tmp.$$"
    mkdir -p "$EDIKT_ROOT/bin" || return $EX_PERMISSION
    if [ -n "$staged_tmp" ] && [ -f "$staged_tmp" ]; then
      mv "$staged_tmp" "$tmp"
    elif ! stage_launcher "$tmp"; then
      rm -f "$tmp" 2>/dev/null || true
      return $EX_NETWORK
    fi
    cp "$EDIKT_ROOT/bin/edikt" "$EDIKT_ROOT/bin/edikt.prev" 2>/dev/null || true
    mv "$tmp" "$EDIKT_ROOT/bin/edikt"
    info "launcher updated: $cur_ver -> $new_ver"
  else
    dim "launcher is current ($cur_ver, content verified)"
  fi

  # Idempotent: re-running edikt install for an already-installed tag
  # exits EX_ALREADY=3. Treat that as "already done, continue".
  if run_launcher_step install "$TAG"; then
    :
  else
    rc=$?
    if [ "$rc" -eq 3 ]; then
      dim "$TAG already installed — continuing"
    else
      return $rc
    fi
  fi
  run_launcher_step use "$TAG" || return $?
  write_settings_json || return $?
}

# ─── Main ───────────────────────────────────────────────────────────────────
printf '\n%bedikt installer%b (mode: %s, tag: %s%s)\n' "$BOLD" "$RESET" "$INSTALL_MODE" "$TAG" "$($DRY_RUN && printf ', dry-run' || true)"
printf '  EDIKT_ROOT = %s\n' "$EDIKT_ROOT"
printf '  state      = %s\n\n' "$STATE"

case "$STATE" in
  fresh_install) do_fresh_install ;;
  legacy_v04)    do_legacy_v04 ;;
  current_v05)   do_current_v05 ;;
  *)
    error "unknown state: $STATE"
    exit 1
    ;;
esac

# ─── Post-install banner ────────────────────────────────────────────────────
echo
if $DRY_RUN; then
  printf '%bDry run complete.%b No files were written, no PATH changes made.\n' "$BOLD" "$RESET"
  printf '  Remove --dry-run to install.\n\n'
else
  printf '%b%bedikt installed.%b\n' "$GREEN" "$BOLD" "$RESET"
  printf '  Version:  %s\n' "$TAG"
  printf '  Launcher: %s/bin/edikt\n' "$EDIKT_ROOT"
  printf '\n  Next: open a new shell (or source your rc) and run:\n'
  printf '    %bedikt doctor%b\n\n' "$BOLD" "$RESET"
  # / MED-13: loud banner when the user opted out of integrity checks.
  # Printed AFTER the success line so it's the last thing the user sees.
  if [ "${EDIKT_INSECURE_BANNER:-0}" = "1" ] || [ "${EDIKT_INSTALL_INSECURE:-0}" = "1" ]; then
    printf '%b%b⚠  INTEGRITY VERIFICATION WAS DISABLED%b\n' "$YELLOW" "$BOLD" "$RESET"
    printf '%b   EDIKT_INSTALL_INSECURE=1 was honored — the downloaded launcher\n' "$YELLOW"
    printf '   was NOT verified against a signed SHA256SUMS. TLS-only trust.\n'
    printf '   Re-install with cosign available for full verification.%b\n\n' "$RESET"
  fi
fi
exit $EX_OK

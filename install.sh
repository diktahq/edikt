#!/usr/bin/env bash
set -euo pipefail
# edikt installer — v0.5.0 thin bootstrap.
#
# This script is intentionally small. All heavy lifting (payload fetch,
# checksum verification, versioned layout, migration from flat layout,
# symlink chain, lock.yaml) lives in the launcher at $EDIKT_ROOT/bin/edikt.
#
# install.sh's job:
#   1. Parse flags (--project | --global | --dry-run | --ref <tag>).
#   2. Detect installed state: fresh | legacy_v04 | current_v05.
#   3. Fetch the launcher for the requested release (curl).
#   4. Install launcher atomically, add to $PATH idempotently.
#   5. Delegate to `edikt migrate` (if legacy) and `edikt install/use`.
#
# Canonical v0.4.x → v0.5.0 cross-major upgrade path. /edikt:upgrade
# redirects here for major-version jumps (Phase 6).
#
# Exit codes:
#   0 success
#   1 network error (launcher or release-tag fetch failed)
#   2 permission error ($EDIKT_ROOT not writable, rc append refused)
#   3 version mismatch (requested tag older than currently installed)
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
#   curl -fsSL ...install.sh | bash -s -- --project
#   curl -fsSL ...install.sh | bash -s -- --dry-run
#   curl -fsSL ...install.sh | bash -s -- --ref v0.5.1
#
# Test overrides (for test/integration/install/):
#   EDIKT_LAUNCHER_SOURCE=<path>  — local launcher file; skip curl
#   EDIKT_RELEASE_TAG=<tag>       — skip GitHub API, use this tag
#   EDIKT_INSTALL_SOURCE=<path>   — forwarded to `edikt install`
#   EDIKT_LAUNCHER_SHA256=<hex>   — pin launcher checksum (takes precedence over sidecar fetch)
#   EDIKT_INSTALL_INSECURE=1      — skip sidecar .sha256 fetch (not recommended)

umask 0022

REPO="diktahq/edikt"
RAW_BASE="https://raw.githubusercontent.com/${REPO}"
API_BASE="https://api.github.com/repos/${REPO}"

# Exit codes
EX_OK=0
EX_NETWORK=1
EX_PERMISSION=2
EX_VERSION=3

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

# ─── Dependency check ───────────────────────────────────────────────────────
command -v curl >/dev/null 2>&1 || { error "curl is required"; exit $EX_NETWORK; }

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
# Matches bin/edikt's EDIKT_ROOT resolution so both see the same layout.
if [ "$INSTALL_MODE" = "project" ]; then
  EDIKT_ROOT="$(pwd)/.edikt"
  CLAUDE_HOME_DIR="$(pwd)/.claude"
else
  EDIKT_ROOT="${EDIKT_HOME:-${HOME}/.edikt}"
  CLAUDE_HOME_DIR="${CLAUDE_HOME:-${HOME}/.claude}"
fi

# ─── State detection ────────────────────────────────────────────────────────
STATE="fresh_install"

# legacy_v04: flat layout — VERSION<0.5.0 OR hooks/ is a real dir (not symlink)
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
# If a v0.5.x launcher is already installed and the requested tag is older,
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
# Source: EDIKT_LAUNCHER_SOURCE env (test override) or raw GitHub for TAG.
LAUNCHER_URL=""
LAUNCHER_SRC_LOCAL=""
if [ -n "${EDIKT_LAUNCHER_SOURCE:-}" ]; then
  warn "EDIKT_LAUNCHER_SOURCE override active: $EDIKT_LAUNCHER_SOURCE"
  LAUNCHER_SRC_LOCAL="$EDIKT_LAUNCHER_SOURCE"
else
  LAUNCHER_URL="$RAW_BASE/$TAG/bin/edikt"
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
    if ! curl -fsSL --retry 2 --max-time 30 "$LAUNCHER_URL" -o "$stage"; then
      error "failed to download launcher from $LAUNCHER_URL"
      error "retry: re-run install.sh; or pass --ref <different-tag>"
      return $EX_NETWORK
    fi
    # ── Integrity verification for remote source ──────────────────────────
    # Precedence: EDIKT_LAUNCHER_SHA256 env > sidecar fetch.
    _observed=$(_sha256_file "$stage") || return $EX_NETWORK
    if [ -n "${EDIKT_LAUNCHER_SHA256:-}" ]; then
      _ref_tmp=$(mktemp)
      printf '%s\n' "$EDIKT_LAUNCHER_SHA256" > "$_ref_tmp"
      if ! _verify_launcher_checksum "$_observed" "$_ref_tmp"; then
        rm -f "$_ref_tmp"
        return $EX_NETWORK
      fi
      rm -f "$_ref_tmp"
    else
      # Fetch sibling .sha256 sidecar from the same raw URL.
      _sidecar_url="${LAUNCHER_URL}.sha256"
      _sidecar_tmp=$(mktemp)
      if curl -fsSL --retry 2 --max-time 15 "$_sidecar_url" -o "$_sidecar_tmp" 2>/dev/null; then
        if ! _verify_launcher_checksum "$_observed" "$_sidecar_tmp"; then
          rm -f "$_sidecar_tmp"
          return $EX_NETWORK
        fi
        rm -f "$_sidecar_tmp"
      else
        rm -f "$_sidecar_tmp" 2>/dev/null || true
        if [ "${EDIKT_INSTALL_INSECURE:-0}" = "1" ]; then
          warn "launcher sidecar .sha256 unavailable — proceeding without integrity check (EDIKT_INSTALL_INSECURE=1)"
        else
          error "launcher sidecar .sha256 unavailable; set EDIKT_INSTALL_INSECURE=1 to override (not recommended)"
          return $EX_NETWORK
        fi
      fi
    fi
  fi
  if [ ! -s "$stage" ]; then
    error "downloaded launcher is empty"
    return $EX_NETWORK
  fi
  # ── Content sanity checks (guard against HTML error pages) ───────────────
  _first_line=$(head -n1 "$stage")
  case "$_first_line" in
    '#!'*) ;;
    *) error "downloaded launcher missing shebang"; return $EX_NETWORK ;;
  esac
  if ! grep -qF 'MIN_PAYLOAD_VERSION=' "$stage"; then
    error "downloaded file does not look like an edikt launcher"
    return $EX_NETWORK
  fi
  if ! sh -n "$stage" 2>/dev/null; then
    error "downloaded launcher failed syntax check (sh -n)"
    return $EX_NETWORK
  fi
  chmod +x "$stage"
  return 0
}

# Atomically put the verified launcher at $EDIKT_ROOT/bin/edikt.
# One-deep backup: previous launcher → bin/edikt.prev before overwrite.
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

# ─── Flow orchestration ─────────────────────────────────────────────────────
run_launcher_step() {
  # Args: subcommand and its args. Honors EDIKT_INSTALL_SOURCE pass-through.
  if $DRY_RUN; then
    dryrun_do "$EDIKT_ROOT/bin/edikt $*"
    return 0
  fi
  "$EDIKT_ROOT/bin/edikt" "$@"
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
  printf '%bMigration complete. Run %b\`edikt doctor\`%b to verify.%b\n' "$GREEN" "$BOLD" "$RESET$GREEN" "$RESET"
}

# Extract LAUNCHER_VERSION from a launcher script's constants.
launcher_script_version() {
  f="$1"
  [ -f "$f" ] || { echo ""; return 0; }
  awk -F'"' '/^LAUNCHER_VERSION=/ {print $2; exit}' "$f"
}

do_current_v05() {
  info "v0.5.0 layout detected at $EDIKT_ROOT"

  # Decide whether to replace launcher script. Fetch a tmp copy, compare
  # embedded LAUNCHER_VERSION against currently installed.
  if $DRY_RUN; then
    dim "would check: embedded LAUNCHER_VERSION vs installed"
    dryrun_do "$EDIKT_ROOT/bin/edikt install $TAG"
    dryrun_do "$EDIKT_ROOT/bin/edikt use $TAG"
    return 0
  fi

  tmp="$EDIKT_ROOT/bin/edikt.tmp.$$"
  mkdir -p "$EDIKT_ROOT/bin" || return $EX_PERMISSION
  if ! stage_launcher "$tmp"; then
    rm -f "$tmp" 2>/dev/null || true
    return $EX_NETWORK
  fi
  new_ver=$(launcher_script_version "$tmp")
  cur_ver=$(launcher_script_version "$EDIKT_ROOT/bin/edikt")
  if [ -n "$new_ver" ] && [ "$new_ver" != "$cur_ver" ]; then
    cp "$EDIKT_ROOT/bin/edikt" "$EDIKT_ROOT/bin/edikt.prev" 2>/dev/null || true
    mv "$tmp" "$EDIKT_ROOT/bin/edikt"
    info "launcher updated: $cur_ver -> $new_ver"
  else
    rm -f "$tmp"
    dim "launcher is current ($cur_ver)"
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
fi
exit $EX_OK

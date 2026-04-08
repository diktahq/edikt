#!/usr/bin/env bash
set -euo pipefail
umask 0022

# edikt installer
# TODO: add SHA-256 manifest verification once the release workflow exists
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash                    # global (default)
#   curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash -s -- --project   # project-only
#   curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash -s -- --dry-run   # preview changes

REPO="diktahq/edikt"
BRANCH="main"
BASE_URL="https://raw.githubusercontent.com/${REPO}/${BRANCH}"

# Parse flags
INSTALL_MODE=""
DRY_RUN=false
for arg in "$@"; do
  case "$arg" in
    --project)  INSTALL_MODE="project" ;;
    --global)   INSTALL_MODE="global" ;;
    --dry-run)  DRY_RUN=true ;;
  esac
done

# Interactive prompt if no flag provided and stdin is a terminal
if [ -z "$INSTALL_MODE" ]; then
  if [ -t 0 ]; then
    echo ""
    echo "  Where should edikt be installed?"
    echo ""
    echo "  [1] Global (default) — available in all projects"
    echo "  [2] Project only     — installed in current directory"
    echo ""
    printf "  Choice [1]: "
    read -r choice
    case "$choice" in
      2) INSTALL_MODE="project" ;;
      *) INSTALL_MODE="global" ;;
    esac
  else
    # Piped install (curl | bash) — default to global, no prompt
    INSTALL_MODE="global"
  fi
fi

if [ "$INSTALL_MODE" = "project" ]; then
  EDIKT_HOME=".edikt"
  CLAUDE_COMMANDS=".claude/commands"
  echo -e "\033[1mInstalling edikt (project-local)...\033[0m"
else
  EDIKT_HOME="${HOME}/.edikt"
  CLAUDE_COMMANDS="${HOME}/.claude/commands"
  echo -e "\033[1mInstalling edikt (global)...\033[0m"
fi

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
DIM='\033[2m'
BOLD='\033[1m'
RESET='\033[0m'

info()  { echo -e "${GREEN}>${RESET} $1"; }
dim()   { echo -e "${DIM}  $1${RESET}"; }
warn()  { echo -e "${RED}!${RESET} $1"; }
error() { echo -e "${RED}error:${RESET} $1" >&2; exit 1; }

# Safe file install — backs up existing files before overwriting
BACKUP_DIR=""
BACKUP_COUNT=0
install_file() {
  local dest="$1"
  if $DRY_RUN; then
    if [ -f "$dest" ]; then
      dim "(would overwrite) $dest"
    else
      dim "(would create)    $dest"
    fi
    return 0
  fi
  # Backup existing file before overwriting
  if [ -f "$dest" ]; then
    if [ -z "$BACKUP_DIR" ]; then
      BACKUP_DIR="${EDIKT_HOME}/backups/$(date +%Y%m%d-%H%M%S)"
      mkdir -p "$BACKUP_DIR"
    fi
    local rel_path="${dest#"$EDIKT_HOME"/}"
    rel_path="${rel_path#"$CLAUDE_COMMANDS"/}"
    mkdir -p "$(dirname "$BACKUP_DIR/$rel_path")"
    cp "$dest" "$BACKUP_DIR/$rel_path"
    ((BACKUP_COUNT++))
  fi
}

# Check dependencies
command -v curl >/dev/null 2>&1 || error "curl is required"
command -v git >/dev/null 2>&1  || error "git is required"

echo

# Detect existing install
if [ -f "${EDIKT_HOME}/VERSION" ]; then
  EXISTING_VER=$(cat "${EDIKT_HOME}/VERSION" 2>/dev/null | tr -d '[:space:]')
  info "Existing edikt installation detected (v${EXISTING_VER})"
  if $DRY_RUN; then
    info "Dry run — showing what would change (no files will be written)"
    echo
  else
    info "Files will be backed up before overwriting"
    echo
  fi
fi

if $DRY_RUN; then
  info "DRY RUN — no files will be written"
  echo
fi

# Create directories (even in dry-run — needed for path checks)
if ! $DRY_RUN; then
  mkdir -p "${EDIKT_HOME}/templates/rules/base"
  mkdir -p "${EDIKT_HOME}/templates/rules/lang"
  mkdir -p "${EDIKT_HOME}/templates/rules/framework"
  mkdir -p "${EDIKT_HOME}/templates/agents"
  mkdir -p "${EDIKT_HOME}/templates/sdlc"
  mkdir -p "${CLAUDE_COMMANDS}/edikt"
  mkdir -p "${CLAUDE_COMMANDS}/edikt/adr"
  mkdir -p "${CLAUDE_COMMANDS}/edikt/invariant"
  mkdir -p "${CLAUDE_COMMANDS}/edikt/guideline"
  mkdir -p "${CLAUDE_COMMANDS}/edikt/gov"
  mkdir -p "${CLAUDE_COMMANDS}/edikt/sdlc"
  mkdir -p "${CLAUDE_COMMANDS}/edikt/docs"
  mkdir -p "${CLAUDE_COMMANDS}/edikt/deprecated"
fi

# Download edikt commands (/edikt:init, /edikt:context, etc.)
info "Installing edikt commands..."

# Flat commands (top-level)
FLAT_COMMANDS=(init upgrade doctor status context brainstorm session team agents mcp capture)
for cmd in "${FLAT_COMMANDS[@]}"; do
  dest="${CLAUDE_COMMANDS}/edikt/${cmd}.md"
  if [ -f "$dest" ] && grep -qF '<!-- edikt:custom -->' "$dest" 2>/dev/null; then
    dim "edikt:${cmd} (skipped — custom)"
  else
    install_file "$dest"
    if ! $DRY_RUN; then
      curl -fsSL "${BASE_URL}/commands/${cmd}.md" -o "$dest"
    fi
    dim "edikt:${cmd}"
  fi
done

# Namespaced commands (subdirectories)
_install_ns_cmd() {
  local ns="$1" cmd="$2"
  local dest="${CLAUDE_COMMANDS}/edikt/${ns}/${cmd}.md"
  if [ -f "$dest" ] && grep -qF '<!-- edikt:custom -->' "$dest" 2>/dev/null; then
    dim "edikt:${ns}:${cmd} (skipped — custom)"
  else
    install_file "$dest"
    if ! $DRY_RUN; then
      curl -fsSL "${BASE_URL}/commands/${ns}/${cmd}.md" -o "$dest"
    fi
    dim "edikt:${ns}:${cmd}"
  fi
}

# adr namespace
for cmd in new compile review; do
  _install_ns_cmd adr "$cmd"
done

# invariant namespace
for cmd in new compile review; do
  _install_ns_cmd invariant "$cmd"
done

# guideline namespace
for cmd in new review; do
  _install_ns_cmd guideline "$cmd"
done

# gov namespace
for cmd in compile review rules-update sync; do
  _install_ns_cmd gov "$cmd"
done

# sdlc namespace
for cmd in prd spec artifacts plan review drift audit; do
  _install_ns_cmd sdlc "$cmd"
done

# docs namespace
for cmd in review intake; do
  _install_ns_cmd docs "$cmd"
done

# deprecated namespace
for cmd in adr invariant compile review-governance rules-update sync prd spec spec-artifacts plan review drift audit docs intake; do
  _install_ns_cmd deprecated "$cmd"
done

# Download rule templates
info "Installing rule templates..."

# Registry
install_file "${EDIKT_HOME}/templates/rules/_registry.yaml"
if ! $DRY_RUN; then
  curl -fsSL "${BASE_URL}/templates/rules/_registry.yaml" -o "${EDIKT_HOME}/templates/rules/_registry.yaml"
fi

# Base rules
for rule in code-quality testing security error-handling frontend architecture api database observability seo; do
  install_file "${EDIKT_HOME}/templates/rules/base/${rule}.md"
  if ! $DRY_RUN; then
    curl -fsSL "${BASE_URL}/templates/rules/base/${rule}.md" -o "${EDIKT_HOME}/templates/rules/base/${rule}.md"
  fi
  dim "base/${rule}"
done

# Language rules
for rule in go typescript python php; do
  install_file "${EDIKT_HOME}/templates/rules/lang/${rule}.md"
  if ! $DRY_RUN; then
    curl -fsSL "${BASE_URL}/templates/rules/lang/${rule}.md" -o "${EDIKT_HOME}/templates/rules/lang/${rule}.md"
  fi
  dim "lang/${rule}"
done

# Framework rules
for rule in chi nextjs laravel symfony rails django; do
  install_file "${EDIKT_HOME}/templates/rules/framework/${rule}.md"
  if ! $DRY_RUN; then
    curl -fsSL "${BASE_URL}/templates/rules/framework/${rule}.md" -o "${EDIKT_HOME}/templates/rules/framework/${rule}.md"
  fi
  dim "framework/${rule}"
done

# Download supporting templates
info "Installing templates..."
for tmpl in CLAUDE.md.tmpl project-context.md.tmpl product-spec.md.tmpl prd.md.tmpl settings.json.tmpl; do
  install_file "${EDIKT_HOME}/templates/${tmpl}"
  if ! $DRY_RUN; then
    curl -fsSL "${BASE_URL}/templates/${tmpl}" -o "${EDIKT_HOME}/templates/${tmpl}"
  fi
  dim "${tmpl}"
done

# Agent templates
for agent in architect dba security api backend frontend qa sre platform docs pm ux data performance compliance mobile seo gtm evaluator; do
  install_file "${EDIKT_HOME}/templates/agents/${agent}.md"
  if ! $DRY_RUN; then
    curl -fsSL "${BASE_URL}/templates/agents/${agent}.md" -o "${EDIKT_HOME}/templates/agents/${agent}.md"
  fi
  dim "agents/${agent}"
done
install_file "${EDIKT_HOME}/templates/agents/_registry.yaml"
if ! $DRY_RUN; then
  curl -fsSL "${BASE_URL}/templates/agents/_registry.yaml" -o "${EDIKT_HOME}/templates/agents/_registry.yaml"
fi
dim "agents/_registry.yaml"

# Hook scripts (Claude Code hooks + git hooks)
if ! $DRY_RUN; then
  mkdir -p "${EDIKT_HOME}/templates/hooks"
  mkdir -p "${EDIKT_HOME}/hooks"
fi

# Claude Code hook scripts — installed to ~/.edikt/hooks/ and referenced from settings.json
# Event logging utility (sourced by other hooks, not executed directly)
install_file "${EDIKT_HOME}/hooks/event-log.sh"
if ! $DRY_RUN; then
  curl -fsSL "${BASE_URL}/templates/hooks/event-log.sh" -o "${EDIKT_HOME}/hooks/event-log.sh"
  curl -fsSL "${BASE_URL}/templates/hooks/event-log.sh" -o "${EDIKT_HOME}/templates/hooks/event-log.sh"
fi
dim "hooks/event-log.sh"

for hook in session-start pre-tool-use post-tool-use pre-compact stop-hook user-prompt-submit post-compact subagent-stop instructions-loaded stop-failure task-created cwd-changed file-changed headless-ask; do
  install_file "${EDIKT_HOME}/hooks/${hook}.sh"
  if ! $DRY_RUN; then
    curl -fsSL "${BASE_URL}/templates/hooks/${hook}.sh" -o "${EDIKT_HOME}/hooks/${hook}.sh"
    chmod +x "${EDIKT_HOME}/hooks/${hook}.sh"
    # Also keep in templates/ for upgrade hash comparison
    curl -fsSL "${BASE_URL}/templates/hooks/${hook}.sh" -o "${EDIKT_HOME}/templates/hooks/${hook}.sh"
  fi
  dim "hooks/${hook}.sh"
done

# Git pre-push hook template
install_file "${EDIKT_HOME}/templates/hooks/pre-push"
if ! $DRY_RUN; then
  curl -fsSL "${BASE_URL}/templates/hooks/pre-push" -o "${EDIKT_HOME}/templates/hooks/pre-push"
  chmod +x "${EDIKT_HOME}/templates/hooks/pre-push"
fi
dim "hooks/pre-push"

# SDLC templates
for sdlc in pull_request_template commit-convention; do
  install_file "${EDIKT_HOME}/templates/sdlc/${sdlc}.md"
  if ! $DRY_RUN; then
    curl -fsSL "${BASE_URL}/templates/sdlc/${sdlc}.md" -o "${EDIKT_HOME}/templates/sdlc/${sdlc}.md"
  fi
  dim "sdlc/${sdlc}"
done

# Version + Changelog
install_file "${EDIKT_HOME}/VERSION"
install_file "${EDIKT_HOME}/CHANGELOG.md"
if ! $DRY_RUN; then
  curl -fsSL "${BASE_URL}/VERSION" -o "${EDIKT_HOME}/VERSION"
  curl -fsSL "${BASE_URL}/CHANGELOG.md" -o "${EDIKT_HOME}/CHANGELOG.md"
fi

echo
if $DRY_RUN; then
  echo -e "${BOLD}Dry run complete — no files were written.${RESET}"
  echo
  echo "  Remove --dry-run to install."
else
  echo -e "${GREEN}${BOLD}edikt installed.${RESET}"
  if [ "$BACKUP_COUNT" -gt 0 ]; then
    echo
    echo -e "  Backed up ${BACKUP_COUNT} files to: ${BACKUP_DIR}"
  fi
fi
echo
echo "  Commands:  ${CLAUDE_COMMANDS}/edikt/"
echo "  Templates: ${EDIKT_HOME}/templates/"
echo
echo "  Open any project in Claude Code and run:"
echo -e "  ${BOLD}/edikt:init${RESET}"
echo

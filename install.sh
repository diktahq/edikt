#!/usr/bin/env bash
set -euo pipefail
umask 0022

# edikt installer
# TODO: add SHA-256 manifest verification once the release workflow exists
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash           # global (default)
#   curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash -s -- --project  # project-only

REPO="diktahq/edikt"
BRANCH="main"
BASE_URL="https://raw.githubusercontent.com/${REPO}/${BRANCH}"

# Parse flags — allow non-interactive mode via --global or --project
INSTALL_MODE=""
for arg in "$@"; do
  case "$arg" in
    --project) INSTALL_MODE="project" ;;
    --global)  INSTALL_MODE="global" ;;
  esac
done

# Interactive prompt if no flag provided
if [ -z "$INSTALL_MODE" ]; then
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
error() { echo -e "${RED}error:${RESET} $1" >&2; exit 1; }

# Check dependencies
command -v curl >/dev/null 2>&1 || error "curl is required"
command -v git >/dev/null 2>&1  || error "git is required"

echo

# Create directories
mkdir -p "${EDIKT_HOME}/templates/rules/base"
mkdir -p "${EDIKT_HOME}/templates/rules/lang"
mkdir -p "${EDIKT_HOME}/templates/rules/framework"
mkdir -p "${EDIKT_HOME}/templates/agents"
mkdir -p "${EDIKT_HOME}/templates/sdlc"
mkdir -p "${CLAUDE_COMMANDS}/edikt"

# Download edikt commands (/edikt:init, /edikt:context, etc.)
EDIKT_COMMANDS=(init context plan status intake doctor rules-update upgrade adr invariant prd agents mcp team docs sync audit review review-governance session spec spec-artifacts drift compile)
info "Installing edikt commands..."
for cmd in "${EDIKT_COMMANDS[@]}"; do
  dest="${CLAUDE_COMMANDS}/edikt/${cmd}.md"
  if [ -f "$dest" ] && grep -qF '<!-- edikt:custom -->' "$dest" 2>/dev/null; then
    dim "edikt:${cmd} (skipped — custom)"
  else
    curl -fsSL "${BASE_URL}/commands/${cmd}.md" -o "$dest"
    dim "edikt:${cmd}"
  fi
done

# Download rule templates
info "Installing rule templates..."

# Registry
curl -fsSL "${BASE_URL}/templates/rules/_registry.yaml" -o "${EDIKT_HOME}/templates/rules/_registry.yaml"

# Base rules
for rule in code-quality testing security error-handling frontend architecture api database observability seo; do
  curl -fsSL "${BASE_URL}/templates/rules/base/${rule}.md" -o "${EDIKT_HOME}/templates/rules/base/${rule}.md"
  dim "base/${rule}"
done

# Language rules
for rule in go typescript python php; do
  curl -fsSL "${BASE_URL}/templates/rules/lang/${rule}.md" -o "${EDIKT_HOME}/templates/rules/lang/${rule}.md"
  dim "lang/${rule}"
done

# Framework rules
for rule in chi nextjs laravel symfony rails django; do
  curl -fsSL "${BASE_URL}/templates/rules/framework/${rule}.md" -o "${EDIKT_HOME}/templates/rules/framework/${rule}.md"
  dim "framework/${rule}"
done

# Download supporting templates
info "Installing templates..."
for tmpl in CLAUDE.md.tmpl project-context.md.tmpl product-spec.md.tmpl prd.md.tmpl settings.json.tmpl; do
  curl -fsSL "${BASE_URL}/templates/${tmpl}" -o "${EDIKT_HOME}/templates/${tmpl}"
  dim "${tmpl}"
done

# Agent templates
for agent in architect dba security api backend frontend qa sre platform docs pm ux data performance compliance mobile seo gtm; do
  curl -fsSL "${BASE_URL}/templates/agents/${agent}.md" -o "${EDIKT_HOME}/templates/agents/${agent}.md"
  dim "agents/${agent}"
done
curl -fsSL "${BASE_URL}/templates/agents/_registry.yaml" -o "${EDIKT_HOME}/templates/agents/_registry.yaml"
dim "agents/_registry.yaml"

# Hook scripts (Claude Code hooks + git hooks)
mkdir -p "${EDIKT_HOME}/templates/hooks"
mkdir -p "${EDIKT_HOME}/hooks"

# Claude Code hook scripts — installed to ~/.edikt/hooks/ and referenced from settings.json
# Event logging utility (sourced by other hooks, not executed directly)
curl -fsSL "${BASE_URL}/templates/hooks/event-log.sh" -o "${EDIKT_HOME}/hooks/event-log.sh"
curl -fsSL "${BASE_URL}/templates/hooks/event-log.sh" -o "${EDIKT_HOME}/templates/hooks/event-log.sh"
dim "hooks/event-log.sh"

for hook in session-start pre-tool-use post-tool-use pre-compact stop-hook user-prompt-submit post-compact subagent-stop instructions-loaded; do
  curl -fsSL "${BASE_URL}/templates/hooks/${hook}.sh" -o "${EDIKT_HOME}/hooks/${hook}.sh"
  chmod +x "${EDIKT_HOME}/hooks/${hook}.sh"
  # Also keep in templates/ for upgrade hash comparison
  curl -fsSL "${BASE_URL}/templates/hooks/${hook}.sh" -o "${EDIKT_HOME}/templates/hooks/${hook}.sh"
  dim "hooks/${hook}.sh"
done

# Git pre-push hook template
curl -fsSL "${BASE_URL}/templates/hooks/pre-push" -o "${EDIKT_HOME}/templates/hooks/pre-push"
chmod +x "${EDIKT_HOME}/templates/hooks/pre-push"
dim "hooks/pre-push"

# SDLC templates
for sdlc in pull_request_template commit-convention; do
  curl -fsSL "${BASE_URL}/templates/sdlc/${sdlc}.md" -o "${EDIKT_HOME}/templates/sdlc/${sdlc}.md"
  dim "sdlc/${sdlc}"
done

# Version + Changelog
curl -fsSL "${BASE_URL}/VERSION" -o "${EDIKT_HOME}/VERSION"
curl -fsSL "${BASE_URL}/CHANGELOG.md" -o "${EDIKT_HOME}/CHANGELOG.md"

echo
echo -e "${GREEN}${BOLD}edikt installed.${RESET}"
echo
echo "  Commands:  ${CLAUDE_COMMANDS}/edikt/"
echo "  Templates: ${EDIKT_HOME}/templates/"
echo
echo "  Open any project in Claude Code and run:"
echo -e "  ${BOLD}/edikt:init${RESET}"
echo

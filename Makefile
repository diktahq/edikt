# edikt Makefile — developer convenience targets
#
# SANDBOX SAFETY
# --------------
# `make dev` sandboxes the payload (hooks, templates, agents) in .sandbox/
# but Claude Code commands still come from ~/.claude/commands/edikt/.
#
# `make dev-full` also swaps the commands: backs up the installed commands,
# symlinks ~/.claude/commands/edikt → repo commands/, and restores on dev-off.
# This is the recommended target for working on command files.
#
# .sandbox/ is gitignored. Run `make sandbox-clean` to start fresh.

REPO_ROOT    := $(shell pwd)
SANDBOX_DIR  := $(REPO_ROOT)/.sandbox
CLAUDE_CMDS  := $(HOME)/.claude/commands/edikt
CMDS_BACKUP  := $(SANDBOX_DIR)/commands-backup
PYTHON       := python3
PYTEST       := $(PYTHON) -m pytest

# Launcher from this repo, payload in sandbox.
EDIKT := EDIKT_ROOT=$(SANDBOX_DIR) $(REPO_ROOT)/bin/edikt

.PHONY: sandbox sandbox-clean sandbox-status

## sandbox: initialise the local sandbox (safe, idempotent)
sandbox:
	@mkdir -p $(SANDBOX_DIR)
	@echo "Sandbox ready at $(SANDBOX_DIR)"

## sandbox-clean: destroy the sandbox and start fresh
sandbox-clean: dev-full-off
	@rm -rf $(SANDBOX_DIR)
	@echo "Sandbox removed."

## sandbox-status: show what's installed in the sandbox
sandbox-status:
	@$(EDIKT) list 2>/dev/null || echo "(sandbox is empty — run 'make dev-full' first)"

# ─── Local development ────────────────────────────────────────────────────────

.PHONY: dev dev-full dev-full-off dev-off dev-check

## dev: sandbox payload only (hooks, templates, agents) — commands still from ~/.claude
dev: sandbox
	@$(EDIKT) dev link $(REPO_ROOT)
	@echo ""
	@echo "Payload sandbox active (hooks, templates, agents from repo)."
	@echo "Commands still come from ~/.claude/commands/edikt/ (installed version)."
	@echo "For command changes use: make dev-full"

## bin-link: symlink bin/edikt to ~/.local/bin/edikt so the 0.5.0 launcher
##           is on PATH in any terminal (safe — ~/.local/bin is user-space)
bin-link:
	@mkdir -p $(HOME)/.local/bin
	@if [ -L "$(HOME)/.local/bin/edikt" ]; then \
	  echo "Already linked: $(HOME)/.local/bin/edikt → $$(readlink $(HOME)/.local/bin/edikt)"; \
	else \
	  ln -s "$(REPO_ROOT)/bin/edikt" "$(HOME)/.local/bin/edikt"; \
	  echo "✓ Linked: $(HOME)/.local/bin/edikt → $(REPO_ROOT)/bin/edikt"; \
	  echo ""; \
	  echo "Make sure ~/.local/bin is on PATH. Add to ~/.zshrc if needed:"; \
	  echo '  export PATH="$$HOME/.local/bin:$$PATH"'; \
	fi

## bin-unlink: remove the ~/.local/bin/edikt symlink
bin-unlink:
	@rm -f "$(HOME)/.local/bin/edikt" && echo "✓ Removed ~/.local/bin/edikt"

## dev-full: sandbox payload AND swap commands (full dev environment)
##           backs up ~/.claude/commands/edikt, symlinks it to repo commands/
dev-full: sandbox bin-link
	@$(EDIKT) dev link $(REPO_ROOT)
	@# Swap commands — back up installed, symlink repo
	@if [ -L "$(CLAUDE_CMDS)" ]; then \
	  echo "Commands already a symlink — skipping backup"; \
	elif [ -d "$(CLAUDE_CMDS)" ]; then \
	  echo "Backing up installed commands to $(CMDS_BACKUP)..."; \
	  mkdir -p "$(SANDBOX_DIR)"; \
	  cp -R "$(CLAUDE_CMDS)" "$(CMDS_BACKUP)"; \
	  rm -rf "$(CLAUDE_CMDS)"; \
	  ln -s "$(REPO_ROOT)/commands" "$(CLAUDE_CMDS)"; \
	  echo "✓ ~/.claude/commands/edikt → $(REPO_ROOT)/commands"; \
	else \
	  ln -s "$(REPO_ROOT)/commands" "$(CLAUDE_CMDS)"; \
	  echo "✓ ~/.claude/commands/edikt → $(REPO_ROOT)/commands (fresh link)"; \
	fi
	@echo ""
	@echo "Full dev mode active:"
	@echo "  payload  → .sandbox/ (hooks, templates, agents)"
	@echo "  commands → $(REPO_ROOT)/commands"
	@echo "  Edits are live. Run 'make dev-full-off' to restore."

## dev-full-off: deactivate full dev mode, restore commands (keeps bin-link)
##               run `make bin-unlink` separately when you install a real release
dev-full-off:
	@$(EDIKT) dev unlink 2>/dev/null || true
	@# Restore commands
	@if [ -L "$(CLAUDE_CMDS)" ] && [ -d "$(CMDS_BACKUP)" ]; then \
	  rm -f "$(CLAUDE_CMDS)"; \
	  cp -R "$(CMDS_BACKUP)" "$(CLAUDE_CMDS)"; \
	  rm -rf "$(CMDS_BACKUP)"; \
	  echo "✓ Restored installed commands to ~/.claude/commands/edikt/"; \
	elif [ -L "$(CLAUDE_CMDS)" ]; then \
	  rm -f "$(CLAUDE_CMDS)"; \
	  echo "✓ Removed command symlink (no backup found — re-run install if needed)"; \
	else \
	  echo "(commands were not swapped, nothing to restore)"; \
	fi

## dev-off: deactivate payload sandbox only
dev-off:
	@$(EDIKT) dev unlink 2>/dev/null || echo "(no dev link active)"

## dev-check: confirm what's active
dev-check:
	@echo "=== Payload ==="
	@$(EDIKT) version
	@$(EDIKT) doctor; true
	@echo ""
	@echo "=== Commands ==="
	@if [ -L "$(CLAUDE_CMDS)" ]; then \
	  echo "  ~/.claude/commands/edikt → $$(readlink $(CLAUDE_CMDS)) (repo symlink ✓)"; \
	else \
	  echo "  ~/.claude/commands/edikt = installed version (run 'make dev-full' to swap)"; \
	fi

# ─── Testing ──────────────────────────────────────────────────────────────────

.PHONY: test test-hooks test-launcher test-sdk test-governance \
        test-regression test-migration test-all

## test: fast offline suite (Layer 1 hooks + Layer 3 launcher, no API key)
test:
	SKIP_INTEGRATION=1 bash test/run.sh

## test-hooks: hook unit tests only
test-hooks:
	@for f in test/unit/hooks/test_*.sh; do \
	  echo "[$$f]"; bash "$$f" $(REPO_ROOT); \
	done

## test-launcher: launcher unit tests only
test-launcher:
	@for f in test/unit/launcher/test_*.sh; do \
	  echo "[$$f]"; bash "$$f" $(REPO_ROOT); \
	done

## test-governance: governance integrity tests (no API key)
test-governance:
	$(PYTEST) test/integration/governance/ -v

## test-regression: regression museum (no API key)
test-regression:
	$(PYTEST) test/integration/regression/ -v

## test-migration: migration tests (no API key)
test-migration:
	$(PYTEST) test/integration/migration/ -v

## test-sdk: Layer 2 SDK tests (requires claude auth or ANTHROPIC_API_KEY)
test-sdk:
	$(PYTEST) test/integration/test_*.py test/integration/test_e2e_*.py \
	  test/integration/test_sidecar_only_*.py -v

## test-all: complete test suite including SDK tests
test-all:
	bash test/run.sh

# ─── Global install (touches your real ~/.edikt/) ─────────────────────────────

.PHONY: install-global install-local dev-global dev-global-off

## install-global: install edikt globally from this repo (modifies ~/.edikt/)
install-global:
	@echo "⚠️  This will modify your real ~/.edikt/ and ~/.claude/commands/edikt/"
	@echo "Press Ctrl-C to abort, Enter to continue..."
	@read _confirm
	bash install.sh --global --yes

## install-local: install the CURRENT git tag from this working tree end-to-end
##                — no network fetch. Exercises the full install.sh path: launcher
##                staging, payload copy, write_settings_json (ADR-017 permissions
##                block), managed-region sidecar, pre-v0.5.0 backup.
##                Useful for testing a v0.5.0-rc* locally before pushing the tag.
##                EDIKT_INSTALL_INSECURE=1 is set because SHA256SUMS doesn't exist
##                for an unpushed tag — the post-install banner will reflect this.
install-local:
	@echo "⚠️  This will install your current working tree as a live version,"
	@echo "   running write_settings_json and backing up ~/.claude/settings.json."
	@echo "   Reversible via: edikt rollback v0.5.0"
	@read -p "Press Enter to continue (Ctrl-C to abort)..." _confirm
	@TAG="$$(git describe --tags --exact-match 2>/dev/null || git rev-parse --abbrev-ref HEAD)"; \
	if [ -z "$$TAG" ] || [ "$$TAG" = "HEAD" ]; then \
	  echo "error: no current tag; checkout a tag or set EDIKT_LOCAL_TAG=v<x.y.z>"; \
	  exit 1; \
	fi; \
	echo "Installing working tree as $$TAG"; \
	EDIKT_LAUNCHER_SOURCE="$(REPO_ROOT)/bin/edikt" \
	EDIKT_INSTALL_SOURCE="$(REPO_ROOT)" \
	EDIKT_RELEASE_TAG="$$TAG" \
	EDIKT_INSTALL_INSECURE=1 \
	bash install.sh --global --ref "$$TAG"

## dev-global: link your real ~/.edikt/ to this repo's working tree
dev-global:
	@echo "⚠️  This will make your live Claude Code sessions use this working tree."
	@read -p "Press Enter to continue..." _confirm
	bin/edikt dev link $(REPO_ROOT)

## dev-global-off: deactivate global dev link
dev-global-off:
	bin/edikt dev unlink

# ─── Help ─────────────────────────────────────────────────────────────────────

.PHONY: help

## help: list all targets with descriptions
help:
	@echo ""
	@echo "edikt — development targets"
	@echo "==========================="
	@echo ""
	@echo "RECOMMENDED WORKFLOW:"
	@echo "  make dev-full     — full sandbox (payload + commands from repo)"
	@echo "  make dev          — payload only (hooks/templates/agents from repo)"
	@echo "  make dev-check    — confirm what's active"
	@echo "  make dev-full-off — restore everything"
	@echo ""
	@echo "TESTING (no API key needed):"
	@echo "  make test         — fast offline suite (~30s)"
	@echo "  make test-governance / test-regression / test-migration"
	@echo "  make test-sdk     — needs ANTHROPIC_API_KEY or claude auth (~5min)"
	@echo ""
	@echo "ALL TARGETS:"
	@grep -E '^## ' $(MAKEFILE_LIST) | grep -v 'global' | sed 's/## /  make /'
	@echo ""
	@echo "GLOBAL (modifies real ~/.edikt/ — use carefully):"
	@grep -E '^## .*global' $(MAKEFILE_LIST) | sed 's/## /  make /'
	@echo ""

.DEFAULT_GOAL := help

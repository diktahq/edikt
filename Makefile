# edikt Makefile — developer convenience targets
#
# SANDBOX SAFETY
# --------------
# Most targets use EDIKT_ROOT=.sandbox/ to keep all edikt state local
# to the repo directory. Your real ~/.edikt/ and ~/.claude/ are never
# touched unless you explicitly use a *-global target.
#
# .sandbox/ is gitignored. Run `make sandbox-clean` to start fresh.

REPO_ROOT   := $(shell pwd)
SANDBOX_DIR := $(REPO_ROOT)/.sandbox
PYTHON      := python3
PYTEST      := $(PYTHON) -m pytest

# ─── Sandbox helpers ──────────────────────────────────────────────────────────

# Activate a sandboxed edikt environment.
# EDIKT_ROOT points inside the repo; HOME stays real (SDK tests need it).
SANDBOX_ENV := EDIKT_ROOT=$(SANDBOX_DIR)

# Launcher from this repo (not the globally installed one).
EDIKT := $(SANDBOX_ENV) $(REPO_ROOT)/bin/edikt

.PHONY: sandbox sandbox-clean sandbox-status

## sandbox: initialise the local sandbox (safe, idempotent)
sandbox:
	@mkdir -p $(SANDBOX_DIR)
	@echo "Sandbox ready at $(SANDBOX_DIR)"

## sandbox-clean: destroy the sandbox and start fresh
sandbox-clean:
	@rm -rf $(SANDBOX_DIR)
	@echo "Sandbox removed."

## sandbox-status: show what's installed in the sandbox
sandbox-status:
	@$(EDIKT) list 2>/dev/null || echo "(sandbox is empty — run 'make dev' first)"

# ─── Local development ────────────────────────────────────────────────────────

.PHONY: dev dev-off dev-check

## dev: link the sandbox to this repo's working tree (live edits, no reinstall)
dev: sandbox
	@$(EDIKT) dev link $(REPO_ROOT)
	@echo ""
	@echo "Dev mode active. Edits to templates/, commands/, hooks/ are live."
	@echo "Run 'make dev-off' to deactivate."

## dev-off: deactivate dev mode in the sandbox
dev-off:
	@$(EDIKT) dev unlink 2>/dev/null || echo "(no dev link active)"

## dev-check: confirm which version is active in the sandbox
dev-check:
	@$(EDIKT) version
	@$(EDIKT) doctor

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
#
# These targets are intentionally separate and clearly named.
# They modify your real global installation.

.PHONY: install-global dev-global dev-global-off

## install-global: install edikt globally from this repo (modifies ~/.edikt/)
install-global:
	@echo "⚠️  This will modify your real ~/.edikt/ and ~/.claude/commands/edikt/"
	@echo "Press Ctrl-C to abort, Enter to continue..."
	@read _confirm
	bash install.sh --global --yes

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
	@echo "SANDBOX (safe — uses .sandbox/ inside this repo, never touches ~/.edikt/):"
	@grep -E '^## ' $(MAKEFILE_LIST) | grep -v 'global' | sed 's/## /  make /'
	@echo ""
	@echo "GLOBAL (modifies your real ~/.edikt/ — use carefully):"
	@grep -E '^## .*global' $(MAKEFILE_LIST) | sed 's/## /  make /'
	@echo ""

.DEFAULT_GOAL := help

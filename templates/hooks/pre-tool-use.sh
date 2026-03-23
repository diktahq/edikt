#!/usr/bin/env bash
# edikt: PreToolUse hook (Write|Edit) — warn if project-context.md is missing
# Fires before every file write or edit to ensure edikt is properly initialized.

if [ -f '.edikt/config.yaml' ] && [ ! -f 'docs/project-context.md' ]; then
  echo '⚠️  edikt: docs/project-context.md not found. Run /edikt:init to complete setup.'
fi

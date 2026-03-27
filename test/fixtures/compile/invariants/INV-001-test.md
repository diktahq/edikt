# INV-001 — No vendor dependencies

**Status:** Active

## Rule

The project must have zero runtime vendor dependencies. All functionality is implemented in plain shell scripts and markdown. No npm, pip, cargo, or go modules.

## Why

- Zero installation friction
- No supply chain risk

## Enforcement

- No package.json, requirements.txt, go.mod, or Cargo.toml at the root

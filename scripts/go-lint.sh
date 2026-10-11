#!/usr/bin/env bash
# go-lint.sh — the Go lint gates for tools/edikt, in one place.
#
# Both lefthook (pre-push) and .github/workflows/sidecar-checks.yml call this,
# so the flags cannot drift between a local run and CI. Tool versions come
# from mise.toml.
#
#   scripts/go-lint.sh vet|staticcheck|gosec|all
#
# gosec excludes G701-G705, its experimental taint-analysis rules: they flag
# every exec/os call reachable from any input, including values already
# validated per INV-006, which the INV-006 unit tests and the INV-004 hook lint
# enforce instead. A finding that does not apply is suppressed at its site with
# `#nosec <rule> -- <reason>`, never by excluding the rule here.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)/tools/edikt"

run_vet()         { go vet ./...; }
run_staticcheck() { staticcheck ./...; }   # config: tools/edikt/staticcheck.conf
run_gosec()       { gosec -quiet -severity high -confidence medium -exclude-generated \
                      -exclude=G701,G702,G703,G704,G705 ./...; }

case "${1:-all}" in
  vet)         run_vet ;;
  staticcheck) run_staticcheck ;;
  gosec)       run_gosec ;;
  all)         run_vet && run_staticcheck && run_gosec ;;
  *) echo "usage: scripts/go-lint.sh vet|staticcheck|gosec|all" >&2; exit 3 ;;
esac

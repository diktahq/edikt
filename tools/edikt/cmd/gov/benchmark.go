package gov

// benchmark.go — `bin/edikt gov benchmark` parent subcommand.
//
// SPEC-009 Plan C Phase 2 / ADR-040. The benchmark subgroup holds  // edikt-guard:allow
// adversarial quality-measurement commands for verify: directives;
// the first concrete child is `cheat-rate` (see benchmark_cheatrate.go).
// Per ADR-029 Rule 3 this falls under the `gov <subcommand>` permit —  // edikt-guard:allow
// no new top-level verb is introduced.

import (
	"github.com/spf13/cobra"
)

var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Adversarial benchmarks for verify: quality measurement",
	Long: `Adversarial benchmarks that measure whether the verify: shell commands
declared in a sidecar actually catch the behavior they claim to catch.

Subcommands:
  cheat-rate    Dispatch an adversary agent per verify and report how
                often a verify accepts a behavior it should reject
                (target: <20%).`,
}

func init() {
	Cmd.AddCommand(benchmarkCmd)
}

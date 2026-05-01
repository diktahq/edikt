// Package gov contains the cobra subcommands for `edikt gov`.
package gov

import (
	"github.com/spf13/cobra"
)

// Cmd is the `edikt gov` parent command.
var Cmd = &cobra.Command{
	Use:   "gov",
	Short: "Governance commands (compile, check)",
}

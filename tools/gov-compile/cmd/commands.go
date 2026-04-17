package cmd

import (
	"github.com/diktahq/edikt/tools/gov-compile/cmd/gov"
)

func init() {
	rootCmd.AddCommand(gov.Cmd)
}

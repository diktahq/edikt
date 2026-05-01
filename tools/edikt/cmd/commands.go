package cmd

import (
	"github.com/diktahq/edikt/tools/edikt/cmd/gov"
)

func init() {
	rootCmd.AddCommand(gov.Cmd)
}

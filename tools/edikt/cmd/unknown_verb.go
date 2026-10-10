package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// unknownVerb is what a command group does when it is reached with a verb it
// does not have — including with no verb at all, where the group itself is
// not runnable. It names what was not recognised, lists the group's verbs,
// and exits non-zero.
//
// Exit code 3 is "invalid args" in edikt's convention (ADR-029 Rule 2's // edikt-guard:allow
// contract, as `sidecar approve` uses it): distinct from 1 (the command ran
// and reported a failure) so a caller can tell a wrong invocation from a
// real negative result.
func unknownVerb(cmd *cobra.Command, args []string) error {
	group := cmd.CommandPath()
	var b strings.Builder
	if len(args) > 0 {
		fmt.Fprintf(&b, "unknown %s subcommand: %s", group, args[0])
	} else if cmd.HasSubCommands() {
		fmt.Fprintf(&b, "%s needs a subcommand", group)
	} else {
		fmt.Fprintf(&b, "%s cannot be run on its own", group)
	}
	if names := subcommandNames(cmd); len(names) > 0 {
		fmt.Fprintf(&b, "\n  available: %s", strings.Join(names, ", "))
		fmt.Fprintf(&b, "\n  see: %s --help", group)
	}
	return &exitCodeError{code: 3, msg: b.String()}
}

func subcommandNames(cmd *cobra.Command) []string {
	var names []string
	for _, c := range cmd.Commands() {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		names = append(names, c.Name())
	}
	sort.Strings(names)
	return names
}

// applyUnknownVerbHandler gives every command group the same behaviour,
// recursively: a group with subcommands and no run function of its own
// refuses an unknown verb instead of printing help and exiting 0.
func applyUnknownVerbHandler(cmd *cobra.Command) {
	for _, c := range cmd.Commands() {
		applyUnknownVerbHandler(c)
		if !c.HasSubCommands() {
			continue
		}
		if c.Run != nil || c.RunE != nil {
			// A group that is also runnable on its own (`edikt hook`, say)
			// keeps its own behaviour; cobra only reaches RunE when no
			// subcommand matched, and overriding it would break the verb.
			continue
		}
		c.Args = cobra.ArbitraryArgs
		c.RunE = unknownVerb
	}
}

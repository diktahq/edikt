package gov

// detect_specialists.go — `bin/edikt gov detect-specialists`, replacing the
// inline SPECIALIST-agent routing table in commands/sdlc/post-flight.md
// (PLAN-v080 Phase 24). Not the retired governance signal-routing table — a
// different table on a different subject, which is why the consumer-migration
// scan exempts lines naming a specialist.
//
// THIS IS THE MINIMAL TABLE, AND IT SAYS SO. The full routing lives in
// commands/_shared-agent-routing.md; this is the extension-level subset
// post-flight uses to seed `gov post-flight-scope`, which then composes
// (auto ∪ required) − never. Copying the full table in would create a second
// authority that drifts from the first.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var specialistRouting = []struct {
	pattern    *regexp.Regexp
	specialist string
}{
	{regexp.MustCompile(`\.go$`), "backend"},
	{regexp.MustCompile(`\.(ts|tsx)$`), "frontend"},
	{regexp.MustCompile(`\.sql$`), "dba"},
	{regexp.MustCompile(`templates/hooks/.*\.sh$`), "security"},
	{regexp.MustCompile(`\.github/workflows/`), "sre"},
}

func init() {
	var files string
	cmd := &cobra.Command{
		Use:   "detect-specialists",
		Short: "Map changed file paths to specialist slugs",
		Long: `Print the comma-separated, sorted set of specialists implied by a
newline-separated list of changed paths.

Prints an empty line when nothing matches — that is a real answer ("no
specialist is implied"), and post-flight skips L3 on it. Deduplicated and
sorted so the output is stable for a given input.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			seen := map[string]bool{}
			for _, line := range strings.Split(files, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				for _, r := range specialistRouting {
					if r.pattern.MatchString(line) {
						seen[r.specialist] = true
					}
				}
			}
			out := make([]string, 0, len(seen))
			for s := range seen {
				out = append(out, s)
			}
			sort.Strings(out)
			fmt.Fprintln(cmd.OutOrStdout(), strings.Join(out, ","))
			return nil
		},
	}
	cmd.Flags().StringVar(&files, "files", "", "newline-separated changed paths")
	Cmd.AddCommand(cmd)
}

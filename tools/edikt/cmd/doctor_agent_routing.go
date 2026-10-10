package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/ediktpaths"
	"gopkg.in/yaml.v3"
)

// A dispatch names a subagent_type. When no agent by that name is installed,
// the host answers "Agent type not found" and the caller's natural recovery is
// general-purpose adopting the template's prose — a specialist review with no
// specialist, indistinguishable in the transcript from the real thing.
//
// This is INV-019's class seen from the diagnostic side: the registry declares  // edikt-guard:allow
// what every project gets, and what a project HAS is not evidence of what it
// was promised. Measured on this repo: 30 templates shipped, 7 agents
// installed, and the routing table named five that were never installed.
type agentRegistry struct {
	Always   []string `yaml:"always"`
	Optional []string `yaml:"optional"`
}

// resolveAgentRegistry finds the registry that DECLARES the always-installed
// set, through the ADR-005 §1 template ladder (internal/ediktpaths): the  // edikt-guard:allow
// project's .edikt/templates/ override, then the installed payload, then a
// checkout of edikt itself. It used to try the repo's templates/ FIRST, so a
// project override was ignored whenever a templates/ tree sat beside it.
//
// The ladder's candidate list is walked rather than ResolveTemplate's single
// winner: an unparsable or empty registry falls through to the next rung, as
// it always has.
func resolveAgentRegistry(projectRoot string) (agentRegistry, string, bool) {
	candidates := ediktpaths.Candidates(projectRoot, filepath.Join("agents", "_registry.yaml"))
	for _, c := range candidates {
		raw, err := os.ReadFile(c) //nolint:gosec // payload path
		if err != nil {
			continue
		}
		var reg agentRegistry
		if err := yaml.Unmarshal(raw, &reg); err != nil {
			continue
		}
		if len(reg.Always) > 0 {
			return reg, c, true
		}
	}
	return agentRegistry{}, "", false
}

func installedAgents(projectRoot string) map[string]bool {
	out := map[string]bool{}
	entries, err := os.ReadDir(filepath.Join(projectRoot, ".claude", "agents"))
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out[strings.TrimSuffix(e.Name(), ".md")] = true
	}
	return out
}

// runAgentRoutingCheck reports every agent the registry says every project
// gets that this project does not have. An OPTIONAL agent is never a finding:
// it was not promised.
func runAgentRoutingCheck(projectRoot string, out io.Writer) (errN, warnN int) {
	reg, source, ok := resolveAgentRegistry(projectRoot)
	if !ok {
		fmt.Fprintln(out, "  WARN: no agent registry resolved — the declared agent set is UNMEASURED, "+
			"which is not the same as complete. Run /edikt:upgrade to reinstall the payload")
		return 0, 1
	}
	have := installedAgents(projectRoot)
	var missing []string
	for _, a := range reg.Always {
		if !have[a] {
			missing = append(missing, a)
		}
	}
	if len(missing) == 0 {
		return 0, 0
	}
	sort.Strings(missing)
	present := len(reg.Always) - len(missing)
	fmt.Fprintf(out, "  🔴 ERROR: %d of %d always-installed agent(s) present, %d missing: %s (declared in %s). "+
		"A dispatch to a missing agent does not fail loudly: the host reports the type as unknown and the work "+
		"falls to a general-purpose agent adopting the prompt, so the review reads as a specialist's. "+
		"Run: /edikt:upgrade\n",
		present, len(reg.Always), len(missing), strings.Join(missing, ", "), filepath.Base(source))
	return 1, 0
}

package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/diktahq/edikt/tools/edikt/internal/projectroot"
	"github.com/diktahq/edikt/tools/edikt/internal/reextract"
)

// runContractState prints the one-line extraction-contract summary on out and
// nothing else. Exit 0 validated, 10 rejected or unrecorded, 11 unmeasured.
//
// It writes nothing: no state file, no journal event, no snapshot, no trust
// record. It reads the installed extractor (or the one named file) and the
// embedded validation record, so it can be called from any surface without
// changing what the surface is about to do. Outside any project it resolves
// from the working directory, as doctor does for its other checks.
func runContractState(out io.Writer, agentFile string, agentFileGiven bool) error {
	var r reextract.ContractReport
	if agentFileGiven {
		r = reextract.ReportAgentFileContract(agentFile)
	} else {
		root, found, err := projectroot.FindFromCwd()
		if err != nil {
			return fmt.Errorf("doctor --contract-state: getwd: %w", err)
		}
		if !found {
			if root, err = os.Getwd(); err != nil {
				return fmt.Errorf("doctor --contract-state: getwd: %w", err)
			}
		}
		r = reextract.ReportInstalledContract(root)
	}
	fmt.Fprintln(out, r.Summary)
	switch r.State {
	case reextract.ContractStateValidated:
		return nil
	case reextract.ContractStateRejected, reextract.ContractStateUnrecorded:
		return &exitCodeError{code: 10}
	}
	return &exitCodeError{code: 11}
}

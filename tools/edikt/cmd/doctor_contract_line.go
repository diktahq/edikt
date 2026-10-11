package cmd

import (
	"github.com/diktahq/edikt/tools/edikt/internal/reextract"
)

// missingSidecarContractLine is the one line every doctor path that tells the
// reader to run a compile command for a missing sidecar prints directly under
// that remedy: the sidecar check, the delivery-gaps finding and the
// rejected-options check's "no sidecar" note.
//
// The remedy runs the installed extraction contract. When that contract is
// validated the remedy needs no qualification and this returns "", so the
// output stays byte-identical to what it was before the line existed. In any
// other state (rejected, unrecorded, unmeasured) it returns the contract
// report summary prefixed with "contract: ". Unmeasured is never silent.
//
// It never says the remedy is blocked: compiling stays allowed, and the
// summary tells the reader to review what it produces.
//
// The leading indent is part of the line so every path prints the same bytes.
//
// projectRoot is the root the calling check already works from. Doctor passes
// its working directory to those checks and they find nothing from a
// subdirectory, so there is no second root walk here to disagree with them.
func missingSidecarContractLine(projectRoot string) string {
	r := reextract.ReportInstalledContract(projectRoot)
	if r.State == reextract.ContractStateValidated {
		return ""
	}
	return "           contract: " + r.Summary
}

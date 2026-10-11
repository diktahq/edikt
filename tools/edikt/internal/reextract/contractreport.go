package reextract

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

// ContractStateUnmeasured is the reported state when the contract's state could
// not be determined: no readable record, no resolvable version, or an
// unreadable agent file. It is never folded into "validated".
const ContractStateUnmeasured = "unmeasured"

// Closed set of reasons an unmeasured report may give. Static strings chosen by
// error KIND: resolver errors embed absolute paths and must never be echoed
// into a line that is shown in someone else's project.
const (
	reasonNoAgent         = "no extractor is installed; run /edikt:upgrade to install it"
	reasonProjectUnusable = "this project's extractor is present but declares no usable prompt_version, and the profile's copy cannot stand in for it; run /edikt:upgrade to restore it from this version's template, or add the prompt_version line that template carries"
	reasonNoVersion       = "the extractor declares no usable prompt_version; run /edikt:upgrade to restore it from this version's template, or add the prompt_version line that template carries"
	reasonFileNoVersion   = "the named agent file declares no usable prompt_version; check the path given to --agent-file, and reinstall edikt if this version's template is missing"
	reasonRecord          = "this build's validation record could not be read; reinstall edikt"
	reasonFileUnread      = "the named agent file could not be read; check the path given to --agent-file, and reinstall edikt if this version's template is missing"
	reasonUnresolvable    = "the extractor's prompt version could not be resolved; run edikt doctor for details"
)

// ContractReport is the one answer every surface gives about the extraction
// contract: its version, its state, and a single consumer-safe line.
type ContractReport struct {
	PromptVersion string
	State         string
	Summary       string
}

var promptVersionShape = regexp.MustCompile(`^v[0-9]+$`)

// ReportContract is the single function that decides what is reported about a
// contract version, so upgrade, doctor, compile and reextract cannot disagree.
func ReportContract(promptVersion string) ContractReport {
	return reportFrom(fixtureProofYAML, promptVersion)
}

func reportFrom(record []byte, promptVersion string) ContractReport {
	// Validate the value before it is rendered: a version that is not
	// vN is unmeasured, and is never interpolated into the line.
	if !promptVersionShape.MatchString(promptVersion) {
		return unmeasured(reasonUnresolvable)
	}
	state, _ := contractStateFrom(record, promptVersion)
	r := ContractReport{PromptVersion: promptVersion, State: state}
	switch state {
	case ContractStateValidated:
		r.Summary = fmt.Sprintf("extraction contract %s is validated in this build's validation record", promptVersion)
	case ContractStateRejected:
		r.Summary = fmt.Sprintf("extraction contract %s is REJECTED in this build's validation record: sidecars it writes may drop or merge rules, so review each one against its prose", promptVersion)
	case ContractStateUnrecorded:
		r.Summary = fmt.Sprintf("extraction contract %s has no entry in this build's validation record: neither validated nor rejected, so review each sidecar it writes against its prose", promptVersion)
	default:
		return unmeasured(reasonRecord)
	}
	return r
}

func unmeasured(reason string) ContractReport {
	return ContractReport{
		PromptVersion: phasea.ExtractorPromptVersionUnknown,
		State:         ContractStateUnmeasured,
		Summary:       "extraction contract state is UNMEASURED: " + reason,
	}
}

// ReportInstalledContract reports the contract of the extractor installed for
// the project at root.
func ReportInstalledContract(root string) ContractReport {
	v, err := phasea.ResolveExtractorPromptVersion(root)
	if err != nil {
		switch {
		case errors.Is(err, phasea.ErrProjectDefinitionUnusable):
			return unmeasured(reasonProjectUnusable)
		case errors.Is(err, phasea.ErrNoPromptVersion):
			return unmeasured(reasonNoVersion)
		case errors.Is(err, phasea.ErrExtractorNotFound):
			return unmeasured(reasonNoAgent)
		default:
			return unmeasured(reasonUnresolvable)
		}
	}
	return ReportContract(v)
}

// ReportAgentFileContract reports the contract of ONE named agent file, so a
// caller can preview the template it is about to install. It reads and
// reports; it feeds no ledger and no dispatch.
func ReportAgentFileContract(path string) ContractReport {
	if filepath.Base(path) != "sidecar-extractor.md" {
		return unmeasured(reasonFileUnread)
	}
	if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() {
		return unmeasured(reasonFileUnread)
	}
	ver, readable, err := phasea.ReadAgentPromptVersion(path)
	switch {
	case !readable:
		return unmeasured(reasonFileUnread)
	case err != nil, ver <= 0:
		return unmeasured(reasonFileNoVersion)
	}
	return ReportContract(fmt.Sprintf("v%d", ver))
}

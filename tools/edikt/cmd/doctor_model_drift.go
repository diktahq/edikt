package cmd

// doctor_model_drift.go — extractor model drift, reported as information
// (D57), mirroring doctor_contract_drift.go for
// the sibling field. A sidecar's recorded extractor_model no longer
// matching the currently-resolved model only says the sidecar was
// extracted under a different pin (or a different inherited session model)
// than whatever is configured now — expected whenever extractor.model
// changes or a session runs under a different model, and never itself
// evidence of a problem. It never changes doctor's exit code or
// warn/error tally.

import (
	"fmt"
	"io"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// extractorModelDriftNote names one sidecar whose recorded extractor_model
// no longer matches the currently-resolved extractor model.
type extractorModelDriftNote struct {
	ArtifactID string
	Extracted  string
	Current    string
}

// checkExtractorModelDrift compares every sidecar's recorded
// extractor_model (when present and not "unrecorded") against the
// project's currently-resolved extractor model. Returns nil when the
// current model itself cannot be resolved — an unmeasured comparison is
// reported as nothing, never as "no drift".
func checkExtractorModelDrift(projectRoot string) []extractorModelDriftNote {
	current, err := phasea.ResolveExtractorAgentModel(projectRoot)
	if err != nil || current == phasea.ExtractorModelUnknown {
		return nil
	}
	cfg, cerr := govrun.LoadConfig(projectRoot)
	if cerr != nil {
		return nil
	}
	dirs := []string{cfg.Paths.Decisions, cfg.Paths.Invariants, cfg.Paths.Guidelines}
	pairs, derr := sidecar.Discover(projectRoot, dirs)
	if derr != nil {
		return nil
	}
	var notes []extractorModelDriftNote
	for _, p := range pairs {
		if p.Sidecar == nil {
			continue
		}
		extracted := p.Sidecar.ExtractorModel
		if extracted == "" || extracted == phasea.ExtractorContractUnrecorded {
			continue
		}
		if extracted != current {
			notes = append(notes, extractorModelDriftNote{
				ArtifactID: p.ArtifactID,
				Extracted:  extracted,
				Current:    current,
			})
		}
	}
	return notes
}

// runExtractorModelDriftCheck prints one INFO line per drifted sidecar. It
// deliberately returns nothing for doctor's warn/error tally — model drift
// is expected, not a health problem.
func runExtractorModelDriftCheck(projectRoot string, out io.Writer) {
	for _, n := range checkExtractorModelDrift(projectRoot) {
		fmt.Fprintf(out, "  INFO: %s extracted under model %s, current %s\n", n.ArtifactID, n.Extracted, n.Current)
	}
}

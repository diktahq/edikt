package cmd

// doctor_contract_drift.go — extractor contract drift, reported as
// information (D57(2), AC-7.6). Unlike agent-definition drift (a WARN — the
// installed copy itself may be stale), a sidecar's recorded
// extractor_contract no longer matching the currently-resolved extractor
// agent's bytes only says the sidecar was extracted before the agent
// definition last changed — the ordinary, expected state whenever the
// extractor prompt evolves. It is never itself evidence of a problem, so it
// never changes doctor's exit code or warn/error tally: a report that
// measures something is not the same claim as a health verdict about it.

import (
	"fmt"
	"io"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// extractorContractDriftNote names one sidecar whose recorded
// extractor_contract no longer matches the currently-resolved extractor
// agent definition.
type extractorContractDriftNote struct {
	ArtifactID string
	Extracted  string
	Current    string
}

// checkExtractorContractDrift compares every sidecar's recorded
// extractor_contract (when present and not "unrecorded") against the
// project's currently-resolved extractor contract digest. Returns nil when
// the current digest itself cannot be resolved — an unmeasured comparison
// is reported as nothing, never as "no drift".
func checkExtractorContractDrift(projectRoot string) []extractorContractDriftNote {
	current, err := phasea.ResolveExtractorContractDigest(projectRoot)
	if err != nil || current == phasea.ExtractorContractUnrecorded {
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
	var notes []extractorContractDriftNote
	for _, p := range pairs {
		if p.Sidecar == nil {
			continue
		}
		extracted := p.Sidecar.ExtractorContract
		if extracted == "" || extracted == phasea.ExtractorContractUnrecorded {
			continue
		}
		if extracted != current {
			notes = append(notes, extractorContractDriftNote{
				ArtifactID: p.ArtifactID,
				Extracted:  extracted,
				Current:    current,
			})
		}
	}
	return notes
}

// runExtractorContractDriftCheck prints one INFO line per drifted sidecar.
// It deliberately returns nothing for doctor's warn/error tally — contract
// drift is expected, not a health problem (AC-7.6).
func runExtractorContractDriftCheck(projectRoot string, out io.Writer) {
	for _, n := range checkExtractorContractDrift(projectRoot) {
		fmt.Fprintf(out, "  INFO: %s extracted under contract %s, current %s\n", n.ArtifactID, n.Extracted, n.Current)
	}
}

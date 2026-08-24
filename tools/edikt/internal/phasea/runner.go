// Package phasea implements the conditional resync phase of two-phase
// gov:compile. It dispatches stale-sidecar regenerations to
// the locked sidecar-extractor agent, one fresh subagent context per
// artifact, parallel up to a concurrency cap, continue-on-error.
package phasea

import (
	"os"
	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate")


// Task describes one Phase A unit of work.
type Task struct {
	ArtifactType string // "adr", "invariant", "guideline"
	ArtifactID   string // "ADR-NNN", "INV-NNN", or guideline slug
	ParentPath   string
	SidecarPath  string
	// ProjectRoot is the tree the dispatch must write into. It exists
	// because the prompt carries only an artifact ID: the subagent resolves
	// that ID against its OWN working directory's .edikt/config.yaml, so
	// without this the write lands wherever the binary happened to be
	// invoked from, no matter which root the caller targeted. Measured
	// 2026-08-13 (F-029): `gov reextract .measure/runA --only ADR-010`  edikt-guard:allow
	// rewrote the LIVE corpus and then reported "wrote no sidecar",
	// because the writer and the check were looking at different trees.
	//
	// Empty means "the current working directory", which is correct for
	// the ordinary in-repo compile.
	ProjectRoot string
}

// DefaultExtractorModel is the model the extractor dispatch pins when
// nothing overrides it.
//
// Before this pin existed the dispatch passed no --model at all, so every
// extraction — including the banked greenfield baseline — was produced by
// whatever the CLI defaulted to at that moment. That made every
// extraction-quality measurement unattributable: a difference between two
// runs could not be assigned to the prompt, the corpus, or a model that
// changed underneath both.
//
// Pinning does not retroactively attribute the baseline; it makes runs
// from here on attributable, which is what measuring a prompt change
// requires. The corollary is that the first run after this pin may differ
// from the baseline for model reasons rather than prompt reasons — treat
// a baseline taken before it as un-comparable until re-taken.
//
// Changing this value changes what every future measurement means, so
// treat it as a governance decision rather than a version bump — ADR-040  // edikt-guard:allow
// required an amending ADR to change the benchmark adversary's model for
// the same reason.
const DefaultExtractorModel = "claude-opus-5"

// ExtractorModelEnv overrides DefaultExtractorModel for one run.
const ExtractorModelEnv = "EDIKT_EXTRACTOR_MODEL"

// ResolveExtractorModel returns the model id the extractor dispatch should
// pin, in precedence order: explicit argument, then ExtractorModelEnv,
// then DefaultExtractorModel. The result is validated before return, so a
// caller that gets a value back can pass it to argv unchecked.
//
// The environment value is operator-supplied and reaches an argv element,
// which is INV-006's definition of externally-controlled — hence the  // edikt-guard:allow
// validation here rather than trust at the call site.
func ResolveExtractorModel(explicit string) (string, error) {
	model := explicit
	if model == "" {
		model = os.Getenv(ExtractorModelEnv)
	}
	if model == "" {
		model = DefaultExtractorModel
	}
	if err := idvalidate.ModelID(model); err != nil {
		return "", err
	}
	return model, nil
}






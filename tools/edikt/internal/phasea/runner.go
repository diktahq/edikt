// Package phasea implements the conditional resync phase of two-phase
// gov:compile. It dispatches stale-sidecar regenerations to
// the locked sidecar-extractor agent, one fresh subagent context per
// artifact, continue-on-error. The concurrency cap is a PLAN field
// (budget.max_concurrency, SPEC-013 SR-012) read by the tier-1 dispatcher;
// no decorator in this process bounds anything (dispatch_bounds.go).
package phasea

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

// The three symbols that used to live here — the dispatching-session
// model constant, its env-var override, and the resolver that combined
// them — were deleted, comments included: the dispatching session no
// longer dispatches the extractor at all (the two-phase compile split
// removed the last Go-code path that shells out to `claude -p` for
// extraction), so their one remaining honest job — validating an
// operator-supplied override before a `--model` flag no code passes
// anywhere — had no call site left to protect. The extractor's own model
// is unaffected: ResolveExtractorAgentModel (agentmodel.go), which reads
// the installed sidecar-extractor.md frontmatter, is retained and remains
// the answer.

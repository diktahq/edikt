package phaseb

import (
	"os"
	"path/filepath"
	"strings"
)

// ONE TEXT, FIVE SURFACES — and this is the fifth.
//
// The artifact-kinds contract is authored once, in the guideline's dated
// amendment, and carried verbatim by the three intake interviews and by the
// governance-authoring skill. The first four are files a human edits; this one
// is COMPILE OUTPUT, so it cannot be edited to match — it has to be READ from
// the source at render time, or it drifts the moment the amendment changes and
// nobody notices, because nobody diffs a generated file against a guideline.
//
// The read is deterministic: the block between the two markers, taken from the
// guideline whose amendment carries it. No re-derivation, no summary — a
// paraphrase here is a fifth copy of the contract with different words, which
// is the failure this whole arrangement exists to prevent.
const (
	contractBlockStart = "<!-- edikt:artifact-kinds-contract:start"
	contractBlockEnd   = "<!-- edikt:artifact-kinds-contract:end -->"
)

// contractBlockForTopic hands the caller-supplied block to the ONE topic that
// carries it. Every other topic legitimately has none.
func contractBlockForTopic(block, topic string) string {
	if topic != "governance-authoring" {
		return ""
	}
	return block
}

// ContractBlockFor returns the verbatim artifact-kinds contract block for a
// topic, or "" when the topic is not the one that carries it or the source
// cannot be read.
//
// Absence is silent BY DESIGN here and only here: every other topic legitimately
// has no contract block, so a missing one is not a finding. The gate that makes
// this honest lives outside the renderer — test-artifact-kinds-contract-drift.sh
// compares all five copies byte for byte, so a block that failed to render is
// caught there rather than assumed here.
func ContractBlockFor(projectRoot, topic string) string {
	if topic != "governance-authoring" {
		return ""
	}
	dir := filepath.Join(projectRoot, "docs", "guidelines")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // corpus path
		if err != nil {
			continue
		}
		if block := extractContractBlock(string(raw)); block != "" {
			return block
		}
	}
	return ""
}

// extractContractBlock returns the marked block, end marker included, or "".
func extractContractBlock(body string) string {
	i := strings.Index(body, contractBlockStart)
	if i < 0 {
		return ""
	}
	j := strings.Index(body[i:], contractBlockEnd)
	if j < 0 {
		return ""
	}
	return strings.TrimRight(body[i:i+j+len(contractBlockEnd)], "\n")
}

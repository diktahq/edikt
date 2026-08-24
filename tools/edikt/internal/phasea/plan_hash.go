package phasea

// plan_hash.go — the ADR-020 content-derivation arithmetic: corpus_hash,
// config_hash, and plan_id, exactly as templates/schemas/phasea-dispatch-plan.v1.schema.json
// documents them (that schema, not spec.md's now-superseded formula, is
// authoritative — see its created_from.corpus_hash/config_hash field docs).
// No timestamp, no random component, anywhere in this file.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

func sha256Hex(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func readFileOrNil(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// corpusHash is sha256 hex of the sorted, newline-joined
// "<item_id> <reason> <artifact_sha256>" lines for every candidate item —
// the FULL work-list, computed before any ceiling is applied, so plan_id is
// identical whether the plan is emitted or refused for exceeding budget.
// items MUST already be sorted ascending by item_id (computeWorkList's own
// contract) — this function does not re-sort, so a caller that skips that
// contract gets a wrong hash rather than a silently-corrected one.
func corpusHash(items []workItem) string {
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = fmt.Sprintf("%s %s %s", it.ItemID, it.Reason, it.ArtifactSHA256)
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// configHash is sha256 hex of "<ceiling>\n<force_ceiling>\n<max_attempts_per_item>\n<prompt_version>"
// — the four configuration inputs the landed schema documents (the
// force_ceiling addition is the SPEC-013 pre-flight correction; ceiling/
// max_attempts_per_item/prompt_version are the original three).
func configHash(cfg *Config, promptVersion string) string {
	s := fmt.Sprintf("%d\n%d\n%d\n%s", cfg.Ceiling, cfg.ForceCeiling, cfg.MaxAttemptsPerItem, promptVersion)
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// derivePlanID is "pap-" + the first 12 hex characters of
// sha256(corpusHashHex + configHashHex) — the two hex STRINGS concatenated,
// not their raw bytes, per the schema's own description.
func derivePlanID(corpusHashHex, configHashHex string) string {
	sum := sha256.Sum256([]byte(corpusHashHex + configHashHex))
	return "pap-" + hex.EncodeToString(sum[:])[:12]
}

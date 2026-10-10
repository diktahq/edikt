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

// corpusHash is corpusHashWithSuccession with no predecessor: the id a plan
// carries when nothing it would collide with is terminal.
func corpusHash(items []workItem) string {
	return corpusHashWithSuccession(items, nil)
}

// corpusHashWithSuccession is sha256 hex of the newline-joined lines
//
//	<item_id> <reason> <artifact_sha256> <sidecar_sha256|->
//
// for every candidate item — the FULL work-list, computed before any ceiling
// is applied, so plan_id is identical whether the plan is emitted or refused
// for exceeding budget — followed, in chain order, by one
//
//	supersedes <plan_id>
//
// line per terminal plan this one succeeds.
//
// The fourth field is the sha256 of the item's PRE-DISPATCH sidecar bytes
// (the bytes the pin captures), or "-" when no sidecar exists. Without it a
// re-requested extraction of an unchanged artifact recomputed the id of a
// plan that was already consumed or FAILED, and Plan() wrote over that closed
// plan's file and pins (#11, c1-a). With it, an extraction that changed the
// sidecar moves the id on its own.
//
// It does not move the id when the extractor is STABLE — byte-identical
// output, so the sidecar returns to the bytes the last plan pinned (c1-c). The
// `supersedes` lines are what give that fixed point a fresh id: Plan()
// appends the terminal id and derives again (resolveTerminalCollision). The
// chain is a pure function of on-disk state — plan files and receipts — so
// identical on-disk state, receipts included, always names the identical
// plan. No timestamp, no random input.
//
// items MUST already be sorted ascending by item_id (computeWorkList's own
// contract) — this function does not re-sort, so a caller that skips that
// contract gets a wrong hash rather than a silently-corrected one.
func corpusHashWithSuccession(items []workItem, supersedes []string) string {
	lines := make([]string, 0, len(items)+len(supersedes))
	for _, it := range items {
		lines = append(lines, fmt.Sprintf("%s %s %s %s", it.ItemID, it.Reason, it.ArtifactSHA256, sidecarDigest(it.beforeSidecar)))
	}
	for _, id := range supersedes {
		lines = append(lines, "supersedes "+id)
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// sidecarDigest is the corpus line's fourth field: sha256 hex of the
// pre-dispatch sidecar bytes, or "-" when there is no sidecar.
func sidecarDigest(before []byte) string {
	if before == nil {
		return "-"
	}
	sum := sha256.Sum256(before)
	return hex.EncodeToString(sum[:])
}

// configHash is sha256 hex of
// "<ceiling>\n<force_ceiling>\n<max_attempts_per_item>\n<prompt_version>\n<contract_digest>"
// — the five configuration inputs the landed schema documents. The
// force_ceiling addition was the SPEC-013 pre-flight correction; the
// contract_digest addition means two plans built from a prompt that was
// patched and then reverted (same prompt_version, different bodies
// in between) never share a plan_id.
func configHash(cfg *Config, promptVersion, contractDigest string) string {
	s := fmt.Sprintf("%d\n%d\n%d\n%s\n%s", cfg.Ceiling, cfg.ForceCeiling, cfg.MaxAttemptsPerItem, promptVersion, contractDigest)
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

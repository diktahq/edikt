package phaseb

// phaseb_property_test.go — SPEC-009 Plan A Phase 11 / AC-11.2.
//
// Property-based tests for Phase B compile-time constraints (ADR-036 §2),
// enforced by validatePhaseBConstraints in merge.go. The three properties
// pinned across a randomized corpus of directives:
//
//   - verify_kind: behavioral + empty falsifying_observation       → MUST fail
//   - verify_kind: structural (any falsifying_observation shape)   → MUST pass
//   - no verify_kind AND no verify field                            → MUST pass
//
// The Merge function is the production entry point — driving the property
// through Merge (rather than re-implementing validatePhaseBConstraints in
// the test) keeps the test honest: any regression in the constraint
// dispatch, the validator wiring, or the error-message contract surfaces
// here.

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// mergeOne builds a single-pair corpus around d and runs Merge against a
// fresh tempdir. Returns the error from Merge (nil on success).
func mergeOne(t *testing.T, d sidecar.Directive) error {
	t.Helper()
	root := t.TempDir()
	pair := mkPair(t, root, "ADR-001-property", "ptopic", []sidecar.Directive{d})
	_, err := Merge(root, []sidecar.Pair{pair}, Options{
		CompiledAt:      "2026-05-23T00:00:00Z",
		CompilerVersion: "0.6.0-property",
	})
	return err
}

// randomFalsifyingObservation returns a non-empty falsifying-observation
// string of varying shape (length, whitespace, punctuation) so the
// behavioral-positive property is exercised across plausible inputs.
func randomFalsifyingObservation(r *rand.Rand) string {
	templates := []string{
		"A test that exercises the violating path passes.",
		"go test ./... exits 0 when the rule is broken.",
		"   leading whitespace observation   ",
		"Observation with punctuation: foo, bar; baz.",
		"Short.",
	}
	return templates[r.Intn(len(templates))]
}

// TestPropertyPhaseB pins the three Phase B constraint properties.
func TestPropertyPhaseB(t *testing.T) {
	se := sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}

	t.Run("behavioral_without_falsifying_observation_fails", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260523))
		const N = 50
		for i := 0; i < N; i++ {
			// Behavioral kind with empty falsifying_observation must fail.
			// Verify present so the "verify set but verify_kind empty"
			// branch is not triggered first — we want to exercise the
			// behavioral+empty-observation path specifically.
			d := sidecar.Directive{
				Text:                  "behavioral directive",
				SourceExcerpt:         se,
				Verify:                "go test ./...",
				VerifyKind:            "behavioral",
				FalsifyingObservation: "", // explicitly empty
			}
			// Random no-op fields that should not affect the verdict.
			if r.Intn(2) == 1 {
				d.Intent = "an intent claim"
			}
			err := mergeOne(t, d)
			if err == nil {
				t.Fatalf("iter %d: expected error for behavioral kind without falsifying_observation, got nil", i)
			}
			if !strings.Contains(err.Error(), "falsifying_observation is empty") {
				t.Errorf("iter %d: error does not mention falsifying_observation: %v", i, err)
			}
		}
	})

	t.Run("structural_passes_regardless_of_observation", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260524))
		const N = 50
		for i := 0; i < N; i++ {
			d := sidecar.Directive{
				Text:          "structural directive",
				SourceExcerpt: se,
				Verify:        "grep -q foo bar.txt",
				VerifyKind:    "structural",
			}
			// Randomize the observation field — structural kind does not
			// require it, and Phase B must accept either shape.
			if r.Intn(2) == 1 {
				d.FalsifyingObservation = randomFalsifyingObservation(r)
			}
			err := mergeOne(t, d)
			if err != nil {
				t.Errorf("iter %d: structural kind unexpectedly failed Phase B: %v", i, err)
			}
		}
	})

	t.Run("no_verify_and_no_verify_kind_passes", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260525))
		const N = 50
		for i := 0; i < N; i++ {
			// Pure v1.1-shape directive: no verify, no verify_kind.
			// Phase B must accept it unconditionally; the constraint
			// "verify set but verify_kind empty" only fires when Verify
			// is non-empty.
			d := sidecar.Directive{
				Text:          "legacy directive",
				SourceExcerpt: se,
			}
			// Random optional fields that are explicitly orthogonal to
			// the constraint: Intent + FalsifyingObservation may be
			// present without verify or verify_kind and still pass.
			if r.Intn(2) == 1 {
				d.Intent = "legacy intent"
			}
			if r.Intn(2) == 1 {
				d.FalsifyingObservation = randomFalsifyingObservation(r)
			}
			err := mergeOne(t, d)
			if err != nil {
				t.Errorf("iter %d: legacy v1.1 directive unexpectedly failed Phase B: %v", i, err)
			}
		}
	})

	t.Run("verify_without_verify_kind_fails", func(t *testing.T) {
		// Sanity property: the other arm of validatePhaseBConstraints.
		// If verify is set but verify_kind is empty, Phase B MUST fail
		// with the canonical migration-pointer error.
		r := rand.New(rand.NewSource(20260526))
		const N = 50
		for i := 0; i < N; i++ {
			d := sidecar.Directive{
				Text:          "directive with verify but no verify_kind",
				SourceExcerpt: se,
				Verify:        "grep ok x",
			}
			if r.Intn(2) == 1 {
				d.Intent = "ancillary intent"
			}
			err := mergeOne(t, d)
			if err == nil {
				t.Fatalf("iter %d: expected error for verify-without-verify_kind, got nil", i)
			}
			if !strings.Contains(err.Error(), "verify_kind is empty") {
				t.Errorf("iter %d: error does not mention verify_kind: %v", i, err)
			}
		}
	})

	// SPEC-009 Plan C Phase 4 — bidirectional fixture gate properties.
	// Every behavioral directive MUST declare both positive and negative
	// fixture paths; missing either is a Phase B compile error. Files need
	// not exist on disk — declaration is sufficient (existence is a
	// benchmark-time concern, validated in Plan C Phase 5+).

	t.Run("behavioral_without_positive_fixture_fails", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260527))
		const N = 50
		for i := 0; i < N; i++ {
			d := sidecar.Directive{
				Text:                  "behavioral directive without positive fixture",
				SourceExcerpt:         se,
				Verify:                "go test ./...",
				VerifyKind:            "behavioral",
				FalsifyingObservation: randomFalsifyingObservation(r),
				// PositiveFixturePath intentionally empty
				NegativeFixturePath: "test/fixtures/bidirectional/example-behavioral-directive.negative.sh",
			}
			if r.Intn(2) == 1 {
				d.Intent = "ancillary intent"
			}
			err := mergeOne(t, d)
			if err == nil {
				t.Fatalf("iter %d: expected error for behavioral kind without positive_fixture_path, got nil", i)
			}
			if !strings.Contains(err.Error(), "positive_fixture_path is empty") {
				t.Errorf("iter %d: error does not mention positive_fixture_path: %v", i, err)
			}
		}
	})

	t.Run("behavioral_without_negative_fixture_fails", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260528))
		const N = 50
		for i := 0; i < N; i++ {
			d := sidecar.Directive{
				Text:                  "behavioral directive without negative fixture",
				SourceExcerpt:         se,
				Verify:                "go test ./...",
				VerifyKind:            "behavioral",
				FalsifyingObservation: randomFalsifyingObservation(r),
				PositiveFixturePath:   "test/fixtures/bidirectional/example-behavioral-directive.positive.sh",
				// NegativeFixturePath intentionally empty
			}
			if r.Intn(2) == 1 {
				d.Intent = "ancillary intent"
			}
			err := mergeOne(t, d)
			if err == nil {
				t.Fatalf("iter %d: expected error for behavioral kind without negative_fixture_path, got nil", i)
			}
			if !strings.Contains(err.Error(), "negative_fixture_path is empty") {
				t.Errorf("iter %d: error does not mention negative_fixture_path: %v", i, err)
			}
		}
	})

	t.Run("behavioral_with_both_fixtures_passes", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260529))
		const N = 50
		for i := 0; i < N; i++ {
			d := sidecar.Directive{
				Text:                  "behavioral directive with both fixtures",
				SourceExcerpt:         se,
				Verify:                "go test ./...",
				VerifyKind:            "behavioral",
				FalsifyingObservation: randomFalsifyingObservation(r),
				PositiveFixturePath:   "test/fixtures/bidirectional/example-behavioral-directive.positive.sh",
				NegativeFixturePath:   "test/fixtures/bidirectional/example-behavioral-directive.negative.sh",
			}
			if r.Intn(2) == 1 {
				d.Intent = "ancillary intent"
			}
			err := mergeOne(t, d)
			if err != nil {
				t.Errorf("iter %d: behavioral directive with both fixture paths unexpectedly failed Phase B: %v", i, err)
			}
		}
	})
}

package sidecar

// migration_property_test.go — SPEC-009 Plan A Phase 11 / AC-11.1.
//
// Property-based tests for the v1.1 → v1.2 schema migration semantics.
//
// The migration logic itself lives in tools/edikt/cmd/migrate_sidecars.go
// (runV12MigrationPass — package-private to cmd). Replicating the loop
// inline here keeps the property under test in the same package as the
// Directive/Prohibition structs and avoids an import cycle. The semantic
// contract under test (ADR-036 §3, SPEC-009 sre F6 mitigation):
//
//	for every directive d:
//	  if d.Verify != "" && d.VerifyKind == "":
//	    d.VerifyKind = "structural"
//
// Three properties pinned (manual randomized table tests using math/rand
// with a fixed seed for determinism — testing/quick is too rigid to
// generate the conditional-field shapes we want):
//
//   - non-empty verify + empty verify_kind  → migrated to "structural"
//   - empty verify                          → verify_kind stays empty
//   - migration is idempotent (apply twice = apply once)
//
// No dependency on the actual cmd-layer pass; the migration is a pure
// field rewrite that can be expressed directly over Directive values.

import (
	"math/rand"
	"testing"
)

// migrateV12Directive applies the v1.1 → v1.2 backfill rule to a single
// directive. Mirrors the cmd/migrate_sidecars.go loop body exactly so the
// property assertions track the production rule.
func migrateV12Directive(d *Directive) bool {
	if d.Verify != "" && d.VerifyKind == "" {
		d.VerifyKind = "structural"
		return true
	}
	return false
}

// migrateV12Prohibition is the symmetric pass for Prohibition. Same rule.
func migrateV12Prohibition(p *Prohibition) bool {
	if p.Verify != "" && p.VerifyKind == "" {
		p.VerifyKind = "structural"
		return true
	}
	return false
}

// randDirectiveV11 generates a v1.1-shaped directive: verify_kind, intent,
// falsifying_observation, human_approved_at are all empty (the v1.2 fields
// did not exist pre-migration). Verify is non-empty with probability
// hasVerifyProb so both branches of the migration rule are exercised.
func randDirectiveV11(r *rand.Rand, hasVerifyProb float64) Directive {
	d := Directive{
		Text: "directive text",
		SourceExcerpt: SourceExcerpt{
			LineStart: 1, LineEnd: 1, Quote: "x",
		},
	}
	if r.Float64() < hasVerifyProb {
		d.Verify = "echo ok"
	}
	return d
}

func randProhibitionV11(r *rand.Rand, hasVerifyProb float64) Prohibition {
	p := Prohibition{
		Text: "prohibition text",
		SourceExcerpt: SourceExcerpt{
			LineStart: 1, LineEnd: 1, Quote: "y",
		},
	}
	if r.Float64() < hasVerifyProb {
		p.Verify = "echo ok"
	}
	return p
}

// TestPropertyMigrate pins the three v1.1 → v1.2 migration properties over
// a randomized corpus of 500 directives + 500 prohibitions.
func TestPropertyMigrate(t *testing.T) {
	t.Run("non_empty_verify_yields_structural", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260523))
		const N = 500
		for i := 0; i < N; i++ {
			d := randDirectiveV11(r, 1.0) // every entry has verify
			migrateV12Directive(&d)
			if d.Verify == "" {
				t.Fatalf("iter %d: precondition violated (verify must be non-empty)", i)
			}
			if d.VerifyKind != "structural" {
				t.Errorf("iter %d: verify=%q → verify_kind=%q; want structural",
					i, d.Verify, d.VerifyKind)
			}
		}
	})

	t.Run("empty_verify_yields_empty_verify_kind", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260523))
		const N = 500
		for i := 0; i < N; i++ {
			d := randDirectiveV11(r, 0.0) // no entry has verify
			migrateV12Directive(&d)
			if d.Verify != "" {
				t.Fatalf("iter %d: precondition violated (verify must be empty)", i)
			}
			if d.VerifyKind != "" {
				t.Errorf("iter %d: empty verify → verify_kind=%q; want empty",
					i, d.VerifyKind)
			}
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		r := rand.New(rand.NewSource(20260523))
		const N = 500
		for i := 0; i < N; i++ {
			d := randDirectiveV11(r, 0.5)
			migrateV12Directive(&d)
			afterFirst := d.VerifyKind

			modified := migrateV12Directive(&d)
			if modified {
				t.Errorf("iter %d: second migration reported a change; want idempotent", i)
			}
			if d.VerifyKind != afterFirst {
				t.Errorf("iter %d: verify_kind drifted on second pass: %q → %q",
					i, afterFirst, d.VerifyKind)
			}
		}
	})

	t.Run("prohibitions_track_same_rule", func(t *testing.T) {
		// Symmetric pass — same three properties applied to Prohibition.
		r := rand.New(rand.NewSource(20260524))
		const N = 500
		for i := 0; i < N; i++ {
			p := randProhibitionV11(r, 0.5)
			hadVerify := p.Verify != ""
			migrateV12Prohibition(&p)
			if hadVerify && p.VerifyKind != "structural" {
				t.Errorf("iter %d: prohibition with verify → verify_kind=%q; want structural", i, p.VerifyKind)
			}
			if !hadVerify && p.VerifyKind != "" {
				t.Errorf("iter %d: prohibition without verify → verify_kind=%q; want empty", i, p.VerifyKind)
			}
			// idempotence
			modified := migrateV12Prohibition(&p)
			if modified {
				t.Errorf("iter %d: prohibition second migration reported a change", i)
			}
		}
	})
}

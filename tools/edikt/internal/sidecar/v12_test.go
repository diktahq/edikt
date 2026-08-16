package sidecar

// v12_test.go — SPEC-009 Plan A Phase 2.
//
// Pins the schema v1.2 additive contract: VerifyKind, Intent,
// FalsifyingObservation, HumanApprovedAt are optional per-directive (and
// per-prohibition) fields. The Go loader's KnownFields(true) decoder
// accepts the new fields and rejects unknown ones. The conditional-required
// constraints across fields (verify_kind required when verify is set;
// falsifying_observation required when verify_kind == behavioral; etc.)
// are JSON-schema-uncheckable and Phase B compile-time enforced — those
// fixtures sit in invalid/ but Go-loader accepts them; Plan A Phase 4
// implements the enforcement.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestV12Fixtures verifies the v1.2 fixture matrix:
//   - test/fixtures/sidecars/v1.2/valid/*.edikt.yaml  → Load() returns no error
//   - test/fixtures/sidecars/v1.2/invalid/*.edikt.yaml → Go-loader rejects
//     the fixtures listed in goLoaderInvalid (currently 1 — the rest are
//     schema-only or Phase-B-only invalid and have file-header comments
//     documenting which layer rejects them).
func TestV12Fixtures(t *testing.T) {
	root := repoRoot(t)
	validDir := filepath.Join(root, "test", "fixtures", "sidecars", "v1.2", "valid")
	invalidDir := filepath.Join(root, "test", "fixtures", "sidecars", "v1.2", "invalid")

	// Valid fixtures: all MUST load without error.
	validEntries, err := os.ReadDir(validDir)
	if err != nil {
		t.Fatalf("read valid dir: %v", err)
	}
	if len(validEntries) == 0 {
		t.Fatal("no valid v1.2 fixtures found — fixture directory empty")
	}
	for _, e := range validEntries {
		if !strings.HasSuffix(e.Name(), ".edikt.yaml") {
			continue
		}
		name := e.Name()
		t.Run("valid/"+name, func(t *testing.T) {
			path := filepath.Join(validDir, name)
			s, err := Load(path)
			if err != nil {
				t.Fatalf("Load(%s) returned unexpected error: %v", name, err)
			}
			if s == nil {
				t.Fatalf("Load(%s) returned nil sidecar without error", name)
			}
			// Per-fixture targeted assertions on v1.2 field presence.
			switch name {
			case "behavioral-with-all-fields.edikt.yaml":
				d := s.Directives[0]
				if d.VerifyKind != "behavioral" {
					t.Errorf("verify_kind: got %q, want behavioral", d.VerifyKind)
				}
				if d.Intent == "" {
					t.Error("intent: expected non-empty")
				}
				if d.FalsifyingObservation == "" {
					t.Error("falsifying_observation: expected non-empty")
				}
				if d.HumanApprovedAt == "" {
					t.Error("human_approved_at: expected non-empty")
				}
			case "tooling-without-approval.edikt.yaml":
				d := s.Directives[0]
				if d.VerifyKind != "tooling" {
					t.Errorf("verify_kind: got %q, want tooling", d.VerifyKind)
				}
				if d.HumanApprovedAt != "" {
					t.Errorf("human_approved_at: got %q, want empty (tooling kind doesn't require approval)", d.HumanApprovedAt)
				}
			case "structural-only.edikt.yaml":
				d := s.Directives[0]
				if d.VerifyKind != "structural" {
					t.Errorf("verify_kind: got %q, want structural", d.VerifyKind)
				}
			case "legacy-no-verify-kind.edikt.yaml":
				d := s.Directives[0]
				if d.VerifyKind != "" {
					t.Errorf("verify_kind: got %q, want empty (legacy v1.1 shape)", d.VerifyKind)
				}
				if d.Verify != "" {
					t.Errorf("verify: got %q, want empty (legacy fixture without verify)", d.Verify)
				}
			}
		})
	}

	// Invalid fixtures: categorize by which layer rejects.
	// Go-loader-rejected fixtures: must fail Load() with an error.
	// Schema-only or Phase-B-only fixtures: Go-loader accepts; file-header
	// comments document which downstream layer enforces.
	goLoaderInvalid := map[string]bool{
		"unknown-field-additionalproperties.edikt.yaml": true,
	}

	invalidEntries, err := os.ReadDir(invalidDir)
	if err != nil {
		t.Fatalf("read invalid dir: %v", err)
	}
	if len(invalidEntries) == 0 {
		t.Fatal("no invalid v1.2 fixtures found — fixture directory empty")
	}
	for _, e := range invalidEntries {
		if !strings.HasSuffix(e.Name(), ".edikt.yaml") {
			continue
		}
		name := e.Name()
		t.Run("invalid/"+name, func(t *testing.T) {
			path := filepath.Join(invalidDir, name)
			_, err := Load(path)
			if goLoaderInvalid[name] {
				if err == nil {
					t.Errorf("Load(%s): expected error (KnownFields/additionalProperties layer), got nil", name)
				}
			} else {
				// Schema-only or Phase-B-only invalid. Go-loader accepts.
				// AJV schema-validate CI catches schema-only cases; Plan A
				// Phase 4 catches the Phase-B-only conditional-required cases.
				if err != nil {
					t.Errorf("Load(%s): expected nil error (rejection is at schema or compile layer, not Go-loader); got: %v", name, err)
				}
			}
		})
	}
}

// TestV12_VerifyKindAccessibleViaStruct is a smoke test that the new struct
// fields are reachable from external callers. Catches a regression where
// the YAML tag is correct but the Go field name diverges or the field is
// unexported.
func TestV12_VerifyKindAccessibleViaStruct(t *testing.T) {
	d := Directive{
		Text:                  "test directive",
		Verify:                "true",
		VerifyKind:            "behavioral",
		Intent:                "test intent",
		FalsifyingObservation: "test observation",
		HumanApprovedAt:       "2026-05-22T19:00:00Z",
	}
	if d.VerifyKind != "behavioral" {
		t.Errorf("VerifyKind: got %q", d.VerifyKind)
	}
	if d.Intent == "" || d.FalsifyingObservation == "" || d.HumanApprovedAt == "" {
		t.Error("v1.2 fields not accessible on Directive")
	}

	p := Prohibition{
		Text:                  "test prohibition",
		Verify:                "true",
		VerifyKind:            "tooling",
		Intent:                "test intent",
		FalsifyingObservation: "test observation",
		HumanApprovedAt:       "2026-05-22T19:00:00Z",
	}
	if p.VerifyKind != "tooling" {
		t.Errorf("Prohibition.VerifyKind: got %q", p.VerifyKind)
	}
}

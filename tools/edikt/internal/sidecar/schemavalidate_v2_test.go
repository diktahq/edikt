package sidecar

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRootV2(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..", "..", "..")
}

// TestSchemaMirrorV2IsByteIdentical guards the embedded v2 copy against drift
// from the authoritative file, exactly as the v1 mirror is guarded. Two copies
// of a schema that may silently disagree is worse than one.
func TestSchemaMirrorV2IsByteIdentical(t *testing.T) {
	authoritative := filepath.Join(repoRootV2(t), "templates", "schemas", "gov-sidecar.v2.schema.json")
	want, err := os.ReadFile(authoritative)
	if err != nil {
		t.Fatalf("read authoritative v2 schema: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(govSidecarSchemaV2)) {
		t.Fatalf("embedded v2 schema mirror has DRIFTED from %s.\n"+
			"The authoritative file is templates/schemas/gov-sidecar.v2.schema.json.\n"+
			"Resync with:\n"+
			"  cp templates/schemas/gov-sidecar.v2.schema.json tools/edikt/internal/sidecar/schema/",
			authoritative)
	}
}

// TestV2RejectsSingularSourceExcerpt is the AC-1.2 assertion.
//
// It runs the validator against the real fixture and matches the error TEXT. It
// deliberately does not grep the schema or the Go source for the string
// "source_excerpt": that would prove a field name appears somewhere, never that
// anything rejects it. A rule's check must exercise the behaviour, not its
// spelling (GL-002).
//
// Rejection lives here rather than in Load() because D22/D45 ruled Load
// deliberately permissive, and v12_test.go pins that. This is the gate.
func TestV2RejectsSingularSourceExcerpt(t *testing.T) {
	fixture := filepath.Join(repoRootV2(t), "test", "fixtures", "sidecars", "v2",
		"invalid", "singular-source-excerpt.yaml")
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	err = ValidateRawAgainstSchemaV2(raw)
	if err == nil {
		t.Fatal("v1 singular source_excerpt was ACCEPTED by the v2 gate — a half-migrated " +
			"sidecar would validate with zero anchors and silently lose its grounding")
	}
	// Match the singular key's own message, not the bare substring "source_excerpt":
	// that substring also occurs inside "source_excerpts", so a validator that only
	// reported the missing PLURAL field would have satisfied a Contains check while
	// saying nothing about the offending singular key. The assertion must be able to
	// tell the two apart, or it is agreeing with a coincidence.
	const wantSingular = "additional properties 'source_excerpt' not allowed"
	const wantPlural = "missing property 'source_excerpts'"
	if !strings.Contains(err.Error(), wantSingular) {
		t.Fatalf("rejected, but the error never names the offending singular key, so the user "+
			"cannot tell what to fix.\nwant substring: %q\ngot: %v", wantSingular, err)
	}
	if !strings.Contains(err.Error(), wantPlural) {
		t.Fatalf("rejected, but the error never says which field is missing.\n"+
			"want substring: %q\ngot: %v", wantPlural, err)
	}
	t.Logf("rejected naming the field: %v", err)
}

// TestV2AcceptsMultiAndSingleAnchor is the isolation control. Without it, a gate
// that rejected everything would pass the test above.
func TestV2AcceptsMultiAndSingleAnchor(t *testing.T) {
	dir := filepath.Join(repoRootV2(t), "test", "fixtures", "sidecars", "v2", "valid")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read valid fixture dir: %v", err)
	}
	checked := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if err := ValidateRawAgainstSchemaV2(raw); err != nil {
			t.Errorf("%s should validate under v2 but did not: %v", e.Name(), err)
		}
		checked++
	}
	// INV-013: an empty fixture dir would otherwise make this test pass having
	// observed nothing at all.
	if checked == 0 {
		t.Fatal("no valid v2 fixtures found — this control measured nothing, which is " +
			"UNMEASURED, not a pass")
	}
	t.Logf("validated %d valid v2 fixture(s)", checked)
}

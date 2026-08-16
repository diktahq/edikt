package sidecar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Marshal returns Load's cached canonical bytes. That is correct for the
// read-only fingerprint path and a footgun for every mutating caller: the
// mutation is silently discarded and the write reports success.
//
// It has now bitten THREE independent call sites:
//
//	cmd/sidecar.go        defended locally by loadForMutation (fixed at the
//	                      call site, not at the source — which is why the
//	                      next two callers repeated it)
//	govrun/twophase.go    the migration_preserved strip was a NO-OP; ADR-034
//	                      requires that strip, and one sidecar carried the
//	                      field through every compile
//	sidecar/bodydrift.go  StampBodyDigest wrote no digest at all
//
// The fix was to remove the footgun at the source rather than defend a fourth
// call site: the memoized variant is gone and MarshalFresh's semantics are now
// the default, because the default should be the one that is correct when you
// are not thinking about it. This test asserts that directly — if a memoized
// Marshal ever returns, it fails here rather than in whichever caller mutates
// next.
func TestMarshalReflectsPostLoadMutation(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.edikt.yaml")
	if err := os.WriteFile(p, []byte("schema_version: 1\ntopic: demo\npath: x.md\nsignals: [demo]\ndirectives: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}

	sc.BodyDigest = BodyDigest("anything")

	out, err := Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "body_digest") {
		t.Fatal("Marshal dropped a field mutated after Load — the memoized " +
			"variant is back, and every mutating caller is silently losing writes")
	}
}

// The specific regression: stripping MigrationPreserved must actually remove
// it from the bytes that get written. Asserts the OUTCOME (what lands on
// disk), not that a particular function was called.
func TestStrippedMigrationPreservedDoesNotSurviveSerialization(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.edikt.yaml")
	y := "schema_version: 1\ntopic: demo\npath: x.md\nsignals: [demo]\ndirectives: []\n" +
		"migration_preserved:\n  schema_detected: v0.5x-full\n  directives:\n    - \"legacy\"\n"
	if err := os.WriteFile(p, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if sc.MigrationPreserved == nil {
		t.Fatal("fixture did not load migration_preserved — the test would prove nothing")
	}

	sc.MigrationPreserved = nil
	out, err := Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "migration_preserved") {
		t.Error("migration_preserved survived the strip — ADR-034 requires that " +
			"steady-state sidecars carry no transient field")
	}
}

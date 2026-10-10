package govrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NO CODE PATH REACHABLE FROM A DOCUMENTED NON-WRITING FLAG MAY WRITE A
// SOURCE ARTIFACT.
//
// `--check`, `--check --json` and `--dry-run` all set CheckOnly. Today the
// check branch returns before any writer is reached, which makes the safety
// an ORDERING property: invisible in the diff that breaks it, and untestable
// through the CLI because the refusal happens first. So the rule is asserted
// on the writer itself, where it cannot be reordered away.
func TestWriteSourceSidecar_RefusesUnderTheNonWritingFlags(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ADR-900.edikt.yaml")
	original := "# yaml-language-server: $schema=../x.json\nschema_version: 2\ntopic: testing\n"
	if err := os.WriteFile(p, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	err := writeSourceSidecar(TwoPhaseOptions{CheckOnly: true}, p, []byte("schema_version: 2\n"))
	if err == nil {
		t.Fatal("a write under CheckOnly must be refused")
	}
	if !strings.Contains(err.Error(), "--check") || !strings.Contains(err.Error(), "--dry-run") {
		t.Errorf("the refusal must name the flags whose contract it protects: %v", err)
	}
	after, _ := os.ReadFile(p)
	if string(after) != original {
		t.Fatalf("a refused write must leave the file byte-identical:\n%s", after)
	}
}

// And the write that IS allowed keeps the header the file opened with.
// Marshal emits canonical bytes with no comments, so a strip-and-rewrite
// drops the `$schema` hint the templates ship and the validator demands —
// the same loss the sidecar writer in the command layer exists to prevent.
func TestWriteSourceSidecar_KeepsTheSchemaHeader(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ADR-900.edikt.yaml")
	header := "# yaml-language-server: $schema=../../.edikt/schemas/gov-sidecar.v2.schema.json"
	if err := os.WriteFile(p, []byte(header+"\nschema_version: 2\nmigration_preserved:\n  topic: hooks\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeSourceSidecar(TwoPhaseOptions{}, p, []byte("schema_version: 2\ntopic: hooks\n")); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(after), header+"\n") {
		t.Fatalf("the strip dropped the schema header:\n%s", after)
	}
	if strings.Contains(string(after), "migration_preserved") {
		t.Fatalf("the strip did not take effect:\n%s", after)
	}
}

// A file that never had a header does not gain one: the writer preserves,
// it does not invent.
func TestWriteSourceSidecar_DoesNotInventAHeader(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ADR-900.edikt.yaml")
	if err := os.WriteFile(p, []byte("schema_version: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSourceSidecar(TwoPhaseOptions{}, p, []byte("schema_version: 2\ntopic: hooks\n")); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	if strings.Contains(string(after), "yaml-language-server") {
		t.Fatalf("the writer invented a header:\n%s", after)
	}
}

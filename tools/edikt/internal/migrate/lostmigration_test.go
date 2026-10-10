package migrate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// stageLostAndHealthy builds a project carrying exactly one artifact whose
// legacy content was destroyed by a pre-fix migration and one whose content
// survived. Both are mid-migration (Phase B has not run), which is the only
// state in which either can be observed.
func stageLostAndHealthy(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "edikt_version: \"0.8.0-rc1\"\npaths:\n  decisions: docs/architecture/decisions\n"
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	write := func(id, preserved string) {
		body := "---\nstatus: accepted\n---\n\n# " + id + " — Test\n\n## Decision\n\nTest rule.\n"
		if err := os.WriteFile(filepath.Join(adrDir, id+"-test.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		sc := "schema_version: 2\ntopic: needs-extraction\npath: \"docs/architecture/decisions/" + id + "-test.md\"\n" +
			"signals: []\ndirectives: []\nmigration_preserved:\n" + preserved
		if err := os.WriteFile(filepath.Join(adrDir, id+"-test.edikt.yaml"), []byte(sc), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// LOST: what a v0.7.0-rc1 … v0.7.3 migration left behind for an
	// unparseable block — the classification, and none of the content.
	write("ADR-001", "  schema_detected: unknown\n")
	// HEALTHY: the same unparseable block migrated on a current build.
	write("ADR-002", "  schema_detected: unknown\n  raw_body: \"- Services MUST pin every dependency. (ref: ADR-002)\"\n")
	return root
}

// The check must name the artifact that lost content and must NOT name the one
// that kept it. Both carry schema_detected: unknown — the signature is the
// ABSENCE of content beside it, not the classification.
func TestDoctorLostMigration_NamesOnlyTheLostArtifact(t *testing.T) {
	root := stageLostAndHealthy(t)
	var buf bytes.Buffer
	warns, ran := RunLostMigrationCheck(root, []string{"docs/architecture/decisions", "docs/architecture/invariants", "docs/guidelines"}, &buf)
	out := buf.String()

	if !ran {
		t.Fatalf("check did not run against a project with sidecars — an absent measurement is not a pass\n%s", out)
	}
	if warns != 1 {
		t.Errorf("want 1 warning (one lost artifact), got %d\n%s", warns, out)
	}
	if !strings.Contains(out, "lost: ADR-001") {
		t.Errorf("the artifact that LOST its block was not named:\n%s", out)
	}
	if strings.Contains(out, "lost: ADR-002") {
		t.Errorf("the artifact whose block was PRESERVED was reported as lost — "+
			"the check is keying on schema_detected rather than on missing content:\n%s", out)
	}
}

// A project where every preserved block still has its content must report a
// measured OK, not silence.
func TestDoctorLostMigration_HealthyCorpusReportsMeasuredOK(t *testing.T) {
	root := stageLostAndHealthy(t)
	// Give the lost artifact its content back.
	p := filepath.Join(root, "docs", "architecture", "decisions", "ADR-001-test.edikt.yaml")
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	fixed := strings.Replace(string(body),
		"  schema_detected: unknown\n",
		"  schema_detected: unknown\n  raw_body: \"- Services MUST validate input. (ref: ADR-001)\"\n", 1)
	if err := os.WriteFile(p, []byte(fixed), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	warns, ran := RunLostMigrationCheck(root, []string{"docs/architecture/decisions", "docs/architecture/invariants", "docs/guidelines"}, &buf)
	if !ran {
		t.Fatalf("check did not run:\n%s", buf.String())
	}
	if warns != 0 {
		t.Errorf("healthy corpus reported %d warning(s):\n%s", warns, buf.String())
	}
	if !strings.Contains(buf.String(), "OK:") {
		t.Errorf("a corpus that WAS inspected must say so rather than printing nothing:\n%s", buf.String())
	}
}

// The three states must stay distinct. Collapsing "no preservation block" into
// "healthy" would make a project that never migrated indistinguishable from one
// that migrated cleanly.
func TestClassifyLostMigration_ThreeStates(t *testing.T) {
	cases := []struct {
		name string
		sc   *sidecar.Sidecar
		want LostMigrationState
	}{
		{"no preservation block", &sidecar.Sidecar{}, LostMigrationNotApplicable},
		{"nil sidecar", nil, LostMigrationNotApplicable},
		{"directives preserved", &sidecar.Sidecar{MigrationPreserved: &sidecar.MigrationPreserved{
			SchemaDetected: "v0.5x-full", Directives: []string{"x"}}}, LostMigrationPreserved},
		{"raw_body preserved", &sidecar.Sidecar{MigrationPreserved: &sidecar.MigrationPreserved{
			SchemaDetected: "unknown", RawBody: "- x"}}, LostMigrationPreserved},
		{"schema_detected only", &sidecar.Sidecar{MigrationPreserved: &sidecar.MigrationPreserved{
			SchemaDetected: "unknown"}}, LostMigrationLost},
		{"empty block, no classification", &sidecar.Sidecar{MigrationPreserved: &sidecar.MigrationPreserved{}},
			LostMigrationLost},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyLostMigration(tc.sc); got != tc.want {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const testHeader = "# yaml-language-server: $schema=../../.edikt/schemas/prd-sidecar.v1.schema.json"

func headerOfFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return schemaHeaderOf(b)
}

// Every verb that rewrites a PRD sidecar goes through saveSidecar; the
// header the file opened with is the header it opens with after.
func TestSidecarWriter_EveryPRDVerbKeepsTheSchemaHeader(t *testing.T) {
	body := testHeader + "\nid: PRD-1\nstatus: accepted\nrequirements:\n  - id: FR-001\n    status: proposed\n_sync:\n  md_hash: x\n"
	at := "2026-09-02T00:00:00Z"
	cases := map[string]func(p string) error{
		"ship": func(p string) error {
			_, err := runPRD(t, "ship", "--file", p, "--frs", "FR-001", "--author", "me", "--at", at)
			return err
		},
		"close": func(p string) error {
			_, err := runPRD(t, "close", "--file", p, "--status", "cancelled", "--reason", "budget", "--author", "me", "--at", at)
			return err
		},
		"link-spec": func(p string) error {
			_, err := runPRD(t, "link-spec", "--file", p, "--spec", "SPEC-1", "--author", "me", "--at", at)
			return err
		},
		"supersede": func(p string) error {
			newP := writePRD(t, testHeader+"\nid: PRD-2\nstatus: draft\n")
			_, err := runCmd(t, "prd", "supersede", "--old-file", p, "--new-file", newP, "--old-id", "PRD-1", "--new-id", "PRD-2", "--author", "me", "--at", at)
			if err == nil && headerOfFile(t, newP) != testHeader {
				t.Errorf("supersede dropped the NEW sidecar's header")
			}
			return err
		},
		"util yaml record": func(p string) error {
			_, err := runCmd(t, "util", "yaml", "record", p, "--action", "review", "--note", "Rubric 9/10", "--affected", "FR-001", "--author", "me", "--at", at)
			return err
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			p := writePRD(t, body)
			if err := run(p); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if got := headerOfFile(t, p); got != testHeader {
				t.Fatalf("%s lost the schema header; line 1 is now %q", name, got)
			}
		})
	}
}

// The spec kind, through the shared writer, from the shipped template.
func TestSidecarWriter_SpecKindKeepsTheSchemaHeader(t *testing.T) {
	tmpl, err := os.ReadFile(filepath.Join("..", "..", "..", "templates", "spec-sidecar.yaml.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	// Render the template the way the spec command does: the block and
	// nullable placeholders become YAML, the quoted ones become text.
	body := strings.NewReplacer(
		"{{schema_path}}", "../../.edikt/schemas/spec-sidecar.v1.schema.json",
		"{{source_prd_or_null}}", "PRD-001", "{{source_brainstorm_or_null}}", "null", "{{source_prompt_or_null}}", "null",
		"{{source_prd_coverage_block}}", "source_prd_coverage: null",
	).Replace(string(tmpl))
	body = strings.ReplaceAll(strings.ReplaceAll(body, "{{", "x"), "}}", "x")
	p := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	want := schemaHeaderOf([]byte(body))
	if want == "" {
		t.Fatal("the shipped spec template must open with the schema header")
	}
	if _, err := runCmd(t, "util", "yaml", "record", p, "--action", "review", "--note", "Rubric 8/10", "--author", "me", "--at", "2026-09-02T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if got := headerOfFile(t, p); got != want {
		t.Fatalf("spec sidecar lost its header; line 1 is now %q", got)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "action: review") {
		t.Fatalf("record did not append the entry:\n%s", b)
	}
}

// The guard is the writer's, not the library's: a document built without the
// comment still lands with the header the file had.
func TestSidecarWriter_ReattachesAHeaderTheDocumentLost(t *testing.T) {
	p := writePRD(t, testHeader+"\nid: PRD-1\n")
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("id: PRD-1\nstatus: shipped\n"), &doc); err != nil {
		t.Fatal(err)
	}
	if err := saveSidecar(p, &doc); err != nil {
		t.Fatal(err)
	}
	if got := headerOfFile(t, p); got != testHeader {
		t.Fatalf("writer did not re-attach the header; line 1 is %q", got)
	}
}

// The validate step: a lost or empty header is a finding, never a pass.
func TestSchemaValidate_RequireSchemaHeaderIsAFinding(t *testing.T) {
	schema := filepath.Join("..", "..", "..", "templates", "schemas", "prd-sidecar.v1.schema.json")
	tmpl, err := os.ReadFile(filepath.Join("..", "..", "..", "templates", "prd-sidecar.yaml.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	// A body that validates: the template with its placeholders filled.
	r := strings.NewReplacer("{{schema_path}}", "../x.json", "{{id}}", "PRD-001", "{{title}}", "T", "{{slug}}", "t",
		"{{rigor}}", "solo", "{{author}}", "a", "{{created_at}}", "2026-09-02T00:00:00Z")
	valid := r.Replace(string(tmpl))
	// Any placeholder left sits inside a quoted string the schema does not constrain.
	valid = strings.ReplaceAll(strings.ReplaceAll(valid, "{{", "x"), "}}", "x")
	write := func(body string) string {
		p := filepath.Join(t.TempDir(), "prd.yaml")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	headerless := write(strings.Join(strings.Split(valid, "\n")[1:], "\n"))
	empty := write("# yaml-language-server: $schema=\n" + strings.Join(strings.Split(valid, "\n")[1:], "\n"))
	present := write(valid)

	if out, err := runCmd(t, "util", "schema-validate", headerless, schema); err != nil && !strings.Contains(out, "does not conform") {
		t.Fatalf("fixture must otherwise validate (without the flag): %v\n%s", err, out)
	}
	out, err := runCmd(t, "util", "schema-validate", "--require-schema-header", headerless, schema)
	if err == nil || !strings.Contains(out, "SIDECAR HEADER") || !strings.Contains(out, "missing") {
		t.Fatalf("a lost header must be a finding: err=%v\n%s", err, out)
	}
	out, err = runCmd(t, "util", "schema-validate", "--require-schema-header", empty, schema)
	if err == nil || !strings.Contains(out, "empty") {
		t.Fatalf("an empty header must be a finding: err=%v\n%s", err, out)
	}
	out, err = runCmd(t, "util", "schema-validate", "--require-schema-header", present, schema)
	if err != nil && strings.Contains(out, "SIDECAR HEADER") {
		t.Fatalf("a present header must not be a finding: %v\n%s", err, out)
	}
}

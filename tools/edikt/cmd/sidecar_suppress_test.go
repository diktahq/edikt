package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// `suppressed_directives` was READABLE AND UNWRITABLE: five places in the
// binary subtract it at render time, the compile command documents the merge
// formula that uses it, and its own misuse warning says "use
// suppressed_directives instead" — while no verb could write one.
//
// The case that matters is not hypothetical. An accepted ADR is refined by a
// later accepted ADR that withdraws one clause. The parent is immutable, so
// the extractor keeps emitting the withdrawn directive: two active directives
// requiring opposite things, and the documented remedy could not be performed.
// The only way through was hand-editing the sidecar YAML, which is what the
// enrich command's own rationale exists to prevent — and the first attempt at
// it broke the file on an apostrophe.

func runSuppress(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return runCmd(t, append([]string{"sidecar", "suppress-directive"}, args...)...)
}

func sidecarWithDirective(t *testing.T, dir, text string) string {
	t.Helper()
	body := "schema_version: 2\n" +
		"topic: testing\n" +
		"path: docs/architecture/decisions/ADR-900-x.md\n" +
		"signals: []\n" +
		"directives:\n" +
		"  - text: " + yamlQuote(text) + "\n" +
		"    source_excerpts:\n" +
		"      - line_start: 1\n" +
		"        line_end: 1\n" +
		"        quote: " + yamlQuote(text) + "\n"
	return writeTempSidecar(t, dir, "ADR-900-x", body)
}

const withdrawn = "Hooks MUST shell out to python3 for JSON. (ref: ADR-900)"

func TestSuppressDirective_WritesTheSuppressionWithItsReason(t *testing.T) {
	p := sidecarWithDirective(t, t.TempDir(), withdrawn)

	out, err := runSuppress(t, "--path", p, "--text", withdrawn, "--reason", "withdrawn by ADR-901, which forbids shelling out")
	if err != nil {
		t.Fatalf("suppression must succeed: %v\n%s", err, out)
	}
	sc, lerr := sidecar.Load(p)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if len(sc.SuppressedDirectives) != 1 || sc.SuppressedDirectives[0] != withdrawn {
		t.Fatalf("the exact text must be recorded, got %+v", sc.SuppressedDirectives)
	}
	// The directive itself is untouched: the parent is immutable and the
	// extractor will emit it again. Suppression subtracts at render time.
	if len(sc.Directives) != 1 {
		t.Fatalf("suppression must not delete the directive, got %d", len(sc.Directives))
	}
	// THE REASON IS NOT IN THE SIDECAR, and that is deliberate:
	// suppressed_directives is a list of strings and the schema is closed, so
	// a reason field is a schema change and a schema change is an ADR. The
	// decision is recorded where this project already records a human's
	// decision about a sidecar — beside the approval diffs — so the record
	// exists today and the sidecar keeps the shape every reader handles.
	raw, _ := os.ReadFile(p)
	if strings.Contains(string(raw), "ADR-901") {
		t.Errorf("the reason must NOT be written into the sidecar (the schema has no field for it):\n%s", raw)
	}
	recDir := filepath.Join(filepath.Dir(p), SuppressionRecordDir)
	found := ""
	if entries, derr := os.ReadDir(recDir); derr == nil {
		for _, e := range entries {
			b, _ := os.ReadFile(filepath.Join(recDir, e.Name()))
			if strings.Contains(string(b), "ADR-901") && strings.Contains(string(b), withdrawn) {
				found = e.Name()
			}
		}
	}
	if found == "" {
		t.Errorf("the reason must be recorded under %s so a reader knows which record withdrew it", SuppressionRecordDir)
	}
}

// Exact-string match, and a miss is a refusal: a suppression that matches
// nothing is already a compile warning, so writing one is writing a defect.
func TestSuppressDirective_RefusesWhenNoDirectiveMatches(t *testing.T) {
	p := sidecarWithDirective(t, t.TempDir(), withdrawn)
	out, err := runSuppress(t, "--path", p, "--text", "Hooks MUST NOT shell out.", "--reason", "typo in the text")
	if err == nil {
		t.Fatalf("a text matching no directive must be refused:\n%s", out)
	}
	combined := out + " " + err.Error()
	if !strings.Contains(combined, "no directive") && !strings.Contains(combined, "matches") {
		t.Errorf("the refusal must say the text matched nothing:\n%s", combined)
	}
	sc, _ := sidecar.Load(p)
	if len(sc.SuppressedDirectives) != 0 {
		t.Errorf("a refused suppression must write nothing")
	}
}

func TestSuppressDirective_RefusesADuplicate(t *testing.T) {
	p := sidecarWithDirective(t, t.TempDir(), withdrawn)
	if _, err := runSuppress(t, "--path", p, "--text", withdrawn, "--reason", "first"); err != nil {
		t.Fatal(err)
	}
	out, err := runSuppress(t, "--path", p, "--text", withdrawn, "--reason", "second")
	if err == nil {
		t.Fatalf("a duplicate suppression must be refused:\n%s", out)
	}
	sc, _ := sidecar.Load(p)
	if len(sc.SuppressedDirectives) != 1 {
		t.Fatalf("the duplicate must not be appended, got %+v", sc.SuppressedDirectives)
	}
}

// A reason is not optional: the whole point of the record is that a later
// reader knows which record withdrew the clause.
func TestSuppressDirective_RefusesWithoutAReason(t *testing.T) {
	p := sidecarWithDirective(t, t.TempDir(), withdrawn)
	if out, err := runSuppress(t, "--path", p, "--text", withdrawn); err == nil {
		t.Fatalf("a suppression with no reason must be refused:\n%s", out)
	}
}

// The apostrophe case, which is how the hand-edit broke: the writer is the
// YAML marshaller, so quoting is not the author's problem.
func TestSuppressDirective_HandlesQuotesAndApostrophes(t *testing.T) {
	text := "A hook's payload MUST NOT be \"trusted\" input. (ref: ADR-900)"
	p := sidecarWithDirective(t, t.TempDir(), text)
	if out, err := runSuppress(t, "--path", p, "--text", text, "--reason", "withdrawn by ADR-901's clause 3"); err != nil {
		t.Fatalf("an apostrophe must not break the write: %v\n%s", err, out)
	}
	sc, lerr := sidecar.Load(p)
	if lerr != nil {
		t.Fatalf("the file must still parse: %v", lerr)
	}
	if len(sc.SuppressedDirectives) != 1 || sc.SuppressedDirectives[0] != text {
		t.Fatalf("the text must round-trip exactly, got %+v", sc.SuppressedDirectives)
	}
}

// A missing sidecar is exit 2, matching add-manual-directive's contract.
func TestSuppressDirective_MissingSidecarIsExitTwo(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "ADR-999-x.edikt.yaml")
	out, err := runSuppress(t, "--path", missing, "--text", withdrawn, "--reason", "x")
	if err == nil {
		t.Fatalf("a missing sidecar must be refused:\n%s", out)
	}
}

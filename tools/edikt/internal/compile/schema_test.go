package compile

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/parse"
)

// completeFields is the set a fully-conforming v0.5.0+ block emits.
var completeFields = map[string]bool{
	"paths":                 true,
	"scope":                 true,
	"directives":            true,
	"manual_directives":     true,
	"suppressed_directives": true,
	"source_hash":           true,
	"directives_hash":       true,
	"compiler_version":      true,
}

func docWith(path string, present map[string]bool, hasSentinel bool) *parse.Document {
	fields := make(map[string]bool, len(present))
	for k, v := range present {
		fields[k] = v
	}
	return &parse.Document{
		Path: path,
		Sentinel: parse.Sentinel{
			Present:       hasSentinel,
			PresentFields: fields,
		},
	}
}

func TestValidateSchema_AllComplete(t *testing.T) {
	docs := []*parse.Document{
		docWith("docs/architecture/decisions/ADR-001.md", completeFields, true),
		docWith("docs/architecture/invariants/INV-001.md", completeFields, true),
	}
	if err := ValidateSchema(docs); err != nil {
		t.Fatalf("expected nil error for complete blocks, got: %v", err)
	}
}

func TestValidateSchema_NoSentinelIsSkipped(t *testing.T) {
	// No-sentinel docs are handled separately by the caller — ValidateSchema
	// must NOT flag them as schema-incomplete.
	docs := []*parse.Document{
		docWith("a.md", nil, false),
	}
	if err := ValidateSchema(docs); err != nil {
		t.Fatalf("expected nil error when sentinel absent (separate failure mode), got: %v", err)
	}
}

func TestValidateSchema_MissingSourceHash(t *testing.T) {
	missing := map[string]bool{}
	for k, v := range completeFields {
		missing[k] = v
	}
	delete(missing, "source_hash")

	docs := []*parse.Document{
		docWith("docs/architecture/decisions/ADR-042.md", missing, true),
	}
	err := ValidateSchema(docs)
	if err == nil {
		t.Fatal("expected SchemaError, got nil")
	}
	se, ok := err.(*SchemaError)
	if !ok {
		t.Fatalf("expected *SchemaError, got %T", err)
	}
	if len(se.Blocks) != 1 {
		t.Fatalf("expected 1 incomplete block, got %d", len(se.Blocks))
	}
	if se.Blocks[0].Path != "docs/architecture/decisions/ADR-042.md" {
		t.Errorf("unexpected path: %s", se.Blocks[0].Path)
	}
	if len(se.Blocks[0].Missing) != 1 || se.Blocks[0].Missing[0] != "source_hash" {
		t.Errorf("expected missing [source_hash], got %v", se.Blocks[0].Missing)
	}
}

func TestValidateSchema_MultipleBlocksMultipleGaps(t *testing.T) {
	a := map[string]bool{"directives": true, "manual_directives": true, "suppressed_directives": true, "compiler_version": true}
	b := map[string]bool{"directives": true, "source_hash": true, "directives_hash": true, "manual_directives": true, "suppressed_directives": true}

	docs := []*parse.Document{
		docWith("a.md", a, true),
		docWith("b.md", b, true),
		docWith("c.md", completeFields, true),
	}
	err := ValidateSchema(docs)
	if err == nil {
		t.Fatal("expected SchemaError, got nil")
	}
	se := err.(*SchemaError)
	if len(se.Blocks) != 2 {
		t.Fatalf("expected 2 incomplete blocks, got %d", len(se.Blocks))
	}
	// Sorted by path: a.md before b.md
	if se.Blocks[0].Path != "a.md" || se.Blocks[1].Path != "b.md" {
		t.Errorf("expected sorted [a.md, b.md], got [%s, %s]", se.Blocks[0].Path, se.Blocks[1].Path)
	}
	// a.md missing source_hash + directives_hash
	gotA := strings.Join(se.Blocks[0].Missing, ",")
	if !strings.Contains(gotA, "source_hash") || !strings.Contains(gotA, "directives_hash") {
		t.Errorf("a.md should be missing source_hash and directives_hash, got: %v", se.Blocks[0].Missing)
	}
	// b.md missing compiler_version only
	if len(se.Blocks[1].Missing) != 1 || se.Blocks[1].Missing[0] != "compiler_version" {
		t.Errorf("b.md should be missing only compiler_version, got: %v", se.Blocks[1].Missing)
	}
}

func TestValidateSchema_LegacyV02xBlockFails(t *testing.T) {
	// A pre-ADR-008 block carrying only content_hash (deprecated) is missing
	// every required field.
	legacy := map[string]bool{
		"paths":        true,
		"scope":        true,
		"directives":   true,
		"content_hash": true,
	}
	docs := []*parse.Document{
		docWith("docs/architecture/decisions/ADR-001.md", legacy, true),
	}
	err := ValidateSchema(docs)
	if err == nil {
		t.Fatal("expected legacy block to be flagged, got nil")
	}
	se := err.(*SchemaError)
	for _, required := range []string{"source_hash", "directives_hash", "compiler_version", "manual_directives", "suppressed_directives"} {
		found := false
		for _, m := range se.Blocks[0].Missing {
			if m == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected legacy block to be missing %q, missing list: %v", required, se.Blocks[0].Missing)
		}
	}
}

func TestSchemaError_RenderSingularPlural(t *testing.T) {
	// Singular
	se := &SchemaError{Blocks: []IncompleteBlock{{Path: "a.md", Missing: []string{"source_hash"}}}}
	out := se.Error()
	if !strings.Contains(out, "1 sentinel block ") || strings.Contains(out, "1 sentinel blocks ") {
		t.Errorf("expected singular 'block' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "is incomplete") {
		t.Errorf("expected 'is incomplete' for singular, got:\n%s", out)
	}

	// Plural
	se = &SchemaError{Blocks: []IncompleteBlock{
		{Path: "a.md", Missing: []string{"source_hash"}},
		{Path: "b.md", Missing: []string{"compiler_version"}},
	}}
	out = se.Error()
	if !strings.Contains(out, "2 sentinel blocks") {
		t.Errorf("expected plural 'blocks' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "are incomplete") {
		t.Errorf("expected 'are incomplete' for plural, got:\n%s", out)
	}
}

func TestSchemaError_NamesPerArtifactCommands(t *testing.T) {
	se := &SchemaError{Blocks: []IncompleteBlock{{Path: "a.md", Missing: []string{"source_hash"}}}}
	out := se.Error()
	for _, cmd := range []string{
		"/edikt:adr:compile",
		"/edikt:invariant:compile",
		"/edikt:guideline:compile",
	} {
		if !strings.Contains(out, cmd) {
			t.Errorf("expected error output to redirect to %s, got:\n%s", cmd, out)
		}
	}
}

func TestSchemaError_EmptyBlocksReturnsPassMessage(t *testing.T) {
	// Defensive: a SchemaError with no blocks should not render the
	// failure template (this state shouldn't happen in practice — if all
	// blocks pass, ValidateSchema returns nil — but the rendering must
	// degrade gracefully).
	se := &SchemaError{}
	out := se.Error()
	if !strings.Contains(out, "passed") {
		t.Errorf("empty SchemaError should render a pass message, got:\n%s", out)
	}
}

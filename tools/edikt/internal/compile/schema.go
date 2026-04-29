package compile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/parse"
)

// requiredSentinelFields are the keys ADR-008 mandates on every sentinel
// block. Presence (not value-non-empty) is what's checked — `manual_directives:
// []` and an absent `manual_directives:` decode to the same nil slice but
// mean different things under the schema, so the parser tracks which keys
// it actually saw.
var requiredSentinelFields = []string{
	"source_hash",
	"directives_hash",
	"compiler_version",
	"manual_directives",
	"suppressed_directives",
}

// IncompleteBlock describes one source document whose parsed sentinel block
// is missing one or more ADR-008-required fields.
type IncompleteBlock struct {
	Path    string
	Missing []string
}

// SchemaError is the structured error returned by ValidateSchema when one or
// more sentinel blocks fail the completeness check. Callers can inspect
// Blocks for per-document detail or call Error() for a human-readable report
// matching the markdown command's output format.
type SchemaError struct {
	Blocks []IncompleteBlock
}

// Error formats the report exactly the way commands/gov/compile.md §12a
// renders it, so the Go binary path and the LLM markdown path emit
// equivalent error output.
func (e *SchemaError) Error() string {
	if len(e.Blocks) == 0 {
		return "schema validation passed (no incomplete blocks)"
	}
	var b strings.Builder
	n := len(e.Blocks)
	plural := "s"
	verb := "are"
	if n == 1 {
		plural = ""
		verb = "is"
	}
	fmt.Fprintf(&b, "✗ %d sentinel block%s are missing required fields.\n\n", n, plural)
	b.WriteString("  ADR-008 requires every directives block to carry source_hash,\n")
	b.WriteString("  directives_hash, compiler_version, manual_directives, and\n")
	fmt.Fprintf(&b, "  suppressed_directives. The block%s below %s incomplete:\n\n", plural, verb)
	for _, blk := range e.Blocks {
		fmt.Fprintf(&b, "    %s: missing [%s]\n", blk.Path, strings.Join(blk.Missing, ", "))
	}
	b.WriteString("\n")
	b.WriteString("  Re-run the matching <artifact>:compile to regenerate under the\n")
	b.WriteString("  current schema:\n\n")
	b.WriteString("    /edikt:adr:compile {ADR-NNN}\n")
	b.WriteString("    /edikt:invariant:compile {INV-NNN}\n")
	b.WriteString("    /edikt:guideline:compile {slug}\n\n")
	b.WriteString("  Then re-run /edikt:gov:compile.")
	return b.String()
}

// ValidateSchema checks every sentinel block in `docs` for the five required
// ADR-008 fields. Documents without a sentinel block are skipped — the
// no-sentinel case is handled separately by the caller (it's a different
// failure mode with its own remediation).
//
// Returns nil when every block is complete. Returns a *SchemaError listing
// every incomplete block (sorted by path) when one or more blocks are
// missing fields. Callers MUST NOT proceed to Group()/render when a
// SchemaError is returned — partial-writing governance.md from incomplete
// inputs is exactly the failure mode this gate exists to prevent.
func ValidateSchema(docs []*parse.Document) error {
	var bad []IncompleteBlock
	for _, doc := range docs {
		if !doc.Sentinel.Present {
			// No-sentinel case — caller handles separately.
			continue
		}
		var missing []string
		for _, f := range requiredSentinelFields {
			if !doc.Sentinel.PresentFields[f] {
				missing = append(missing, f)
			}
		}
		if len(missing) > 0 {
			bad = append(bad, IncompleteBlock{Path: doc.Path, Missing: missing})
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Slice(bad, func(i, j int) bool { return bad[i].Path < bad[j].Path })
	return &SchemaError{Blocks: bad}
}

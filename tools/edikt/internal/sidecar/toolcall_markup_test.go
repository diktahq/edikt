package sidecar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validSidecar = `schema_version: 1
topic: "testing"
path: "docs/architecture/decisions/ADR-001-test.md"
signals:
  - "test signal"
directives: []
verification: []
`

func writeSidecar(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ADR-001-test.edikt.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

// TestLoad_ToolCallMarkupGetsNamedError covers the observed failure where a
// resumed extractor subagent flushed its own tool-call framing into the
// file it was writing. yaml.v3 reports that as an opaque scan error, which
// sends the user hunting through their prose for a YAML bug that isn't
// there. The loader must name the actual cause.
func TestLoad_ToolCallMarkupGetsNamedError(t *testing.T) {
	// The exact shape reported from the field: framing appended after the
	// last key.
	p := writeSidecar(t, validSidecar+"</content>\n</invoke>\n")

	_, err := Load(p)
	if err == nil {
		t.Fatal("expected a load error for a sidecar containing tool-call markup")
	}
	msg := err.Error()
	if !strings.Contains(msg, "tool-call markup") {
		t.Fatalf("error must name tool-call markup as the cause; got: %v", err)
	}
	if !strings.Contains(msg, "</content>") {
		t.Fatalf("error must quote the offending marker; got: %v", err)
	}
	if !strings.Contains(msg, "line 8") {
		t.Fatalf("error must give the offending line number; got: %v", err)
	}
	if !strings.Contains(msg, "compile") {
		t.Fatalf("error must name the remedy; got: %v", err)
	}
}

// A directive that legitimately quotes framing inline must still load —
// the check matches whole lines only, so writing a rule *about* the
// framing is not treated as leaked framing.
func TestLoad_InlineMarkupInDirectiveTextIsFine(t *testing.T) {
	body := `schema_version: 1
topic: "testing"
path: "docs/architecture/decisions/ADR-001-test.md"
signals:
  - "test signal"
directives:
  - text: "Sidecars MUST NOT contain a trailing </content> tag. (ref: ADR-001)"
    source_excerpt:
      line_start: 1
      line_end: 1
      quote: "Sidecars must not contain framing."
`
	if _, err := Load(writeSidecar(t, body)); err != nil {
		t.Fatalf("inline markup inside a quoted directive is valid YAML: %v", err)
	}
}

// A plain YAML syntax error must keep its original parser message rather
// than being misattributed to markup.
func TestLoad_OrdinaryYAMLErrorKeepsParserMessage(t *testing.T) {
	_, err := Load(writeSidecar(t, "schema_version: 1\ntopic: \"testing\"\n  bad_indent: x\n"))
	if err == nil {
		t.Fatal("expected a parse error")
	}
	if strings.Contains(err.Error(), "tool-call markup") {
		t.Fatalf("ordinary YAML errors must not be attributed to markup; got: %v", err)
	}
}

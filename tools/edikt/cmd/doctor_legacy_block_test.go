package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyBlockCheck_RedForIncludedWarnForRetired(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	block := "[edikt:directives:start]: #\nsource_hash: pending\n[edikt:directives:end]: #\n"
	files := map[string]string{
		"ADR-001-live.md":    "---\ntype: adr\nid: ADR-001\nstatus: accepted\n---\n# ADR-001\n\n**Status:** Accepted\n\n" + block,
		"ADR-002-retired.md": "---\ntype: adr\nid: ADR-002\nstatus: superseded-by-ADR-001\n---\n# ADR-002\n\n**Status:** Superseded by ADR-001\n\n" + block,
		"ADR-003-fenced.md":  "---\ntype: adr\nid: ADR-003\nstatus: accepted\n---\n# ADR-003\n\n**Status:** Accepted\n\n```\n" + block + "```\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	errs, warns := runLegacyBlockCheck(root, []string{"docs/architecture/decisions"}, &out)
	s := out.String()
	if errs != 1 || warns != 1 {
		t.Fatalf("want 1 error (live) and 1 warning (retired), got errs=%d warns=%d:\n%s", errs, warns, s)
	}
	if !strings.Contains(s, "🔴 ERROR: unstripped legacy block present — docs/architecture/decisions/ADR-001-live.md:10") {
		t.Errorf("live artifact must be a 🔴 with file:line:\n%s", s)
	}
	if !strings.Contains(s, "WARN: unstripped legacy block present on a skip-listed artifact — docs/architecture/decisions/ADR-002-retired.md") {
		t.Errorf("retired artifact must be a WARN:\n%s", s)
	}
	if strings.Contains(s, "ADR-003") {
		t.Errorf("a fenced mention must not be reported:\n%s", s)
	}
	// Sensitivity control: nothing to report prints nothing.
	var quiet bytes.Buffer
	if e, w := runLegacyBlockCheck(t.TempDir(), []string{"docs/architecture/decisions"}, &quiet); e != 0 || w != 0 || quiet.Len() != 0 {
		t.Fatalf("empty project must be silent: %d %d %q", e, w, quiet.String())
	}
}

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoutedSources_AllResolve(t *testing.T) {
	root := t.TempDir()
	mustMkAll(t, filepath.Join(root, "docs", "architecture", "decisions"))
	mustMkAll(t, filepath.Join(root, "docs", "architecture", "invariants"))
	mustMkAll(t, filepath.Join(root, ".claude", "rules", "governance"))

	mustWrite(t, filepath.Join(root, "docs", "architecture", "decisions", "ADR-001-test.md"), "# ADR-001\n")
	mustWrite(t, filepath.Join(root, "docs", "architecture", "invariants", "INV-001-test.md"), "# INV-001\n")
	mustWrite(t, filepath.Join(root, ".claude", "rules", "governance.md"),
		"- Rule one. (ref: ADR-001)\n- Rule two. (ref: INV-001)\n")

	var buf bytes.Buffer
	errs, warns, ran := runRoutedSourcesCheck(root, &buf)
	if !ran {
		t.Fatal("expected ran=true with .claude/rules present")
	}
	if errs != 0 || warns != 0 {
		t.Fatalf("clean fixture should not error/warn: %d/%d, output:\n%s", errs, warns, buf.String())
	}
	if !strings.Contains(buf.String(), "Routed sources — 2 of 2 resolve") {
		t.Errorf("missing OK summary: %s", buf.String())
	}
}

func TestRoutedSources_MissingSource(t *testing.T) {
	root := t.TempDir()
	mustMkAll(t, filepath.Join(root, "docs", "architecture", "decisions"))
	mustMkAll(t, filepath.Join(root, ".claude", "rules"))
	mustWrite(t, filepath.Join(root, ".claude", "rules", "governance.md"),
		"- Rule one. (ref: ADR-999)\n")

	var buf bytes.Buffer
	errs, _, ran := runRoutedSourcesCheck(root, &buf)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if errs != 1 {
		t.Fatalf("want 1 error for missing ADR-999, got %d. output:\n%s", errs, buf.String())
	}
	if !strings.Contains(buf.String(), "Missing source for routed directive: ADR-999") {
		t.Errorf("missing FAIL line: %s", buf.String())
	}
}

func TestRoutedSources_NoRulesDir(t *testing.T) {
	root := t.TempDir()
	_, _, ran := runRoutedSourcesCheck(root, &bytes.Buffer{})
	if ran {
		t.Errorf("ran=true on bare project without .claude/rules")
	}
}

func TestRoutedSources_CitationFromTopicFile(t *testing.T) {
	root := t.TempDir()
	mustMkAll(t, filepath.Join(root, "docs", "architecture", "decisions"))
	mustMkAll(t, filepath.Join(root, ".claude", "rules", "governance"))
	mustWrite(t, filepath.Join(root, "docs", "architecture", "decisions", "ADR-005-test.md"), "# ADR-005\n")
	// No top-level governance.md — only a topic file.
	mustWrite(t, filepath.Join(root, ".claude", "rules", "governance", "architecture.md"),
		"- Some rule (ref: ADR-005)\n")

	var buf bytes.Buffer
	errs, _, ran := runRoutedSourcesCheck(root, &buf)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if errs != 0 {
		t.Errorf("want clean, got %d errors:\n%s", errs, buf.String())
	}
}

// TestRoutedSources_MixedState — commands/doctor.md:129 promises "one line
// per missing source". A routing surface citing several missing IDs
// alongside a resolvable one must FAIL once per missing ID (not just once
// overall) and must not list the resolvable ID as missing. Ported from the
// deleted test_doctor_source_check.py::test_mixed_state_lists_each_missing
// when that pytest file's target (a Python heredoc in commands/doctor.md)
// was fully migrated to this Go implementation.
func TestRoutedSources_MixedState(t *testing.T) {
	root := t.TempDir()
	mustMkAll(t, filepath.Join(root, "docs", "architecture", "decisions"))
	mustMkAll(t, filepath.Join(root, "docs", "architecture", "invariants"))
	mustMkAll(t, filepath.Join(root, ".claude", "rules", "governance"))

	mustWrite(t, filepath.Join(root, "docs", "architecture", "decisions", "ADR-001-test.md"), "# ADR-001\n")
	mustWrite(t, filepath.Join(root, ".claude", "rules", "governance", "architecture.md"), strings.Join([]string{
		"- Rule citing ADR-001. (ref: ADR-001)",
		"- Rule citing ADR-777. (ref: ADR-777)",
		"- Rule citing INV-777. (ref: INV-777)",
		"",
	}, "\n"))

	var buf bytes.Buffer
	errs, _, ran := runRoutedSourcesCheck(root, &buf)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if errs != 2 {
		t.Fatalf("want 2 errors for ADR-777 and INV-777, got %d. output:\n%s", errs, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "Missing source for routed directive: ADR-777") {
		t.Errorf("missing FAIL line for ADR-777: %s", out)
	}
	if !strings.Contains(out, "Missing source for routed directive: INV-777") {
		t.Errorf("missing FAIL line for INV-777: %s", out)
	}
	if strings.Contains(out, "Missing source for routed directive: ADR-001") {
		t.Errorf("ADR-001 has a source file and must not be reported missing: %s", out)
	}
}

func mustMkAll(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
}
func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stageArtifacts(t *testing.T, files map[string][]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "edikt_version: \"0.8.0\"\npaths:\n" +
		"  decisions: docs/architecture/decisions\n" +
		"  invariants: docs/architecture/invariants\n"
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, names := range files {
		dir := filepath.Join(root, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, n := range names {
			if err := os.WriteFile(filepath.Join(dir, n), []byte("# "+n+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

// BOTH paths must be named. Which of two artifacts is the intruder is not
// something the tool can know — the newer file may be the correct one. Naming
// one would be a guess presented as a finding.
func TestDuplicateIDCheck_NamesBothHolders(t *testing.T) {
	root := stageArtifacts(t, map[string][]string{
		"docs/architecture/invariants": {"INV-017-attestation.md", "INV-017-scope-split.md", "INV-018-other.md"},
	})
	var buf bytes.Buffer
	warns, ran := runDuplicateIDCheck(root, &buf)
	out := buf.String()

	if !ran {
		t.Fatalf("check did not run against a populated corpus:\n%s", out)
	}
	if warns != 1 {
		t.Errorf("want 1 warning for 1 duplicated id, got %d\n%s", warns, out)
	}
	for _, want := range []string{"INV-017", "INV-017-attestation.md", "INV-017-scope-split.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not name %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "INV-018") {
		t.Errorf("a non-duplicated id was reported:\n%s", out)
	}
}

// The same id in two different KINDS is still a collision — governance refers
// to ids, and a (ref: X) tail cannot disambiguate two artifacts.
func TestDuplicateIDCheck_CrossDirectoryCollision(t *testing.T) {
	root := stageArtifacts(t, map[string][]string{
		"docs/architecture/decisions":  {"ADR-005-a.md"},
		"docs/architecture/invariants": {"ADR-005-stray-copy.md"},
	})
	var buf bytes.Buffer
	warns, _ := runDuplicateIDCheck(root, &buf)
	if warns == 0 {
		t.Errorf("a cross-directory id collision was not reported:\n%s", buf.String())
	}
}

// A clean corpus must report a MEASURED ok with its denominator, not silence.
func TestDuplicateIDCheck_CleanCorpusReportsCount(t *testing.T) {
	root := stageArtifacts(t, map[string][]string{
		"docs/architecture/invariants": {"INV-001-a.md", "INV-002-b.md"},
	})
	var buf bytes.Buffer
	warns, ran := runDuplicateIDCheck(root, &buf)
	if !ran {
		t.Fatal("check did not run")
	}
	if warns != 0 {
		t.Errorf("clean corpus warned: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "2 artifact id(s) inspected") {
		t.Errorf("clean result carries no denominator:\n%s", buf.String())
	}
}

// A project with no artifacts must report ran=false rather than a clean pass —
// an unexamined corpus is not a clean one.
func TestDuplicateIDCheck_EmptyCorpusDoesNotReportClean(t *testing.T) {
	root := stageArtifacts(t, map[string][]string{})
	var buf bytes.Buffer
	_, ran := runDuplicateIDCheck(root, &buf)
	if ran {
		t.Errorf("an empty corpus was reported as inspected:\n%s", buf.String())
	}
}

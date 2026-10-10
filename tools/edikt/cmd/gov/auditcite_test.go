package gov

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// auditCiteFixture builds a minimal repo: a decisions dir with ADR-001 and
// ADR-002, and a git repo with one commit, so audit-cite has real subjects
// to resolve against — a corpus of one is still a corpus.
func auditCiteFixture(t *testing.T) (root string, headCommit string) {
	t.Helper()
	root = t.TempDir()
	decisions := filepath.Join(root, "docs/architecture/decisions")
	if err := os.MkdirAll(decisions, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(decisions, "ADR-001-first.md"), "# ADR-001: First\n\nA decision.\n")

	run(t, root, "git", "init", "-q")
	run(t, root, "git", "-c", "user.email=t@example.com", "-c", "user.name=t", "add", ".")
	run(t, root, "git", "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "-m", "seed")
	out := strings.TrimSpace(run(t, root, "git", "rev-parse", "HEAD"))
	return root, out
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}

// Every citation resolves: an artifact id present in the corpus, a real
// commit, and a path that exists.
func TestAuditCite_AllResolve(t *testing.T) {
	root, head := auditCiteFixture(t)
	decisions := filepath.Join(root, "docs/architecture/decisions")
	mustWrite(t, filepath.Join(decisions, "ADR-002-second.md"), "# ADR-002: Second\n\n"+
		"Amends ADR-001. See commit "+head+". Also see `docs/architecture/decisions/ADR-001-first.md`.\n")

	bin := buildBinary(t)
	cmd := exec.Command(bin, "gov", "audit-cite", "ADR-002")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected exit 0, got err=%v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "every citation resolved") {
		t.Errorf("expected a clean resolution report, got: %s", out)
	}
}

// An unresolved citation of each kind — a nonexistent artifact id, a hash
// that isn't a real object, and a path that doesn't exist — is exit 1, and
// each finding must say what was looked for.
func TestAuditCite_UnresolvedCitationsListedWithWhatWasLookedFor(t *testing.T) {
	root, _ := auditCiteFixture(t)
	decisions := filepath.Join(root, "docs/architecture/decisions")
	mustWrite(t, filepath.Join(decisions, "ADR-002-second.md"), "# ADR-002: Second\n\n"+
		"Amends ADR-900. See commit deadbeef1234. Also see `docs/architecture/decisions/ADR-999-missing.md`.\n")

	bin := buildBinary(t)
	cmd := exec.Command(bin, "gov", "audit-cite", "ADR-002")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if !isExitCode(err, 1) {
		t.Fatalf("expected exit 1, got err=%v\noutput: %s", err, out)
	}
	s := string(out)
	for _, want := range []string{
		"UNRESOLVED", "ADR-900", "deadbeef1234", "ADR-999-missing.md",
		"looked for:",
		// D118 (finding 9): every unresolved-citation finding names the
		// subtractive remedy (remove the assertion) beside the additive one
		// (fix the citation) — bok read the additive framing as the only
		// option and drafted an ADR to supply the missing authority.
		"remedy:", "remove the assertion",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("expected output to contain %q, got: %s", want, s)
		}
	}
}

// No citations at all is a measurement, not a pass — the auditor must say
// nothing was resolved rather than reporting a clean bill silently.
func TestAuditCite_NoCitationsIsAMeasurementNotAPass(t *testing.T) {
	root, _ := auditCiteFixture(t)
	decisions := filepath.Join(root, "docs/architecture/decisions")
	mustWrite(t, filepath.Join(decisions, "ADR-002-second.md"), "# ADR-002: Second\n\nStands alone.\n")

	bin := buildBinary(t)
	cmd := exec.Command(bin, "gov", "audit-cite", "ADR-002")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected exit 0, got err=%v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "nothing was resolved (this is a measurement, not a pass)") {
		t.Errorf("expected the no-subject statement, got: %s", out)
	}
}

// Usage errors — an artifact that doesn't exist at all — are exit 3, distinct
// from an exit-1 unresolved-citation finding.
func TestAuditCite_UnknownArtifactIsUsageError(t *testing.T) {
	root, _ := auditCiteFixture(t)
	bin := buildBinary(t)
	cmd := exec.Command(bin, "gov", "audit-cite", "ADR-999")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if !isExitCode(err, 3) {
		t.Fatalf("expected exit 3, got err=%v\noutput: %s", err, out)
	}
}

// A path citation that tries to leave the repository is refused, never
// resolved — `../../etc/passwd` is a refusal.
func TestAuditCite_PathEscapeIsRefusedNotResolved(t *testing.T) {
	root, _ := auditCiteFixture(t)
	decisions := filepath.Join(root, "docs/architecture/decisions")
	mustWrite(t, filepath.Join(decisions, "ADR-002-second.md"), "# ADR-002: Second\n\n"+
		"See `../../../../../../etc/passwd.conf`.\n")

	bin := buildBinary(t)
	cmd := exec.Command(bin, "gov", "audit-cite", "ADR-002")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if !isExitCode(err, 1) {
		t.Fatalf("expected exit 1 (refused, reported as unresolved), got err=%v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "UNRESOLVED") {
		t.Errorf("expected an UNRESOLVED finding for the escaping path, got: %s", out)
	}
}

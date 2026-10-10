package cmd

import (
	"bytes"
	"crypto/md5" //nolint:gosec // provenance identity, not a security boundary
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// D83(e): doctor compared RAW template bytes against an installed file that
// edikt itself transformed at install time — substitutions applied, stack
// blocks filtered, provenance frontmatter stamped — so twelve agents that
// were correctly installed reported as drifted. The agents pass already knows
// the answer: `edikt_template_hash` records the md5 of the raw template the
// install came from, so equality there means "installed from this template",
// whatever the bytes look like afterwards.

func plantAgentPair(t *testing.T, root, slug, tmplBody, installedBody string) (string, string) {
	t.Helper()
	tmplDir := filepath.Join(root, "templates", "agents")
	instDir := filepath.Join(root, "project", ".claude", "agents")
	for _, d := range []string{tmplDir, instDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tp := filepath.Join(tmplDir, slug+".md")
	ip := filepath.Join(instDir, slug+".md")
	if err := os.WriteFile(tp, []byte(tmplBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ip, []byte(installedBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return tp, ip
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec // provenance identity
	return hex.EncodeToString(sum[:])
}

func TestAgentDrift_ProvenanceHashMatchIsNotDrift(t *testing.T) {
	root := t.TempDir()
	tmpl := "---\nname: docs\n---\n# docs\n{{stack}} body\n"
	// What edikt actually installs: substitutions applied, the provenance
	// stamp added. Byte-different from the template on purpose.
	installed := "---\nname: docs\nedikt_template_hash: " + md5hex(tmpl) + "\nedikt_template_version: 0.9.0\n---\n# docs\ngo body\n"
	plantAgentPair(t, root, "docs", tmpl, installed)

	warns := checkAgentDrift(root, filepath.Join(root, "claude"), filepath.Join(root, "project"))
	if len(warns) != 0 {
		t.Fatalf("an agent whose provenance hash matches the template is NOT drifted: %+v", warns)
	}
}

func TestAgentDrift_ProvenanceHashMismatchIsDrift(t *testing.T) {
	root := t.TempDir()
	tmpl := "---\nname: docs\n---\n# docs\nnew body\n"
	installed := "---\nname: docs\nedikt_template_hash: " + md5hex("an older template") + "\n---\n# docs\nold body\n"
	plantAgentPair(t, root, "docs", tmpl, installed)

	warns := checkAgentDrift(root, filepath.Join(root, "claude"), filepath.Join(root, "project"))
	if len(warns) != 1 {
		t.Fatalf("a provenance hash from a different template IS drift, got %d", len(warns))
	}
}

// A pre-provenance install has no hash to compare. Byte comparison is all
// that is left — and the report must SAY that is what it did, because a
// byte difference there may be edikt's own tailoring rather than staleness.
func TestAgentDrift_LegacyInstallSaysHowItCompared(t *testing.T) {
	root := t.TempDir()
	tmpl := "---\nname: docs\n---\n# docs\nbody\n"
	installed := "---\nname: docs\n---\n# docs\ndifferent\n"
	plantAgentPair(t, root, "docs", tmpl, installed)

	var out bytes.Buffer
	n := runAgentDriftCheck(root, filepath.Join(root, "claude"), filepath.Join(root, "project"), &out)
	if n != 1 {
		t.Fatalf("a legacy install that differs is still reported, got %d", n)
	}
	if !strings.Contains(out.String(), "no provenance") {
		t.Errorf("the report must say the comparison was byte-wise for lack of provenance:\n%s", out.String())
	}
}

// The custom marker still wins over everything.
func TestAgentDrift_CustomMarkerIsNeverDrift(t *testing.T) {
	root := t.TempDir()
	tmpl := "---\nname: docs\n---\n# docs\nbody\n"
	installed := "---\nname: docs\n---\n<!-- edikt:custom -->\n# mine\n"
	plantAgentPair(t, root, "docs", tmpl, installed)
	if warns := checkAgentDrift(root, filepath.Join(root, "claude"), filepath.Join(root, "project")); len(warns) != 0 {
		t.Fatalf("a custom agent is the user's: %+v", warns)
	}
}

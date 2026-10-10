package cmd

import (
	"bytes"
	"crypto/md5" //nolint:gosec // provenance identity, not a security boundary — same rule as md5Hex
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// AC-7.7 / D57(3) — agent currency is decided by the template's raw-byte
// digest (edikt_template_hash), never by prompt_version. A body edit that
// leaves prompt_version unchanged must still be reported as drifted:
// otherwise a same-version prompt patch would silently never reach an
// installed copy, which is exactly the kind of stale-definition field
// failure doctor's agent-drift check exists to catch.
func TestUpgradeAgents_CurrentByDigest(t *testing.T) {
	ediktRoot, claudeRoot, projectRoot := stageAgentDrift(t)

	// The CURRENT shipped template: prompt_version 4, a patched body.
	currentTemplate := "---\nname: sidecar-extractor\nmaxTurns: 8\nprompt_version: 4\n---\npatched prompt body\n"
	tmplPath := filepath.Join(ediktRoot, "templates", "agents", "sidecar-extractor.md")
	if err := os.WriteFile(tmplPath, []byte(currentTemplate), 0o644); err != nil {
		t.Fatal(err)
	}

	// The installed copy was installed from an EARLIER template revision
	// that happened to declare the SAME prompt_version (4) but a different
	// body. Its provenance stamp names that earlier revision's own digest,
	// not the current template's.
	oldTemplate := "---\nname: sidecar-extractor\nmaxTurns: 8\nprompt_version: 4\n---\noriginal prompt body\n"
	oldDigest := md5.Sum([]byte(oldTemplate)) //nolint:gosec
	oldHash := hex.EncodeToString(oldDigest[:])

	installed := "---\nname: sidecar-extractor\nmaxTurns: 8\nprompt_version: 4\nedikt_template_hash: " + oldHash + "\n---\noriginal prompt body\n"
	projAgents := filepath.Join(projectRoot, ".claude", "agents")
	if err := os.MkdirAll(projAgents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projAgents, "sidecar-extractor.md"), []byte(installed), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	n := runAgentDriftCheck(ediktRoot, claudeRoot, projectRoot, &out)
	if n != 1 {
		t.Fatalf("same prompt_version, different body must still report drift: got %d warning(s):\n%s", n, out.String())
	}
}

// Control: the same scenario but with the installed copy's provenance hash
// matching the CURRENT template exactly — no drift, whatever prompt_version
// says.
func TestUpgradeAgents_CurrentByDigest_NoDriftWhenHashMatches(t *testing.T) {
	ediktRoot, claudeRoot, projectRoot := stageAgentDrift(t)

	currentTemplate := "---\nname: sidecar-extractor\nmaxTurns: 8\nprompt_version: 4\n---\npatched prompt body\n"
	tmplPath := filepath.Join(ediktRoot, "templates", "agents", "sidecar-extractor.md")
	if err := os.WriteFile(tmplPath, []byte(currentTemplate), 0o644); err != nil {
		t.Fatal(err)
	}
	currentDigest := md5.Sum([]byte(currentTemplate)) //nolint:gosec
	currentHash := hex.EncodeToString(currentDigest[:])

	installed := "---\nname: sidecar-extractor\nmaxTurns: 8\nprompt_version: 4\nedikt_template_hash: " + currentHash + "\n---\npatched prompt body\n"
	projAgents := filepath.Join(projectRoot, ".claude", "agents")
	if err := os.MkdirAll(projAgents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projAgents, "sidecar-extractor.md"), []byte(installed), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if n := runAgentDriftCheck(ediktRoot, claudeRoot, projectRoot, &out); n != 0 {
		t.Fatalf("matching provenance hash must not report drift: got %d warning(s):\n%s", n, out.String())
	}
}

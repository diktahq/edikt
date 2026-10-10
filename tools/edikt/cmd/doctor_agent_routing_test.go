package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A dispatch names a subagent_type; if no agent by that name is installed,
// the host reports "Agent type not found" and the caller's natural recovery
// is general-purpose adopting the template's text — a specialist review that
// never had a specialist. This repo shipped 30 agent templates and installed
// 7 while commands/_shared-agent-routing.md routed to five that were never
// installed, so every routed review was nominal.

func plantAgents(t *testing.T, root string, installed []string, registryAlways []string) {
	t.Helper()
	// The installed payload sits ABOVE the repo templates/ rung in the ladder
	// (ADR-005 §1), so without this the developer's real ~/.edikt would answer
	// instead of the registry this helper plants (INV-007).
	t.Setenv("EDIKT_ROOT", filepath.Join(root, "no-such-root"))
	dir := filepath.Join(root, ".claude", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, a := range installed {
		if err := os.WriteFile(filepath.Join(dir, a+".md"), []byte("---\nname: "+a+"\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tpl := filepath.Join(root, "templates", "agents")
	if err := os.MkdirAll(tpl, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "always:\n"
	for _, a := range registryAlways {
		body += "  - " + a + "\n"
	}
	body += "optional:\n  - performance\n"
	if err := os.WriteFile(filepath.Join(tpl, "_registry.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorAgentRouting_AlwaysAgentNotInstalledIsRed(t *testing.T) {
	root := t.TempDir()
	plantAgents(t, root, []string{"qa", "docs"}, []string{"qa", "docs", "architect", "evaluator"})
	var out bytes.Buffer
	errN, _ := runAgentRoutingCheck(root, &out)
	if errN == 0 {
		t.Fatalf("an always-installed agent that is absent must be RED:\n%s", out.String())
	}
	s := out.String()
	for _, want := range []string{"architect", "evaluator", "🔴"} {
		if !strings.Contains(s, want) {
			t.Errorf("the finding must name %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "performance") {
		t.Errorf("an OPTIONAL agent is not a finding — it was never promised:\n%s", s)
	}
}

// The count is the point: "some agents are missing" reads the same for one
// and for twenty-three.
func TestDoctorAgentRouting_ReportsTheDenominator(t *testing.T) {
	root := t.TempDir()
	plantAgents(t, root, []string{"qa"}, []string{"qa", "architect", "evaluator", "docs"})
	var out bytes.Buffer
	if _, _ = runAgentRoutingCheck(root, &out); !strings.Contains(out.String(), "1 of 4") || !strings.Contains(out.String(), "3 missing") {
		t.Errorf("the finding must carry installed-vs-declared, got:\n%s", out.String())
	}
}

func TestDoctorAgentRouting_EverythingInstalledIsSilent(t *testing.T) {
	root := t.TempDir()
	plantAgents(t, root, []string{"qa", "docs", "architect"}, []string{"qa", "docs", "architect"})
	var out bytes.Buffer
	errN, warnN := runAgentRoutingCheck(root, &out)
	if errN != 0 || warnN != 0 || out.Len() != 0 {
		t.Fatalf("a complete install must be silent, got %d/%d:\n%s", errN, warnN, out.String())
	}
}

// No registry at all: nothing was declared, so nothing can be missing — but
// that is UNMEASURED, not a pass, and it must say so rather than stay quiet.
func TestDoctorAgentRouting_NoRegistryIsUnmeasured(t *testing.T) {
	root := t.TempDir()
	// No payload to fall back to either: "no registry anywhere" is the case
	// under test, and the machine's own install must not stand in for it.
	t.Setenv("EDIKT_ROOT", filepath.Join(root, "no-such-root"))
	if err := os.MkdirAll(filepath.Join(root, ".claude", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, warnN := runAgentRoutingCheck(root, &out)
	if warnN == 0 || !strings.Contains(out.String(), "UNMEASURED") {
		t.Fatalf("a missing registry must read as UNMEASURED:\n%s", out.String())
	}
}

// The project override is the FIRST rung (ADR-005 §1). The registry lookup
// used to try the repo's own templates/ first and the override second, so a
// project that customised its declared agent set had the customisation
// silently ignored whenever a templates/ tree sat beside it.
func TestResolveAgentRegistry_ProjectOverrideBeatsRepoTemplates(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", filepath.Join(root, "no-such-root"))
	write := func(path, always string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("always:\n  - "+always+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	override := filepath.Join(root, ".edikt", "templates", "agents", "_registry.yaml")
	write(filepath.Join(root, "templates", "agents", "_registry.yaml"), "repo-agent")
	write(override, "override-agent")

	reg, source, ok := resolveAgentRegistry(root)
	if !ok {
		t.Fatal("no registry resolved")
	}
	if source != override {
		t.Fatalf("registry resolved from %q, want the project override %q", source, override)
	}
	if len(reg.Always) != 1 || reg.Always[0] != "override-agent" {
		t.Fatalf("declared set %v, want the override's [override-agent]", reg.Always)
	}
}

// The payload rung sits between the override and the repo checkout.
func TestResolveAgentRegistry_PayloadBeatsRepoTemplates(t *testing.T) {
	root := t.TempDir()
	ediktRoot := filepath.Join(root, "edikt-root")
	t.Setenv("EDIKT_ROOT", ediktRoot)
	write := func(path, always string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("always:\n  - "+always+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	payload := filepath.Join(ediktRoot, "current", "templates", "agents", "_registry.yaml")
	write(filepath.Join(root, "templates", "agents", "_registry.yaml"), "repo-agent")
	write(payload, "payload-agent")

	_, source, ok := resolveAgentRegistry(root)
	if !ok || source != payload {
		t.Fatalf("registry resolved from %q (ok=%v), want the payload %q", source, ok, payload)
	}
}

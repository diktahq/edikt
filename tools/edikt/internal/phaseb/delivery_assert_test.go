package phaseb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// writeSurface drops one rendered surface into a scratch project.
// The two YAML-surface cases that lived here (escaped YAML scalars count as
// delivered; unparseable YAML falls back to raw text) were retired with the
// directive index (ADR-083): no rendered YAML surface carries directive text
// any more, so there is no escaped text to normalise. yamlScalars still runs
// over manifest.yaml and delivery-map.yaml, which carry ids and paths only.

func writeSurface(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Sensitivity control for the case above: a rule on NO surface is still
// reported, so parsing YAML did not turn the gate into a pass-everything.
func TestAssertDelivery_AbsentRuleIsStillReported(t *testing.T) {
	root := t.TempDir()
	writeSurface(t, root, ".claude/rules/governance/architecture.md",
		"schema_version: 1\nglobs:\n  \"a/**/*\":\n    directives:\n    - id: \"INV-906:d01\"\n      text: \"Something else entirely that is long enough to exceed the probe length by itself.\"\n")
	writeSurface(t, root, ".claude/skills/edikt-compile/SKILL.md", "# compile\n\nnothing relevant here\n")

	pairs := []sidecar.Pair{{
		ArtifactID: "INV-910",
		Sidecar: &sidecar.Sidecar{Topic: "compile", Directives: []sidecar.Directive{
			{Text: "The governance-compilation pipeline MUST treat artifact prose as read-only input, no exceptions. (ref: INV-910)", ActorScope: true},
		}},
	}}
	missing, err := AssertDelivery(root, pairs)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0].Artifact != "INV-910" {
		t.Fatalf("want exactly INV-910 reported missing, got:\n%s", FormatUndelivered(missing))
	}
}

// An empty corpus is an error, never a pass (INV-013).
func TestAssertDelivery_EmptyCorpusIsNotAPass(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{{ArtifactID: "INV-901", Sidecar: &sidecar.Sidecar{Directives: []sidecar.Directive{{Text: "anything"}}}}}
	if _, err := AssertDelivery(root, pairs); err == nil {
		t.Fatal("no rendered surfaces must be an error, not zero undelivered")
	}
}

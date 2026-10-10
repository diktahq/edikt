package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phaseb"
)

// TestFallbackMarkerStillMatchesWhatPhasebEmits is the DRIFT GUARD.
//
// This file matches a substring rather than copying phaseb's format string, so
// the two cannot diverge silently — but only if something checks. Asserts the
// resolved outcome (the rendered surface contains the marker) rather than
// restating the format, per INV-014.
func TestFallbackMarkerStillMatchesWhatPhasebEmits(t *testing.T) {
	rendered := phaseb.RenderSkill(phaseb.SkillView{Topic: "tooling"}) // no Description
	if !strings.Contains(rendered, fallbackDescriptionMarker) {
		t.Fatalf("phaseb no longer emits %q for an unapproved description; the "+
			"compiled-surface check silently stopped detecting it:\n%s",
			fallbackDescriptionMarker, rendered)
	}
	// NEGATIVE CONTROL: an approved description must NOT trip the marker, or
	// every skill would report degraded.
	ok := phaseb.RenderSkill(phaseb.SkillView{Topic: "tooling", Description: "Designing edikt's binaries."})
	if strings.Contains(ok, fallbackDescriptionMarker) {
		t.Error("a skill WITH an approved description matched the fallback marker")
	}
}

func writeSkill(t *testing.T, root, topic, desc string) {
	t.Helper()
	d := filepath.Join(root, ".claude", "skills", "edikt-"+topic)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: edikt-" + topic + "\ndescription: \"" + desc + "\"\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestFifthCheckFiresOnAPlaceholderSkill — POSITIVE CONTROL.
func TestFifthCheckFiresOnAPlaceholderSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "tooling", "edikt governance for the tooling topic ("+
		fallbackDescriptionMarker+" — run `bin/edikt sidecar approve`)")
	writeSkill(t, root, "release", "Planning or performing a release.")

	gaps := skillsShippingFallbackDescription(filepath.Join(root, ".claude", "skills"))
	if len(gaps) != 1 {
		t.Fatalf("gaps: got %d, want 1", len(gaps))
	}
	g := gaps[0]
	if !strings.Contains(g.Title, "1 of 2") {
		t.Errorf("title must count degraded against total; got %q", g.Title)
	}
	if !strings.Contains(strings.Join(g.Detail, " "), "tooling") {
		t.Errorf("the degraded topic must be named; got %v", g.Detail)
	}
	// The corrected consequence: NOT "renders with no description".
	if strings.Contains(g.Consequence, "no description") {
		t.Error("consequence still says the topic renders with no description; " +
			"it renders WITH an approval instruction in the routing field")
	}
	if !strings.Contains(g.Consequence, "description") ||
		!strings.Contains(g.Consequence, "host") {
		t.Errorf("consequence must say what the field is FOR; got %q", g.Consequence)
	}
}

// TestFifthCheckSilentWhenEveryDescriptionIsApproved — NEGATIVE CONTROL. A
// false positive here would report edikt-dev's own 11 approved skills as
// degraded on every doctor run.
func TestFifthCheckSilentWhenEveryDescriptionIsApproved(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "tooling", "Designing edikt's binaries and dev helpers.")
	writeSkill(t, root, "release", "Planning or performing a release.")
	if gaps := skillsShippingFallbackDescription(filepath.Join(root, ".claude", "skills")); gaps != nil {
		t.Errorf("approved descriptions must not report a gap; got %v", gaps)
	}
}

// TestOutputCheckUnmeasuredIsNotAPass — with no compiled output there is
// nothing to judge, and the check must decline rather than print ok (INV-013).
func TestOutputCheckUnmeasuredIsNotAPass(t *testing.T) {
	var w bytes.Buffer
	warns, ran := runOutputGapsCheck(t.TempDir(), &w)
	if ran {
		t.Error("no compiled output exists; the check must report unmeasured")
	}
	if warns != 0 || strings.Contains(w.String(), "ok") {
		t.Errorf("a project with no compiled surface must never read ok: %q", w.String())
	}
}

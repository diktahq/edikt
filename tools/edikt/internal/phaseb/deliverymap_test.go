package phaseb

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// RouteOf is the single routing decision, so its table is pinned directly.
// Every case here is a rule that used to live in a renderer's local `if`, and
// the point of the function is that there is now one copy of each.
func TestRouteOf_Table(t *testing.T) {
	cases := []struct {
		name       string
		grade      string
		actorScope bool
		tier3      bool
		want       string
	}{
		{"must goes to the topic file (write-time retired, ADR-083)", gradeMust, false, false, SurfaceTopicFile},
		{"must in a tier-3 topic goes to the skill", gradeMust, false, true, SurfaceSkill},
		{"advisory goes to the topic file", gradeAdvisory, false, false, SurfaceTopicFile},
		{"advisory in a tier-3 topic goes to the skill", gradeAdvisory, false, true, SurfaceSkill},

		// ADR-065 excludes actor-scoped entries from the index "and only there":
		// they "still render in the ambient/topic-file surfaces". So an
		// actor-scoped MUST takes advisory's home — topic file, or skill when
		// the topic is tier 3 — and NOTHING routes to SurfaceUnrouted. The old
		// row here pinned "actor-scoped must reaches no surface" as ADR-065's
		// asymmetry; it was a defect, and 7 of INV-010's 9 directives were
		// reaching nothing under it.
		{"actor-scoped must reaches its topic file", gradeMust, true, false, SurfaceTopicFile},
		{"actor-scoped must in a tier-3 topic reaches its skill", gradeMust, true, true, SurfaceSkill},
		{"actor-scoped advisory still reaches its topic file", gradeAdvisory, true, false, SurfaceTopicFile},
		{"actor-scoped advisory in tier-3 reaches its skill", gradeAdvisory, true, true, SurfaceSkill},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RouteOf(c.grade, c.actorScope, c.tier3); got != c.want {
				t.Fatalf("RouteOf(%q, actorScope=%v, tier3=%v) = %q, want %q",
					c.grade, c.actorScope, c.tier3, got, c.want)
			}
		})
	}
}

// TestDeliveryMap_NeverRoutesWriteTime pins the re-homing (ADR-083, Phase 0
// AC-0.3): every directive — must, advisory, actor-scoped — is recorded with a
// surface that still exists, and no row of RouteOf can name the retired one.
func TestDeliveryMap_NeverRoutesWriteTime(t *testing.T) {
	pairs := []sidecar.Pair{{
		ArtifactID: "ADR-901",
		Sidecar: &sidecar.Sidecar{
			Topic: "architecture",
			Paths: []string{"a/**/*.go"},
			Directives: []sidecar.Directive{
				{Text: "Advisory by class."},
				{Text: "Must by enforcement.", Enforcement: "deny"},
				{Text: "Actor-scoped must.", Enforcement: "deny", ActorScope: true},
			},
			Prohibitions: []sidecar.Prohibition{{Text: "MUST NOT do it."}},
		},
	}}
	dm := BuildDeliveryMap(pairs, map[string]bool{})
	seen := map[string]string{}
	for _, entries := range dm {
		for _, e := range entries {
			seen[e.ID] = e.Surface
		}
	}
	if len(seen) != 4 {
		t.Fatalf("routing table must record EVERY directive; got %d of 4: %v", len(seen), seen)
	}
	for id, surface := range seen {
		switch surface {
		case SurfaceTopicFile, SurfaceSkill:
		default:
			t.Errorf("%s: routed to %q, which is not a surface that exists after ADR-083", id, surface)
		}
		if surface == "write-time" {
			t.Errorf("%s: the retired write-time surface is still being routed to", id)
		}
	}
	if seen["ADR-901:d02"] != SurfaceTopicFile {
		t.Errorf("must-grade directive must be recorded as topic-file routed, got %q", seen["ADR-901:d02"])
	}
	if seen["ADR-901:d03"] != SurfaceTopicFile {
		t.Errorf("actor-scoped must must be recorded as topic-file routed, got %q", seen["ADR-901:d03"])
	}
}

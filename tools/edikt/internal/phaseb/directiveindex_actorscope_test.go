package phaseb

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ADR-065 — actor-scoped directives constrain an edikt-internal automated
// operation (the compilation pipeline, gov:compile, migrate, upgrade), not
// file content a human edit could violate. hook match's write-time
// MUST-grade selection excludes ActorScope: true entries unconditionally,
// because that channel only observes Claude-Code-mediated Edit/Write calls
// — which the Go-internal pipeline operations never make — so delivering
// these as PreToolUse bounces is noise with no enforcement value through
// this specific surface.

func TestBuildDirectiveIndex_ActorScopeExcludedFromWriteTimeDelivery(t *testing.T) {
	pair := sidecar.Pair{
		ArtifactID: "INV-999",
		Sidecar: &sidecar.Sidecar{
			Paths: []string{"docs/architecture/**/*.md"},
			Directives: []sidecar.Directive{
				{Text: "The pipeline MUST write only to the sidecar. (ref: INV-999)", ActorScope: true},
				{Text: "A human editing prose MUST NOT be interrupted by this rule. (ref: INV-999)", ActorScope: false},
			},
			Prohibitions: []sidecar.Prohibition{
				{Text: "MUST NOT let gov:compile write the parent .md. (ref: INV-999)", ActorScope: true},
				{Text: "MUST NOT ship with no permissions block. (ref: INV-999)", ActorScope: false},
			},
		},
	}

	idx := BuildDirectiveIndex([]sidecar.Pair{pair})
	entries := idx["docs/architecture/**/*.md"]

	if len(entries) != 2 {
		var texts []string
		for _, e := range entries {
			texts = append(texts, e.Text)
		}
		t.Fatalf("expected exactly 2 entries (the two untagged), got %d: %v", len(entries), texts)
	}
	for _, e := range entries {
		if strings.Contains(e.Text, "pipeline MUST write only") || strings.Contains(e.Text, "gov:compile write the parent") {
			t.Fatalf("actor-scoped entry leaked into write-time index: %q", e.Text)
		}
	}
	sawHumanDirective, sawPermissionsProhibition := false, false
	for _, e := range entries {
		if strings.Contains(e.Text, "A human editing prose") {
			sawHumanDirective = true
		}
		if strings.Contains(e.Text, "no permissions block") {
			sawPermissionsProhibition = true
		}
	}
	if !sawHumanDirective || !sawPermissionsProhibition {
		t.Fatalf("untagged entries must still be delivered — sawHumanDirective=%v sawPermissionsProhibition=%v", sawHumanDirective, sawPermissionsProhibition)
	}
}

// Sensitivity control (GL-002): confirm the exclusion is scoped to
// ActorScope specifically, not to the artifact or to directives/
// prohibitions generally — an untagged sidecar with no ActorScope fields
// set at all must be completely unaffected by this change.
func TestBuildDirectiveIndex_NoActorScopeFieldsSetIsUnaffected(t *testing.T) {
	pair := sidecar.Pair{
		ArtifactID: "ADR-999",
		Sidecar: &sidecar.Sidecar{
			Paths: []string{"docs/**/*.md"},
			Directives: []sidecar.Directive{
				{Text: "Ordinary directive one. (ref: ADR-999)"},
				{Text: "Ordinary directive two. (ref: ADR-999)"},
			},
			Prohibitions: []sidecar.Prohibition{
				{Text: "MUST NOT do the rejected thing. (ref: ADR-999)"},
			},
		},
	}
	idx := BuildDirectiveIndex([]sidecar.Pair{pair})
	entries := idx["docs/**/*.md"]
	if len(entries) != 3 {
		t.Fatalf("absent actor_scope must behave exactly as today — expected all 3 entries present, got %d", len(entries))
	}
}

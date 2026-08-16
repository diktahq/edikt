package contradiction

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func pair(artifactID, topic string, directives []sidecar.Directive) sidecar.Pair {
	return sidecar.Pair{
		ArtifactID: artifactID,
		Sidecar: &sidecar.Sidecar{
			Topic:      topic,
			Directives: directives,
		},
	}
}

func directive(text string) sidecar.Directive {
	return sidecar.Directive{Text: text}
}

// TestDetect_PlantedContradiction pins AC-9.1: a corpus containing a
// planted contradiction (two ADRs, same topic, one MUST X and one MUST NOT
// X on the same subject) is detected.
func TestDetect_PlantedContradiction(t *testing.T) {
	pairs := []sidecar.Pair{
		pair("ADR-001", "storage", []sidecar.Directive{
			directive("Diagram images MUST be stored in MinIO. (ref: ADR-001)"),
		}),
		pair("ADR-002", "storage", []sidecar.Directive{
			directive("Diagram images MUST NOT be stored in MinIO. (ref: ADR-002)"),
		}),
	}
	conflicts := Detect(pairs)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(conflicts), conflicts)
	}
	c := conflicts[0]
	if c.Topic != "storage" {
		t.Errorf("Topic = %q, want %q", c.Topic, "storage")
	}
	gotSources := map[string]bool{c.A.Source: true, c.B.Source: true}
	if !gotSources["ADR-001"] || !gotSources["ADR-002"] {
		t.Errorf("expected sources ADR-001 and ADR-002, got %+v", c)
	}
}

// TestDetect_DifferentTopicsNotFlagged is the control: the SAME opposing
// directives in DIFFERENT topics (different co-loading sets — they never
// land in the same compiled file) must not be flagged. Proves the
// same-topic scoping actually restricts, not just the modality/noun-phrase
// check alone.
func TestDetect_DifferentTopicsNotFlagged(t *testing.T) {
	pairs := []sidecar.Pair{
		pair("ADR-001", "storage", []sidecar.Directive{
			directive("Diagram images MUST be stored in MinIO. (ref: ADR-001)"),
		}),
		pair("ADR-002", "networking", []sidecar.Directive{
			directive("Diagram images MUST NOT be stored in MinIO. (ref: ADR-002)"),
		}),
	}
	if conflicts := Detect(pairs); len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts across different topics, got %d: %+v", len(conflicts), conflicts)
	}
}

// TestDetect_DifferentSubjectsNotFlagged: same topic, both MANDATE/
// PROHIBITION, but about unrelated subjects — the noun-phrase check must
// keep these apart.
func TestDetect_DifferentSubjectsNotFlagged(t *testing.T) {
	pairs := []sidecar.Pair{
		pair("ADR-001", "storage", []sidecar.Directive{
			directive("Diagram images MUST be stored in MinIO. (ref: ADR-001)"),
		}),
		pair("ADR-002", "storage", []sidecar.Directive{
			directive("Backup archives MUST NOT exceed 7 days retention. (ref: ADR-002)"),
		}),
	}
	if conflicts := Detect(pairs); len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts for unrelated subjects, got %d: %+v", len(conflicts), conflicts)
	}
}

// TestDetect_SameDirectionNotFlagged: two MUST directives on the same
// subject agree, they do not conflict — only OPPOSING modality is a
// conflict.
func TestDetect_SameDirectionNotFlagged(t *testing.T) {
	pairs := []sidecar.Pair{
		pair("ADR-001", "storage", []sidecar.Directive{
			directive("Diagram images MUST be stored in MinIO. (ref: ADR-001)"),
		}),
		pair("ADR-002", "storage", []sidecar.Directive{
			directive("Diagram images MUST be stored in MinIO. (ref: ADR-002)"),
		}),
	}
	if conflicts := Detect(pairs); len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts for agreeing directives, got %d: %+v", len(conflicts), conflicts)
	}
}

// TestDetect_RetiredArtifactAlreadyExcludedByCaller: Detect does not
// re-derive the retired-artifact filter — it trusts pairs is already
// filtered, matching Phase B's own contract. This test documents that
// contract rather than testing exclusion logic that doesn't live here: a
// pair simply absent from the input (as if already excluded upstream)
// produces no conflict, because Detect never sees it.
func TestDetect_RetiredArtifactAlreadyExcludedByCaller(t *testing.T) {
	// Only the live (non-retired) side is present, as Phase B would hand it.
	pairs := []sidecar.Pair{
		pair("ADR-002", "storage", []sidecar.Directive{
			directive("Diagram images MUST NOT be stored in MinIO. (ref: ADR-002)"),
		}),
	}
	if conflicts := Detect(pairs); len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts with only one side present, got %d: %+v", len(conflicts), conflicts)
	}
}

// TestDetect_ProhibitionsAreScanned: a MUST in directives[] and a MUST NOT
// in prohibitions[] on the same subject must also be caught — the conflict
// isn't limited to directives[] vs directives[].
func TestDetect_ProhibitionsAreScanned(t *testing.T) {
	pairs := []sidecar.Pair{
		{
			ArtifactID: "ADR-001",
			Sidecar: &sidecar.Sidecar{
				Topic:      "storage",
				Directives: []sidecar.Directive{directive("Diagram images MUST be stored in MinIO. (ref: ADR-001)")},
			},
		},
		{
			ArtifactID: "ADR-002",
			Sidecar: &sidecar.Sidecar{
				Topic: "storage",
				Prohibitions: []sidecar.Prohibition{
					{Text: "Diagram images MUST NOT be stored in MinIO. (ref: ADR-002)"},
				},
			},
		},
	}
	if conflicts := Detect(pairs); len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict spanning directives[] and prohibitions[], got %d: %+v", len(conflicts), conflicts)
	}
}

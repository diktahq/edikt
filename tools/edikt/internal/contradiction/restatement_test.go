package contradiction

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func prohibitionPair(artifactID, topic string, texts ...string) sidecar.Pair {
	var ps []sidecar.Prohibition
	for _, t := range texts {
		ps = append(ps, sidecar.Prohibition{Text: t})
	}
	return sidecar.Pair{
		ArtifactID: artifactID,
		Sidecar:    &sidecar.Sidecar{Topic: topic, Prohibitions: ps},
	}
}

// TestRestatement_PlantedCrossArtifactDuplicate is AC-4.2's positive: the
// finding must NAME BOTH ARTIFACTS. A report that says "a duplicate exists"
// without saying where sends a reader to grep the corpus.
func TestRestatement_PlantedCrossArtifactDuplicate(t *testing.T) {
	pairs := []sidecar.Pair{
		pair("ADR-041", "security", []sidecar.Directive{
			directive("Trust-store entries MUST be verified before any repo command runs. (ref: ADR-041)"),
		}),
		pair("ADR-052", "architecture", []sidecar.Directive{
			directive("Trust store entries MUST be verified before any repo command runs. (ref: ADR-052)"),
		}),
	}
	found := DetectRestatements(pairs)
	if len(found) != 1 {
		t.Fatalf("expected 1 restatement, got %d: %+v", len(found), found)
	}
	r := found[0]
	sources := map[string]bool{r.A.Source: true, r.B.Source: true}
	if !sources["ADR-041"] || !sources["ADR-052"] {
		t.Fatalf("finding must name both artifacts; got A=%s B=%s", r.A.Source, r.B.Source)
	}
	if r.SameTopic {
		t.Errorf("SameTopic = true for security/architecture; the cross-topic case is the one no single surface shows")
	}
	if r.Modality != "MANDATE" {
		t.Errorf("Modality = %q, want MANDATE", r.Modality)
	}
}

// TestRestatement_SameArtifactControl — two copies inside ONE sidecar are a
// within-artifact extraction defect, not this finding. If this control were
// missing, every artifact that repeats itself would drown the cross-artifact
// signal the criterion is about.
func TestRestatement_SameArtifactControl(t *testing.T) {
	pairs := []sidecar.Pair{
		pair("ADR-041", "security", []sidecar.Directive{
			directive("Trust-store entries MUST be verified before any repo command runs. (ref: ADR-041)"),
			directive("Trust store entries MUST be verified before any repo command runs. (ref: ADR-041)"),
		}),
	}
	if found := DetectRestatements(pairs); len(found) != 0 {
		t.Fatalf("same-artifact duplicate must stay silent; got %+v", found)
	}
}

// TestRestatement_DifferentSubjectControl proves the noun-phrase comparison
// actually restricts. Without it the detector could be matching on modality
// alone and would report every MUST in the corpus against every other.
func TestRestatement_DifferentSubjectControl(t *testing.T) {
	pairs := []sidecar.Pair{
		pair("ADR-041", "security", []sidecar.Directive{
			directive("Trust-store entries MUST be verified before any repo command runs. (ref: ADR-041)"),
		}),
		pair("ADR-052", "architecture", []sidecar.Directive{
			directive("Release payloads MUST carry a cosign signature bundle. (ref: ADR-052)"),
		}),
	}
	if found := DetectRestatements(pairs); len(found) != 0 {
		t.Fatalf("different-subject pair must stay silent; got %+v", found)
	}
}

// TestRestatement_OpposingModalityIsNotARestatement — same subject, opposing
// modality is Detect's finding, reported there with the correct framing.
// Reporting it here too would double-count a disagreement as agreement.
func TestRestatement_OpposingModalityIsNotARestatement(t *testing.T) {
	pairs := []sidecar.Pair{
		pair("ADR-001", "storage", []sidecar.Directive{
			directive("Diagram images MUST be stored in MinIO. (ref: ADR-001)"),
		}),
		pair("ADR-002", "storage", []sidecar.Directive{
			directive("Diagram images MUST NOT be stored in MinIO. (ref: ADR-002)"),
		}),
	}
	if found := DetectRestatements(pairs); len(found) != 0 {
		t.Fatalf("opposing modality is a contradiction, not a restatement; got %+v", found)
	}
	if len(Detect(pairs)) != 1 {
		t.Fatal("the same pair must still be reported by Detect")
	}
}

// TestRestatement_ProhibitionsAreScanned is F-006's lesson applied here:
// `verify:` commands live on three collections and a gate that scanned two
// was blind to a third of its subject. Restatements live on directives[] AND
// prohibitions[]; a detector that walked only directives[] would report clean
// over every duplicated MUST NOT in the corpus.
func TestRestatement_ProhibitionsAreScanned(t *testing.T) {
	pairs := []sidecar.Pair{
		prohibitionPair("ADR-030", "tooling",
			"MUST NOT spawn or shell out to any LLM CLI from a tier-2 binary. (ref: ADR-030)"),
		prohibitionPair("INV-012", "tooling",
			"MUST NOT spawn or shell out to any LLM CLI from a tier-2 binary. (ref: INV-012)"),
	}
	found := DetectRestatements(pairs)
	if len(found) != 1 {
		t.Fatalf("expected the duplicated prohibition to be found, got %d: %+v", len(found), found)
	}
	if found[0].Modality != "PROHIBITION" {
		t.Errorf("Modality = %q, want PROHIBITION", found[0].Modality)
	}
}

// TestRestatement_MixedCollectionsAreCompared closes the other half of the
// same class: one copy as a directive, the other as a prohibition. A detector
// that scanned both collections but compared them only within their own kind
// would still miss it.
func TestRestatement_MixedCollectionsAreCompared(t *testing.T) {
	pairs := []sidecar.Pair{
		pair("ADR-030", "tooling", []sidecar.Directive{
			directive("Tier-2 binaries MUST NOT shell out to any LLM CLI. (ref: ADR-030)"),
		}),
		prohibitionPair("INV-012", "tooling",
			"Tier-2 binaries MUST NOT shell out to any LLM CLI. (ref: INV-012)"),
	}
	if found := DetectRestatements(pairs); len(found) != 1 {
		t.Fatalf("a directive/prohibition cross-collection duplicate must be found; got %d: %+v", len(found), found)
	}
}

// TestRestatement_SameTopicIsFlaggedAndLabelled — the within-topic case is
// still a finding; it just has a different remedy, so the label must survive
// to the reader.
func TestRestatement_SameTopicIsFlaggedAndLabelled(t *testing.T) {
	pairs := []sidecar.Pair{
		pair("ADR-015", "tooling", []sidecar.Directive{
			directive("Tier-1 commands MUST NOT depend on tier-2 files at runtime. (ref: ADR-015)"),
		}),
		pair("ADR-021", "tooling", []sidecar.Directive{
			directive("Tier-1 commands MUST NOT depend on tier-2 files at runtime. (ref: ADR-021)"),
		}),
	}
	found := DetectRestatements(pairs)
	if len(found) != 1 || !found[0].SameTopic {
		t.Fatalf("same-topic duplicate must be found and labelled SameTopic; got %+v", found)
	}
}

// TestRestatement_ReportNamesBothArtifacts pins the rendered line, because a
// finding a human cannot act on is the shape F-005 named: a control whose only
// expression is output nobody can use.
func TestRestatement_ReportNamesBothArtifacts(t *testing.T) {
	pairs := []sidecar.Pair{
		pair("ADR-041", "security", []sidecar.Directive{
			directive("Trust-store entries MUST be verified before any repo command runs. (ref: ADR-041)"),
		}),
		pair("ADR-052", "architecture", []sidecar.Directive{
			directive("Trust store entries MUST be verified before any repo command runs. (ref: ADR-052)"),
		}),
	}
	report := RestatementReport(DetectRestatements(pairs))
	for _, want := range []string{"ADR-041", "ADR-052", "security", "architecture", "restatement"} {
		if !strings.Contains(strings.ToLower(report), strings.ToLower(want)) {
			t.Errorf("report omits %q:\n%s", want, report)
		}
	}
}

// TestRestatement_EmptyCorpusReportsNothingNotSuccess — an empty input must
// produce an empty report, never a line that reads as a clean bill of health
// over a corpus that was never scanned.
func TestRestatement_EmptyCorpusReportsNothing(t *testing.T) {
	if found := DetectRestatements(nil); len(found) != 0 {
		t.Fatalf("nil corpus produced findings: %+v", found)
	}
	if r := RestatementReport(nil); r != "" {
		t.Fatalf("empty findings rendered %q; want the empty string so a caller decides what absence means", r)
	}
}

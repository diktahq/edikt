package govrun

import (
	"os"
	"strings"
	"testing"
)

// AC-4.2 — every Phase A path that was observed through an injected runner is
// still exercised after the runner is gone.
//
// Seven govrun test files used TwoPhaseOptions.Runner as an observation point
// for the computed work-list. Deleting ClaudeRunner removed that seam, and the
// cheapest way to make the package green would have been to delete the tests
// with it. This exists to make that impossible without the deletion being
// visible: the criterion is "none deleted to reach green", and a criterion
// nothing checks is one nothing enforces.
//
// It asserts on the FILES and the NAMED TESTS rather than on a count. AC-4.2
// originally said "the eight test files"; there were seven. A count is the
// wrong pin — it drifts, and it says nothing about which path went missing.
func TestPhaseAPathsStillCovered(t *testing.T) {
	// path -> a test function that must still be defined in it, one per
	// Phase A behaviour the injected runner used to observe.
	want := map[string][]string{
		"twophase_bootstrap_test.go": {
			"TestRunTwoPhase_BootstrapsMissingSidecars",
			"TestRunTwoPhase_Headless_RefusesBootstrap",
			"TestRunTwoPhase_CheckMode_RefusesBootstrap",
		},
		"twophase_skip_test.go":          {"TestRunTwoPhase_SupersededArtifactIsFullyExcluded"},
		"twophase_scope_test.go":         {"TestScopeCoverage_IsReported", "TestTopicPaths_CarryDeclaredGlobsNotTheADRPath"},
		"twophase_contradiction_test.go": {"TestRunTwoPhase_DetectsPlantedContradiction", "TestRunTwoPhase_NoConflictsReportsNone"},
		"twophase_index_counts_test.go": {
			"TestRunTwoPhase_IndexReportsRetiredExclusions",
			"TestRunTwoPhase_IndexReportsZeroExclusionsAsMeasured",
		},
		"twophase_status_filter_test.go":   {"TestRunTwoPhase_StatusFilter_ProposedExcluded_BodyAcceptedIncluded"},
		"twophase_extractor_model_test.go": {"TestRunTwoPhase_RecordsExtractorAgentModel"},
	}

	for file, tests := range want {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("%s is gone — a Phase A path was dropped rather than adapted (AC-4.2): %v", file, err)
			continue
		}
		body := string(src)
		for _, fn := range tests {
			if !strings.Contains(body, "func "+fn+"(") {
				t.Errorf("%s no longer defines %s — the path it covered is unobserved", file, fn)
			}
		}
		// A file that survived but no longer drives a compile is a file that
		// stopped testing Phase A. This catches gutting-in-place, which the
		// filename check alone would miss.
		if !strings.Contains(body, "RunTwoPhase(TwoPhaseOptions{") {
			t.Errorf("%s no longer calls RunTwoPhase — it survives but exercises nothing", file)
		}
	}

	// The seam itself must stay gone. If TwoPhaseOptions grows a Runner field
	// again, the tests above could be "adapted" back onto a fake dispatch and
	// this whole criterion would quietly stop meaning anything.
	twophase := readSource(t, "twophase.go")
	if strings.Contains(twophase, "Runner      phasea.Runner") {
		t.Error("TwoPhaseOptions.Runner is back — Phase A can be observed through a fake dispatch again, and INV-012's boundary is reopened")
	}
}

package sidecar

// AC-1.7 measurement harness.
//
// Runs the SHIPPED ADR-056 gate (VerifyAnchors) over a directory of extractor
// output, so the number reported for AC-1.7 is produced by the same code that
// refuses a dispatch in production. An separate measurement implementation
// would be a second definition of "valid anchor", and the two would drift
// (GL-002: match the test to the instrument).
//
// Before ADR-056 this ran ClassifyExcerpt, which accepts a whitespace-
// normalised match. The gate does not. A number from this harness is therefore
// measured against a strictly higher bar than the pre-ADR-056 runs, and is not
// directly comparable to them.
//
// Skipped unless AC17_DIR points at a directory of (parent .md, sidecar
// .edikt.yaml) pairs — a harness, not a CI gate. The gate itself is tested by
// anchorverify_test.go and enforced by internal/phasea.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestAC17GroundingProbe(t *testing.T) {
	dir := os.Getenv("AC17_DIR")
	if dir == "" {
		t.Skip("AC17_DIR unset — measurement harness, not a gate")
	}

	yamls, err := filepath.Glob(filepath.Join(dir, "*.edikt.yaml"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	sort.Strings(yamls)
	if len(yamls) == 0 {
		t.Fatalf("AC17_DIR=%s contains no *.edikt.yaml — UNMEASURED, not zero failures", dir)
	}

	totalAnchors, totalItems, totalFaults := 0, 0, 0
	perArtifact := []string{}

	for _, y := range yamls {
		sc, err := Load(y)
		if err != nil {
			t.Errorf("%s: load failed: %v", filepath.Base(y), err)
			continue
		}
		parent := strings.TrimSuffix(y, ".edikt.yaml") + ".md"
		body, err := os.ReadFile(parent)
		if err != nil {
			t.Fatalf("%s: cannot read parent %s: %v", filepath.Base(y), parent, err)
		}

		v := VerifyAnchors(sc, string(body))
		for _, f := range v.Faults {
			t.Errorf("%s %s", filepath.Base(y), f)
		}

		needsReview := 0
		for _, d := range sc.Directives {
			if d.NeedsReview {
				needsReview++
			}
		}
		for _, p := range sc.Prohibitions {
			if p.NeedsReview {
				needsReview++
			}
		}

		totalAnchors += v.Anchors
		totalItems += v.Items
		totalFaults += len(v.Faults)
		perArtifact = append(perArtifact, fmt.Sprintf(
			"  %-58s %2d item(s), %3d anchor(s), %d fault(s), %d needs_review, %d proposed_path(s), topic=%s",
			filepath.Base(y), v.Items, v.Anchors, len(v.Faults), needsReview,
			len(sc.ProposedPaths), sc.Topic))
	}

	t.Logf("AC-1.7 anchor gate over %d artifact(s):\n%s", len(yamls), strings.Join(perArtifact, "\n"))
	t.Logf("TOTALS: %d fault(s) across %d anchors in %d items over %d artifacts",
		totalFaults, totalAnchors, totalItems, len(yamls))
	if totalAnchors == 0 {
		t.Fatal("zero anchors examined — the harness measured nothing")
	}
}

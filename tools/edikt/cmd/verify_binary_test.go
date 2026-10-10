package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestBinaryFreshness_StaleRevisionRefusesUnlessAllowed(t *testing.T) {
	var w bytes.Buffer
	p := binaryProvenance{Revision: "5734d41c871c000000000000000000000000000a"}
	if err := binaryFreshness(p, "586a890f00000000000000000000000000000000", false, &w); err == nil {
		t.Fatal("a binary built at another commit must refuse to verify")
	}
	w.Reset()
	if err := binaryFreshness(p, "586a890f00000000000000000000000000000000", true, &w); err != nil {
		t.Fatalf("--allow-stale-binary must let it run: %v", err)
	}
	if !strings.Contains(w.String(), "warn: ") {
		t.Fatalf("the override still warns loudly:\n%s", w.String())
	}
}

func TestBinaryFreshness_HeadBuildRunsQuietly(t *testing.T) {
	var w bytes.Buffer
	p := binaryProvenance{Revision: "586a890f00000000000000000000000000000000"}
	if err := binaryFreshness(p, "586a890f", false, &w); err != nil || w.Len() != 0 {
		t.Fatalf("a HEAD build must run without a word: err=%v out=%q", err, w.String())
	}
}

func TestBinaryFreshness_ModifiedAndUnstampedWarn(t *testing.T) {
	var w bytes.Buffer
	if err := binaryFreshness(binaryProvenance{Revision: "586a890f", Modified: true}, "586a890f", false, &w); err != nil || !strings.Contains(w.String(), "MODIFIED") {
		t.Fatalf("a modified-tree build warns and runs: err=%v out=%q", err, w.String())
	}
	w.Reset()
	if err := binaryFreshness(binaryProvenance{}, "586a890f", false, &w); err != nil || !strings.Contains(w.String(), "no vcs.revision") {
		t.Fatalf("an unstamped build warns and runs: err=%v out=%q", err, w.String())
	}
}

// The command tests run in temp directories that are not git checkouts; a
// head of "unknown" is a missing comparison, not a stale runner. And gitSHA's
// "-dirty" suffix is the tree's state, not part of the commit.
func TestBinaryFreshness_UnknownHeadAndDirtySuffix(t *testing.T) {
	var w bytes.Buffer
	p := binaryProvenance{Revision: "586a890f00000000000000000000000000000000"}
	if err := binaryFreshness(p, "unknown", false, &w); err != nil || !strings.Contains(w.String(), "not readable") {
		t.Fatalf("an unreadable head must warn, not refuse: err=%v out=%q", err, w.String())
	}
	w.Reset()
	if err := binaryFreshness(p, "586a890f-dirty", false, &w); err != nil {
		t.Fatalf("a dirty suffix on a matching head must not read as a different commit: %v", err)
	}
}

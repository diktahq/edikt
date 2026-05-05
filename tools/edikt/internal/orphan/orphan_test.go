package orphan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func priorOf(ids ...string) *[]string {
	v := append([]string(nil), ids...)
	return &v
}

func TestDecide_NoOrphans(t *testing.T) {
	sc, block, write := Decide(nil, priorOf("ADR-001"))
	if sc != NoOrphans || block || !write {
		t.Errorf("expected NoOrphans/!block/write, got %v/%v/%v", sc, block, write)
	}
}

func TestDecide_FreshHistory(t *testing.T) {
	sc, block, write := Decide([]string{"ADR-001"}, nil)
	if sc != FirstDetection || block || !write {
		t.Errorf("expected FirstDetection/!block/write, got %v/%v/%v", sc, block, write)
	}
}

func TestDecide_NewOrphan(t *testing.T) {
	// Active → orphan: prior had no overlap with current.
	sc, block, write := Decide([]string{"ADR-002"}, priorOf("ADR-001"))
	if sc != DifferentReset || block || !write {
		t.Errorf("expected DifferentReset/!block/write, got %v/%v/%v", sc, block, write)
	}
}

func TestDecide_PersistentOrphan(t *testing.T) {
	// Same orphan set on consecutive runs → BLOCK.
	sc, block, write := Decide([]string{"ADR-001"}, priorOf("ADR-001"))
	if sc != Consecutive || !block || write {
		t.Errorf("expected Consecutive/block/!write, got %v/%v/%v", sc, block, write)
	}
}

func TestDecide_Recovered(t *testing.T) {
	// Orphan resolved (subset of prior) → reset to first-detection.
	sc, block, write := Decide([]string{"ADR-001"}, priorOf("ADR-001", "ADR-002"))
	if sc != SubsetResolved || block || !write {
		t.Errorf("expected SubsetResolved/!block/write, got %v/%v/%v", sc, block, write)
	}
}

func TestDecide_Superset(t *testing.T) {
	sc, block, write := Decide([]string{"ADR-001", "ADR-002"}, priorOf("ADR-001"))
	if sc != SupersetAdded || block || !write {
		t.Errorf("expected SupersetAdded/!block/write, got %v/%v/%v", sc, block, write)
	}
}

func TestReadStateDetailed_Missing(t *testing.T) {
	root := t.TempDir()
	s, ok, err := ReadStateDetailed(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if s != nil || !ok {
		t.Errorf("missing file: want (nil, true), got (%v, %v)", s, ok)
	}
}

func TestReadStateDetailed_Corrupt(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "history.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, ok, err := ReadStateDetailed(p)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if s != nil || ok {
		t.Errorf("corrupt: want (nil, false), got (%v, %v)", s, ok)
	}
}

func TestWriteStateAt_DeterministicOrdering(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, ".edikt", "state", "compile-history.json")
	ts := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)

	// Same input → byte-equal output (ADR-020 deterministic compile).
	if err := WriteStateAt(p, []string{"ADR-002", "ADR-001"}, "0.6.0", ts); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(p)
	if err := WriteStateAt(p, []string{"ADR-001", "ADR-002"}, "0.6.0", ts); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(p)
	if string(first) != string(second) {
		t.Errorf("non-deterministic output:\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestWriteState_AtomicRename(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".edikt", "state")
	p := filepath.Join(stateDir, "compile-history.json")
	if err := WriteState(p, []string{"ADR-001"}, "0.6.0"); err != nil {
		t.Fatal(err)
	}
	// Final file exists; .tmp does not.
	if _, err := os.Stat(p); err != nil {
		t.Errorf("final file missing: %v", err)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("tmp file leaked: %v", err)
	}
}

func TestWriteStateAt_CreatesParentDir(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "deep", "nested", "history.json")
	if err := WriteStateAt(p, nil, "", time.Now()); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("expected file at %s: %v", p, err)
	}
}

func TestState_RoundTrip_CorruptThenRecover(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "history.json")
	// Seed corrupt file.
	if err := os.WriteFile(p, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Caller treats corrupt as absent → Decide returns FirstDetection.
	state, ok, _ := ReadStateDetailed(p)
	if ok || state != nil {
		t.Fatalf("corrupt file should report ok=false, state=nil")
	}
	sc, _, write := Decide([]string{"ADR-001"}, nil)
	if sc != FirstDetection || !write {
		t.Errorf("recovery: want FirstDetection/write, got %v/%v", sc, write)
	}
	// Subsequent write is healthy.
	ts := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	if err := WriteStateAt(p, []string{"ADR-001"}, "0.6.0", ts); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(p)
	if !strings.Contains(string(body), "\"orphan_adrs\":") {
		t.Errorf("written file missing orphan_adrs: %s", body)
	}
}

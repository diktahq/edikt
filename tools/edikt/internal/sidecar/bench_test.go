package sidecar

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkSidecarLoad_Single measures the cost of one Load call on a
// canonical fixture. Targets a stable corpus (test/fixtures/sidecars/valid)
// so trend lines compare across runs.
//
// Phase 7 of PLAN-sidecar-review-fixes #42 — informational; not a PR
// blocker in v0.6.0.
func BenchmarkSidecarLoad_Single(b *testing.B) {
	root, err := findRepoRootForBench()
	if err != nil {
		b.Skipf("repo root not found: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, "test/fixtures/sidecars/valid/adr-001.edikt.yaml"))
	if err != nil || len(matches) == 0 {
		b.Skip("adr-001 fixture not found")
	}
	path := matches[0]

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Load(path); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSidecarDiscover_50Artifacts measures one Discover walk over a
// 50-artifact corpus. Phase B's hot path calls Discover once per compile;
// holding this number stable bounds the cold-start cost.
//
// ADR-020 budget: Discover + IsStale on 50 artifacts < 50 ms p95.
func BenchmarkSidecarDiscover_50Artifacts(b *testing.B) {
	root := buildSidecarCorpus(b, 50)
	dirs := []string{"docs/architecture/decisions"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Discover(root, dirs); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkIsStale_50Artifacts measures the per-artifact IsStale check
// over the 50-artifact corpus. Reads each parent .md once per call, so
// I/O dominates — useful for tracking the load cost when canon changes
// or templates grow.
func BenchmarkIsStale_50Artifacts(b *testing.B) {
	root := buildSidecarCorpus(b, 50)
	dirs := []string{"docs/architecture/decisions"}
	pairs, err := Discover(root, dirs)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range pairs {
			if p.Sidecar == nil {
				continue
			}
			if _, _, err := p.Sidecar.IsStale(root); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// findRepoRootForBench walks up from the test cwd looking for the repo
// root, identified by test/fixtures/sidecars/valid/.
func findRepoRootForBench() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "test/fixtures/sidecars/valid")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no test/fixtures/sidecars/valid found above cwd")
}

// buildSidecarCorpus lays out N parent.md + N sidecar.edikt.yaml files
// in a temp project root, with each sidecar's source_excerpt anchored to
// a known line in the parent so IsStale returns false (in-sync).
func buildSidecarCorpus(b *testing.B, n int) string {
	b.Helper()
	root := b.TempDir()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("ADR-%03d", i+1)
		base := fmt.Sprintf("%s-bench", id)
		mdPath := filepath.Join(dir, base+".md")
		mdBody := "# " + id + " — bench\n" +
			"\n" +
			"## Decision\n" +
			"\n" +
			"Use bench topic in this fixture for " + id + ".\n"
		if err := os.WriteFile(mdPath, []byte(mdBody), 0o644); err != nil {
			b.Fatal(err)
		}
		ycPath := filepath.Join(dir, base+".edikt.yaml")
		yc := "schema_version: 1\n" +
			"topic: bench-topic\n" +
			"path: docs/architecture/decisions/" + base + ".md\n" +
			"signals:\n  - bench\n" +
			"directives:\n" +
			"  - text: \"Use bench topic in this fixture for " + id + ". (ref: " + id + ")\"\n" +
			"    source_excerpt:\n" +
			"      line_start: 5\n" +
			"      line_end: 5\n" +
			"      quote: \"Use bench topic in this fixture for " + id + ".\"\n"
		if err := os.WriteFile(ycPath, []byte(yc), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return root
}

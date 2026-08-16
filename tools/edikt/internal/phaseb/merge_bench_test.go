package phaseb

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// BenchmarkPhaseBMerge_50Artifacts measures the cost of one Phase B merge
// over a 50-artifact corpus. The corpus is laid out once per outer
// iteration; the timed inner loop calls Merge against the existing
// on-disk topic files (so it exercises the writeAtomicIfChanged
// short-circuit path on the second-and-later iterations — this is the
// no-op recompile shape Phase B is expected to dominate).
//
// ADR-020 / ADR-028 budget: Phase B no-op < 500ms for 50 artifacts.
// Phase 7 of PLAN-sidecar-review-fixes #42 — informational; not a PR
// blocker in v0.6.0.
func BenchmarkPhaseBMerge_50Artifacts(b *testing.B) {
	root, pairs := buildPhaseBCorpus(b, 50)
	opts := Options{CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-bench"}

	// Prime the topic files so the timed loop measures the cache-hit path.
	if _, err := Merge(root, pairs, opts); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Merge(root, pairs, opts); err != nil {
			b.Fatal(err)
		}
	}
}

// buildPhaseBCorpus constructs N sidecar.Pair entries spread across 5
// topics, with each parent .md sized to mimic a typical short ADR. The
// corpus stays in-memory after the helper returns; only the topic
// directory accumulates writes during the benchmark loop.
func buildPhaseBCorpus(b *testing.B, n int) (string, []sidecar.Pair) {
	b.Helper()
	root := b.TempDir()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Fatal(err)
	}
	pairs := make([]sidecar.Pair, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("ADR-%03d", i+1)
		base := fmt.Sprintf("%s-bench", id)
		mdPath := filepath.Join(dir, base+".md")
		if err := os.WriteFile(mdPath, []byte("# "+id+" — bench\n"), 0o644); err != nil {
			b.Fatal(err)
		}
		topic := fmt.Sprintf("bench-topic-%d", i%5)
		ycPath := filepath.Join(dir, base+".edikt.yaml")
		if err := os.WriteFile(ycPath, []byte("placeholder\n"), 0o644); err != nil {
			b.Fatal(err)
		}
		sc := &sidecar.Sidecar{
			SchemaVersion: 1,
			Topic:         topic,
			Path:          "docs/architecture/decisions/" + base + ".md",
			Signals:       []string{"bench"},
			Directives: []sidecar.Directive{{
				Text: "Bench directive for " + id + ". (ref: " + id + ")",
				SourceExcerpt: sidecar.SourceExcerpt{
					LineStart: 1, LineEnd: 1, Quote: "# " + id + " — bench",
				},
			}},
			SourcePath: ycPath,
		}
		pairs = append(pairs, sidecar.Pair{
			ParentPath:  mdPath,
			SidecarPath: ycPath,
			ArtifactID:  id,
			Sidecar:     sc,
		})
	}
	return root, pairs
}

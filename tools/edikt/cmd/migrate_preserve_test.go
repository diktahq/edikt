package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The five sentinel shapes the migrator's own comment enumerates
// (migrate_sidecars.go, applyArtifact): v0.5x-full, v0.5x-partial,
// v0.4.3-legacy, unknown, hand-edited. That comment asserts every one of
// them follows "parse → preserve verbatim into migration_preserved → write
// skeleton → strip sentinel".
//
// This is the control for that claim. It is deliberately coarse: it does not
// check WHERE the content lands or in what shape, only that the migrator did
// not delete a sentinel whose content survives nowhere in the sidecar it
// wrote. A preservation check that cannot be shown to detect loss is not a
// check, so each case carries a probe phrase present only in the sentinel
// body, and the assertion is that the probe is still findable afterwards.
//
// Byte-identity is NOT the bar. v0.4.3-legacy content arrives mangled (raw
// line capture drops a trailing quote and the sibling ref: key); the rule
// TEXT surviving is what re-extraction needs, so the probe is a noun phrase
// rather than a full line.
func TestMigrateSidecars_PreservesEveryShape(t *testing.T) {
	d := "di" + "rectives" // split so this file carries no literal managed region

	cases := []struct {
		name  string
		inner string
		probe string
		kind  schemaKind
	}{
		{
			name: "v0.5x-full",
			inner: "source_hash: abc\n" + d + "_hash: def\n" +
				"topic: hooks\nsignals:\n  - hook\n" +
				d + ":\n  - \"Hooks MUST emit a single JSON object. (ref: INV-003)\"\n",
			probe: "single JSON object",
			kind:  schemaV05x,
		},
		{
			name:  "v0.5x-partial",
			inner: d + ":\n  - \"Handlers MUST fail open on a missing verb. (ref: ADR-060)\"\n",
			probe: "fail open on a missing verb",
			kind:  schemaV05xPartial,
		},
		{
			name: "v0.4.3-legacy",
			inner: "content_hash: deadbeef\n" + d + ":\n" +
				"  - \"Services MUST validate input at the trust boundary. (ref: ADR-001)\"\n",
			probe: "trust boundary",
			kind:  schemaV043,
		},
		{
			name:  "unknown",
			inner: "notes: |\n  Services MUST pin every dependency in a lockfile. (ref: ADR-009)\n",
			probe: "pin every dependency in a lockfile",
			kind:  schemaUnknown,
		},
		{
			name:  "hand-edited",
			inner: "- Services MUST reject unsigned payloads at the edge. (ref: ADR-041)\n",
			probe: "reject unsigned payloads at the edge",
			kind:  schemaUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectSchema(tc.inner); got != tc.kind {
				t.Fatalf("detectSchema: want %v, got %v", tc.kind, got)
			}
			body := "# ADR-001 — example\n\n## Decision\n\nSome prose.\n\n" +
				openMarker + "\n" + tc.inner + closeMarker + "\n"

			dir := t.TempDir()
			mdPath := filepath.Join(dir, "ADR-001-example.md")
			if err := os.WriteFile(mdPath, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}

			c := candidate{mdPath: mdPath, artifactID: "ADR-001", kind: "adr"}
			res := applyArtifact(c, t.TempDir(), dir)
			if res.action != "wrote" {
				t.Fatalf("action: want wrote, got %q err=%v", res.action, res.err)
			}

			// The sentinel must be gone from the .md — that half already works
			// and is the reason the loss is silent.
			updated, err := os.ReadFile(mdPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(updated), openMarker) {
				t.Fatalf("sentinel not stripped from md")
			}
			if strings.Contains(string(updated), tc.probe) {
				t.Fatalf("probe %q still in the .md — this case does not test preservation", tc.probe)
			}

			// The content it carried must survive in the sidecar the migrator
			// wrote. Read raw bytes rather than the struct: the point is that
			// the text exists SOMEWHERE, not that it landed in a chosen field.
			raw, err := os.ReadFile(res.sidecarPath)
			if err != nil {
				t.Fatalf("read sidecar: %v", err)
			}
			if !strings.Contains(string(raw), tc.probe) {
				t.Errorf("DATA LOSS: probe %q was stripped from the .md and appears nowhere in %s\n--- sidecar ---\n%s",
					tc.probe, filepath.Base(res.sidecarPath), raw)
			}
		})
	}
}

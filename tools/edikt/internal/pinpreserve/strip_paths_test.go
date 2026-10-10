package pinpreserve

import (
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// TestStripUnapprovedPaths pins the strip's own decision table; phasea's
// TestRestore_DropsExtractorIntroducedPaths pins where restore calls it.
func TestStripUnapprovedPaths(t *testing.T) {
	globs := []string{"tools/edikt/**/*.go"}
	covering := &sidecar.PathsApproval{ApprovedAt: "2026-08-01T00:00:00Z", GlobsSHA256: sidecar.HashGlobs(globs)}
	rule := []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")}

	cases := []struct {
		name        string
		before      *sidecar.Sidecar
		output      *sidecar.Sidecar
		wantDropped bool
		wantReceipt *sidecar.PathsApproval
	}{
		{"no prior sidecar: introduced paths are dropped", nil,
			&sidecar.Sidecar{Directives: rule, Paths: globs}, true, nil},
		{"prior with empty paths: introduced paths are dropped", &sidecar.Sidecar{Directives: rule},
			&sidecar.Sidecar{Directives: rule, Paths: globs}, true, nil},
		{"a receipt first seen in the output is dropped with the globs", &sidecar.Sidecar{Directives: rule},
			&sidecar.Sidecar{Directives: rule, Paths: globs, PathsApproval: covering}, true, nil},
		{"the prior's own covering receipt keeps them", &sidecar.Sidecar{Directives: rule, PathsApproval: covering},
			&sidecar.Sidecar{Directives: rule, Paths: globs, PathsApproval: covering}, false, covering},
		{"a prior that carried paths is never stripped", &sidecar.Sidecar{Directives: rule, Paths: globs},
			&sidecar.Sidecar{Directives: rule, Paths: []string{"docs/**"}}, false, nil},
		{"no paths in the output is a no-op", nil, &sidecar.Sidecar{Directives: rule}, false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ADR-900.edikt.yaml")
			writeSidecar(t, path, c.output)
			outPaths := append([]string(nil), c.output.Paths...)
			res, err := StripUnapprovedPaths(c.before, path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := sidecar.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if c.wantDropped {
				if len(got.Paths) != 0 || len(res.Dropped) != len(outPaths) {
					t.Fatalf("want the introduced paths dropped: on disk %v, reported %v", got.Paths, res.Dropped)
				}
			} else if len(res.Dropped) != 0 || !sameStrings(got.Paths, outPaths) {
				t.Fatalf("want the paths kept: on disk %v, reported dropped %v", got.Paths, res.Dropped)
			}
			switch {
			case c.wantReceipt == nil && got.PathsApproval != nil:
				t.Errorf("receipt %+v survived", got.PathsApproval)
			case c.wantReceipt != nil && (got.PathsApproval == nil || *got.PathsApproval != *c.wantReceipt):
				t.Errorf("receipt %+v, want %+v", got.PathsApproval, c.wantReceipt)
			}
		})
	}
}

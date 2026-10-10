package pinpreserve

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// TestPreserve_PathsProvenance pins what PreservePinned says about the
// artifact-level `paths:` it leaves behind (#11 comment item 4).
//
// Restore carries `paths:` forward whether or not an approval receipt covers
// them — grandfathering is ADR-060's call and it is unchanged here. What
// changed is the CLAIM: before this, every carried glob set was reported as
// "the approved paths", including a set no human ever approved. Provenance is
// read from the FINAL state of the sidecar (the globs on disk against the
// receipt's hash), so it is the same answer whether restore carried the paths
// or the regenerated sidecar already had them.
func TestPreserve_PathsProvenance(t *testing.T) {
	globs := []string{"commands/**/*.md", "tools/edikt/**/*.go"}
	receipt := func(over []string) *sidecar.PathsApproval {
		return &sidecar.PathsApproval{ApprovedAt: "2026-08-01T00:00:00Z", GlobsSHA256: sidecar.HashGlobs(over)}
	}
	regenerated := func(t *testing.T, sc *sidecar.Sidecar) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "ADR-900.edikt.yaml")
		if sc.Directives == nil {
			sc.Directives = []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")}
		}
		writeSidecar(t, path, sc)
		return path
	}
	pathsPin := func(res *PreserveResult) *PinnedField {
		for _, f := range res.Unrestorable {
			if f.Field == "paths" {
				return &f
			}
		}
		return nil
	}

	cases := []struct {
		name      string
		before    *sidecar.Sidecar
		after     *sidecar.Sidecar
		want      PathsProvenance
		restored  bool
		wantLabel string
		banLabel  string
	}{
		{
			name: "receipted paths restored are approved",
			before: &sidecar.Sidecar{
				Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")},
				Paths:      globs, PathsApproval: receipt(globs),
			},
			after:     &sidecar.Sidecar{},
			want:      PathsApproved,
			restored:  true,
			wantLabel: "approved paths",
		},
		{
			name: "unreceipted paths restored are carried but never called approved",
			before: &sidecar.Sidecar{
				Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")},
				Paths:      globs,
			},
			after:     &sidecar.Sidecar{},
			want:      PathsUnreceipted,
			restored:  true,
			wantLabel: "unreceipted",
			banLabel:  "approved paths",
		},
		{
			name: "a receipt whose hash does not cover the restored globs is receipt-stale",
			before: &sidecar.Sidecar{
				Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")},
				Paths:      globs, PathsApproval: receipt([]string{"docs/**"}),
			},
			after:     &sidecar.Sidecar{},
			want:      PathsReceiptStale,
			restored:  true,
			wantLabel: "receipt",
			banLabel:  "approved paths",
		},
		{
			name: "unchanged unreceipted paths still get a provenance",
			before: &sidecar.Sidecar{
				Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")},
				Paths:      globs,
			},
			after: &sidecar.Sidecar{Paths: globs},
			want:  PathsUnreceipted,
		},
		{
			name: "unchanged receipted paths are approved",
			before: &sidecar.Sidecar{
				Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")},
				Paths:      globs, PathsApproval: receipt(globs),
			},
			after: &sidecar.Sidecar{Paths: globs, PathsApproval: receipt(globs)},
			want:  PathsApproved,
		},
		{
			name: "no paths anywhere is no claim",
			before: &sidecar.Sidecar{
				Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")},
			},
			after: &sidecar.Sidecar{},
			want:  "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := regenerated(t, c.after)
			res, err := PreservePinned("ADR-900", c.before, nil, path)
			if err != nil {
				t.Fatal(err)
			}
			if res.PathsProvenance != c.want {
				t.Fatalf("PathsProvenance = %q, want %q", res.PathsProvenance, c.want)
			}
			if res.PathsRestored != c.restored {
				t.Fatalf("PathsRestored = %v, want %v", res.PathsRestored, c.restored)
			}
			label := res.PathsLabel()
			if c.wantLabel != "" && !strings.Contains(label, c.wantLabel) {
				t.Errorf("label %q does not say %q", label, c.wantLabel)
			}
			if c.banLabel != "" && strings.Contains(label, c.banLabel) {
				t.Errorf("label %q says %q for paths no current receipt covers", label, c.banLabel)
			}
			if !c.restored && label != "" {
				t.Errorf("label %q on a result that restored no paths", label)
			}
		})
	}

	// The per-field report restore hands back must make the same distinction:
	// its Reason is what an operator reads in the reinstated list.
	t.Run("the reinstated pin's reason is conditional on a receipt", func(t *testing.T) {
		for _, c := range []struct {
			before  *sidecar.Sidecar
			approve bool
		}{
			{&sidecar.Sidecar{Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")}, Paths: globs}, false},
			{&sidecar.Sidecar{Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")}, Paths: globs, PathsApproval: receipt(globs)}, true},
		} {
			path := regenerated(t, &sidecar.Sidecar{})
			missing, err := MissingPins("ADR-900", c.before, nil, path)
			if err != nil {
				t.Fatal(err)
			}
			var reason string
			for _, f := range missing {
				if f.Field == "paths" {
					reason = f.Reason
				}
			}
			if reason == "" {
				t.Fatalf("no reinstated paths pin reported: %+v", missing)
			}
			if got := strings.Contains(reason, "approved paths"); got != c.approve {
				t.Errorf("receipt=%v: reason %q — 'approved paths' present = %v, want %v", c.approve, reason, got, c.approve)
			}
		}
	})

	// An unparseable snapshot: nothing was read, so nothing can be claimed.
	t.Run("an unparseable prior makes no claim and does not panic", func(t *testing.T) {
		path := regenerated(t, &sidecar.Sidecar{Paths: globs})
		res, err := PreservePinned("ADR-900", nil, errUnparseable(), path)
		if err != nil {
			t.Fatal(err)
		}
		if res.PathsProvenance != "" {
			t.Errorf("PathsProvenance = %q from a prior nobody could read", res.PathsProvenance)
		}
		if res.PathsLabel() != "" {
			t.Errorf("label %q from a prior nobody could read", res.PathsLabel())
		}
		if pathsPin(res) == nil {
			t.Error("the load-error report must still name paths as at risk")
		}
	})
}

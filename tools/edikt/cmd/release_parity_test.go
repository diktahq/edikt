package cmd

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/release"
)

// TestReleasePackageParity pins that this package's thin wrappers around
// internal/release reproduce byte-identical decisions to the
// pre-relocation implementation (SPEC-015 FR-015): installRepoRe,
// cosignIdentityRegexp, resolveLatestTag, and lookupSHA256SUMS lived
// directly in this package before the move to internal/release. That move
// was a package-boundary change, not a rewrite — this test is the
// behavior-preservation check that promise depends on, exercised through
// the exact composition this package's RunE bodies now use
// (release.ValidateRepo(v), release.CosignIdentityRegexp(installRepo()),
// release.ResolveLatestTag(githubAPI()), release.LookupSHA256SUMS).
func TestReleasePackageParity(t *testing.T) {
	t.Run("ValidateRepo matches the old installRepoRe pattern", func(t *testing.T) {
		cases := []struct {
			repo string
			want bool
		}{
			{"diktahq/edikt", true},
			{"diktahq/edikt-dev", true},
			{"diktahq/edikt; rm -rf /", false},
			{"diktahq//edikt", false},
			{"/diktahq/edikt", false},
			{"diktahq/edikt/", false},
		}
		for _, c := range cases {
			if got := release.ValidateRepo(c.repo); got != c.want {
				t.Errorf("release.ValidateRepo(%q) = %v, want %v", c.repo, got, c.want)
			}
		}
	})

	t.Run("CosignIdentityRegexp composes with installRepo() the same as the old cosignIdentityRegexp()", func(t *testing.T) {
		t.Setenv("EDIKT_INSTALL_REPO", "")
		got := release.CosignIdentityRegexp(installRepo())
		want := `^https://github\.com/diktahq/edikt/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$`
		if got != want {
			t.Errorf("release.CosignIdentityRegexp(installRepo()) = %q, want %q", got, want)
		}
	})

	t.Run("CosignIdentityRegexp retargets when EDIKT_INSTALL_REPO overrides installRepo()", func(t *testing.T) {
		t.Setenv("EDIKT_INSTALL_REPO", "diktahq/edikt-dev")
		got := release.CosignIdentityRegexp(installRepo())
		want := `^https://github\.com/diktahq/edikt-dev/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$`
		if got != want {
			t.Errorf("release.CosignIdentityRegexp(installRepo()) = %q, want %q", got, want)
		}
	})

	t.Run("LookupSHA256SUMS unchanged", func(t *testing.T) {
		doc := []byte("aaaa000000000000000000000000000000000000000000000000000000000001  foo.tar.gz\n")
		digest, listed := release.LookupSHA256SUMS(doc, "foo.tar.gz")
		if !listed || digest != "aaaa000000000000000000000000000000000000000000000000000000000001" {
			t.Errorf("release.LookupSHA256SUMS = (%q, %v), want the listed digest", digest, listed)
		}
	})

	t.Run("ResolveLatestTag honors EDIKT_RELEASE_TAG override the same as before, composed with githubAPI()", func(t *testing.T) {
		t.Setenv("EDIKT_INSTALL_REPO", "")
		t.Setenv("EDIKT_RELEASE_TAG", "v9.9.9")
		got, err := release.ResolveLatestTag(githubAPI())
		if err != nil {
			t.Fatalf("release.ResolveLatestTag(githubAPI()) returned an error: %v", err)
		}
		if got != "v9.9.9" {
			t.Errorf("release.ResolveLatestTag(githubAPI()) = %q, want %q", got, "v9.9.9")
		}
	})
}

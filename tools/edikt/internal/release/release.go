// Package release is the single home for the payload/upgrade verification
// chain: validating an EDIKT_INSTALL_REPO override, resolving the latest
// GitHub release tag, building the cosign certificate-identity regex, and
// looking a digest up in a signed SHA256SUMS document.
//
// SPEC-015 FR-015 moved these four functions out of tools/edikt/cmd/upgrade.go
// verbatim (ValidateRepo <- installRepoRe, ResolveLatestTag <- resolveLatestTag,
// CosignIdentityRegexp <- cosignIdentityRegexp, LookupSHA256SUMS <-
// lookupSHA256SUMS) — a package-boundary move, not a rewrite, made so this
// chain can be imported and its behavior asserted from a test without
// spinning up a cobra command. tools/edikt/cmd/install.go and
// tools/edikt/cmd/upgrade.go are the thin RunE callers into this package now;
// no behavior changed in the move.
//
// This package deliberately does NOT read EDIKT_INSTALL_REPO (or compose the
// download/API URLs) itself — installRepo()/releaseBase()/githubAPI() stay in
// cmd/upgrade.go, since they are shared by more than the four functions that
// moved. Callers pass the repo string or the composed URL in, keeping this
// package a pure verification-chain library.
//
// The bootstrap residue documented in SPEC-015 — install.sh's validate_repo(),
// validate_tag(), resolve_tag(), _cosign_verify_release_checksums(), and
// _verify_launcher_checksum() — stays in shell, on purpose: install.sh
// composes the launcher's own download URL and cosign identity before
// bin/edikt exists anywhere on disk, so there is nothing yet to delegate to.
// This package is not a rewrite of that shell logic; it is the payload/
// upgrade chain, which was Go-only before this move and remains Go-only
// after it. SAC-001's shared fixture (test/fixtures/install-verification-parity/)
// is what keeps the two behaviorally aligned going forward, tested from both
// sides rather than synced by a comment.
package release

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// repoRe validates an EDIKT_INSTALL_REPO-shaped value before it composes a
// URL or the cosign identity regex — an unvalidated value could both build a
// bad URL and widen the identity check to match more than intended.
// Restricted to letters, digits, '.', '_', '-' on both sides of exactly one
// '/'. Mirrors install.sh's own EDIKT_INSTALL_REPO validation exactly
// (F-090); a real regexp here, not a shell glob, so the bracket-class-plus-
// star gotcha that made install.sh's first attempt at this exploitable (glob
// '*' doesn't quantify the preceding class) doesn't apply to this
// implementation.
var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// ValidateRepo reports whether repo is a well-formed <owner>/<repo> value —
// the shape GitHub itself requires and the only shape installRepo()'s caller
// may retarget the download URL and cosign identity to. Moved verbatim from
// upgrade.go's installRepoRe.MatchString.
func ValidateRepo(repo string) bool {
	return repoRe.MatchString(repo)
}

// CosignIdentityRegexp is the expected certificate identity for release
// signing, derived from the same repo value the caller used to compose the
// download URL and GitHub API URL — retargeting the download URL without
// also retargeting this regex would download from one repo's release and
// verify against a different repo's identity, which can only ever fail.
// '.' is the one regex metacharacter a GitHub owner/repo name may contain
// (ValidateRepo restricts it to [A-Za-z0-9_.-]); escaped so a literal '.'
// doesn't widen the match to "any character".
//
// The trailing `v[0-9]+\.[0-9]+\.[0-9]+$` is INV-008's tag-pinning
// enforcement point: the OIDC certificate identity this regex is matched
// against encodes the git ref the release workflow ran from
// (`@refs/tags/vX.Y.Z`), so a workflow run triggered from a branch push
// (`@refs/heads/main`) or any non-semver ref never satisfies it.
func CosignIdentityRegexp(repo string) string {
	escaped := strings.ReplaceAll(repo, ".", `\.`)
	return `^https://github\.com/` + escaped + `/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$`
}

// ResolveLatestTag fetches the latest GitHub release tag from apiURL.
// Respects EDIKT_RELEASE_TAG env override for testing/offline use.
//
// apiURL is caller-composed (cmd/upgrade.go's githubAPI(), itself derived
// from installRepo()) rather than built here — this package validates and
// verifies a repo value, it does not decide which repo is active.
func ResolveLatestTag(apiURL string) (string, error) {
	if override := os.Getenv("EDIKT_RELEASE_TAG"); override != "" {
		return override, nil
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("fetching GitHub API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	// Extract tag_name from JSON without requiring encoding/json (keeps things
	// lean and avoids struct dependency on GitHub API shape).
	tag := extractJSONString(string(body), "tag_name")
	if tag == "" {
		return "", fmt.Errorf("could not parse tag_name from GitHub API response")
	}

	// Sanity check.
	if !isSemverTag(tag) {
		return "", fmt.Errorf("extracted tag does not look like semver: %s", tag)
	}

	return tag, nil
}

// extractJSONString finds the first value for `"key": "value"` in a JSON string.
// This is intentionally simple — we only need a single well-known field.
func extractJSONString(json, key string) string {
	needle := `"` + key + `"`
	idx := strings.Index(json, needle)
	if idx < 0 {
		return ""
	}
	rest := json[idx+len(needle):]
	// Find the colon.
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return ""
	}
	rest = strings.TrimSpace(rest[colon+1:])
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func isSemverTag(tag string) bool {
	t := strings.TrimPrefix(tag, "v")
	parts := strings.SplitN(t, ".", 3)
	if len(parts) < 3 {
		return false
	}
	for _, p := range parts[:2] {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// LookupSHA256SUMS returns the digest a SHA256SUMS document records for name.
//
// listed distinguishes "the document does not cover this asset" from "the
// document covers it and the digest differs" — the caller must treat those
// differently, because only the first is a check that could not run. Folding
// them together is how F-042 shipped: an absent integrity source presented as
// an ordinary error and remedied with a flag that disabled signing too.
//
// Format is coreutils sha256sum: "<hex>  <name>", where a leading "*" on the
// name marks binary mode.
func LookupSHA256SUMS(doc []byte, name string) (digest string, listed bool) {
	for _, line := range strings.Split(string(doc), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		entry := strings.TrimPrefix(fields[1], "*")
		if entry == name {
			return fields[0], true
		}
	}
	return "", false
}

package cmd

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const defaultInstallRepo = "diktahq/edikt"

// installRepoRe validates EDIKT_INSTALL_REPO before it composes a URL or the
// cosign identity regex — an unvalidated value could both build a bad URL
// and widen the identity check to match more than intended. Restricted to
// letters, digits, '.', '_', '-' on both sides of exactly one '/'. Mirrors
// install.sh's own EDIKT_INSTALL_REPO validation exactly (F-090); a real
// regexp here, not a shell glob, so the bracket-class-plus-star gotcha that
// made install.sh's first attempt at this exploitable (glob '*' doesn't
// quantify the preceding class) doesn't apply to this implementation.
var installRepoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// installRepo resolves EDIKT_INSTALL_REPO — same variable name and same
// contract as install.sh's own override (F-090), so a maintainer testing a
// staging release candidate sets one env var for either entry point. It
// RETARGETS both the download URL and the cosign identity check together —
// never disables verification, unlike EDIKT_INSTALL_INSECURE=1. Warns once,
// to stderr, when set; the warning belongs to the caller (RunE), not this
// helper, so it fires exactly once per invocation rather than once per call
// site.
func installRepo() string {
	if v := os.Getenv("EDIKT_INSTALL_REPO"); v != "" {
		return v
	}
	return defaultInstallRepo
}

func releaseBase() string {
	return "https://github.com/" + installRepo() + "/releases/download"
}

func githubAPI() string {
	return "https://api.github.com/repos/" + installRepo() + "/releases/latest"
}

// cosignIdentityRegexp is the expected certificate identity for release
// signing, derived from the same installRepo() value releaseBase() and
// githubAPI() use — retargeting the download URL without also retargeting
// this regex would download from one repo's release and verify against a
// different repo's identity, which can only ever fail. '.' is the one
// regex metacharacter a GitHub owner/repo name may contain (installRepoRe
// restricts it to [A-Za-z0-9_.-]); escaped so a literal '.' doesn't widen
// the match to "any character".
func cosignIdentityRegexp() string {
	escaped := strings.ReplaceAll(installRepo(), ".", `\.`)
	return `^https://github\.com/` + escaped + `/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$`
}

const cosignOIDCIssuer = "https://token.actions.githubusercontent.com"

var upgradeYes bool
var upgradeDryRun bool

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Fetch and install the latest stable release from GitHub",
	Long: `Resolves the latest release tag from the GitHub API, compares with the
currently active version, and if newer: downloads the release tarball,
verifies cosign signature (unless EDIKT_INSTALL_INSECURE=1), extracts,
and activates the new version.

If already up-to-date, reports so and exits 0.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if v := os.Getenv("EDIKT_INSTALL_REPO"); v != "" {
			if !installRepoRe.MatchString(v) {
				return &exitCodeError{code: 3, msg: fmt.Sprintf(
					"EDIKT_INSTALL_REPO must match <owner>/<repo> using only letters, digits, '.', '_', '-' (got: %s)", v)}
			}
			fmt.Fprintf(os.Stderr, "warn: EDIKT_INSTALL_REPO override active: %s\n", v)
			fmt.Fprintf(os.Stderr, "warn: verification is RETARGETED to this repo's signing identity, not disabled — the opposite of EDIKT_INSTALL_INSECURE=1\n")
		}

		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}

		// ASSERT what is installed; do not infer it from a version string.
		//
		// A dev-linked install has current -> versions/dev, a symlink into a
		// working tree. Its "version" parses as 0.0.0, so every published
		// release compares greater and this command would fetch a stable
		// tarball and flip `current` away from the developer's tree — a
		// silent downgrade of a working install. It 404s harmlessly today
		// only because the asset happens not to exist at the computed URL;
		// that is luck, not a guard.
		//
		// The version string is a MODEL of what is installed. The symlink is
		// the fact (INV-014). Read the fact.
		if devLinked, target := isDevLinked(ediktRoot); devLinked {
			return fmt.Errorf("this install is dev-linked (current -> %s).\n"+
				"Refusing to upgrade: fetching a release would replace your working tree link.\n"+
				"To leave dev mode first: edikt use <tag>", target)
		}

		lf, _ := readLock(ediktRoot)
		currentV := normalizeTag(lf.Active)
		if currentV == "" {
			// Try VERSION file.
			if data, err := os.ReadFile(filepath.Join(ediktRoot, "current", "VERSION")); err == nil {
				currentV = normalizeTag(strings.TrimSpace(string(data)))
			}
		}
		if currentV == "" {
			return fmt.Errorf("no active version found — bootstrap via /edikt:upgrade in Claude Code (primary path), or run 'edikt install <tag>' directly")
		}

		// Resolve latest tag.
		latestTag, err := resolveLatestTag()
		if err != nil {
			return fmt.Errorf("resolving latest tag: %w", err)
		}
		latestV := normalizeTag(latestTag)

		// Already up to date?
		if !semverGreater(latestV, currentV) {
			fmt.Fprintf(os.Stderr, "upgrade: already up to date (v%s)\n", currentV)
			return nil
		}

		// Reject cross-major upgrades.
		curMajor := semverMajor(currentV)
		latMajor := semverMajor(latestV)
		if latMajor != curMajor {
			return fmt.Errorf("major upgrade detected (current v%s, latest v%s) — run: curl -fsSL %s/v%s/install.sh | bash",
				currentV, latestV, releaseBase(), latestV)
		}

		fmt.Printf("upgrade: v%s → v%s\n", currentV, latestV)

		if upgradeDryRun {
			fmt.Printf("(dry-run: would install %s and activate it)\n", latestTag)
			return nil
		}

		// Check if already installed.
		targetDir := filepath.Join(ediktRoot, "versions", latestV)
		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "upgrade: installing %s ...\n", latestTag)
			// EDIKT_INSTALL_SOURCE: bypass network fetch with local source.
			if src := os.Getenv("EDIKT_INSTALL_SOURCE"); src != "" {
				isTarball := strings.HasSuffix(src, ".tar.gz") || strings.HasSuffix(src, ".tgz")
				if err := localInstallFromSource(ediktRoot, targetDir, src, isTarball); err != nil {
					return fmt.Errorf("install failed: %w", err)
				}
			} else if err := downloadAndInstall(ediktRoot, latestTag, latestV); err != nil {
				return fmt.Errorf("install failed: %w", err)
			}
			stampPayloadVersion(targetDir, latestV)
			emitEvent(ediktRoot, "version_installed", map[string]interface{}{"version": latestV})
		} else {
			fmt.Fprintf(os.Stderr, "upgrade: %s already installed, skipping fetch\n", latestV)
		}

		// Prompt unless --yes.
		if !upgradeYes {
			fmt.Printf("Activate v%s? [y/N]: ", latestV)
			var reply string
			fmt.Scanln(&reply)
			switch strings.ToLower(strings.TrimSpace(reply)) {
			case "y", "yes":
			default:
				fmt.Fprintln(os.Stderr, "aborted")
				return nil
			}
		}

		// Activate.
		currentLink := filepath.Join(ediktRoot, "current")
		newLink := currentLink + fmt.Sprintf(".new.%d", os.Getpid())
		os.Remove(newLink)
		relTarget := filepath.Join("versions", latestV)
		if err := os.Symlink(relTarget, newLink); err != nil {
			return fmt.Errorf("creating symlink: %w", err)
		}
		if err := os.Rename(newLink, currentLink); err != nil {
			os.Remove(newLink)
			return fmt.Errorf("flipping current: %w", err)
		}

		if err := writeLock(ediktRoot, latestV, "launcher"); err != nil {
			fmt.Fprintf(os.Stderr, "warn: activated but lock.yaml update failed: %v\n", err)
		}

		emitEvent(ediktRoot, "version_activated", map[string]interface{}{"version": latestV})
		fmt.Fprintf(os.Stderr, "upgrade complete: v%s → v%s\n", currentV, latestV)
		return nil
	},
}

// resolveLatestTag fetches the latest GitHub release tag.
// Respects EDIKT_RELEASE_TAG env override for testing/offline use.
func resolveLatestTag() (string, error) {
	if override := os.Getenv("EDIKT_RELEASE_TAG"); override != "" {
		return override, nil
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(githubAPI())
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

// semverGreater returns true if a > b (simple semver comparison, no pre-release).
func semverGreater(a, b string) bool {
	aParts := semverParts(a)
	bParts := semverParts(b)
	for i := 0; i < 3; i++ {
		if aParts[i] > bParts[i] {
			return true
		}
		if aParts[i] < bParts[i] {
			return false
		}
	}
	return false
}

func semverMajor(v string) int {
	parts := semverParts(v)
	return parts[0]
}

func semverParts(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		// Parse numeric prefix only.
		for j, c := range p {
			if c < '0' || c > '9' {
				p = p[:j]
				break
			}
		}
		n := 0
		for _, c := range p {
			n = n*10 + int(c-'0')
		}
		result[i] = n
	}
	return result
}

// checkTarGzSafety scans all tar headers for path traversal without extracting.
// Returns an error if any entry would escape the destination directory.
func checkTarGzSafety(src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if filepath.IsAbs(hdr.Name) {
			return fmt.Errorf("tarball contains absolute path: %s", hdr.Name)
		}
		if strings.HasPrefix(filepath.Clean(hdr.Name), "..") {
			return fmt.Errorf("tarball contains path traversal: %s", hdr.Name)
		}
		if hdr.Typeflag == tar.TypeSymlink {
			if filepath.IsAbs(hdr.Linkname) {
				return fmt.Errorf("tarball contains absolute symlink target: %s -> %s", hdr.Name, hdr.Linkname)
			}
			if strings.HasPrefix(filepath.Clean(hdr.Linkname), "..") {
				return fmt.Errorf("tarball contains symlink target traversal: %s -> %s", hdr.Name, hdr.Linkname)
			}
		}
	}
	return nil
}

// localInstallFromSource copies or extracts a local directory/tarball into
// targetDir. Used by both install and upgrade commands when EDIKT_INSTALL_SOURCE
// is set to bypass the network fetch.
func localInstallFromSource(ediktRoot, targetDir, source string, isTarball bool) error {
	if err := os.MkdirAll(filepath.Join(ediktRoot, "versions"), 0o755); err != nil {
		return fmt.Errorf("creating versions dir: %w", err)
	}
	if isTarball {
		stagingDir := filepath.Join(ediktRoot, fmt.Sprintf(".staging-%d", os.Getpid()))
		if err := os.MkdirAll(stagingDir, 0o755); err != nil {
			return fmt.Errorf("creating staging dir: %w", err)
		}
		defer os.RemoveAll(stagingDir)
		if err := extractTarGz(source, stagingDir); err != nil {
			return fmt.Errorf("extracting tarball: %w", err)
		}
		payloadSrc := stagingDir
		if entries, err := os.ReadDir(stagingDir); err == nil && len(entries) == 1 && entries[0].IsDir() {
			payloadSrc = filepath.Join(stagingDir, entries[0].Name())
		}
		if err := os.Rename(payloadSrc, targetDir); err != nil {
			if err2 := copyDir(payloadSrc, targetDir); err2 != nil {
				return fmt.Errorf("installing: %w", err2)
			}
		}
	} else {
		if err := copyDir(source, targetDir); err != nil {
			return fmt.Errorf("copying source: %w", err)
		}
	}
	return nil
}

// downloadAndInstall fetches the release tarball for tag, verifies its
// checksum (and optionally cosign signature), extracts it into
// $EDIKT_ROOT/versions/<norm>, and writes a minimal manifest.
func downloadAndInstall(ediktRoot, tag, norm string) error {
	insecure := os.Getenv("EDIKT_INSTALL_INSECURE") == "1"
	url := fmt.Sprintf("%s/%s/edikt-payload-%s.tar.gz", releaseBase(), tag, tag)

	// Create a staging directory.
	stagingDir := filepath.Join(ediktRoot, fmt.Sprintf(".staging-%d", os.Getpid()))
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("creating staging dir: %w", err)
	}
	cleanup := func() { os.RemoveAll(stagingDir) }

	// Download tarball.
	tarballPath := filepath.Join(stagingDir, "payload.tar.gz")
	fmt.Fprintf(os.Stderr, "  fetching %s\n", url)
	if err := httpDownload(url, tarballPath); err != nil {
		cleanup()
		return fmt.Errorf("downloading tarball: %w", err)
	}

	// Compute SHA-256.
	observed, err := sha256File(tarballPath)
	if err != nil {
		cleanup()
		return fmt.Errorf("computing checksum: %w", err)
	}

	// ── PAYLOAD INTEGRITY (F-042) ───────────────────────────────────────────
	//
	// This bound the payload's integrity to an UNSIGNED `<url>.sha256` sidecar
	// that release.yml has never published, so the default path 404'd into an
	// error recommending EDIKT_INSTALL_INSECURE=1 — which ALSO disabled cosign.
	// The signed document was fetched, cosign-verified, and then discarded
	// without ever being read. ADR-016 requires the opposite: "the launcher  edikt-guard:allow
	// MUST grep SHA256SUMS for the payload filename and assert the SHA matches
	// the downloaded bytes". install.sh:530 already did this correctly; the Go
	// launcher it delegates to did not.
	//
	// Order is the fix, not just the lookup: SHA256SUMS is cosign-verified
	// FIRST, and the payload digest is then read out of the document that
	// verification covered. Reading it before verifying would authenticate
	// nothing.
	payloadName := fmt.Sprintf("edikt-payload-%s.tar.gz", tag)

	if envSHA := os.Getenv("EDIKT_INSTALL_SHA256"); envSHA != "" {
		// An explicit operator-supplied pin outranks the release document.
		if observed != envSHA {
			cleanup()
			return fmt.Errorf("checksum mismatch: expected %s, got %s", envSHA, observed)
		}
		fmt.Fprintf(os.Stderr, "  checksum: matches EDIKT_INSTALL_SHA256\n")
	} else {
		sumPath := filepath.Join(stagingDir, "SHA256SUMS")
		sigPath := filepath.Join(stagingDir, "SHA256SUMS.sig.bundle")
		sumURL := fmt.Sprintf("%s/%s/SHA256SUMS", releaseBase(), tag)
		sigURL := fmt.Sprintf("%s/%s/SHA256SUMS.sig.bundle", releaseBase(), tag)

		// verified says the signed document is trustworthy and present. Only
		// then is its content usable as an integrity source.
		verified := false

		cosignPath, cosignErr := exec.LookPath("cosign")
		switch {
		case cosignErr != nil:
			// COULD NOT RUN. Downgradable by the insecure flag.
			if !insecure {
				cleanup()
				return fmt.Errorf("cosign not found — refusing install without signature verification.\nInstall cosign or set EDIKT_INSTALL_INSECURE=1")
			}
			fmt.Fprintf(os.Stderr, "  WARN: cosign not on PATH, EDIKT_INSTALL_INSECURE=1 — skipping signature verification\n")
		default:
			if err := httpDownload(sumURL, sumPath); err != nil {
				// COULD NOT RUN.
				if !insecure {
					cleanup()
					return fmt.Errorf("cosign present but could not fetch SHA256SUMS (%s) — refusing install; set EDIKT_INSTALL_INSECURE=1 to skip", sumURL)
				}
				fmt.Fprintf(os.Stderr, "  WARN: could not fetch SHA256SUMS, EDIKT_INSTALL_INSECURE=1 — skipping signature verification\n")
			} else if err := httpDownload(sigURL, sigPath); err != nil {
				// COULD NOT RUN.
				if !insecure {
					cleanup()
					return fmt.Errorf("cosign present but could not fetch SHA256SUMS.sig.bundle (%s) — refusing install; set EDIKT_INSTALL_INSECURE=1 to skip", sigURL)
				}
				fmt.Fprintf(os.Stderr, "  WARN: could not fetch signature bundle, EDIKT_INSTALL_INSECURE=1 — skipping signature verification\n")
			} else {
				verifyCmd := exec.Command(cosignPath,
					"verify-blob",
					"--bundle", sigPath,
					"--certificate-identity-regexp", cosignIdentityRegexp(),
					"--certificate-oidc-issuer", cosignOIDCIssuer,
					sumPath,
				)
				verifyCmd.Stdout = os.Stderr
				verifyCmd.Stderr = os.Stderr
				if err := verifyCmd.Run(); err != nil {
					// VERIFICATION RAN AND FAILED. This is a signature that
					// did not match — tampering, or a document from another
					// signer. EDIKT_INSTALL_INSECURE MUST NOT downgrade this:
					// the flag means "I accept not being able to check", never
					// "I accept a check that came back bad".
					cleanup()
					return fmt.Errorf("cosign verification FAILED for %s — refusing install.\n"+
						"This is a failed signature, not a missing one; EDIKT_INSTALL_INSECURE does not override it", sumURL)
				}
				fmt.Fprintf(os.Stderr, "  cosign: signature verified\n")
				verified = true
			}
		}

		if verified {
			doc, rerr := os.ReadFile(sumPath)
			if rerr != nil {
				cleanup()
				return fmt.Errorf("reading verified SHA256SUMS: %w", rerr)
			}
			expected, listed := lookupSHA256SUMS(doc, payloadName)
			switch {
			case !listed:
				// COULD NOT CHECK: the signed document does not cover this
				// asset. ADR-016 requires it to cover every asset, so this is  edikt-guard:allow
				// an anomaly — but it is an absence, not a mismatch, so the
				// insecure flag may downgrade it. Matches install.sh:531.
				if !insecure {
					cleanup()
					return fmt.Errorf("SHA256SUMS for %s does not list %s — cannot verify payload integrity.\n"+
						"set EDIKT_INSTALL_INSECURE=1 to bypass (NOT recommended)", tag, payloadName)
				}
				fmt.Fprintf(os.Stderr, "  WARN: %s absent from SHA256SUMS, EDIKT_INSTALL_INSECURE=1 — payload UNVERIFIED\n", payloadName)
				fmt.Fprintf(os.Stderr, "  tarball sha256: %s\n", observed)
			case !strings.EqualFold(expected, observed):
				// CHECK RAN AND FAILED. Never downgradable, for the same
				// reason as a failed signature.
				cleanup()
				return fmt.Errorf("payload checksum MISMATCH for %s\n  expected (signed SHA256SUMS): %s\n  observed (downloaded bytes):  %s\n"+
					"refusing install; EDIKT_INSTALL_INSECURE does not override a failed checksum", payloadName, expected, observed)
			default:
				fmt.Fprintf(os.Stderr, "  checksum: %s verified against signed SHA256SUMS\n", payloadName)
			}
		} else {
			// Signature verification was skipped under the insecure flag, so
			// SHA256SUMS carries no authority and is not consulted.
			fmt.Fprintf(os.Stderr, "  WARN: payload integrity NOT verified (TLS-only trust)\n")
			fmt.Fprintf(os.Stderr, "  tarball sha256: %s\n", observed)
		}
	}

	// Extract tarball.
	extractDir := filepath.Join(stagingDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		cleanup()
		return fmt.Errorf("creating extract dir: %w", err)
	}
	if err := extractTarGz(tarballPath, extractDir); err != nil {
		cleanup()
		return fmt.Errorf("extracting tarball: %w", err)
	}

	// Find the payload directory (may be wrapped in a top-level dir).
	payloadSrc := extractDir
	entries, err := os.ReadDir(extractDir)
	if err == nil && len(entries) == 1 && entries[0].IsDir() {
		payloadSrc = filepath.Join(extractDir, entries[0].Name())
	}

	// ── HOOK SYNTAX GATE (F-036 / F-051) ────────────────────────────────────
	//
	// A hook whose bash does not parse exits 2 before doing anything, and 2 is
	// the harness's BLOCKING code — so a broken hook denies every governed
	// write rather than failing open.
	//
	// The shims carry in-script `_allow` fail-open branches for exactly this,
	// and F-051 recorded them as "unreachable by construction". MEASURED, that
	// is not right, and the real shape is worse than either reading:
	//
	//   syntax error AFTER the _allow branch  -> _allow runs, {"continue":true}
	//   syntax error BEFORE the _allow branch -> exit 2, nothing runs, BLOCKED
	//
	// bash reads and executes a script incrementally, so an in-script fail-open
	// covers only errors positioned below it. That is a guarantee nobody can
	// rely on: it silently evaporates the moment an edit lands above the
	// branch, and nothing reports the change in coverage.
	//
	// So the check has to live OUTSIDE the script, and this is the outside.
	// Refusing here is safe in the direction that matters — the user keeps the
	// version they already had, rather than activating one that would block
	// every write.
	if err := checkHookSyntax(payloadSrc); err != nil {
		cleanup()
		return fmt.Errorf("refusing to activate %s: %w", tag, err)
	}

	// Move into versions/<norm>/.
	if err := os.MkdirAll(filepath.Join(ediktRoot, "versions"), 0o755); err != nil {
		cleanup()
		return fmt.Errorf("creating versions dir: %w", err)
	}
	targetDir := filepath.Join(ediktRoot, "versions", norm)
	if err := os.Rename(payloadSrc, targetDir); err != nil {
		// Rename across filesystems may fail — fall back to copy.
		if err2 := copyDir(payloadSrc, targetDir); err2 != nil {
			cleanup()
			return fmt.Errorf("installing version dir: %w", err2)
		}
	}

	cleanup()
	fmt.Fprintf(os.Stderr, "installed %s at %s\n", norm, targetDir)
	return nil
}

// httpDownload downloads url to destPath, returning an error if the HTTP
// status is not 2xx or if any I/O error occurs.
func httpDownload(url, destPath string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// checkHookSyntax runs `bash -n` over every hook script in an extracted
// payload and returns an error naming the first that does not parse.
//
// Scope is stated rather than implied (INV-013): this proves the scripts  edikt-guard:allow
// PARSE. It says nothing about whether they behave correctly, and a payload
// that passes here can still contain a hook that is wrong in every other way.
// What it removes is the one failure mode an in-script guard cannot cover —
// a parse error positioned above the fail-open branch, which exits 2 and
// therefore BLOCKS every governed write.
//
// If bash is absent the check reports that it could not run rather than
// passing. An unverifiable payload is not a verified one, and this function's
// caller refuses on error, keeping the user on the version they already have.
func checkHookSyntax(payloadRoot string) error {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		return fmt.Errorf("bash not found, so installed hooks could not be syntax-checked (a hook that fails to parse exits 2, which BLOCKS every governed write)")
	}

	var scripts []string
	walkErr := filepath.Walk(payloadRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".sh") {
			return nil
		}
		// Hook scripts only: any .sh under a directory named "hooks".
		if strings.Contains(filepath.ToSlash(path), "/hooks/") {
			scripts = append(scripts, path)
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("scanning payload for hook scripts: %w", walkErr)
	}

	// A payload with no hooks at all is UNMEASURED, not clean. Reporting
	// "0 hooks checked, all fine" is the absence-as-pass shape INV-013 exists  edikt-guard:allow
	// to forbid — and it is exactly how a payload that silently stopped
	// shipping hooks would sail through this gate.
	if len(scripts) == 0 {
		return fmt.Errorf("payload contains no hook scripts under a hooks/ directory — refusing rather than reporting an unmeasured payload as checked")
	}

	for _, s := range scripts {
		out, err := exec.Command(bashPath, "-n", s).CombinedOutput()
		if err != nil {
			rel, rerr := filepath.Rel(payloadRoot, s)
			if rerr != nil {
				rel = s
			}
			return fmt.Errorf("hook %s fails `bash -n`, and a hook that does not parse exits 2 — the harness's BLOCKING code — so it would deny every governed write:\n%s",
				rel, strings.TrimSpace(string(out)))
		}
	}
	fmt.Fprintf(os.Stderr, "  hooks: %d scripts pass `bash -n`\n", len(scripts))
	return nil
}

// lookupSHA256SUMS returns the digest a SHA256SUMS document records for name.
//
// listed distinguishes "the document does not cover this asset" from "the
// document covers it and the digest differs" — the caller must treat those
// differently, because only the first is a check that could not run. Folding
// them together is how F-042 shipped: an absent integrity source presented as
// an ordinary error and remedied with a flag that disabled signing too.
//
// Format is coreutils sha256sum: "<hex>  <name>", where a leading "*" on the
// name marks binary mode. Matches the awk in install.sh:530 so the two
// launchers cannot drift on what counts as a match.
func lookupSHA256SUMS(doc []byte, name string) (digest string, listed bool) {
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

// sha256File returns the hex-encoded SHA-256 digest of a file.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractTarGz extracts a .tar.gz archive to destDir.
// Path-traversal guard: rejects absolute paths and ".." components.
func extractTarGz(src, destDir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Safety: reject absolute paths and traversal.
		if filepath.IsAbs(hdr.Name) {
			return fmt.Errorf("tarball contains absolute path: %s", hdr.Name)
		}
		cleaned := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleaned, "..") {
			return fmt.Errorf("tarball contains path traversal: %s", hdr.Name)
		}

		// Skip macOS metadata. AppleDouble (`._<name>`) and `.DS_Store`
		// entries are not part of the canonical payload — they're added
		// by macOS BSD tar when the source tree carries extended
		// attributes. Letting them extract pollutes ~/.edikt/ with
		// phantom files that Claude Code enumerates as broken slash
		// commands. The filter is unconditional: legitimate edikt
		// payloads never carry these names.
		base := filepath.Base(cleaned)
		if strings.HasPrefix(base, "._") || base == ".DS_Store" {
			if hdr.Typeflag == tar.TypeReg {
				if _, err := io.Copy(io.Discard, tr); err != nil {
					return err
				}
			}
			continue
		}

		dest := filepath.Join(destDir, cleaned)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			// #nosec G115 -- hdr.Mode is masked with &0o777 (9 bits), so the int64->FileMode(uint32) conversion cannot overflow.
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			// Safety: reject absolute symlink targets and path traversal in link target.
			if filepath.IsAbs(hdr.Linkname) {
				return fmt.Errorf("tarball contains absolute symlink target: %s -> %s", hdr.Name, hdr.Linkname)
			}
			cleanedLink := filepath.Clean(hdr.Linkname)
			if strings.HasPrefix(cleanedLink, "..") {
				return fmt.Errorf("tarball contains symlink target traversal: %s -> %s", hdr.Name, hdr.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			os.Remove(dest)
			if err := os.Symlink(hdr.Linkname, dest); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func init() {
	upgradeCmd.Flags().BoolVarP(&upgradeYes, "yes", "y", false, "skip confirmation before activating")
	upgradeCmd.Flags().BoolVar(&upgradeDryRun, "dry-run", false, "show what would be done without making changes")
	rootCmd.AddCommand(upgradeCmd)
}

// isDevLinked reports whether $EDIKT_ROOT/current resolves into versions/dev —
// the shape `edikt dev link` creates. Asserted from the filesystem rather than
// inferred from a recorded version, because the recorded version of a dev
// install parses as 0.0.0 and compares less than every release.
func isDevLinked(ediktRoot string) (bool, string) {
	current := filepath.Join(ediktRoot, "current")

	// The explicit marker first. `edikt dev link` writes DEV_SOURCE into the
	// dev directory, so its presence is the fact rather than an inference
	// from layout — and it survives a layout change that would break the
	// symlink-shape reading below.
	if _, err := os.Stat(filepath.Join(current, "DEV_SOURCE")); err == nil {
		if t, lerr := os.Readlink(current); lerr == nil {
			return true, t
		}
		return true, current
	}

	target, err := os.Readlink(current)
	if err != nil {
		return false, ""
	}
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(current), target)
	}
	if filepath.Base(filepath.Clean(resolved)) == "dev" {
		return true, target
	}
	// A dev link may itself point at a symlink into a working tree; resolve
	// once more before concluding it is a normal versioned install.
	if real, err := filepath.EvalSymlinks(resolved); err == nil {
		if filepath.Base(filepath.Clean(real)) == "dev" {
			return true, target
		}
	}
	return false, ""
}

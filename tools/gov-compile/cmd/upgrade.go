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
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	releaseBase = "https://github.com/diktahq/edikt/releases/download"
	githubAPI   = "https://api.github.com/repos/diktahq/edikt/releases/latest"
)

// cosignIdentityRegexp is the expected certificate identity for release signing.
const cosignIdentityRegexp = `^https://github\.com/diktahq/edikt/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$`
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
		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
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
			return fmt.Errorf("no active version found. Run 'edikt install <tag>' first.")
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
				currentV, latestV, releaseBase, latestV)
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
			if err := downloadAndInstall(ediktRoot, latestTag, latestV); err != nil {
				return fmt.Errorf("install failed: %w", err)
			}
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
	resp, err := client.Get(githubAPI)
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

// downloadAndInstall fetches the release tarball for tag, verifies its
// checksum (and optionally cosign signature), extracts it into
// $EDIKT_ROOT/versions/<norm>, and writes a minimal manifest.
func downloadAndInstall(ediktRoot, tag, norm string) error {
	insecure := os.Getenv("EDIKT_INSTALL_INSECURE") == "1"
	url := fmt.Sprintf("%s/%s/edikt-payload-%s.tar.gz", releaseBase, tag, tag)

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

	// Verify checksum.
	if envSHA := os.Getenv("EDIKT_INSTALL_SHA256"); envSHA != "" {
		if observed != envSHA {
			cleanup()
			return fmt.Errorf("checksum mismatch: expected %s, got %s", envSHA, observed)
		}
	} else {
		// Try fetching the .sha256 sidecar.
		sidecarURL := url + ".sha256"
		sidecarPath := filepath.Join(stagingDir, "payload.tar.gz.sha256")
		if err := httpDownload(sidecarURL, sidecarPath); err == nil {
			// Verify against sidecar.
			refData, err := os.ReadFile(sidecarPath)
			if err == nil {
				expected := strings.TrimSpace(strings.Fields(string(refData))[0])
				if observed != expected {
					cleanup()
					return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, observed)
				}
			}
		} else if insecure {
			fmt.Fprintf(os.Stderr, "  WARN: EDIKT_INSTALL_INSECURE=1 — skipping checksum (TLS-only trust)\n")
			fmt.Fprintf(os.Stderr, "  tarball sha256: %s\n", observed)
		} else {
			cleanup()
			return fmt.Errorf("no checksum reference available and EDIKT_INSTALL_SHA256 not set.\nSet EDIKT_INSTALL_INSECURE=1 to override (not recommended).")
		}
	}

	// Cosign verification.
	if cosignPath, err := exec.LookPath("cosign"); err == nil {
		// Download SHA256SUMS and SHA256SUMS.sig.bundle for cosign verify.
		sumURL := fmt.Sprintf("%s/%s/SHA256SUMS", releaseBase, tag)
		sigURL := fmt.Sprintf("%s/%s/SHA256SUMS.sig.bundle", releaseBase, tag)
		sumPath := filepath.Join(stagingDir, "SHA256SUMS")
		sigPath := filepath.Join(stagingDir, "SHA256SUMS.sig.bundle")

		if err := httpDownload(sumURL, sumPath); err == nil {
			if err := httpDownload(sigURL, sigPath); err == nil {
				verifyArgs := []string{
					"verify-blob",
					"--bundle", sigPath,
					"--certificate-identity-regexp", cosignIdentityRegexp,
					"--certificate-oidc-issuer", cosignOIDCIssuer,
					sumPath,
				}
				verifyCmd := exec.Command(cosignPath, verifyArgs...)
				verifyCmd.Stdout = os.Stderr
				verifyCmd.Stderr = os.Stderr
				if err := verifyCmd.Run(); err != nil {
					if !insecure {
						cleanup()
						return fmt.Errorf("cosign verification failed — refusing install")
					}
					fmt.Fprintf(os.Stderr, "  WARN: cosign verification failed but EDIKT_INSTALL_INSECURE=1 — continuing\n")
				} else {
					fmt.Fprintf(os.Stderr, "  cosign: signature verified\n")
				}
			}
		}
	} else if !insecure {
		cleanup()
		return fmt.Errorf("cosign not found — refusing install without signature verification.\nInstall cosign or set EDIKT_INSTALL_INSECURE=1")
	} else {
		fmt.Fprintf(os.Stderr, "  WARN: cosign not on PATH, EDIKT_INSTALL_INSECURE=1 — skipping signature verification\n")
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

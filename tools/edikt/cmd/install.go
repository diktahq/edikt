package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Install exit codes matching the bash launcher contract.
const (
	exitNetwork   = 1 // source not found or network error
	exitChecksum  = 2 // checksum mismatch or empty reference
	exitAlready   = 3 // version already installed
	exitMalicious = 5 // path traversal detected in tarball
)

func installExitError(code int, format string, a ...interface{}) error {
	return &exitCodeError{code: code, msg: fmt.Sprintf(format, a...)}
}

var installCmd = &cobra.Command{
	Use:          "install <tag>",
	Short:        "Install a specific version from a local source or GitHub",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tag := normalizeTag(args[0])

		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return err
		}

		targetDir := filepath.Join(ediktRoot, "versions", tag)
		if _, err := os.Stat(targetDir); err == nil {
			return installExitError(exitAlready, "version %s is already installed", tag)
		}

		source := os.Getenv("EDIKT_INSTALL_SOURCE")
		if source == "" {
			// Network path: delegate to downloadAndInstall.
			if err := downloadAndInstall(ediktRoot, "v"+tag, tag); err != nil {
				return installExitError(exitNetwork, "%v", err)
			}
			writeManifest(targetDir, tag, "")
			emitEvent(ediktRoot, "version_installed", map[string]interface{}{"version": tag})
			return nil
		}

		// Validate source exists.
		if _, err := os.Stat(source); os.IsNotExist(err) {
			return installExitError(exitNetwork, "source not found: %s", source)
		}

		isTarball := strings.HasSuffix(source, ".tar.gz") || strings.HasSuffix(source, ".tgz")

		// Compute sha256 for checksumming.
		var computedSHA string
		if isTarball {
			if h, err := sha256File(source); err == nil {
				computedSHA = h
			}
		} else {
			// Directory source: hash the VERSION file as a proxy.
			if h, err := sha256File(filepath.Join(source, "VERSION")); err == nil {
				computedSHA = h
			}
		}

		// Verify against explicit env override.
		if envSHA := os.Getenv("EDIKT_INSTALL_SHA256"); envSHA != "" {
			if computedSHA != envSHA {
				return installExitError(exitChecksum,
					"checksum mismatch: expected %s, got %s", envSHA, computedSHA)
			}
		} else if isTarball {
			// Opportunistic sidecar check — same helper used by the network branch.
			if err := verifySidecar(source, computedSHA); err != nil {
				return err
			}
		}

		// Install.
		if err := os.MkdirAll(filepath.Join(ediktRoot, "versions"), 0o755); err != nil {
			return fmt.Errorf("creating versions dir: %w", err)
		}

		if isTarball {
			// Pre-scan the tarball for path traversal before extraction so we
			// can return the right exit code (EX_MALICIOUS=5).
			if err := checkTarGzSafety(source); err != nil {
				return installExitError(exitMalicious, "%v", err)
			}
		}

		if err := localInstallFromSource(ediktRoot, targetDir, source, isTarball); err != nil {
			return fmt.Errorf("installing: %w", err)
		}

		writeManifest(targetDir, tag, computedSHA)
		emitEvent(ediktRoot, "version_installed", map[string]interface{}{"version": tag})
		return nil
	},
}

// verifySidecar checks the sibling <tarball>.sha256 file against computedSHA.
// Returns nil if no sidecar exists (local is trusted).
// Returns exitChecksum error if sidecar is empty or hash does not match.
func verifySidecar(tarballPath, computedSHA string) error {
	sidecarPath := tarballPath + ".sha256"
	data, err := os.ReadFile(sidecarPath)
	if os.IsNotExist(err) {
		return nil // no sidecar — local tarball, trust it
	}
	if err != nil {
		return fmt.Errorf("reading sidecar: %w", err)
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return installExitError(exitChecksum, "empty checksum reference — sidecar %s is empty", sidecarPath)
	}
	expected := strings.Fields(raw)[0]
	if computedSHA != expected {
		return installExitError(exitChecksum,
			"checksum mismatch: expected %s, got %s", expected, computedSHA)
	}
	return nil
}

// writeManifest writes versions/<tag>/manifest.yaml and a SHA256SUMS file
// recording the sha256 of every regular file in dir for integrity checking.
func writeManifest(dir, version, sha256hash string) {
	_ = os.MkdirAll(dir, 0o755)
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	content := fmt.Sprintf("version: %q\nsha256: %q\ninstalled_at: %q\n",
		version, sha256hash, ts)
	_ = os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(content), 0o644)
	_ = writePayloadSHA256SUMS(dir)
}

// writePayloadSHA256SUMS walks dir and writes SHA256SUMS listing every regular
// file with its sha256 hash (BSD-style: "<hash>  <relpath>").
func writePayloadSHA256SUMS(dir string) error {
	var sb strings.Builder
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		// Skip the SHA256SUMS file itself and manifest.yaml.
		base := filepath.Base(path)
		if base == "SHA256SUMS" || base == "manifest.yaml" {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		h, err := sha256File(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		sb.WriteString(fmt.Sprintf("%s  %s\n", h, rel))
		return nil
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(sb.String()), 0o644)
}

func init() {
	rootCmd.AddCommand(installCmd)
}

package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// exitTier2Prereq is returned when a Python or venv prerequisite fails.
const exitTier2Prereq = 10

var installBenchmarkCmd = &cobra.Command{
	Use:          "benchmark",
	Short:        "Install the gov-benchmark tier-2 evaluation tool",
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE:         runInstallBenchmark,
}

var uninstallBenchmarkCmd = &cobra.Command{
	Use:          "benchmark",
	Short:        "Uninstall the gov-benchmark tier-2 evaluation tool",
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE:         runUninstallBenchmark,
}

func init() {
	installCmd.AddCommand(installBenchmarkCmd)
	uninstallCmd.AddCommand(uninstallBenchmarkCmd)
}

func runInstallBenchmark(_ *cobra.Command, _ []string) error {
	ediktRoot, err := resolveEdiktRoot()
	if err != nil {
		return err
	}
	claudeRoot := resolveClaudeRoot()

	// Prerequisites — checked before any filesystem writes (ADR-015 fail-fast).
	pyBin := os.Getenv("EDIKT_TIER2_PYTHON")
	if pyBin == "" {
		pyBin = "python3"
	}
	if err := checkPythonVersion(pyBin); err != nil {
		return err
	}

	wheelPath := os.Getenv("EDIKT_TIER2_WHEEL")
	if wheelPath != "" {
		if err := checkWheelChecksum(wheelPath); err != nil {
			return err
		}
	}

	// Copy markdown; track paths for rollback on pip failure.
	copied, err := tier2CopyMarkdown(ediktRoot, claudeRoot)
	if err != nil {
		tier2RollbackMarkdown(claudeRoot, copied)
		return err
	}

	receiptPath := filepath.Join(ediktRoot, ".tier2-benchmark-receipt")
	if writeErr := writeReceipt(receiptPath, copied); writeErr != nil {
		tier2RollbackMarkdown(claudeRoot, copied)
		return fmt.Errorf("writing receipt: %w", writeErr)
	}

	if os.Getenv("EDIKT_TIER2_SKIP_PIP") == "1" {
		sentinelDir := filepath.Join(ediktRoot, "venv", "gov-benchmark")
		if mkErr := os.MkdirAll(sentinelDir, 0o755); mkErr != nil {
			tier2RollbackMarkdown(claudeRoot, copied)
			return mkErr
		}
		_ = os.WriteFile(filepath.Join(sentinelDir, ".pip-skipped"), []byte{}, 0o644)
		return nil
	}

	venvDir := filepath.Join(ediktRoot, "venv", "gov-benchmark")
	if venvErr := tier2Venv(pyBin, venvDir); venvErr != nil {
		tier2RollbackMarkdown(claudeRoot, copied)
		return &exitCodeError{code: exitTier2Prereq, msg: fmt.Sprintf("venv creation failed: %v", venvErr)}
	}
	if pipErr := tier2PipInstall(ediktRoot, venvDir, wheelPath); pipErr != nil {
		tier2RollbackMarkdown(claudeRoot, copied)
		return &exitCodeError{code: exitTier2Prereq, msg: fmt.Sprintf("pip install failed: %v", pipErr)}
	}

	return nil
}

func runUninstallBenchmark(_ *cobra.Command, _ []string) error {
	ediktRoot, err := resolveEdiktRoot()
	if err != nil {
		return err
	}
	claudeRoot := resolveClaudeRoot()

	receiptPath := filepath.Join(ediktRoot, ".tier2-benchmark-receipt")
	venvDir := filepath.Join(ediktRoot, "venv", "gov-benchmark")

	_, receiptErr := os.Stat(receiptPath)
	_, venvErr := os.Stat(venvDir)
	if os.IsNotExist(receiptErr) && os.IsNotExist(venvErr) {
		fmt.Fprintln(os.Stderr, "Already uninstalled")
		return nil
	}

	if !os.IsNotExist(receiptErr) {
		paths, _ := readReceipt(receiptPath)
		tier2RollbackMarkdown(claudeRoot, paths)
		_ = os.Remove(receiptPath)
	}

	if !os.IsNotExist(venvErr) {
		_ = os.RemoveAll(venvDir)
	}

	return nil
}

// checkPythonVersion probes pyBin and returns an error if it is < 3.10 or unreachable.
func checkPythonVersion(pyBin string) error {
	out, runErr := exec.Command(pyBin, "-c",
		"import sys; v=sys.version_info; print(str(v.major)+'.'+str(v.minor))").Output()
	if runErr != nil {
		return &exitCodeError{
			code: exitTier2Prereq,
			msg: fmt.Sprintf("edikt benchmark requires Python 3.10+; could not run python at %s: %v",
				pyBin, runErr),
		}
	}
	version := strings.TrimSpace(string(out))
	parts := strings.SplitN(version, ".", 2)
	if len(parts) != 2 {
		return &exitCodeError{
			code: exitTier2Prereq,
			msg:  fmt.Sprintf("edikt benchmark requires Python 3.10+; unexpected output %q at %s", version, pyBin),
		}
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || major < 3 || (major == 3 && minor < 10) {
		return &exitCodeError{
			code: exitTier2Prereq,
			msg:  fmt.Sprintf("edikt benchmark requires Python 3.10+; found %s at %s", version, pyBin),
		}
	}
	return nil
}

// checkWheelChecksum enforces checksum policy for EDIKT_TIER2_WHEEL.
// Wheels under a /current/ path require EDIKT_TIER2_WHEEL_SHA256.
func checkWheelChecksum(wheelPath string) error {
	expectedSHA := os.Getenv("EDIKT_TIER2_WHEEL_SHA256")
	if strings.Contains(wheelPath, "/current/") && expectedSHA == "" {
		return &exitCodeError{
			code: exitChecksum,
			msg:  "Release install requires EDIKT_TIER2_WHEEL_SHA256",
		}
	}
	if expectedSHA != "" {
		actual, err := sha256File(wheelPath)
		if err != nil {
			return &exitCodeError{
				code: exitChecksum,
				msg:  fmt.Sprintf("Wheel checksum mismatch: could not read %s: %v", wheelPath, err),
			}
		}
		if actual != expectedSHA {
			return &exitCodeError{code: exitChecksum, msg: "Wheel checksum mismatch"}
		}
	}
	return nil
}

// tier2CopyMarkdown copies benchmark.md and attack templates from the edikt
// current layout into claudeRoot. Returns the list of destination paths written.
func tier2CopyMarkdown(ediktRoot, claudeRoot string) ([]string, error) {
	source := os.Getenv("EDIKT_TIER2_SOURCE")
	if source == "" {
		source = filepath.Join(ediktRoot, "current")
	}

	var copied []string

	// benchmark.md
	dstMD := filepath.Join(claudeRoot, "commands", "edikt", "gov", "benchmark.md")
	if err := mkdirAllThroughDanglingSymlinks(filepath.Dir(dstMD)); err != nil {
		return nil, fmt.Errorf("creating gov dir: %w", err)
	}
	if err := tier2CopyFile(filepath.Join(source, "commands", "gov", "benchmark.md"), dstMD); err != nil {
		return nil, fmt.Errorf("copying benchmark.md: %w", err)
	}
	copied = append(copied, dstMD)

	// attack templates
	dstAttacks := filepath.Join(claudeRoot, "commands", "edikt", "templates", "attacks")
	if err := mkdirAllThroughDanglingSymlinks(dstAttacks); err != nil {
		return nil, fmt.Errorf("creating attacks dir: %w", err)
	}
	srcAttacks := filepath.Join(source, "templates", "attacks")
	entries, err := os.ReadDir(srcAttacks)
	if err != nil {
		return nil, fmt.Errorf("reading attacks dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		dstFile := filepath.Join(dstAttacks, entry.Name())
		if cpErr := tier2CopyFile(filepath.Join(srcAttacks, entry.Name()), dstFile); cpErr != nil {
			return nil, fmt.Errorf("copying %s: %w", entry.Name(), cpErr)
		}
		copied = append(copied, dstFile)
	}

	return copied, nil
}

// tier2RollbackMarkdown removes files in paths that are inside claudeRoot.
// Files outside claudeRoot are silently skipped (path guard).
func tier2RollbackMarkdown(claudeRoot string, paths []string) {
	prefix := filepath.Clean(claudeRoot) + string(filepath.Separator)
	for _, p := range paths {
		if strings.HasPrefix(filepath.Clean(p), prefix) {
			_ = os.Remove(p)
		}
	}
}

// mkdirAllThroughDanglingSymlinks is like os.MkdirAll but first creates the
// targets of any dangling symlinks found while walking path components.
// This handles the v0.4.x → versioned-layout migration where
// ~/.claude/commands/edikt may be a symlink to a not-yet-created target.
func mkdirAllThroughDanglingSymlinks(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	current := string(filepath.Separator)
	for _, p := range strings.Split(abs, string(filepath.Separator)) {
		if p == "" {
			continue
		}
		current = filepath.Join(current, p)
		fi, lerr := os.Lstat(current)
		if os.IsNotExist(lerr) {
			break // remainder absent — MkdirAll will create it
		}
		if lerr != nil {
			return lerr
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if _, serr := os.Stat(current); serr == nil {
			continue // symlink resolves fine
		}
		// Dangling symlink — create the target directory.
		target, rerr := os.Readlink(current)
		if rerr != nil {
			return rerr
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		if mkErr := os.MkdirAll(target, 0o755); mkErr != nil {
			return mkErr
		}
	}
	return os.MkdirAll(abs, 0o755)
}

// tier2CopyFile copies src to dst, overwriting dst if it exists.
func tier2CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, cpErr := io.Copy(out, in); cpErr != nil {
		out.Close()
		return cpErr
	}
	return out.Close()
}

// writeReceipt writes one destination path per line to receiptPath.
func writeReceipt(receiptPath string, paths []string) error {
	content := strings.Join(paths, "\n")
	if len(paths) > 0 {
		content += "\n"
	}
	return os.WriteFile(receiptPath, []byte(content), 0o644)
}

// readReceipt reads one path per line from receiptPath, stripping blank lines.
func readReceipt(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// tier2Venv creates a Python virtual environment at venvDir.
func tier2Venv(pyBin, venvDir string) error {
	cmd := exec.Command(pyBin, "-m", "venv", venvDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// tier2PipInstall installs the gov-benchmark package from wheelPath or pyproject.
func tier2PipInstall(ediktRoot, venvDir, wheelPath string) error {
	pip := filepath.Join(venvDir, "bin", "pip")
	target := wheelPath
	if target == "" {
		target = filepath.Join(ediktRoot, "current", "tools", "gov-benchmark")
	}
	cmd := exec.Command(pip, "install", target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

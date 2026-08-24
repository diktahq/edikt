// Package installsettings writes edikt's Claude Code settings.json and its
// managed-region integrity sidecar. It replaces the two python3 heredocs
// install.sh used to run for this (write_settings_json, install.sh:814-951)
// with a single Go verb — see docs/product/specs/SPEC-015-installer-consolidation
// FR-009 for the full design.
//
// Purity: this package performs no process execution (no os/exec) — every
// operation here is file I/O, string substitution, JSON (de)serialization,
// and hashing. The one genuinely process-dependent concern Phase 10
// introduces — probing whether an ALREADY-STAGED (and possibly older)
// launcher binary supports the `install settings` verb at all — is
// deliberately kept OUT of this package for the same reason: that probe
// necessarily execs a binary. CheckCapability below takes that dependency
// as an injected function instead, so this package (and its tests) stay
// free of os/exec. The real, exec-based probe lives in tools/edikt/cmd,
// where install.sh's capability preflight actually calls into a binary.
package installsettings

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Options carries everything Write needs to produce settings.json and its
// sidecar. Callers (cmd/install_settings.go) resolve every path through
// internal/installsurface before building this struct — this package does
// not know about ".claude" or any other harness-specific literal (SP-005).
type Options struct {
	// TemplatePath is $EDIKT_ROOT/templates/settings.json.tmpl.
	TemplatePath string
	// SettingsPath is the resolved destination, e.g. <claude-root>/settings.json.
	SettingsPath string
	// SidecarPath is the resolved managed-sidecar destination, e.g.
	// <edikt-home>/state/settings-managed.json.
	SidecarPath string
	// HookDir is the raw EDIKT_HOOK_DIR value to substitute into the
	// template. Validated against hookDirAllowlist before use.
	HookDir string
	// BackupHome overrides $HOME for the one-shot pre-v0.6.0 backup. Tests
	// set this to an isolated tmp dir; production callers leave it empty
	// and os.Getenv("HOME") is used, matching install.sh:826's literal
	// $HOME (never $EDIKT_HOME, never project-relative).
	BackupHome string
}

// ValidationError reports that a Write input failed validation before any
// file was touched — EDIKT_HOOK_DIR failing the allowlist, or the
// substituted template failing to parse as JSON. cmd/install_settings.go
// classifies this as a "bad input" exit code, distinct from a filesystem
// error.
type ValidationError struct {
	Msg string
}

func (e *ValidationError) Error() string { return e.Msg }

// FatalSidecarError reports that the managed-region integrity sidecar
// could not be written. INV-005 requires this sidecar to detect
// managed-region drift on the next upgrade; install.sh's old
// `python3 ... || true` swallowed exactly this failure — a fatal absence
// rendered as success. This type exists so that mistake cannot be repeated
// silently: every caller of Write that reaches this stage MUST treat it as
// fatal, never optional.
type FatalSidecarError struct {
	Err error
}

func (e *FatalSidecarError) Error() string {
	// Named for the adopter, not for edikt's invariant register: the sidecar
	// is what lets edikt notice hand-edits to its managed block, so failing to
	// write it is fatal rather than a warning.
	return fmt.Sprintf("managed-region sidecar write failed (fatal — without it edikt cannot detect edits to its managed block): %v", e.Err)
}

func (e *FatalSidecarError) Unwrap() error { return e.Err }

// hookDirAllowlist is INV-006's allowlist, not install.sh's old denylist.
// The denylist (install.sh:844-849) rejected only `"`, `\`, `|`, tab, CR,
// LF — leaving `$`, backticks, `(`, `)`, and `;` all permitted through into
// `"command": "<dir>/session-start.sh"` in settings.json, a value Claude
// Code shell-interprets at hook-fire time. This pattern permits only an
// absolute path built from letters, digits, '.', '_', '/', '-'.
var hookDirAllowlist = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

// ValidateHookDir reports whether dir is safe to interpolate into a shell
// command string. Exported so cmd/install_settings.go (and tests) can
// check a value before doing anything else with it.
func ValidateHookDir(dir string) error {
	if dir == "" {
		return &ValidationError{Msg: "EDIKT_HOOK_DIR is empty"}
	}
	if !hookDirAllowlist.MatchString(dir) {
		return &ValidationError{Msg: fmt.Sprintf(
			"EDIKT_HOOK_DIR %q does not match the allowed pattern ^/[A-Za-z0-9._/-]+$ "+
				"(must be an absolute path built only from letters, digits, '.', '_', '/', '-')", dir)}
	}
	if strings.Contains(dir, "..") {
		return &ValidationError{Msg: fmt.Sprintf("EDIKT_HOOK_DIR %q must not contain '..'", dir)}
	}
	return nil
}

// Write performs the full FR-009 sequence: validate EDIKT_HOOK_DIR, read
// the template, substitute it into the raw text, parse+re-serialize as
// JSON, back up any pre-existing settings.json (one-shot), write the new
// settings.json atomically, then write the managed sidecar (fatal on
// failure — see FatalSidecarError).
//
// skipped reports whether the template was absent — that mirrors
// install.sh:817-820's own behavior (warn and skip, return success) for
// parity with the pre-Go install; it is not an error condition.
func Write(opts Options) (skipped bool, err error) {
	if err := ValidateHookDir(opts.HookDir); err != nil {
		return false, err
	}

	raw, err := os.ReadFile(opts.TemplatePath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("reading template %s: %w", opts.TemplatePath, err)
	}

	// String substitution FIRST (so the parser only ever sees valid paths
	// substituted into valid JSON string positions), THEN parse — the same
	// order the python implementation used, and the reason a naive
	// sed-based substitution was neither JSON-aware nor escape-safe
	// (install.sh:853-857, audit HI-2).
	substituted := strings.ReplaceAll(string(raw), "${EDIKT_HOOK_DIR}", opts.HookDir)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(substituted), &parsed); err != nil {
		return false, &ValidationError{Msg: fmt.Sprintf(
			"template produced invalid JSON after substitution: %v (check that EDIKT_HOOK_DIR=%q is a safe path)",
			err, opts.HookDir)}
	}

	if err := backupPreV060(opts.SettingsPath, opts.BackupHome); err != nil {
		// Best-effort per install.sh:831/835 (`mkdir -p ... || true`,
		// `cp ... 2>/dev/null`) — a backup failure must never block the
		// actual settings write.
		fmt.Fprintf(os.Stderr, "warn: pre-v0.6.0 backup of %s failed: %v\n", opts.SettingsPath, err)
	}

	encoded, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return false, fmt.Errorf("re-encoding settings.json: %w", err)
	}
	encoded = append(encoded, '\n')

	if err := atomicWrite(opts.SettingsPath, encoded, 0o644); err != nil {
		return false, fmt.Errorf("writing %s: %w", opts.SettingsPath, err)
	}

	if err := writeManagedSidecar(opts.SettingsPath, opts.SidecarPath, parsed); err != nil {
		return false, &FatalSidecarError{Err: err}
	}

	return false, nil
}

// atomicWrite writes data to path via a temp file in the SAME directory,
// fsync, then rename — never a direct write to the live path.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup: once the rename below succeeds this is a no-op
	// (nothing left at tmpPath); on any earlier failure it removes the
	// half-written temp file rather than leaving it behind.
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// backupPreV060 mirrors install.sh:822-838 exactly: a ONE-SHOT backup of an
// existing settings.json, guarded by a marker file so only the first write
// after this feature shipped creates it. Always under $HOME/.edikt/backup
// — literal $HOME, matching the shell (never $EDIKT_HOME, never
// project-relative) — so it survives even a --project install's own
// $CLAUDE_HOME override.
func backupPreV060(destPath, homeOverride string) error {
	if _, err := os.Stat(destPath); err != nil {
		return nil // nothing pre-existing to back up
	}

	home := homeOverride
	if home == "" {
		home = os.Getenv("HOME")
	}
	if home == "" {
		return fmt.Errorf("HOME is not set — cannot resolve backup root")
	}

	backupRoot := filepath.Join(home, ".edikt", "backup")
	marker := filepath.Join(backupRoot, "pre-v0.6.0-marker")
	if _, err := os.Stat(marker); err == nil {
		return nil // already backed up once — one-shot
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	backupDir := filepath.Join(backupRoot, "pre-v0.6.0-"+ts)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		return err
	}
	backupFile := filepath.Join(backupDir, "settings.json")
	if err := os.WriteFile(backupFile, data, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(marker, []byte(backupDir+"\n"), 0o600); err != nil {
		return err
	}
	// Belt-and-suspenders: os.WriteFile only applies mode when CREATING a
	// file; chmod explicitly so an existing, looser-permissioned file
	// (e.g. an inherited umask) can't leave either file world-readable.
	_ = os.Chmod(backupFile, 0o600)
	_ = os.Chmod(marker, 0o600)
	return nil
}

// managedSidecar is the on-disk shape of settings-managed.json — field
// names and semantics carried over verbatim from the python implementation
// (install.sh:930-936) so an existing sidecar written by the old installer
// is still a valid instance of this struct.
type managedSidecar struct {
	SettingsPath    string   `json:"settings_path"`
	ManagedKeys     []string `json:"managed_keys"`
	ManagedHash     string   `json:"managed_hash"`
	SentinelVersion int      `json:"sentinel_version"`
	InstalledAt     string   `json:"installed_at"`
}

// writeManagedSidecar computes sha256 over the canonicalized `permissions`
// key and writes the sidecar 0600. Go's json.Marshal sorts map keys at
// every nesting level, which is the equivalent of Python's
// json.dumps(..., sort_keys=True) — Go map iteration order is randomized
// per-process, so this sort (not iteration order) is the only source of
// the hash's determinism.
func writeManagedSidecar(settingsPath, sidecarPath string, data map[string]interface{}) error {
	managed := map[string]interface{}{}
	if perms, ok := data["permissions"]; ok {
		managed["permissions"] = perms
	}
	canonical, err := json.Marshal(managed)
	if err != nil {
		return fmt.Errorf("canonicalizing managed keys: %w", err)
	}
	sum := sha256.Sum256(canonical)

	sidecar := managedSidecar{
		SettingsPath:    settingsPath,
		ManagedKeys:     []string{"permissions"},
		ManagedHash:     hex.EncodeToString(sum[:]),
		SentinelVersion: 1,
		InstalledAt:     time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	encoded, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding sidecar: %w", err)
	}
	encoded = append(encoded, '\n')

	if err := atomicWrite(sidecarPath, encoded, 0o600); err != nil {
		return err
	}
	return os.Chmod(sidecarPath, 0o600)
}

// CapabilityProbe reports whether the binary at path supports
// `install settings`, along with the version string it reports. The real,
// exec-based implementation lives in cmd/ (this package stays exec-free,
// see the package doc comment); tests inject a fake.
type CapabilityProbe func(binaryPath string) (supported bool, version string, err error)

// CheckCapability runs probe against binaryPath and returns a descriptive
// error — naming the reported version — when the binary does not support
// `install settings`, or when probe itself could not determine that (the
// binary is missing, not executable, or crashed). A nil return means it is
// safe to proceed with binaryPath as the new launcher.
//
// This exists to catch exactly the failure install.sh used to hit only
// AFTER PATH was modified and `current` was flipped: `install.sh --ref
// v0.7.2` stages a pre-0.8.0 binary that has no `install settings` verb at
// all; cobra reports "unknown command" the moment anything tries to run
// it. Calling this BEFORE the staged binary is placed, PATH is touched, or
// any version is activated turns that into a clean, named refusal instead
// of a half-finished install with no settings.json.
func CheckCapability(binaryPath string, probe CapabilityProbe) error {
	if probe == nil {
		return fmt.Errorf("capability preflight: no probe supplied")
	}
	supported, version, err := probe(binaryPath)
	if err != nil {
		return fmt.Errorf("capability preflight: could not determine whether %s supports `install settings`: %w", binaryPath, err)
	}
	if !supported {
		v := version
		if v == "" {
			v = "unknown"
		}
		return fmt.Errorf(
			"capability preflight: staged binary %s reports version %s, which does not support "+
				"`install settings` (this verb was added in v0.8.0). This happens when --ref points at "+
				"a release older than v0.8.0 — re-run install.sh without --ref for the latest release, "+
				"or pass --ref pointing at v0.8.0 or newer",
			binaryPath, v)
	}
	return nil
}

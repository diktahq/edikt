package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/diktahq/edikt/tools/edikt/internal/installsettings"
	"github.com/diktahq/edikt/tools/edikt/internal/installsurface"
	"github.com/spf13/cobra"
)

// install settings' own internal exit-code convention — the same
// small-integer shape used elsewhere across cmd/ (see cmd/version_gate.go,
// cmd/sidecar_approve_kinds.go): 2=permission/not-found, 3=bad-input,
// 4=fatal. installSettingsExitToShellEx below is what keeps this from
// colliding with install.sh's own, DIFFERENT numbering for the same
// digits.
const (
	installSettingsExitOK         = 0
	installSettingsExitPermission = 2
	installSettingsExitBadInput   = 3
	installSettingsExitFatal      = 4
)

// installSettingsExitToShellEx converts this command's own exit code
// (installSettingsExit* above) into install.sh's EX_* contract
// (EX_OK=0, EX_NETWORK=1, EX_PERMISSION=2, EX_VERSION=3, EX_GENERAL=4).
// install.sh's write_settings_json wrapper execs this binary and checks $?
// against those EX_* constants directly — a raw pass-through of this
// command's own code would collide with a DIFFERENT meaning at the same
// digit: this command's "bad input" is 3, which is install.sh's
// EX_VERSION, so an invalid EDIKT_HOOK_DIR would misreport to install.sh's
// caller as a version mismatch instead of what it actually is. This
// function is the single place that translation happens; nothing else in
// this file should compare a raw exit code against install.sh's constants.
func installSettingsExitToShellEx(code int) int {
	switch code {
	case installSettingsExitOK:
		return 0 // EX_OK
	case installSettingsExitPermission:
		return 2 // EX_PERMISSION
	case installSettingsExitBadInput:
		// EX_GENERAL — install.sh has no dedicated "bad input" code; this
		// is exactly what the old python3 path returned for the
		// equivalent failures (bad EDIKT_HOOK_DIR, invalid JSON after
		// substitution). NOT 3 — 3 is EX_VERSION, a different failure
		// class entirely (see doc comment above).
		return 4
	case installSettingsExitFatal:
		// EX_GENERAL — covers the sidecar write failure, now fatal
		// instead of the old `python3 ... || true` swallow.
		return 4
	default:
		// Never let an unrecognized internal code fall through as a
		// numerically-coincidental EX_* value.
		return 4
	}
}

var installSettingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Write settings.json and its managed-region integrity sidecar",
	Long: `install settings reads $EDIKT_ROOT/templates/settings.json.tmpl,
substitutes EDIKT_HOOK_DIR, and writes the result atomically to the active
Claude Code settings.json, plus a managed-region integrity sidecar under
$EDIKT_HOME/state/settings-managed.json. Replaces install.sh's two former
script-interpreter heredocs the installer no longer needs.

Reads EDIKT_ROOT (template source), CLAUDE_HOME (settings.json
destination — resolved via internal/claudepaths), and EDIKT_HOOK_DIR (the
value substituted into the template) from the environment. install.sh's
write_settings_json is now a thin wrapper that exports all three before
invoking this verb.

Exit codes (mapped onto install.sh's EX_* contract — see
installSettingsExitToShellEx):
  0  success, or a missing template (a warned no-op, matching the
     pre-Go behavior)
  2  permission error writing settings.json, the sidecar, or their
     parent directories
  4  invalid EDIKT_HOOK_DIR, invalid JSON after substitution, or the
     managed sidecar could not be written (fatal)`,
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ediktRoot, err := resolveEdiktRoot()
		if err != nil {
			return &exitCodeError{code: installSettingsExitToShellEx(installSettingsExitBadInput), msg: err.Error()}
		}

		roots := installsurface.ResolveGlobalRoots()
		settingsPath, err := installsurface.Resolve(installsurface.HarnessClaudeCode, installsurface.KindSettingsFile, roots)
		if err != nil {
			return &exitCodeError{code: installSettingsExitToShellEx(installSettingsExitBadInput), msg: err.Error()}
		}
		sidecarPath, err := installsurface.Resolve(installsurface.HarnessClaudeCode, installsurface.KindManagedSidecar, roots)
		if err != nil {
			return &exitCodeError{code: installSettingsExitToShellEx(installSettingsExitBadInput), msg: err.Error()}
		}

		opts := installsettings.Options{
			TemplatePath: filepath.Join(ediktRoot, "templates", "settings.json.tmpl"),
			SettingsPath: settingsPath,
			SidecarPath:  sidecarPath,
			HookDir:      os.Getenv("EDIKT_HOOK_DIR"),
		}

		skipped, werr := installsettings.Write(opts)
		if werr != nil {
			return &exitCodeError{code: installSettingsExitToShellEx(classifySettingsError(werr)), msg: werr.Error()}
		}
		if skipped {
			fmt.Fprintf(os.Stderr, "warn: settings.json.tmpl not found at %s — skipping settings.json write\n", opts.TemplatePath)
			return nil
		}
		fmt.Fprintf(os.Stderr, "wrote %s (hook dir: %s)\n", settingsPath, opts.HookDir)
		return nil
	},
}

// classifySettingsError maps an installsettings error to this command's own
// exit-code convention. installSettingsExitToShellEx (above) does the
// second translation, into install.sh's EX_* contract.
func classifySettingsError(err error) int {
	var verr *installsettings.ValidationError
	if errors.As(err, &verr) {
		return installSettingsExitBadInput
	}
	var ferr *installsettings.FatalSidecarError
	if errors.As(err, &ferr) {
		return installSettingsExitFatal
	}
	if errors.Is(err, os.ErrPermission) {
		return installSettingsExitPermission
	}
	return installSettingsExitFatal
}

func init() {
	installCmd.AddCommand(installSettingsCmd)
	mutatingCommands["settings"] = true
}

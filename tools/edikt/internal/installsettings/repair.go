package installsettings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RepairReport says what Repair changed, by name, so the caller prints a
// line per repair and nothing when there was nothing to repair.
type RepairReport struct {
	Substituted int  // occurrences of ${EDIKT_HOOK_DIR} replaced
	AddedType   bool // statusLine.type inserted
}

func (r RepairReport) Empty() bool { return r.Substituted == 0 && !r.AddedType }

// Repair fixes the two mechanical defects a shipped settings.json is known to
// carry, in place, without touching anything else in the file:
//
//   - a literal ${EDIKT_HOOK_DIR} left unsubstituted in a hook command. The
//     host runs the command through a shell, which DOES expand it — but
//     nothing sets EDIKT_HOOK_DIR in a hook's environment, so it expands to
//     the empty string and every hook fails with "/<hook>.sh: No such file or
//     directory". Every occurrence is replaced with opts.HookDir, and
//   - a statusLine object with no type key (the host refuses to load the
//     whole file); "type": "command" is inserted.
//
// It lived in /edikt:upgrade's prose as two "auto-repair before continuing"
// pre-flights. Moving it here keeps doctor read-only — doctor names this verb
// as the remedy — and gives the repair one implementation, tested. The file is
// rewritten only when something changed, atomically, and the managed-region
// sidecar is recomputed from the repaired content when a sidecar path is set.
//
// The hook dir is validated only when it is about to be substituted. It is
// the input to one repair, not both: validating it up front made a
// statusLine-only repair refuse with "EDIKT_HOOK_DIR is empty", so the remedy
// doctor names failed on exactly the file it was named for (#11, item 7).
// Counting first and validating second keeps the refusal for the case that
// needs the value — a placeholder never becomes an empty string.
func Repair(opts Options) (RepairReport, error) {
	var rep RepairReport
	raw, err := os.ReadFile(opts.SettingsPath)
	if err != nil {
		return rep, fmt.Errorf("reading %s: %w", opts.SettingsPath, err)
	}
	text := string(raw)
	rep.Substituted = strings.Count(text, "${EDIKT_HOOK_DIR}")
	if rep.Substituted > 0 {
		if err := ValidateHookDir(opts.HookDir); err != nil {
			return RepairReport{}, err
		}
		text = strings.ReplaceAll(text, "${EDIKT_HOOK_DIR}", jsonStringFragment(opts.HookDir))
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return RepairReport{}, &ValidationError{Msg: fmt.Sprintf("%s is not valid JSON (after substitution): %v", opts.SettingsPath, err)}
	}
	if sl, ok := parsed["statusLine"].(map[string]interface{}); ok {
		if _, has := sl["type"]; !has {
			sl["type"] = "command"
			rep.AddedType = true
		}
	}
	if rep.Empty() {
		return rep, nil
	}
	encoded, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return RepairReport{}, fmt.Errorf("re-encoding %s: %w", opts.SettingsPath, err)
	}
	encoded = append(encoded, '\n')
	// Write to the link's TARGET. A user-scope settings.json is commonly a
	// symlink into a dotfiles checkout, and the atomic rename below would
	// otherwise replace the link with a regular file.
	dest := opts.SettingsPath
	if resolved, rerr := filepath.EvalSymlinks(dest); rerr == nil {
		dest = resolved
	}
	if err := atomicWrite(dest, encoded, 0o644); err != nil {
		return RepairReport{}, fmt.Errorf("writing %s: %w", opts.SettingsPath, err)
	}
	if opts.SidecarPath != "" {
		if err := writeManagedSidecar(opts.SettingsPath, opts.SidecarPath, parsed); err != nil {
			return RepairReport{}, &FatalSidecarError{Err: err}
		}
	}
	return rep, nil
}

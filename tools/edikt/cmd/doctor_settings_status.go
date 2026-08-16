package cmd

// doctor_settings_status.go — settings.json statusLine.type validation.
// Replaces the python heredoc previously embedded in commands/doctor.md
// (Phase 11.5 of PLAN-v060-governance-accuracy).
//
// A statusLine block missing the required `type` field invalidates the
// WHOLE settings.json from Claude Code's perspective; every hook stops
// firing too. Critical signal — surface as ERROR.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// runStatusLineTypeCheck validates the statusLine block in
// .claude/settings.json. Returns (errors, warnings, ran).
func runStatusLineTypeCheck(projectRoot string, w io.Writer) (errs, warns int, ran bool) {
	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return 0, 0, false
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		// Settings unparseable — covered by the placeholder check
		// upstream and by the existing doctor.go logic. Skip silently.
		return 0, 0, false
	}
	sl, ok := s["statusLine"].(map[string]any)
	if !ok {
		// statusLine absent or non-object — nothing to check.
		return 0, 0, true
	}
	if _, ok := sl["type"]; !ok {
		fmt.Fprintf(w,
			"  ERROR: settings.json statusLine block missing the required type field — Claude Code refuses to load the entire settings file. Run /edikt:upgrade to auto-repair, or add \"type\": \"command\" manually as the first key inside the statusLine object.\n",
		)
		return 1, 0, true
	}
	return 0, 0, true
}

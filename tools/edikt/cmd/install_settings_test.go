package cmd

import "testing"

// TestExitCodeMapping pins AC-10.8: this command's own exit-code
// convention (2=permission, 3=bad-input, 4=fatal — the same shape used
// elsewhere in cmd/, see cmd/version_gate.go) must NOT pass straight
// through to install.sh's EX_* contract (EX_OK=0, EX_NETWORK=1,
// EX_PERMISSION=2, EX_VERSION=3, EX_GENERAL=4). The two schemes assign
// different meanings to the same digits — in particular, an unmapped
// bad-input (3) would surface to install.sh's caller as EX_VERSION (3),
// misreporting a validation failure (e.g. a bad EDIKT_HOOK_DIR) as a
// version mismatch.
func TestExitCodeMapping(t *testing.T) {
	cases := []struct {
		name   string
		goCode int
		wantEx int
	}{
		{"ok maps to EX_OK", installSettingsExitOK, 0},
		{"permission maps to EX_PERMISSION", installSettingsExitPermission, 2},
		{"bad-input maps to EX_GENERAL, not EX_VERSION", installSettingsExitBadInput, 4},
		{"fatal (sidecar write) maps to EX_GENERAL", installSettingsExitFatal, 4},
		{"unrecognized code falls back to EX_GENERAL", 99, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := installSettingsExitToShellEx(c.goCode)
			if got != c.wantEx {
				t.Errorf("installSettingsExitToShellEx(%d) = %d, want %d", c.goCode, got, c.wantEx)
			}
		})
	}

	// The specific bug this mapping exists to prevent: Go's bad-input code
	// and install.sh's EX_VERSION happen to share the digit 3. A
	// pass-through (or a broken mapping) would let a bad-input failure
	// misreport itself as a version mismatch to whatever called install.sh.
	if installSettingsExitBadInput == 3 && installSettingsExitToShellEx(installSettingsExitBadInput) == 3 {
		t.Fatal("bad-input (3) must not surface as EX_VERSION (3) — that collision is exactly what this mapping prevents")
	}
}

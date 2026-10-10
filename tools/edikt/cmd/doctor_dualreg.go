package cmd

// doctor_dualreg.go — ADR-050 §5: hooks belong at project scope. A hook
// registered at BOTH the user-level (<claude-root>/settings.json) and the
// project (.claude/settings.json) fires twice per event (Claude Code merges
// scopes) — the double-print field bug. Doctor DETECTS the overlap and
// OFFERS the removal from user-level settings interactively; it NEVER
// removes silently (ruled: upgrade must not edit settings outside the repo
// unprompted). Non-TTY runs print the finding and the remediation only.
//
// The user-level settings' edikt-managed manifest covers the "permissions"
// key only (INV-005), so pruning duplicated hook entries does not disturb
// the managed-key hash.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

type hookSettings struct {
	Hooks map[string][]hookGroup `json:"hooks"`
}

type hookGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// hookBasenames returns event → set of edikt hook basenames registered.
func hookBasenames(path string) (map[string]map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Settings templates carry ${EDIKT_HOOK_DIR} placeholders; strip for parse.
	var hs hookSettings
	if err := json.Unmarshal(raw, &hs); err != nil {
		return nil, err
	}
	out := map[string]map[string]bool{}
	for event, groups := range hs.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				base := filepath.Base(h.Command)
				if !strings.HasSuffix(base, ".sh") {
					continue
				}
				if out[event] == nil {
					out[event] = map[string]bool{}
				}
				out[event][base] = true
			}
		}
	}
	return out, nil
}

// dualHookRegistrations returns the sorted "event/hook.sh" pairs registered
// at both scopes. Missing or unparseable files mean no overlap is provable —
// returned as empty with no error (doctor stays quiet rather than guessing).
func dualHookRegistrations(claudeRoot, projectRoot string) []string {
	userReg, err := hookBasenames(filepath.Join(claudeRoot, "settings.json"))
	if err != nil {
		return nil
	}
	projReg, err := hookBasenames(filepath.Join(projectRoot, ".claude", "settings.json"))
	if err != nil {
		return nil
	}
	var dups []string
	for event, bases := range projReg {
		for base := range bases {
			if userReg[event][base] {
				dups = append(dups, event+"/"+base)
			}
		}
	}
	sort.Strings(dups)
	return dups
}

// removeUserHookEntries removes the given event/hook pairs from the
// user-level settings hooks arrays, preserving everything else. Writes a
// .bak alongside before rewriting.
func removeUserHookEntries(claudeRoot string, pairs []string) error {
	// Follow the symlink: the user-scope settings.json is commonly a link
	// into a dotfiles checkout, and writing through the link path would
	// replace it with a regular file — quietly detaching the user's
	// dotfiles from the file edikt then edits (FR-034).
	path := resolvedUserSettings(claudeRoot)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var full map[string]json.RawMessage
	if err := json.Unmarshal(raw, &full); err != nil {
		return err
	}
	var hooks map[string][]hookGroup
	if err := json.Unmarshal(full["hooks"], &hooks); err != nil {
		return err
	}
	want := map[string]bool{}
	for _, p := range pairs {
		want[p] = true
	}
	for event, groups := range hooks {
		var newGroups []hookGroup
		for _, g := range groups {
			var kept []hookEntry
			for _, h := range g.Hooks {
				if want[event+"/"+filepath.Base(h.Command)] {
					continue
				}
				kept = append(kept, h)
			}
			if len(kept) > 0 {
				g.Hooks = kept
				newGroups = append(newGroups, g)
			}
		}
		if len(newGroups) > 0 {
			hooks[event] = newGroups
		} else {
			delete(hooks, event)
		}
	}
	enc, err := json.Marshal(hooks)
	if err != nil {
		return err
	}
	full["hooks"] = enc
	out, err := json.MarshalIndent(full, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".bak", raw, 0o644); err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// runDualRegistrationCheck prints findings and, when in is a terminal-fed
// reader and the operator consents, performs the user-level removal.
// Returns the warning count.
func runDualRegistrationCheck(claudeRoot, projectRoot string, in io.Reader, interactive bool, out io.Writer) int {
	s, _ := hookmatch.ReadSummary()
	errN, _ := runDualRegistrationCheckWithCost(claudeRoot, projectRoot, in, interactive, s, out)
	return errN
}

// runDualRegistrationCheckWithCost is the check proper. It takes the journal
// Summary as an ARGUMENT rather than reading it: doctor reads the journal
// once and shares it with the hook-chain check, so a large journal is not
// scanned twice per run.
//
// A doubled hook is an ERROR, not a warning (FR-034). It ran for weeks on the
// reference machine as a warning printed on every run and acted on by nobody;
// 13 hooks fired twice per event the whole time. The cost beside it is
// MEASURED from the journal — and when the journal has never been written,
// the cost reads UNMEASURED rather than implying none (INV-013).  // edikt-guard:allow
func runDualRegistrationCheckWithCost(claudeRoot, projectRoot string, in io.Reader, interactive bool,
	journal hookmatch.Summary, out io.Writer) (errN, warnN int) {
	dups := dualHookRegistrations(claudeRoot, projectRoot)
	if len(dups) == 0 {
		return 0, 0
	}
	cost := "cost UNMEASURED — the hook journal has never been written on this machine, which is not the same as no cost"
	if journal.JournalFound {
		cost = fmt.Sprintf("measured cost: %d hook invocation(s) recorded in the journal, each doubled event counted twice", journal.Total)
	}
	// (ref: ADR-050 — new installs register hooks at project scope only)
	fmt.Fprintf(out, "  🔴 ERROR: %d hook(s) registered at BOTH user and project scope — each fires twice per event (%s):\n",
		len(dups), cost)
	for _, d := range dups {
		fmt.Fprintf(out, "    dual: %s\n", d)
	}
	if interactive {
		fmt.Fprintf(out, "  Remove the duplicated entries from %s (a .bak is written beside it)? [y/N]: ", resolvedUserSettings(claudeRoot))
		reader := bufio.NewReader(in)
		answer, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(answer)) == "y" {
			if err := removeUserHookEntries(claudeRoot, dups); err != nil {
				fmt.Fprintf(out, "  ERROR: could not update user settings: %v\n", err)
			} else {
				fmt.Fprintf(out, "  removed %d duplicated user-scope hook entr(ies) from %s; project scope now owns them\n",
					len(dups), resolvedUserSettings(claudeRoot))
			}
		} else {
			fmt.Fprintln(out, "  left as-is — the duplicate-invocation guard covers the interim")
		}
	} else {
		// (ref: ADR-050 — new installs register hooks at project scope only)
		fmt.Fprintln(out, "  Hooks belong at project scope. Re-run `edikt doctor` interactively to remove the user-scope duplicates, or edit them out manually; the duplicate-invocation guard covers the interim.")
	}
	return 1, 0
}

// resolvedUserSettings names the file the remedy will actually edit. The
// user-scope settings.json is often a symlink into a dotfiles checkout, and a
// remedy that names the link while editing the target — or worse, replaces
// the link — is a remedy the user cannot audit (FR-034).
func resolvedUserSettings(claudeRoot string) string {
	p := filepath.Join(claudeRoot, "settings.json")
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

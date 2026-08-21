package cmd

// doctor_hook_paths.go — verifies every hook command registered in a
// settings.json actually resolves to a file on disk. Claude Code does not
// expand env vars in `command:` strings, so an unsubstituted
// `${EDIKT_HOOK_DIR}` placeholder fails every hook outright; a `$HOME`
// form (the global-mode install shape) is legitimate — the shell expands
// it — but only if the resulting path actually exists.
//
// A `--project` install is a THIRD shape this check has to cover:
// commands/init.md substitutes `${EDIKT_HOOK_DIR}` directly to a fully-
// resolved absolute project path at install time, so a project-mode
// settings.json contains neither the placeholder nor `$HOME` — invisible
// to a check that only recognized those two forms. That gap meant write-
// time enforcement could silently stop firing after a `--project` install
// with nothing in `doctor` to catch it. This resolves EVERY registered
// hook command unconditionally: expand `$HOME` and `$CLAUDE_PROJECT_DIR`
// when present, use the command as-is otherwise (the project form is
// already absolute), then stat it.
//
// A FOURTH shape, found on a real downstream project (and reproducible
// against this repo's own tools/edikt/.claude/settings.json): a
// user-registered hook quoted as `"$CLAUDE_PROJECT_DIR"/.claude/hooks/
// foo.sh`, for spacing safety. Extraction now goes through the same JSON
// decode doctor's basename checks already use (hookSettings/hookEntry in
// doctor_dualreg.go) instead of a hand-rolled regex — the regex was
// escape-blind: `"([^"]+)"` stops at the FIRST literal `"`, which for a
// quoted-variable command is the escaped quote right after
// `$CLAUDE_PROJECT_DIR`'s opening quote, capturing only a lone backslash.
// JSON decoding resolves escapes correctly by construction; any embedded
// `"` characters left in the decoded command are shell quoting around a
// path component, not part of the path, and are stripped after variable
// substitution.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// runHookPathCheck inspects one settings.json's raw content for hook
// command entries and reports how many are unresolvable. projectRoot is
// the directory `$CLAUDE_PROJECT_DIR`/`${CLAUDE_PROJECT_DIR}` resolve
// against. Returns (errors, warnings) — errors for a hook that will fail,
// a warning only when $HOME itself couldn't be resolved (resolution is
// unmeasured, not necessarily broken).
func runHookPathCheck(path, content, projectRoot string, out io.Writer) (errN, warnN int) {
	// ${EDIKT_HOOK_DIR} is a TEMPLATE placeholder: no shell expands it, so a
	// settings.json still carrying it will fail every hook.
	if strings.Contains(content, "${EDIKT_HOOK_DIR}") {
		fmt.Fprintf(out, "  ERROR: %s contains unsubstituted placeholder %q — hooks will fail. Re-run /edikt:init to regenerate, or substitute manually.\n",
			path, "${EDIKT_HOOK_DIR}")
		return 1, 0
	}

	var hs hookSettings
	if err := json.Unmarshal([]byte(content), &hs); err != nil {
		// Not this check's concern — settings.json validity is checked
		// elsewhere. Nothing to resolve.
		return 0, 0
	}

	home, herr := os.UserHomeDir()
	missing := 0
	for _, groups := range hs.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				raw := h.Command
				resolved := raw
				if strings.Contains(resolved, "$HOME") {
					if herr != nil {
						fmt.Fprintf(out, "  WARN: %s references $HOME but the home directory could not be resolved (%v) — hook resolution is UNMEASURED\n",
							path, herr)
						warnN++
						continue
					}
					resolved = strings.Replace(resolved, "$HOME", home, 1)
				}
				resolved = strings.ReplaceAll(resolved, "${CLAUDE_PROJECT_DIR}", projectRoot)
				resolved = strings.ReplaceAll(resolved, "$CLAUDE_PROJECT_DIR", projectRoot)
				// Any remaining double quotes are shell quoting around a path
				// component (e.g. `"$CLAUDE_PROJECT_DIR"/.claude/hooks/x.sh`),
				// not part of the path itself.
				resolved = strings.ReplaceAll(resolved, `"`, "")
				if _, statErr := os.Stat(resolved); statErr != nil {
					fmt.Fprintf(out, "  ERROR: %s references hook %s which does not resolve to a file (%s) — that hook will fail\n",
						path, raw, resolved)
					missing++
				}
			}
		}
	}
	return missing, warnN
}

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
// hook command unconditionally: expand `$HOME` when present, use the
// command as-is otherwise (the project form is already absolute), then
// stat it.

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// runHookPathCheck inspects one settings.json's raw content for hook
// command entries and reports how many are unresolvable. Returns
// (errors, warnings) — errors for a hook that will fail, a warning only
// when $HOME itself couldn't be resolved (resolution is unmeasured, not
// necessarily broken).
func runHookPathCheck(path, content string, out io.Writer) (errN, warnN int) {
	// ${EDIKT_HOOK_DIR} is a TEMPLATE placeholder: no shell expands it, so a
	// settings.json still carrying it will fail every hook.
	if strings.Contains(content, "${EDIKT_HOOK_DIR}") {
		fmt.Fprintf(out, "  ERROR: %s contains unsubstituted placeholder %q — hooks will fail. Re-run /edikt:init to regenerate, or substitute manually.\n",
			path, "${EDIKT_HOOK_DIR}")
		return 1, 0
	}

	home, herr := os.UserHomeDir()
	missing := 0
	for _, m := range hookCommandRe.FindAllStringSubmatch(content, -1) {
		resolved := m[1]
		if strings.Contains(resolved, "$HOME") {
			if herr != nil {
				fmt.Fprintf(out, "  WARN: %s references $HOME but the home directory could not be resolved (%v) — hook resolution is UNMEASURED\n",
					path, herr)
				warnN++
				continue
			}
			resolved = strings.Replace(resolved, "$HOME", home, 1)
		}
		if _, statErr := os.Stat(resolved); statErr != nil {
			fmt.Fprintf(out, "  ERROR: %s references hook %s which does not resolve to a file (%s) — that hook will fail\n",
				path, m[1], resolved)
			missing++
		}
	}
	return missing, warnN
}

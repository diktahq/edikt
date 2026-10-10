package cmd

// doctor_hook_paths.go — verifies every hook command registered in a
// settings.json actually resolves to a file on disk. The host runs each
// `command:` through a shell, which expands variables — but nothing sets
// EDIKT_HOOK_DIR in a hook's environment, so an unsubstituted
// `${EDIKT_HOOK_DIR}` placeholder expands to the empty string and fails
// every hook outright. A `$HOME` form (the global-mode install shape) is
// legitimate for the same reason — the shell expands it — but only if the
// resulting path actually exists.
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
	"github.com/diktahq/edikt/tools/edikt/internal/hook"
	"io"
	"os"
	"path/filepath"
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
		fmt.Fprintf(out, "  ERROR: %s contains unsubstituted placeholder %q — hooks will fail. %s\n",
			path, "${EDIKT_HOOK_DIR}", settingsRepairRemedy(path, projectRoot, "substitutes it in place"))
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
				// The host runs the command through a shell, so the file that
				// must exist is the script the shell would run, not the whole
				// string: `bash '<path>/x.sh' session` names <path>/x.sh. Quotes
				// are shell quoting (e.g. `"$CLAUDE_PROJECT_DIR"/.claude/hooks/x.sh`),
				// not part of the path.
				resolved = hookScriptPath(resolved)
				if _, statErr := os.Stat(resolved); statErr != nil {
					if name := retiredHookName(projectRoot, raw); name != "" {
						// A 0.8.x project after the binary upgrades and before
						// /edikt:upgrade runs: the shim is gone with the release
						// that retired it, and the registration is stale, not
						// broken. The shim's absence is fail-open (the host
						// reports a missing command and continues), so this is
						// a WARN with the one remedy, not a missing-path RED.
						fmt.Fprintf(out, "  WARN: %s registers hook %s — registered hook retired in this version — run /edikt:upgrade to remove the entry\n",
							path, name)
						warnN++
						continue
					}
					fmt.Fprintf(out, "  🔴 ERROR: %s references hook %s which does not resolve to a file (%s) — that hook will fail; every event it is registered for stops firing\n",
						path, raw, resolved)
					missing++
				}
			}
		}
	}
	return missing, warnN
}

// settingsRepairRemedy names the repair that acts on the settings file a
// finding was about. doctor checks two files — the project's
// .claude/settings.json and the user-scope one under the claude root — and
// `install settings --repair` alone resolves only the second, so naming it
// for a project finding sent the user to repair a file that was not broken
// (#11, item 7). The project file gets `--repair --project`, which
// /edikt:upgrade runs unprompted; the user file gets plain `--repair`, which
// upgrade offers and asks about first. Both keep the
// `install settings --repair` substring.
func settingsRepairRemedy(path, projectRoot, what string) string {
	if isProjectSettingsFile(path, projectRoot) {
		return "Run /edikt:upgrade (its pre-flight runs `edikt install settings --repair --project`, which " + what + " in the project's file)."
	}
	return "This is the user-scope file: run `edikt install settings --repair` (with EDIKT_HOOK_DIR set when a placeholder is substituted), or let /edikt:upgrade offer it — it asks before touching user-scope settings."
}

// isProjectSettingsFile reports whether path is projectRoot's own
// .claude/settings.json. A relative path is resolved against the working
// directory, the way doctor reads it.
func isProjectSettingsFile(path, projectRoot string) bool {
	if projectRoot == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return samePath(abs, filepath.Join(projectRoot, ".claude", "settings.json"))
}

// retiredHookName reports the registry name of a settings.json hook command
// whose shim this release retired (registry.yaml `retired:`), or "" when the
// command is not a retired hook — including when no registry resolves, in
// which case nothing can be said and the caller falls through to its
// ordinary missing-path error.
func retiredHookName(projectRoot, command string) string {
	base := strings.TrimSuffix(filepath.Base(strings.TrimSpace(command)), ".sh")
	if base == "" {
		return ""
	}
	regPath, _, err := hook.ResolveRegistryPath(projectRoot)
	if err != nil {
		return ""
	}
	reg, err := hook.LoadRegistryFile(regPath)
	if err != nil {
		return ""
	}
	for _, name := range reg.RetiredNames() {
		if name == base {
			return name
		}
	}
	return ""
}

// hookInterpreters are the programs a hook command may name before its
// script. The script is then the first argument that is not a flag.
var hookInterpreters = map[string]bool{
	"bash": true, "sh": true, "zsh": true, "dash": true, "env": true,
	"python": true, "python3": true, "node": true, "ruby": true, "perl": true,
}

// hookScriptPath returns the file a hook command runs: it splits the command
// as a POSIX shell does (single quotes literal, double quotes grouping,
// backslash escapes outside single quotes), skips leading VAR=value
// assignments, steps over an interpreter and its flags, and returns the
// script argument. A command it cannot split returns unchanged.
func hookScriptPath(command string) string {
	words, ok := shellWords(command)
	if !ok || len(words) == 0 {
		return command
	}
	i := 0
	for i < len(words) && isEnvAssignment(words[i]) {
		i++
	}
	if i < len(words) && hookInterpreters[filepath.Base(words[i])] {
		i++
		for i < len(words) && (strings.HasPrefix(words[i], "-") || isEnvAssignment(words[i])) {
			i++
		}
	}
	if i >= len(words) {
		return command
	}
	return words[i]
}

func isEnvAssignment(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	for j, r := range w[:eq] {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || j > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// shellWords splits s into words the way a POSIX shell does for quoting. It
// reports false on an unterminated quote.
func shellWords(s string) ([]string, bool) {
	var words []string
	var cur strings.Builder
	inWord := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, false
			}
			cur.WriteString(s[i+1 : i+1+j])
			i += j + 1
			inWord = true
		case c == '"':
			i++
			for ; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				cur.WriteByte(s[i])
			}
			if i >= len(s) {
				return nil, false
			}
			inWord = true
		case c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
			inWord = true
		case c == ' ' || c == '\t' || c == '\n':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, true
}

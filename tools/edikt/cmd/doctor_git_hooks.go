package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EDIKT SHIPS EXACTLY ONE GIT HOOK — pre-push — and `edikt init` installs it
// into `.git/hooks/`. That matters for this check: setting `core.hooksPath`
// makes git ignore `.git/hooks` ENTIRELY, so a repository that configures a
// hooks path and installs edikt's hook the documented way runs neither. bok
// had never run a git hook in its history for this reason.
//
// The check therefore asserts outcomes, not settings (INV-014): does the  // edikt-guard:allow
// configured path resolve, does edikt's own hook sit where git will look, is
// what is there executable, and can a message check there actually fail.
const ediktGitHook = "pre-push"

// messageCheckSignals are the shapes a commit-message check takes. Matching
// prose is deliberately narrow: this reports a hook that READS the previous
// message, not any hook that mentions a commit.
var messageCheckSignals = []string{
	"git log -1 --format=%B",
	"git log -1 --pretty=%B",
	"git log --format=%B -1",
}

// gitConfigValue reads one config key from a repository, "" when unset or
// when this is not a repository at all.
func gitConfigValue(root, key string) string {
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func isGitRepo(root string) bool {
	return gitTopLevel(root) != ""
}

// gitTopLevel is the repository root, which is what a RELATIVE core.hooksPath
// resolves against — git reads it from the top level, not from the working
// directory. Resolving it against the cwd made this check report a missing
// hooks path from any subdirectory of a repository that has one.
func gitTopLevel(root string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// hookReadsPreviousMessage reports whether a hook script validates a commit
// message it reads from history rather than from $1.
func hookReadsPreviousMessage(path string) bool {
	f, err := os.Open(path) //nolint:gosec // path built from the repo's own config
	if err != nil {
		return false
	}
	defer f.Close()
	usesArg := false
	readsLog := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 512*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.Contains(line, "$1") {
			usesArg = true
		}
		for _, sig := range messageCheckSignals {
			if strings.Contains(line, sig) {
				readsLog = true
			}
		}
	}
	return readsLog && !usesArg
}

// runGitHooksPathCheck asserts the OUTCOME the repository's core.hooksPath
// produces: that it resolves, that the hooks edikt installed are there and
// executable, and that a message check sits where it can actually fail.
//
// Every remedy below is printed only after its precondition is established
// (FR-032) — doctor never prescribes an action it has not tested.
// runGitHooksPathCheck asserts the OUTCOME the repository's git hook
// configuration produces. Every remedy is printed only after its precondition
// is established (FR-032) — doctor never prescribes an action it has not
// tested.
func runGitHooksPathCheck(projectRoot string, out io.Writer) (errN, warnN int) {
	if !isGitRepo(projectRoot) {
		return 0, 0 // nothing to measure; not a finding
	}
	top := gitTopLevel(projectRoot)
	if top == "" {
		return 0, 0
	}
	configured := gitConfigValue(projectRoot, "core.hooksPath")

	effective := filepath.Join(top, ".git", "hooks")
	if configured != "" {
		effective = configured
		if !filepath.IsAbs(effective) {
			effective = filepath.Join(top, effective)
		}
		info, err := os.Stat(effective)
		if err != nil || !info.IsDir() {
			fmt.Fprintf(out, "  🔴 ERROR: core.hooksPath is %s, which does not resolve to a directory (%s) — "+
				"ZERO git hooks run in this repository, and git says nothing about it. "+
				"Run: git config --unset core.hooksPath  (or create the directory and put the hooks in it)\n",
				configured, effective)
			return 1, 0
		}
	}

	// edikt's own hook, where init puts it, versus where git will look.
	installed := filepath.Join(top, ".git", "hooks", ediktGitHook)
	if _, err := os.Stat(installed); err == nil && configured != "" {
		if _, err := os.Stat(filepath.Join(effective, ediktGitHook)); err != nil {
			fmt.Fprintf(out, "  🔴 ERROR: edikt's %s hook is installed at .git/hooks/%s, but core.hooksPath is %s — "+
				"git reads only the configured path, so edikt's hook never runs. "+
				"Run: cp .git/hooks/%s %s/\n",
				ediktGitHook, ediktGitHook, configured, ediktGitHook, configured)
			errN++
		}
	}

	// Whatever IS in the effective directory: can it run, and can it fail?
	entries, err := os.ReadDir(effective)
	if err != nil {
		return errN, warnN
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".sample") {
			continue
		}
		p := filepath.Join(effective, e.Name())
		fi, serr := os.Stat(p)
		if serr != nil {
			continue
		}
		if fi.Mode()&0o111 == 0 {
			fmt.Fprintf(out, "  WARN: %s in the hooks path is not executable — git skips it silently\n", e.Name())
			warnN++
			continue
		}
		// The can't-fail shape: a message check in a hook that does not
		// receive the message path as $1.
		if e.Name() != "commit-msg" && hookReadsPreviousMessage(p) {
			fmt.Fprintf(out, "  🔴 ERROR: %s validates a commit message read from `git log`, not the one being written — "+
				"only a hook that receives the message path as $1 (commit-msg) can fail the commit being made, "+
				"so this check passes whatever was just typed. Move it to commit-msg and read $1\n", e.Name())
			errN++
		}
	}
	return errN, warnN
}

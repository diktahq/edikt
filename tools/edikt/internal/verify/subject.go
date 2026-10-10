package verify

// subject.go — SPEC-018 SR-004. A verify whose subject cannot be read reports  edikt-guard:allow
// UNMEASURED, never passed.
//
// WHY A PRE-FLIGHT RATHER THAN A STRICTER EXIT RULE
//
// The exit code cannot carry this. 42 of 158 verify commands begin with `!`,
// which is correct — absence IS the assertion for a prohibition ("this pattern
// must not appear"). But `!` also flips the exit that a MISSING file produces,
// so "the forbidden thing is absent" and "the file I was checking is gone"
// arrive as the same 0. Proven by execution before this landed:
// `! rg -l 'FORBIDDEN' src/*.go` exits 0 with src present, 0 after rm -rf src,
// and 0 after chmod 000 src.
//
// So the subject is established BEFORE the command runs, where the two cases
// are still distinguishable.
//
// DELIBERATELY CONSERVATIVE. A false UNMEASURED is a gate that cries wolf, and
// this runs over every verify in the corpus. A token is only treated as a
// subject when it is unambiguously a repo path: it contains a separator, is not
// a flag, and is not one of the tool-pattern forms (`./...`, `**`) that name no
// single file. Anything else is left alone and the command runs as before.

import (
	"os"
	"path/filepath"
	"strings"
)

// unreadableSubjects returns the first path the command names that cannot be
// read, or "" when every named path is readable.
func unreadableSubjects(cwd, command string) string {
	// test/lib/gotest.sh runs `go test` FROM tools/edikt, so the package
	// arguments after it (`./internal/hook/`, `./cmd/`) are paths relative to
	// that directory, not to the repo root the runner sits in. Resolving them
	// against the wrong root reported every gotest-backed criterion as
	// "subject unreadable" — an unmeasured verdict for a subject that exists.
	// The helper's contract is fixed and documented in its header, so the
	// pre-flight follows it rather than guessing from the token shape.
	// TWO DIFFERENT ROOTS, and conflating them is what broke this before.
	//
	//   shellRoot   — where the shell is. `cd tools/edikt && …` moves it, and
	//                 EVERY path after that resolves there, exactly as the
	//                 shell would resolve it.
	//   wrapperRoot — where test/lib/gotest.sh interprets its `./pkg/`
	//                 arguments (tools/edikt). The shell's cwd does NOT move,
	//                 so a later `&& bash test/x.sh` is back at the project
	//                 root. It applies to that helper's arguments only.
	shellRoot := cwd
	wrapperRoot := ""
	toks := strings.Fields(command)
	skipNext := false
	for i, tok := range toks {
		if skipNext {
			skipNext = false // the cd's own argument named the new root
			continue
		}
		// wrapperRoot governs test/lib/gotest.sh's OWN arguments and nothing
		// past them. `&&`, `||`, `;`, `|` and a subshell `(` each start a new
		// command, and paths in that command resolve against the shell's cwd
		// again. Without this the root leaked across the `&&` in
		// `… gotest.sh './internal/hook/…' 'Test' && ( ./test/run.sh …`,
		// because the only existing reset needed a repo-path token NOT
		// starting with "./" and every token in between (`'Test'`, `&&`, `(`)
		// takes an earlier `continue`. `./test/run.sh` was then looked for
		// under tools/edikt/, where it does not exist, and a criterion whose
		// subject was present reported UNMEASURED — the cry-wolf failure this
		// file's header calls worse than the hole it closes.
		if isCommandSeparator(tok) {
			wrapperRoot = ""
			continue
		}
		p := strings.Trim(tok, "'\"`;|&()")
		if p == "cd" && i+1 < len(toks) {
			dir := strings.Trim(toks[i+1], "'\"`;|&()")
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(shellRoot, dir)
			}
			shellRoot = dir
			wrapperRoot = ""
			skipNext = true
			continue
		}
		if p == "test/lib/gotest.sh" {
			wrapperRoot = filepath.Join(shellRoot, "tools", "edikt")
			continue
		}
		if !looksLikeRepoPath(p) {
			// `go`, `test`, `&&`: not a path, and it says nothing about
			// where the following paths resolve.
			continue
		}
		subjectRoot := shellRoot
		if wrapperRoot != "" && strings.HasPrefix(p, "./") {
			subjectRoot = wrapperRoot
		} else {
			wrapperRoot = "" // past the helper's own arguments
		}
		// A glob names a set, not a file. Its containing directory is the
		// thing that must exist — `src/*.go` says nothing about which files
		// are in src/, but it does assume src/ is there.
		check := p
		if i := strings.IndexAny(p, "*?["); i >= 0 {
			check = filepath.Dir(p[:i])
			if check == "." || check == "" {
				continue
			}
		}
		abs := check
		if !filepath.IsAbs(abs) && cwd != "" {
			abs = filepath.Join(subjectRoot, check)
		}
		f, err := os.Open(abs)
		if err != nil {
			return check
		}
		f.Close()
	}
	return ""
}

// isCommandSeparator reports whether tok ends the simple command it follows.
// Only whole-token operators count: `2>&1` is a redirection inside a command,
// not a break between two, and `done;` does end one.
func isCommandSeparator(tok string) bool {
	switch tok {
	case "&&", "||", ";", "|", "&", "(", ")", "{", "}":
		return true
	}
	return strings.HasSuffix(tok, ";")
}

// looksLikeRepoPath is the conservative half, and it is deliberately strict.
//
// The first draft was not strict enough and the corpus said so immediately: it
// held 25 verifies across 19 artifacts, and the "missing paths" included
// `2>/dev/null`, `origin`, `build|install)|tools/edikt` and a pytest node id.
// A gate that cries wolf on a fifth of the corpus is worse than the hole it
// closes, so the rule is now: a token is a subject only if it is unmistakably
// one, and everything else runs exactly as before.
func looksLikeRepoPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "-") {
		return false // a flag
	}
	if !strings.Contains(p, "/") {
		return false // a bare word: a subcommand, a remote, a binary
	}
	// Shell plumbing. `2>/dev/null` and `>/dev/null` contain a slash and are
	// not subjects.
	if strings.ContainsAny(p, "><") {
		return false
	}
	// Regex and shell syntax. An alternation or an escape means this token is a
	// pattern being matched, not a file being read.
	if strings.ContainsAny(p, "|\\()$`") {
		return false
	}
	if strings.Contains(p, "...") || strings.Contains(p, "://") || strings.Contains(p, "::") {
		return false // go ./... , a URL, a pytest node id
	}
	if strings.HasPrefix(p, "/") {
		return false // absolute paths are not this repo's relative subject
	}
	// Must name a file with an extension, or a directory. Anything else is too
	// ambiguous to hold a verify on.
	base := p
	if i := strings.IndexAny(p, "*?["); i >= 0 {
		base = p[:i]
	}
	if strings.HasSuffix(base, "/") {
		return true
	}
	return strings.Contains(filepath.Base(base), ".")
}

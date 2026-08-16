package sidecar

import (
	"strings"
)

// Precondition extraction for `verify:` commands.
//
// A verify command asserts that a rule HOLDS. When it references a file that
// is not in the tree, the command exits non-zero for a reason that has
// nothing to do with the rule — and the runner records `failed`, which is
// the same thing it records for a genuine violation.
//
// That conflation caused a real failure: greenfield extraction emitted
// `rg -q '…' internal/rag/chunk.go` for a file absent from the tree, rg
// exited non-zero, the criterion scored FAIL, and COMPILE_EXIT was 1. A
// missing file read as a governance violation.
//
// "PRECONDITION ABSENT" and "RULE VIOLATED" are different states. Nothing
// distinguishes them today. This file extracts the path operands so they
// can be checked before the command is ever run.

// ─── Tokenizer ────────────────────────────────────────────────────────────

// shTok is one token of a verify command, carrying whether the shell would
// have treated it as a literal.
//
// Quoting is the single most important signal here, and dropping it is what
// makes a naive scan useless: `grep -q '.edikt/state/.evidence-reads'
// .gitignore` contains TWO slash-bearing tokens and only the second is a
// path. The first is a regex. A tokenizer that forgets which was quoted
// reports a missing file that was never a file.
type shTok struct {
	text   string
	quoted bool
}

// tokenizeShell splits a command on unquoted whitespace, honoring single
// quotes, double quotes and backslash escapes, and recursing into `$(…)`
// command substitutions.
//
// It is NOT a shell parser and does not try to be. It resolves exactly what
// the precondition question needs — which words survive as literals — and
// declines everything else by marking it unresolvable downstream.
func tokenizeShell(cmd string) []shTok {
	var out []shTok
	var cur strings.Builder
	curQuoted := false
	inSingle, inDouble := false, false
	depth := 0 // $( ) nesting
	var sub strings.Builder

	flush := func() {
		if cur.Len() > 0 {
			out = append(out, shTok{text: cur.String(), quoted: curQuoted})
			cur.Reset()
			curQuoted = false
		}
	}

	runes := []rune(cmd)
	for i := 0; i < len(runes); i++ {
		c := runes[i]

		// Inside a command substitution: buffer until the matching paren,
		// then tokenize the inside as its own command. Without this,
		// `test "$(grep -v '^#' tools/edikt/check/…exempt | …)"` hides a
		// real path inside a double-quoted token.
		if depth > 0 {
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth == 0 {
					out = append(out, tokenizeShell(sub.String())...)
					sub.Reset()
					continue
				}
			}
			sub.WriteRune(c)
			continue
		}

		if c == '$' && i+1 < len(runes) && runes[i+1] == '(' {
			depth = 1
			i++
			continue
		}

		switch {
		case c == '\\' && !inSingle && i+1 < len(runes):
			i++
			cur.WriteRune(runes[i])
			curQuoted = true
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			curQuoted = true
		case c == '"' && !inSingle:
			inDouble = !inDouble
			curQuoted = true
		case (c == ' ' || c == '\t' || c == '\n') && !inSingle && !inDouble:
			flush()
		default:
			cur.WriteRune(c)
		}
	}
	flush()
	return out
}

// ─── Path operand extraction ──────────────────────────────────────────────

// shellOperators are tokens that structure a command rather than name
// anything in the tree.
var shellOperators = map[string]bool{
	"&&": true, "||": true, "|": true, ";": true, "!": true,
	"(": true, ")": true, "{": true, "}": true, "then": true,
	"else": true, "fi": true, "do": true, "done": true, "[": true, "]": true,
}

// pathExts are the extensions this repo's verify commands actually name.
// Used only to catch an extension-bearing operand with no slash in it;
// anything containing "/" is already a candidate.
var pathExts = []string{
	".sh", ".go", ".md", ".yaml", ".yml", ".json", ".tmpl",
	".exempt", ".py", ".txt", ".lock", ".mod", ".sum",
}

// extensionlessPaths are repo-root files that name a path while carrying
// neither a slash nor an extension, so neither candidate rule sees them.
//
// A closed list rather than a general "starts with a dot" rule. Scanning
// the corpus for bare operands turned up exactly one real path (.gitignore)
// among six candidates — the others were `SPEC-009` and a Go test name,
// which a dotfile-shaped rule would not have caught anyway, and a broader
// rule would only add ways to be wrong. If a seventh appears, it is one
// line here and a test case beside it.
var extensionlessPaths = map[string]bool{
	".gitignore": true, ".gitattributes": true, ".dockerignore": true,
	".editorconfig": true, "Makefile": true, "Dockerfile": true,
}

// testFileFlags are the test(1) unary operators whose operand is a path.
// `test -f templates/…` is the single most common verify shape in the
// corpus, and its operand is a path even when it has no slash.
var testFileFlags = map[string]bool{
	"-f": true, "-x": true, "-e": true, "-d": true, "-s": true,
	"-r": true, "-w": true, "-L": true, "-h": true,
}

// nameOnlyPredicates are commands whose path operands are NAMES to be
// reasoned about, not files to be opened. `git check-ignore .edikt/state/x`
// asks whether that path WOULD be ignored and exits 0 for a path that does
// not exist — so a missing file is not a missing precondition.
//
// The corpus taught this: ADR-038 and ADR-050 both verify gitignore
// coverage of runtime state files that are absent by design, and the first
// version of this check reported both as broken preconditions.
var nameOnlyPredicates = map[string]bool{
	"check-ignore": true,
	"check-attr":   true,
}

// PathOperand is one candidate path found in a verify command.
type PathOperand struct {
	// Raw is the operand as written. Base is the directory it resolves
	// against, set when the command `cd`s first.
	Raw  string
	Base string

	// AssertedAbsent marks an operand under a NEGATED existence test
	// (`test ! -f X`). There, a missing file is the rule HOLDING, not a
	// missing precondition. ADR-044 verifies that a file ADR-044 deleted
	// stays deleted; flagging it inverts the finding.
	AssertedAbsent bool

	// Unresolvable marks an operand this check declines to judge: a shell
	// expansion, a glob, a `~`, an absolute path, or a name-only predicate
	// operand. Reported separately and NEVER as present — a path nobody
	// resolved is not a path somebody found.
	Unresolvable bool
	Reason       string
}

// Resolved returns the operand path joined to its `cd` base, slash-cleaned.
func (p PathOperand) Resolved() string {
	r := strings.TrimPrefix(p.Raw, "./")
	if p.Base == "" {
		return strings.TrimSuffix(r, "/")
	}
	return strings.TrimSuffix(strings.TrimSuffix(p.Base, "/")+"/"+r, "/")
}

// ExtractVerifyPaths returns the path operands a verify command references.
//
// The rules are deliberately conservative in one direction: an operand this
// cannot resolve is marked Unresolvable rather than guessed at, because the
// whole point of the check is to stop a non-answer being scored as an
// answer.
func ExtractVerifyPaths(cmd string) []PathOperand {
	toks := tokenizeShell(cmd)
	var out []PathOperand
	seen := map[string]bool{}

	// base tracks a leading `cd DIR &&`. INV-011's verify is
	// `cd tools/edikt && go test ./internal/phasea/ …`; resolving that
	// operand against the repo root reports a directory that is plainly
	// there as missing.
	base := ""
	// negate is armed by a `!`. It becomes assertAbsent ONLY when consumed
	// by a file-test flag. `! grep -q p FILE` is a negated MATCH, not a
	// negated existence claim — FILE still has to be there, and in fact a
	// negated grep over a missing file passes vacuously, which is a worse
	// problem than the one this check reports.
	negate := false
	assertAbsent := false
	// nameOnly is armed by a name-only predicate and applies until the next
	// command separator.
	nameOnly := false
	expectCd := false

	add := func(raw string, unresolvable bool, reason string) {
		if raw == "" {
			return
		}
		key := base + "\x00" + raw
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, PathOperand{
			Raw: raw, Base: base,
			AssertedAbsent: assertAbsent,
			Unresolvable:   unresolvable, Reason: reason,
		})
	}

	expectPath := false
	for _, t := range toks {
		text := t.text

		// A quoted operand is a pattern, a message, or data — never a path
		// this check will stat. `grep -q 'internal/phasea/runner.go' FILE`
		// names a path inside its PATTERN; treating it as a precondition
		// would report a file that the command never opens.
		if t.quoted {
			expectPath = false
			continue
		}
		if text == "" {
			continue
		}
		if text == "!" {
			negate = true
			continue
		}
		if shellOperators[text] {
			// A separator ends the reach of a name-only predicate, and of a
			// pending negation that never found its test.
			nameOnly = false
			negate = false
			continue
		}
		if text == "cd" {
			expectCd = true
			continue
		}
		if expectCd {
			expectCd = false
			if !strings.ContainsAny(text, "$`*?~") && !strings.HasPrefix(text, "/") {
				base = strings.TrimSuffix(text, "/")
			}
			continue
		}
		if nameOnlyPredicates[text] {
			nameOnly = true
			continue
		}
		// Redirections: >f, 2>f, >>f, <f, 2>&1, and the trailing ")" forms.
		if strings.HasPrefix(text, ">") || strings.HasPrefix(text, "<") ||
			strings.HasPrefix(text, "2>") || strings.HasPrefix(text, "&>") ||
			strings.HasPrefix(text, "1>") {
			expectPath = false
			continue
		}
		// VAR=value assignments name an environment, not a file.
		if i := strings.Index(text, "="); i > 0 && !strings.ContainsAny(text[:i], "/.-") {
			expectPath = false
			continue
		}

		if strings.HasPrefix(text, "-") {
			if testFileFlags[text] {
				expectPath = true
				assertAbsent = negate
				negate = false
			}
			continue
		}

		isCandidate := expectPath ||
			strings.Contains(text, "/") ||
			hasAnySuffix(text, pathExts) ||
			extensionlessPaths[text]
		if !isCandidate {
			expectPath, assertAbsent = false, false
			continue
		}

		switch {
		case nameOnly:
			add(text, true, "name-only predicate operand (need not exist)")
		case strings.ContainsAny(text, "$`"):
			add(text, true, "shell expansion")
		case strings.HasPrefix(text, "~"):
			add(text, true, "home-relative")
		case strings.ContainsAny(text, "*?["):
			add(text, true, "glob")
		case strings.HasPrefix(text, "/"):
			// Absolute paths are machine-specific. /dev/null and friends are
			// not corpus preconditions and stating otherwise would make the
			// check environment-dependent.
			add(text, true, "absolute path")
		default:
			add(text, false, "")
		}
		expectPath, assertAbsent = false, false
	}
	return out
}

func hasAnySuffix(s string, sfx []string) bool {
	for _, e := range sfx {
		if strings.HasSuffix(s, e) {
			return true
		}
	}
	return false
}

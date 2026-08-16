package cmd

// doctor_topic_scope.go — durability backstop for two topic-scope
// integrity problems a real grade-3/10 gov:compile/grade-compile run
// surfaced against a real consumer project. Both are authored-source problems
// (missing paths: globs), not compile bugs — but a project can reach
// either state silently, and `doctor`/`gov compile`'s only existing
// signal for them is a one-shot line in the compile summary
// (internal/govrun/twophase.go), which does not persist. This makes both
// a standing, repeatable `doctor` signal instead.
//
// SHADOW AMBIENT CORE (paths: "**"). scopeFor (internal/phaseb/merge.go)
// unions a topic's scope from its contributing sidecars, and ANY
// undeclared sidecar (no paths:) unscopes the WHOLE topic to `**` — set
// union with "everywhere" is "everywhere". That topic file then loads on
// every edit regardless of subject, the exact ambient-cost shape the
// 46k->3k reduction was built to remove. scopeFor already names the
// culprits in the topic file's own `<!-- scope: ... -->` comment; this
// check aggregates that per-file signal into one doctor-visible count so a
// reader doesn't have to open every topic file to notice.
//
// UNREACHABLE TOPICS. A topic every one of whose sidecars is undeclared
// retires to tier 3 entirely: no topic file at all, reached only via its
// skill package, which a host only loads on an explicit trigger match. For
// a security-sensitive topic this means the rule reaches a developer only
// if they think to invoke the skill first — never on an ordinary edit. A
// retired topic's signature is detectable purely from the filesystem: its
// skill package exists (every topic gets one) but its topic file does not.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var unscopedNoteRE = regexp.MustCompile(`<!-- scope: \*\* — (UNSCOPED because [^>]+?) -->`)

// runTopicScopeCheck reports (a) topics whose compiled topic file carries
// scopeFor's UNSCOPED marker (shadow ambient core) and (b) topics that
// have a skill package but no topic file (retired to tier 3, unreachable
// at write time). Returns the warning count and whether the check ran
// (false when the governance dir doesn't exist yet).
func runTopicScopeCheck(projectRoot string, out io.Writer) (warnCount int, ran bool) {
	govDir := filepath.Join(projectRoot, ".claude", "rules", "governance")
	skillsDir := filepath.Join(projectRoot, ".claude", "skills")

	govEntries, err := os.ReadDir(govDir)
	if err != nil {
		return 0, false
	}

	var shadowed []string
	topicFiles := map[string]bool{}
	for _, e := range govEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		topicFiles[name] = true
		body, err := os.ReadFile(filepath.Join(govDir, e.Name()))
		if err != nil {
			continue
		}
		if m := unscopedNoteRE.FindSubmatch(body); m != nil {
			shadowed = append(shadowed, fmt.Sprintf("%s (%s)", name, string(m[1])))
		}
	}
	sort.Strings(shadowed)

	var unreachable []string
	if skillEntries, err := os.ReadDir(skillsDir); err == nil {
		for _, e := range skillEntries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "edikt-") {
				continue
			}
			topic := strings.TrimPrefix(e.Name(), "edikt-")
			if !topicFiles[topic] {
				unreachable = append(unreachable, topic)
			}
		}
	}
	sort.Strings(unreachable)

	if len(shadowed) == 0 && len(unreachable) == 0 {
		return 0, true
	}

	if len(shadowed) > 0 {
		fmt.Fprintf(out, "  WARN: %d topic(s) carry a shadow ambient core — paths: \"**\", loading on every edit regardless of subject:\n", len(shadowed))
		for _, s := range shadowed {
			fmt.Fprintf(out, "    unscoped: %s\n", s)
		}
		warnCount++
	}
	if len(unreachable) > 0 {
		fmt.Fprintf(out, "  WARN: %d topic(s) are unreachable at write time — no topic file, reached only by explicit skill invocation: %s\n", len(unreachable), strings.Join(unreachable, ", "))
		warnCount++
	}
	return warnCount, true
}

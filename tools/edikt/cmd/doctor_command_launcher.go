package cmd

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A command body that names a repo-relative `bin/edikt` hands the agent a
// path a user's project does not have: the launcher is installed on PATH,
// and only edikt's own development checkout builds into bin/. The agent's
// natural recovery from "no such file" is to go hunting for the binary,
// which is the reach the tier boundary exists to prevent — so this is
// reported where the user can see it, not left to the release harness
// (bok round 2, D81 a: 32 shipped bodies named the dev path).
//
// `.edikt/bin/edikt` is the project-mode install path and is correct.
var devBinaryPathRe = regexp.MustCompile(`(^|[^./[:alnum:]_-])bin/edikt`)

// scanCommandLauncherPaths reports each installed command body that names
// the dev-only binary path, and how many lines named it. It reads files
// line by line: a command body is markdown of unbounded size and doctor
// must not load an arbitrary file whole.
func scanCommandLauncherPaths(commandsDir string, out io.Writer) (warnN int) {
	type hit struct {
		path string
		line int
		text string
	}
	var hits []hit
	err := filepath.WalkDir(commandsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil //nolint:nilerr // an unreadable entry is skipped, never fatal
		}
		f, oerr := os.Open(p) //nolint:gosec // path comes from the walk
		if oerr != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for n := 1; sc.Scan(); n++ {
			line := sc.Text()
			if strings.Contains(line, ".edikt/bin/edikt") {
				continue
			}
			if devBinaryPathRe.MatchString(line) {
				hits = append(hits, hit{p, n, strings.TrimSpace(line)})
			}
		}
		return nil
	})
	if err != nil {
		return 0
	}
	if len(hits) == 0 {
		return 0
	}
	files := map[string]bool{}
	for _, h := range hits {
		files[h.path] = true
	}
	fmt.Fprintf(out, "  WARN: %d installed command body(ies) name `bin/edikt`, a path this project does not have (%d line(s)) — the launcher is `edikt` on PATH; run /edikt:upgrade to refresh the commands\n",
		len(files), len(hits))
	shown := 0
	for _, h := range hits {
		if shown == 3 {
			fmt.Fprintf(out, "        … and %d more\n", len(hits)-shown)
			break
		}
		fmt.Fprintf(out, "        %s:%d: %s\n", h.path, h.line, truncateLine(h.text, 90))
		shown++
	}
	return 1
}

func truncateLine(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Package migrate is the home of everything that knows the legacy in-body
// sentinel format exists: the fence-aware scanner that detects it, and (from
// Phase 2 of PLAN-migration-blindness) the frozen lifter that reads it.
//
// ADR-082 (a): migration state is per artifact and is two facts — an
// unstripped legacy block present, a sidecar present. This file answers the
// first. It reports FILES, never versions, and it is the only migration-aware
// code a steady-state path may call.
package migrate

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LegacyOpen is the opener of the retired in-body directive block. The other
// two sentinel families — `[edikt:start]: #` (CLAUDE.md's managed block) and
// `<!-- edikt:status:start -->` — are live and are not this scanner's
// business.
const LegacyOpen = "[edikt:directives:start]: #"

// Finding is one unstripped legacy block: the file and the 1-based line its
// opener sits on.
type Finding struct {
	Path string
	Line int
}

// ScanFile returns the 1-based line of every LegacyOpen that sits at column
// zero OUTSIDE a fenced code block.
//
// Fence tracking follows the sentinel parser it replaces in the steady-state
// path (CommonMark: a closer must use the opener's character and be at least
// as long). A naive regex false-positives on INV-005, whose every mention of
// the marker is inside a fence, and on the artifacts whose blocks are
// documentary — design doc §7. That is why this is sixty lines and not
// twenty, and why it ships with a positive-control fixture pair.
func ScanFile(path string) ([]int, error) {
	f, err := os.Open(path) //nolint:gosec // caller-supplied artifact path
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []int
	inFence := false
	var openerChar byte
	openerLen := 0
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		n++
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if c, l := fencePrefix(trimmed); l >= 3 {
			switch {
			case !inFence:
				inFence, openerChar, openerLen = true, c, l
				continue
			case c == openerChar && l >= openerLen:
				inFence = false
				continue
			}
		}
		if inFence {
			continue
		}
		if strings.TrimRight(line, " \t\r") == LegacyOpen {
			lines = append(lines, n)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return lines, nil
}

// Scan walks every .md under each of dirs (relative to root, recursively) and
// returns the unstripped legacy blocks it finds, sorted by path then line.
// A dir that does not exist is skipped: absence of a directory is not a
// finding, and not an error.
func Scan(root string, dirs []string) ([]Finding, error) {
	var out []Finding
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		abs := filepath.Join(root, dir)
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		err := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".md") {
				return nil
			}
			lines, serr := ScanFile(p)
			if serr != nil {
				return serr
			}
			rel, rerr := filepath.Rel(root, p)
			if rerr != nil {
				rel = p
			}
			for _, l := range lines {
				out = append(out, Finding{Path: rel, Line: l})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

// Files returns the distinct paths in findings, in order.
func Files(findings []Finding) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range findings {
		if !seen[f.Path] {
			seen[f.Path] = true
			out = append(out, f.Path)
		}
	}
	return out
}

// fencePrefix reports the fence character and run length of a trimmed line
// that opens or closes a fenced code block, or (0, 0) when it is not a fence.
func fencePrefix(trimmed string) (byte, int) {
	if len(trimmed) == 0 {
		return 0, 0
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return 0, 0
	}
	n := 1
	for n < len(trimmed) && trimmed[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0
	}
	return c, n
}

package sidecar

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsStale reports whether the sidecar's recorded directive quotes still
// match the live parent .md prose, per ADR-028's drift contract:
// the sidecar is stale if any directive's source_excerpt.quote no longer
// appears at line_start..line_end in the parent body.
//
// projectRoot is used to resolve sidecar.path when it is relative.
// reason is empty when not stale; populated with the first violation found
// when stale (the dispatcher uses it for log lines, not control flow).
func (s *Sidecar) IsStale(projectRoot string) (stale bool, reason string, err error) {
	parentPath := s.Path
	if !filepath.IsAbs(parentPath) {
		parentPath = filepath.Join(projectRoot, s.Path)
	}
	data, err := os.ReadFile(parentPath)
	if err != nil {
		return false, "", fmt.Errorf("read parent %s: %w", parentPath, err)
	}
	lines := strings.Split(string(data), "\n")

	for i, d := range s.Directives {
		if d.SourceExcerpt.LineStart > len(lines) || d.SourceExcerpt.LineEnd > len(lines) {
			return true, fmt.Sprintf("directive[%d]: lines %d-%d outside body length %d",
				i, d.SourceExcerpt.LineStart, d.SourceExcerpt.LineEnd, len(lines)), nil
		}
		sliceStart := d.SourceExcerpt.LineStart - 1
		sliceEnd := d.SourceExcerpt.LineEnd
		passage := strings.Join(lines[sliceStart:sliceEnd], "\n")
		if !strings.Contains(passage, strings.TrimSpace(d.SourceExcerpt.Quote)) {
			return true, fmt.Sprintf("directive[%d]: quote not found at lines %d-%d",
				i, d.SourceExcerpt.LineStart, d.SourceExcerpt.LineEnd), nil
		}
	}
	return false, "", nil
}

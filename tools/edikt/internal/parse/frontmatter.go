// Package parse extracts YAML frontmatter and edikt sentinel blocks from
// markdown source documents.
package parse

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter is the YAML header of an ADR/INV/guideline source document.
// Fields are optional — legacy documents may lack any or all of them.
type Frontmatter struct {
	Type        string   `yaml:"type"`
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Status      string   `yaml:"status"`
	Supersedes  string   `yaml:"supersedes,omitempty"`
	NoDirectives string  `yaml:"no-directives,omitempty"`
	// BoldStatus is read from the `**Status:** …` line in the body when
	// frontmatter is absent (legacy INVs).
}

// Document is a parsed source file: path + frontmatter + body + sentinel.
type Document struct {
	Path        string
	Frontmatter Frontmatter
	// BoldStatus holds the status detected from a bolded `**Status:**` line in
	// the body when no frontmatter `status:` is present. Used for legacy
	// INV-001/INV-002 that pre-date the frontmatter convention.
	BoldStatus string
	Body       string   // full file contents
	Sentinel   Sentinel // the [edikt:directives:start/end] block, if any
}

// LoadDocument reads a file from disk, parses its frontmatter and sentinel
// block, and returns a Document. Non-existent files or unreadable ones
// return an error.
func LoadDocument(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	body := normalizeLineEndings(string(data))

	fm, err := parseFrontmatter(body)
	if err != nil {
		return nil, fmt.Errorf("frontmatter %s: %w", path, err)
	}

	boldStatus := extractBoldStatus(body)

	sentinel, err := ExtractSentinel(body)
	if err != nil {
		return nil, fmt.Errorf("sentinel %s: %w", path, err)
	}

	return &Document{
		Path:        path,
		Frontmatter: fm,
		BoldStatus:  boldStatus,
		Body:        body,
		Sentinel:    sentinel,
	}, nil
}

// LoadDocuments walks absDir (an absolute or relative-to-CWD path on disk)
// and returns every *.md file parsed as a Document, sorted by path.
// A missing directory returns an empty slice, not an error.
// The fsys parameter is accepted for API compatibility but currently unused;
// future tests can pass an in-memory FS when the implementation is updated.
func LoadDocuments(_ fs.FS, absDir string) ([]*Document, error) {
	var entries []string
	err := filepath.WalkDir(absDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return filepath.SkipAll
			}
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".md") {
			entries = append(entries, p)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sortStringsStable(entries)

	docs := make([]*Document, 0, len(entries))
	for _, p := range entries {
		doc, err := LoadDocument(p)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// IsIncluded reports whether a document should be included in the compile
// output. Applies the status filter from compile.md §3:
//   - ADRs: include if status == "accepted". Fallback: body has **Status:** Accepted.
//   - INVs: include if status == "active" OR no status (backwards compat). Skip "revoked".
//   - Guidelines: include unconditionally.
//
// kind is one of: "adr", "inv", "guideline".
func (d *Document) IsIncluded(kind string) bool {
	fmStatus := strings.ToLower(strings.TrimSpace(d.Frontmatter.Status))
	boldStatus := strings.ToLower(strings.TrimSpace(d.BoldStatus))

	switch kind {
	case "adr":
		if fmStatus == "accepted" {
			return true
		}
		if fmStatus == "" && boldStatus == "accepted" {
			return true
		}
		return false
	case "inv":
		if fmStatus == "" && boldStatus == "" {
			return true // legacy, no status means active
		}
		if fmStatus == "active" || boldStatus == "active" {
			return true
		}
		return false
	case "guideline":
		return true
	default:
		return false
	}
}

// normalizeLineEndings converts CRLF to LF for deterministic hashing.
func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// parseFrontmatter extracts the YAML between the first two `---` fences.
// Returns an empty struct (no error) if the document has no frontmatter.
func parseFrontmatter(body string) (Frontmatter, error) {
	var fm Frontmatter
	if !strings.HasPrefix(body, "---\n") {
		return fm, nil
	}
	rest := body[4:]
	end := strings.Index(rest, "\n---\n")
	if end == -1 {
		return fm, nil // malformed but not an error at parse time
	}
	yamlBytes := []byte(rest[:end])
	if bytes.TrimSpace(yamlBytes) == nil {
		return fm, nil
	}
	if err := yaml.Unmarshal(yamlBytes, &fm); err != nil {
		return fm, err
	}
	return fm, nil
}

// extractBoldStatus pulls "**Status:** <value>" from the first 40 lines of
// the body. Matches INV-001/INV-002 legacy header form and ADRs written
// before the YAML frontmatter convention landed.
func extractBoldStatus(body string) string {
	lines := strings.SplitN(body, "\n", 40)
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "**Status:**") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(trim, "**Status:**"))
		return v
	}
	return ""
}

func sortStringsStable(ss []string) {
	// Stable alphabetical sort. Using stdlib sort with a cmp func would pull
	// in sort; tiny inlined impl keeps the dep tree minimal.
	// (Caller uses this on filesystem walks, small lists.)
	n := len(ss)
	for i := 1; i < n; i++ {
		j := i
		for j > 0 && ss[j-1] > ss[j] {
			ss[j-1], ss[j] = ss[j], ss[j-1]
			j--
		}
	}
}

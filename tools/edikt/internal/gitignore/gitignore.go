// Package gitignore implements the AC-019 .gitignore management
// previously embedded as a Python heredoc in commands/gov/compile.md
// (lines 383-405 pre-Phase 11.5).
//
// EnsureEntry appends an entry to the project's .gitignore unless one
// of its line-equal variants is already present. Trailing-slash
// normalisation is honoured — `.edikt/state/` and `.edikt/state` are
// treated as equivalent, matching the original python script's
// `for variant in [...]` loop.
package gitignore

import (
	"os"
	"path/filepath"
	"strings"
)

// Outcome reports what EnsureEntry did. Useful for human-readable
// summaries and tests.
type Outcome int

const (
	// AlreadyPresent means an exact-line variant of entry was found.
	AlreadyPresent Outcome = iota
	// Appended means the entry was appended to an existing file.
	Appended
	// Created means the .gitignore file was created with the entry.
	Created
)

// EnsureEntry ensures `entry` is line-present in projectRoot/.gitignore.
// Returns the outcome and a wrapped I/O error if the file could not be
// read or written. The caller decides how to handle a read/write
// failure (the python script downgrades errors to non-blocking).
func EnsureEntry(projectRoot, entry string) (Outcome, error) {
	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return 0, err
		}
		// Fresh file.
		body := entry + "\n"
		if werr := os.WriteFile(gitignorePath, []byte(body), 0o644); werr != nil {
			return 0, werr
		}
		return Created, nil
	}

	content := string(data)
	// Build the set of variants that count as "already present".
	// The original python script tested both `.edikt/state/` and
	// `.edikt/state`; we generalise to {entry, entry without trailing
	// slash, entry with trailing slash}.
	variants := []string{entry}
	stripped := strings.TrimRight(entry, "/")
	if stripped != entry {
		variants = append(variants, stripped)
	} else {
		variants = append(variants, entry+"/")
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// Ignore comments and inline whitespace; the python original
		// matched on the raw line but `splitlines` strips the trailing
		// newline only, so trim trailing whitespace here too.
		trimmed := strings.TrimRight(line, " \t\r")
		for _, v := range variants {
			if trimmed == v {
				return AlreadyPresent, nil
			}
		}
	}

	// Append. Insert a leading newline if the file is non-empty and
	// does not already end with one (matches the python behaviour).
	prefix := ""
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		prefix = "\n"
	}
	body := content + prefix + entry + "\n"
	if werr := os.WriteFile(gitignorePath, []byte(body), 0o644); werr != nil {
		return 0, werr
	}
	return Appended, nil
}

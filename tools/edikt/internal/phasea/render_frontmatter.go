package phasea

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RenderExtractorFrontmatter renders the sidecar-extractor agent's
// frontmatter from extractor.model config.
//
// Under "inherit" the returned bytes carry no `model:` key at all — the
// subagent then inherits whatever model dispatched it. Under an explicit
// value, the returned bytes carry `model: <value>` immediately after
// `prompt_version:` (matching the shipped template's own historical
// position for that key).
//
// This is the ONE function both Phase A (before every dispatch) and
// `edikt upgrade`'s agent-reconciliation comparison in commands/upgrade.md
// call: two independent implementations of "what should the model: line
// be" is exactly the shape a reconciliation guard must never take — it
// must read the resolved outcome, not re-derive it from the one place
// that decides it.
//
// templateBytes is expected to be a well-formed agent file: a `---\n`
// fence, frontmatter, a `\n---\n` closing fence, then the body. A file
// with no frontmatter fence is returned unchanged with an error — this
// function never guesses at a malformed agent file's structure.
func RenderExtractorFrontmatter(templateBytes []byte, extractorModel string) ([]byte, error) {
	body := strings.ReplaceAll(string(templateBytes), "\r\n", "\n")
	if !strings.HasPrefix(body, "---\n") {
		return templateBytes, fmt.Errorf("no frontmatter fence")
	}
	rest := body[4:]
	end := strings.Index(rest, "\n---\n")
	if end == -1 {
		return templateBytes, fmt.Errorf("unterminated frontmatter fence")
	}
	frontmatter := rest[:end]
	after := rest[end:] // starts with "\n---\n"

	lines := strings.Split(frontmatter, "\n")
	out := make([]string, 0, len(lines)+1)
	promptVersionIdx := -1
	for _, line := range lines {
		if strings.HasPrefix(line, "model:") {
			// Drop any existing model: line — including a stale one under
			// an old explicit pin — rather than editing it in place. The
			// insertion point below is always computed fresh.
			continue
		}
		out = append(out, line)
		if strings.HasPrefix(line, "prompt_version:") {
			promptVersionIdx = len(out) - 1
		}
	}

	if extractorModel != ExtractorModelInherit {
		modelLine := "model: " + extractorModel
		if promptVersionIdx >= 0 {
			out = append(out[:promptVersionIdx+1], append([]string{modelLine}, out[promptVersionIdx+1:]...)...)
		} else {
			// No prompt_version: line to anchor after (an unusual agent
			// file, or a caller testing a minimal fixture) — append at the
			// end of the frontmatter rather than refusing.
			out = append(out, modelLine)
		}
	}

	return []byte("---\n" + strings.Join(out, "\n") + after), nil
}

// RenderInstalledExtractorFrontmatter re-renders the project-local installed
// sidecar-extractor.md's frontmatter from extractorModel and writes it back
// if the bytes changed. A missing installed file is not an error — there is
// nothing to render, and the caller's own resolution (ResolveExtractorAgentModel)
// already has a defined UNKNOWN path for that case.
//
// Called before every Phase A dispatch, so what ResolveExtractorAgentModel
// reads immediately afterward always reflects the current config, never a
// stale prior render.
func RenderInstalledExtractorFrontmatter(projectRoot, extractorModel string) error {
	path := filepath.Join(projectRoot, ExtractorAgentRelPath)
	current, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	rendered, err := RenderExtractorFrontmatter(current, extractorModel)
	if err != nil {
		return fmt.Errorf("render %s: %w", path, err)
	}
	if string(rendered) == string(current) {
		return nil
	}
	if err := os.WriteFile(path, rendered, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

package cmd

// doctor_agents.go — agent-definition drift check.
//
// Field failure (bok-services 2026-08-07): a stale user-level copy of
// sidecar-extractor.md (old maxTurns, old prompt) silently shadowed the
// payload template. Claude Code caches agent definitions at session start,
// so every extractor dispatch in the session produced zero files with an
// empty final response — ~30k tokens per round, no error anywhere. Doctor
// is where a user looks first, so doctor must surface the skew.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// agentDriftWarning describes one installed agent copy that differs from
// the active payload template.
type agentDriftWarning struct {
	Slug          string
	InstalledPath string
	TemplatePath  string
}

// checkAgentDrift compares every agent template under
// <ediktRoot>/templates/agents/ against installed copies in the project
// (.claude/agents/) and the Claude root (<claudeRoot>/agents/). A copy that
// exists, differs byte-wise from the template, and does not declare
// `<!-- edikt:custom -->` is reported as drifted. Missing copies are fine —
// project installs are optional and per-repo.
func checkAgentDrift(ediktRoot, claudeRoot, projectRoot string) []agentDriftWarning {
	tmplDir := filepath.Join(ediktRoot, "templates", "agents")
	entries, err := os.ReadDir(tmplDir)
	if err != nil {
		return nil
	}

	installDirs := []string{
		filepath.Join(projectRoot, ".claude", "agents"),
		filepath.Join(claudeRoot, "agents"),
	}

	var warns []agentDriftWarning
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, "_") {
			continue
		}
		tmplPath := filepath.Join(tmplDir, name)
		tmplBody, err := os.ReadFile(tmplPath)
		if err != nil {
			continue
		}
		for _, dir := range installDirs {
			installed := filepath.Join(dir, name)
			body, err := os.ReadFile(installed)
			if err != nil {
				continue
			}
			if bytes.Equal(body, tmplBody) {
				continue
			}
			if bytes.Contains(body, []byte("<!-- edikt:custom -->")) {
				continue
			}
			warns = append(warns, agentDriftWarning{
				Slug:          strings.TrimSuffix(name, ".md"),
				InstalledPath: installed,
				TemplatePath:  tmplPath,
			})
		}
	}
	return warns
}

// runAgentDriftCheck prints the drift warnings and returns the warning
// count for doctor's tally.
func runAgentDriftCheck(ediktRoot, claudeRoot, projectRoot string, out io.Writer) int {
	warns := checkAgentDrift(ediktRoot, claudeRoot, projectRoot)
	for _, w := range warns {
		fmt.Fprintf(out,
			"  WARN: agent %q at %s differs from the active payload template (%s).\n"+
				"        Claude Code caches agent definitions at session start — a stale copy causes\n"+
				"        silent zero-file extractor dispatches. Update it (or mark it <!-- edikt:custom -->),\n"+
				"        then restart the Claude session.\n",
			w.Slug, w.InstalledPath, w.TemplatePath)
	}
	return len(warns)
}

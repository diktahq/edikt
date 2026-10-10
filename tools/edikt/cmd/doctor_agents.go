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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

// agentDriftWarning describes one installed agent copy that differs from
// the active payload template.
// md5Hex is the install-time identity of a template's RAW bytes — the same
// hash `edikt init` stamps into an installed agent's frontmatter. It is a
// provenance key, never a security boundary.
func md5Hex(b []byte) string {
	sum := md5.Sum(b) //nolint:gosec // provenance identity, not a security boundary
	return hex.EncodeToString(sum[:])
}

// frontmatterValue reads one scalar key from a leading YAML frontmatter
// block, "" when the block or the key is absent.
func frontmatterValue(body []byte, key string) string {
	text := string(body)
	if !strings.HasPrefix(text, "---") {
		return ""
	}
	end := strings.Index(text[3:], "\n---")
	if end < 0 {
		return ""
	}
	for _, line := range strings.Split(text[3:3+end], "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+":") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, key+":")), "\"'")
		}
	}
	return ""
}

type agentDriftWarning struct {
	Slug          string
	InstalledPath string
	TemplatePath  string
	// NoProvenance records that the comparison was byte-wise because the
	// installed file carries no edikt_template_hash. The report says so: a
	// byte difference there may be edikt's own install-time tailoring.
	NoProvenance bool
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

	// sidecar-extractor.md's model: line is config-derived, not a template
	// deviation — an installed copy with a model: line the shipped template
	// does not carry (or vice versa) is Phase A's own rendering, never
	// drift. Resolved once per check, reused for every candidate install
	// dir below, via the same RenderExtractorFrontmatter function Phase A
	// itself calls before every dispatch: this guard reads the resolved
	// outcome, it does not re-derive it.
	extractorModel := phasea.ExtractorModelInherit
	if cfg, cerr := phasea.LoadConfig(projectRoot); cerr == nil {
		extractorModel = cfg.ExtractorModel
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
		compareBody := tmplBody
		if name == filepath.Base(phasea.ExtractorAgentRelPath) {
			if rendered, rerr := phasea.RenderExtractorFrontmatter(tmplBody, extractorModel); rerr == nil {
				compareBody = rendered
			}
		}
		for _, dir := range installDirs {
			installed := filepath.Join(dir, name)
			body, err := os.ReadFile(installed)
			if err != nil {
				continue
			}
			if bytes.Contains(body, []byte("<!-- edikt:custom -->")) {
				continue
			}
			// PROVENANCE FIRST, bytes only as a fallback.
			//
			// edikt TRANSFORMS a template as it installs it: substitutions
			// applied, stack blocks filtered, the provenance stamp added. So
			// the installed file is byte-different from the template by
			// design, and comparing raw bytes reported twelve correctly
			// installed agents as drifted on a consumer project (D83 e).
			// `edikt_template_hash` is the md5 of the RAW template the
			// install came from — the same identity the upgrade agents pass
			// uses — so equality there means "installed from this template",
			// whatever the bytes became afterwards.
			stored := frontmatterValue(body, "edikt_template_hash")
			if stored != "" {
				if stored == md5Hex(tmplBody) {
					continue
				}
				warns = append(warns, agentDriftWarning{
					Slug:          strings.TrimSuffix(name, ".md"),
					InstalledPath: installed,
					TemplatePath:  tmplPath,
				})
				continue
			}
			// No provenance: a pre-v0.6.0 install. Bytes are all there is,
			// and the report says so rather than implying certainty.
			// compareBody is tmplBody as-is for every agent except
			// sidecar-extractor.md, where it is the config-rendered
			// expectation (see above) so a model: line present or absent
			// by configuration is never read as a byte-level deviation.
			if bytes.Equal(body, compareBody) {
				continue
			}
			warns = append(warns, agentDriftWarning{
				Slug:          strings.TrimSuffix(name, ".md"),
				InstalledPath: installed,
				TemplatePath:  tmplPath,
				NoProvenance:  true,
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
		how := "its provenance hash names a different template"
		if w.NoProvenance {
			how = "no provenance stamp, so this is a byte comparison — the difference may be edikt's own install-time tailoring"
		}
		fmt.Fprintf(out,
			"  WARN: agent %q at %s differs from the active payload template (%s; %s).\n"+
				"        Claude Code caches agent definitions at session start — a stale copy causes\n"+
				"        silent zero-file extractor dispatches. Update it (or mark it <!-- edikt:custom -->),\n"+
				"        then restart the Claude session.\n",
			w.Slug, w.InstalledPath, w.TemplatePath, how)
	}
	return len(warns)
}

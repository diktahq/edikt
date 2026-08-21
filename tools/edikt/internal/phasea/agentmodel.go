package phasea

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/diktahq/edikt/tools/edikt/internal/claudepaths"
	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
)

// ExtractorAgentRelPath is where the locked extractor agent is installed in a
// project. Its frontmatter `model:` is the authoritative pin (ADR-055 §1).
const ExtractorAgentRelPath = ".claude/agents/sidecar-extractor.md"

// candidateAgentPaths returns every location the sidecar-extractor agent
// might resolve from, in resolution order: project-local first (what a
// project explicitly installed under .claude/agents/), then the active
// Claude profile's own agents directory — the same claudepaths.ResolveClaudeRoot()
// chain doctor's own agent-drift check already uses (cmd/doctor_agents.go).
//
// Real dispatch (phasea/runner.go) shells out to the external `claude` CLI,
// which resolves subagents opaquely, outside this codebase. There is no
// in-repo "broader resolver" to mirror exactly — but a project-local-only
// check is narrower than what a real Claude Code session can actually see,
// and that narrowness is exactly what let a resolvable-elsewhere agent
// report as a hard failure instead of a degraded UNKNOWN (F4/F5,
// docs/internal/issues/agentmodel-resolver-no-global-fallback.md).
func candidateAgentPaths(projectRoot string) []string {
	return []string{
		filepath.Join(projectRoot, ExtractorAgentRelPath),
		filepath.Join(claudepaths.ResolveClaudeRoot(), "agents", "sidecar-extractor.md"),
	}
}

// ExtractorModelUnknown is what a run reports when it cannot determine which
// model will perform extraction.
//
// It is a distinct value and NEVER a fallback. Substituting
// DefaultExtractorModel — or the CLI's --model value — is exactly how ADR-054
// came to print `claude-opus-5` over work Sonnet did: a value that was not
// measured was rendered as one that was. INV-013 forbids that shape, and
// ADR-055 §4 names this specific case.
const ExtractorModelUnknown = "UNKNOWN"

// ErrNoAgentModel reports that the extractor agent exists but pins no model.
var ErrNoAgentModel = errors.New("extractor agent declares no model key")

type agentFrontmatter struct {
	Model         string `yaml:"model"`
	PromptVersion int    `yaml:"prompt_version"`
}

// ExtractorPromptVersionUnknown is what a caller reports when the extraction
// contract's version cannot be read. Same rule as ExtractorModelUnknown: it is
// a distinct value, never a fallback, and a caller that needs the version to
// mean something must refuse rather than substitute one.
const ExtractorPromptVersionUnknown = "UNKNOWN"

// ErrNoPromptVersion reports that the extractor agent exists but declares no
// prompt_version key.
var ErrNoPromptVersion = errors.New("extractor agent declares no prompt_version key")

// ResolveExtractorPromptVersion reads the extraction contract's own version
// from the installed subagent's frontmatter.
//
// It is read from the INSTALLED agent (.claude/agents/), not from templates/,
// for the same reason ResolveExtractorAgentModel is: the installed file is what
// the host actually forks. A template that has moved ahead of the installed
// copy describes work nobody has run.
//
// The version identifies a batch of extractions. Two sidecars written under
// different prompt versions were produced by different contracts and are not
// comparable, so a re-extraction ledger keyed on it starts a fresh batch when
// the contract changes rather than reporting old work as current.
func ResolveExtractorPromptVersion(projectRoot string) (string, error) {
	var firstErr error
	for _, path := range candidateAgentPaths(projectRoot) {
		data, err := os.ReadFile(path)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("reading %s: %w", path, err)
			}
			continue
		}
		fm, err := parseAgentFrontmatter(data, path)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if fm.PromptVersion <= 0 {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", path, ErrNoPromptVersion)
			}
			continue
		}
		return fmt.Sprintf("v%d", fm.PromptVersion), nil
	}
	return ExtractorPromptVersionUnknown, firstErr
}

func parseAgentFrontmatter(data []byte, path string) (agentFrontmatter, error) {
	var fm agentFrontmatter
	body := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(body, "---\n") {
		return fm, fmt.Errorf("%s: no frontmatter fence", path)
	}
	rest := body[4:]
	end := strings.Index(rest, "\n---\n")
	if end == -1 {
		return fm, fmt.Errorf("%s: unterminated frontmatter fence", path)
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return fm, fmt.Errorf("%s: %w", path, err)
	}
	return fm, nil
}

// ResolveExtractorAgentModel reads the model that will ACTUALLY perform
// extraction, from the subagent's own frontmatter.
//
// WHY NOT THE CLI FLAG. `--model` pins the session that runs the slash
// command. That session forks a subagent per artifact (ADR-027), and the
// subagent's frontmatter governs its model. The two are different values and
// were different in practice for three months: the CLI said claude-opus-5, the
// agent said sonnet, and every sidecar in the corpus was written by the
// latter while every log line reported the former (D27, ADR-055).
//
// Returns ExtractorModelUnknown with a non-nil error when the agent file is
// missing or pins nothing. Callers MUST report that verbatim rather than
// falling back — see ExtractorModelUnknown.
func ResolveExtractorAgentModel(projectRoot string) (string, error) {
	var firstErr error
	for _, path := range candidateAgentPaths(projectRoot) {
		data, err := os.ReadFile(path)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("reading %s: %w", path, err)
			}
			continue
		}
		fm, err := parseAgentFrontmatter(data, path)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		model := strings.TrimSpace(fm.Model)
		if model == "" {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", path, ErrNoAgentModel)
			}
			continue
		}
		// Shape-only validation, for the reason ADR-054 gave and this ADR
		// keeps: a closed list of known model IDs needs editing on every
		// model release, and a stale list refusing a valid model is a worse
		// failure than a typo the CLI reports itself.
		if err := idvalidate.ModelID(model); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", path, err)
			}
			continue
		}
		return model, nil
	}
	return ExtractorModelUnknown, firstErr
}

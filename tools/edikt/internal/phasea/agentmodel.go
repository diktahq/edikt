package phasea

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"

	"github.com/diktahq/edikt/tools/edikt/internal/claudepaths"
	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
)

// ExtractorAgentRelPath is where the locked extractor agent is installed in a
// project. Its frontmatter `model:` is the authoritative pin (ADR-055 §1).
const ExtractorAgentRelPath = ".claude/agents/sidecar-extractor.md"

// candidateKind names where an extractor definition came from.
type candidateKind string

const (
	candidateProject candidateKind = "project"
	candidateProfile candidateKind = "profile"
)

// extractorProfilePath is where the active Claude profile keeps its copy of the
// extractor (the same claudepaths.ResolveClaudeRoot() chain doctor's own
// agent-drift check uses).
func extractorProfilePath() string {
	return filepath.Join(claudepaths.ResolveClaudeRoot(), "agents", "sidecar-extractor.md")
}

// selectExtractorDefinition is the ONE decision about which file is the
// extractor. The host runs the project's extractor when the project has one,
// and only falls back to the profile's when the project has none, so:
//
//   - the project's file is present on disk (a regular file, an unreadable one,
//     a directory, a dangling link: anything that exists)  -> it is the
//     selection, whether or not it is usable;
//   - otherwise the profile's file, if present            -> the selection;
//   - otherwise nothing is installed                      -> present is false.
//
// Every lookup (prompt version, model, contract digest) reads only the file this
// returns, so they cannot disagree about which extractor they describe.
func selectExtractorDefinition(projectRoot string) (path string, kind candidateKind, present bool) {
	if p := filepath.Join(projectRoot, ExtractorAgentRelPath); definitionPresent(p) {
		return p, candidateProject, true
	}
	if p := extractorProfilePath(); definitionPresent(p) {
		return p, candidateProfile, true
	}
	return "", "", false
}

// extractorNotFound is the error every lookup returns when nothing is installed.
func extractorNotFound(projectRoot string) error {
	return fmt.Errorf("%w; looked at: %s, %s", ErrExtractorNotFound,
		filepath.Join(projectRoot, ExtractorAgentRelPath), extractorProfilePath())
}

// ExtractorModelUnknown is what a run reports when it cannot determine which
// model will perform extraction.
//
// It is a distinct value and NEVER a fallback. Substituting the
// dispatching session's own model default — or its CLI `--model` value —
// is exactly how ADR-054 came to print `claude-opus-5` over work Sonnet
// did: a value that was not measured was rendered as one that was.
// INV-013 forbids that shape, and ADR-055 §4 names this specific case.
const ExtractorModelUnknown = "UNKNOWN"

// ErrNoAgentModel reports that the extractor agent exists but pins no model.
var ErrNoAgentModel = errors.New("extractor agent declares no model key")

type agentFrontmatter struct {
	Model string `yaml:"model"`
	// PromptVersion is derived from PromptVersionNode after the decode and is
	// 0 unless the file wrote a plain decimal integer (see plainVersionRe).
	PromptVersion     int       `yaml:"-"`
	PromptVersionNode yaml.Node `yaml:"prompt_version"`
}

// plainVersionRe is the only spelling of prompt_version that reads as a
// version: digits as written, no sign, no leading zero, no exponent, no
// underscore, no fraction. YAML would happily read 011 as octal 9, 1_1 as 11
// and 4e0 as a float; a contract version is a name, not a number to evaluate.
var plainVersionRe = regexp.MustCompile(`^[1-9][0-9]*$`)

func plainVersion(n yaml.Node) int {
	if n.Kind != yaml.ScalarNode || n.Tag != "!!int" || !plainVersionRe.MatchString(n.Value) {
		return 0
	}
	v, err := strconv.Atoi(n.Value)
	if err != nil {
		return 0
	}
	return v
}

// ExtractorPromptVersionUnknown is what a caller reports when the extraction
// contract's version cannot be read. Same rule as ExtractorModelUnknown: it is
// a distinct value, never a fallback, and a caller that needs the version to
// mean something must refuse rather than substitute one.
const ExtractorPromptVersionUnknown = "UNKNOWN"

// ErrNoPromptVersion reports that the extractor agent exists but declares no
// prompt_version key.
var ErrNoPromptVersion = errors.New("extractor agent declares no prompt_version key")

// ReadAgentPromptVersion reads prompt_version from ONE agent file. It is the
// single per-file reader: the installed-agent resolver and any caller that
// previews a named template both go through it, so they cannot parse the
// frontmatter two ways.
//
// readable is false when the file cannot be read at all. err is non-nil when
// it was read but its frontmatter is malformed. version <= 0 with a nil err
// means the file declares no prompt_version.
func ReadAgentPromptVersion(path string) (version int, readable bool, err error) {
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		return 0, false, nil
	}
	fm, perr := parseAgentFrontmatter(data, path)
	if perr != nil {
		return 0, true, perr
	}
	return fm.PromptVersion, true, nil
}

// ErrExtractorNotFound reports that no extractor definition exists at any candidate path.
var ErrExtractorNotFound = errors.New("extractor definition not found")

// ErrProjectDefinitionUnusable reports that the project's own extractor file
// is present but unusable (no prompt_version, malformed frontmatter, or it
// cannot be read) while a lower-precedence definition exists. The host runs the
// PROJECT's file when one exists, so answering with the profile's version would
// name a contract nobody ran. The resolver reports no version instead.
var ErrProjectDefinitionUnusable = errors.New("project extractor definition is present but unusable")

// definitionPresent reports whether something exists at path, apart from
// whether it can be used. Absent (including a missing parent) is not present;
// a directory, an unreadable file, a dangling symlink or any other error is.
func definitionPresent(path string) bool {
	_, err := os.Lstat(path)
	if err == nil {
		return true
	}
	return !(os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR))
}

// ResolveExtractorPromptVersionDetail reads the extraction contract's version
// from the installed subagent's frontmatter, from the one file
// selectExtractorDefinition picks.
//
// It reads the INSTALLED agent, not templates/, because the installed file is
// what the host actually forks; a template that has moved ahead of the
// installed copy describes work nobody has run. The version identifies a batch
// of extractions: sidecars written under different versions were produced by
// different contracts and are not comparable.
//
// The fact behind the rules: the host runs the project's extractor when the
// project has one, and only runs the profile's when it has none. So:
//
//   - nothing installed                  -> ErrExtractorNotFound, naming both paths;
//   - the project's file exists          -> it alone answers, whatever the
//     profile holds. If it is usable its version is returned; if it is unusable
//     (no prompt_version, malformed frontmatter, unreadable, a directory, a
//     dangling link) the version is UNKNOWN with ErrProjectDefinitionUnusable
//     when a profile copy exists (its version must not stand in), or
//     ErrNoPromptVersion when the project's file is the only definition;
//   - no project file, a profile file    -> the profile's file answers, with
//     ErrNoPromptVersion when it declares no version.
//
// Two usable copies that pin different versions are not an error: the project's
// version is the answer, and the note says the profile's differs, as
// information. The note is empty when exactly one definition exists and it is
// usable.
func ResolveExtractorPromptVersionDetail(projectRoot string) (version string, note string, err error) {
	sel, kind, ok := selectExtractorDefinition(projectRoot)
	if !ok {
		return ExtractorPromptVersionUnknown, "", extractorNotFound(projectRoot)
	}
	ver, problem := readDefinitionVersion(sel)
	profile := extractorProfilePath()
	profilePresent := kind == candidateProject && definitionPresent(profile)
	if problem != "" {
		if kind == candidateProject && profilePresent {
			return ExtractorPromptVersionUnknown, "", fmt.Errorf("%w: %s (%s)", ErrProjectDefinitionUnusable, sel, problem)
		}
		return ExtractorPromptVersionUnknown, "", fmt.Errorf("definition found at %s but %s: %w", sel, problem, ErrNoPromptVersion)
	}
	if profilePresent && sameFile(sel, profile) {
		// One file reached by two paths is one definition, not two.
		note = fmt.Sprintf("selected %s (%s); the project's extractor is a link to the profile's copy; one definition (%s)", sel, kind, profile)
	} else if profilePresent {
		var what string
		if pv, pproblem := readDefinitionVersion(profile); pproblem != "" {
			what = fmt.Sprintf("it %s", pproblem)
		} else if pv == ver {
			what = fmt.Sprintf("it pins v%d as well", pv)
		} else {
			what = fmt.Sprintf("it pins v%d, which differs from the project's v%d (information only; nothing is refused)", pv, ver)
		}
		note = fmt.Sprintf("found 2 definitions, selected %s (%s); the profile's copy %s is ignored because the project has its own: %s",
			sel, kind, profile, what)
	}
	return fmt.Sprintf("v%d", ver), note, nil
}

// sameFile reports whether two paths resolve to the same file once links are
// followed. A path that cannot be resolved is never the same as another.
func sameFile(a, b string) bool {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false
	}
	return ra == rb
}

// readDefinitionVersion reads one file's prompt_version. problem is empty when
// the file is usable, and otherwise says why it is not.
func readDefinitionVersion(path string) (version int, problem string) {
	ver, readable, perr := ReadAgentPromptVersion(path)
	switch {
	case !readable:
		return 0, "is present but cannot be read"
	case perr != nil:
		return 0, strings.TrimPrefix(perr.Error(), path+": ")
	case ver <= 0:
		return 0, "carries no prompt_version"
	}
	return ver, ""
}

// ResolveExtractorPromptVersion keeps the original signature for callers
// that only need the answer; the selection note is dropped here and
// surfaced by the callers that print (phasea plan).
func ResolveExtractorPromptVersion(projectRoot string) (string, error) {
	v, _, err := ResolveExtractorPromptVersionDetail(projectRoot)
	return v, err
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
	fm.PromptVersion = plainVersion(fm.PromptVersionNode)
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
// Reads only the file selectExtractorDefinition picks: the project's when the
// project has one (even an unusable one), the profile's only when it has none.
//
// Returns ExtractorModelUnknown with a non-nil error when the agent file is
// missing or pins nothing. Callers MUST report that verbatim rather than
// falling back — see ExtractorModelUnknown.
func ResolveExtractorAgentModel(projectRoot string) (string, error) {
	path, _, ok := selectExtractorDefinition(projectRoot)
	if !ok {
		return ExtractorModelUnknown, extractorNotFound(projectRoot)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ExtractorModelUnknown, fmt.Errorf("reading %s: %w", path, err)
	}
	fm, err := parseAgentFrontmatter(data, path)
	if err != nil {
		return ExtractorModelUnknown, err
	}
	model := strings.TrimSpace(fm.Model)
	if model == "" {
		return ExtractorModelUnknown, fmt.Errorf("%s: %w", path, ErrNoAgentModel)
	}
	// Shape-only validation, for the reason ADR-054 gave and this ADR
	// keeps: a closed list of known model IDs needs editing on every
	// model release, and a stale list refusing a valid model is a worse
	// failure than a typo the CLI reports itself.
	if err := idvalidate.ModelID(model); err != nil {
		return ExtractorModelUnknown, fmt.Errorf("%s: %w", path, err)
	}
	return model, nil
}

// ExtractorContractUnrecorded is what a run reports when the extractor
// agent definition's body digest cannot be determined. Same rule as
// ExtractorModelUnknown: a distinct value, never a fallback.
const ExtractorContractUnrecorded = "unrecorded"

// ResolveExtractorContractDigest returns the hex SHA-256 of the selected
// extractor agent definition's exact bytes: the same file
// ResolveExtractorAgentModel and ResolveExtractorPromptVersion read (the
// project's when the project has one, the profile's only when it has none).
// Distinguishes two dispatches sharing a prompt_version but not a body,
// such as a prompt patched and then reverted.
//
// Returns (ExtractorContractUnrecorded, err) when nothing is installed or the
// selected file cannot be read, never a guess, never a zero-value digest
// standing in for an observation that was not made, and never the profile's
// digest while a project file exists.
func ResolveExtractorContractDigest(projectRoot string) (string, error) {
	path, _, ok := selectExtractorDefinition(projectRoot)
	if !ok {
		return ExtractorContractUnrecorded, extractorNotFound(projectRoot)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ExtractorContractUnrecorded, fmt.Errorf("reading %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

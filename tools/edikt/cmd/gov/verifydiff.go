package gov

// verifydiff.go — `bin/edikt gov verify-diff` subcommand.
//
// Implements the prompt-construction logic from commands/gov/verify-diff.md
// (Steps 1–5) in Go tier-2. Agent dispatch is NOT performed here — the
// subcommand handles prompt construction, stub mode, and EDIKT_CAPTURE_PROMPT
// capture only. The actual governance-verifier agent dispatch remains the
// responsibility of the tier-1 markdown command.
//
// AC-7.2 (SPEC-009 Plan A Phase 7): this binary is the entry point for the  // edikt-guard:allow
// hermetic prompt-capture test. Set EDIKT_VERIFIER_STUB=1 and
// EDIKT_CAPTURE_PROMPT=<path> to capture the constructed prompt JSON without
// invoking an LLM.
//
// No-LLM contract: this file MUST NOT import or invoke any LLM CLI. The
// no-llm-in-tier-2 CI grep gate enforces this.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/spf13/cobra"
	"golang.org/x/text/unicode/norm"
)

const verifyDiffAgentVersion = "1.1.0"

// refRangeRe is the allowlist for the ref range argument (INV-006).  // edikt-guard:allow
// Mirrors the regex in commands/gov/verify-diff.md Step 1.
var refRangeRe = regexp.MustCompile(`^[a-z0-9._/~^@-]+\.\.[a-z0-9._/~^@-]+$`)

// captureDirective is one per-directive entry in the EDIKT_CAPTURE_PROMPT JSON.
// Carries both the dispatched shape fields AND the source_text so AC-7.3 can
// verify that text did not leak into Intent-shape blocks.
type captureDirective struct {
	DirectiveID           string `json:"directive_id"`
	Shape                 string `json:"shape"` // "intent" or "text"
	Intent                string `json:"intent,omitempty"`
	FalsifyingObservation string `json:"falsifying_observation,omitempty"`
	SourceText            string `json:"source_text,omitempty"` // set for intent shape; AC-7.3 reference
	Text                  string `json:"text,omitempty"`        // set for text shape
}

type captureMeta struct {
	Topic        string `json:"topic"`
	AgentVersion string `json:"agent_version"`
}

type capturePayload struct {
	Topic      string             `json:"topic"`
	DiffFile   string             `json:"diff_file"`
	Directives []captureDirective `json:"directives"`
	Meta       captureMeta        `json:"meta"`
}

// stubVerdictEntry and friends mirror the governance-verifier-verdict schema.
type stubVerdictEntry struct {
	DirectiveID string `json:"directive_id"`
	Status      string `json:"status"`
	Rationale   string `json:"rationale"`
}

type stubVerdictMeta struct {
	Topic        string `json:"topic"`
	RanAt        string `json:"ran_at"`
	AgentVersion string `json:"agent_version"`

	// Stub marks the whole document as fixture output. It is always true
	// here — only vdWriteStub constructs this type — and it is written as a
	// FIELD rather than left implicit in the per-verdict rationale string,
	// because a reader parsing verdicts[].status sees PASS and has no reason
	// to read prose next to it.
	//
	// This stub fails in the GREENING direction: it writes PASS for every
	// directive without evaluating a diff. A stub that reds a build is
	// noticed within the hour; one that greens it is not noticed at all.
	Stub bool `json:"stub"`
	// StubReason states what was not done, in the artifact itself.
	StubReason string `json:"stub_reason,omitempty"`
}

type stubVerdictDoc struct {
	Verdicts []stubVerdictEntry `json:"verdicts"`
	Meta     stubVerdictMeta    `json:"meta"`
}

// topicEntry groups the directives from one sidecar contributing to a topic.
type topicEntry struct {
	artifactID string
	directives []sidecar.Directive
}

var verifyDiffCmd = &cobra.Command{
	Use:   "verify-diff [since-ref..to-ref]",
	Short: "Construct governance-verifier prompt and capture or stub-dispatch it",
	Long: `Implements Steps 1–5 of commands/gov/verify-diff.md in Go tier-2.

Does NOT dispatch the governance-verifier agent — use the tier-1 slash command
/edikt:gov:verify-diff for full agent dispatch.

Environment variables:
  EDIKT_VERIFIER_STUB=1          Skip agent dispatch; write a canned PASS
                                  verdict to .edikt/state/gov-verify/.
  EDIKT_CAPTURE_PROMPT=<path>    Write the constructed prompt JSON to <path>
                                  before dispatch/stub (enables AC-7.2).

Ref range defaults to HEAD~1..HEAD when omitted.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		err := runVerifyDiff(args)
		exitFromExitErr(err)
		return err
	},
}

func init() {
	Cmd.AddCommand(verifyDiffCmd)
}

func runVerifyDiff(args []string) error {
	// --- Step 1: Parse and validate ref range (INV-006). ---  // edikt-guard:allow
	rangeArg := "HEAD~1..HEAD"
	if len(args) > 0 {
		rangeArg = args[0]
	}
	rangeNorm := vdNormalizeRef(rangeArg)
	if !refRangeRe.MatchString(rangeNorm) {
		return &exitErr{code: 1, msg: fmt.Sprintf(
			`{"status":"error","reason":"ref range contains disallowed characters","input":%s}`,
			vdJSONString(rangeArg),
		)}
	}

	// --- Step 2: Compute the diff. ---
	changedFiles, diffFile, err := vdComputeDiff(rangeNorm)
	if err != nil {
		return &exitErr{code: 1, msg: err.Error()}
	}
	if diffFile != "" {
		defer os.Remove(diffFile)
	}
	if len(changedFiles) == 0 {
		fmt.Println(`{"status":"skipped","reason":"empty diff"}`)
		return nil
	}

	// --- Step 3: Discover sidecars from governance dirs. ---
	projectRoot, err := os.Getwd()
	if err != nil {
		return &exitErr{code: 2, msg: fmt.Sprintf("getwd: %v", err)}
	}
	dirs := govrun.GovernanceDirs(projectRoot)
	pairs, err := sidecar.Discover(projectRoot, dirs)
	if err != nil {
		return &exitErr{code: 2, msg: fmt.Sprintf("discover sidecars: %v", err)}
	}

	// --- Step 4: Match changed files against sidecar paths and group by topic. ---
	topicMap := map[string][]topicEntry{}
	for _, p := range pairs {
		if p.Sidecar == nil || p.Skip {
			continue
		}
		if !vdSidecarInScope(p.Sidecar, changedFiles) {
			continue
		}
		topic := p.Sidecar.Topic
		topicMap[topic] = append(topicMap[topic], topicEntry{
			artifactID: p.ArtifactID,
			directives: p.Sidecar.Directives,
		})
	}

	if len(topicMap) == 0 {
		fmt.Println(`{"status":"skipped","reason":"no compiled governance"}`)
		return nil
	}

	// --- Step 5: Build and capture/stub prompt per topic. ---
	capturePath := os.Getenv("EDIKT_CAPTURE_PROMPT")
	stubMode := os.Getenv("EDIKT_VERIFIER_STUB") == "1"

	topicNames := make([]string, 0, len(topicMap))
	for t := range topicMap {
		topicNames = append(topicNames, t)
	}
	sort.Strings(topicNames)

	for _, topic := range topicNames {
		dispatched := vdBuildDispatch(topicMap[topic])

		// Write EDIKT_CAPTURE_PROMPT BEFORE dispatch/stub (INV-004: only  // edikt-guard:allow
		// caller-constructed data, no agent output in the captured file).
		if capturePath != "" {
			payload := capturePayload{
				Topic:      topic,
				DiffFile:   diffFile,
				Directives: dispatched,
				Meta: captureMeta{
					Topic:        topic,
					AgentVersion: verifyDiffAgentVersion,
				},
			}
			if werr := vdWriteCapture(capturePath, payload); werr != nil {
				fmt.Fprintf(os.Stderr, "warning: EDIKT_CAPTURE_PROMPT write failed: %v\n", werr)
			}
		}

		if stubMode {
			if werr := vdWriteStub(projectRoot, topic, dispatched); werr != nil {
				fmt.Fprintf(os.Stderr, "warning: stub verdict write for topic %s: %v\n", topic, werr)
			}
		}
		// Agent dispatch is intentionally omitted — tier-1 owns that step.
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Printf(`{"status":"ok","range":%s,"topics_processed":%d,"note":"prompt-construction only; agent dispatch is tier-1","ran_at":%s}`+"\n",
		vdJSONString(rangeNorm), len(topicNames), vdJSONString(ts))
	return nil
}

// vdSidecarInScope returns true when any changed file matches the sidecar's
// Paths globs (code-scope patterns) or its Path (governance doc path).
//
// B2b (SPEC-010 phase 8): an undeclared Paths list means "everywhere", not  edikt-guard:allow
// "matches nothing" — the same direction phaseb/merge.go's scopeFor already
// settled ("an undeclared sidecar contributes everywhere ... under-scoping
// silently disables governance, which is the worse failure for a tool whose
// product is enforcement"). Before this fix, the two consumers disagreed:
// merge.go's compiled topic files treated an undeclared sidecar as
// unrestricted while this function treated it as never in scope.
func vdSidecarInScope(sc *sidecar.Sidecar, changedFiles []string) bool {
	if len(sc.Paths) == 0 {
		return len(changedFiles) > 0
	}
	for _, changed := range changedFiles {
		// Governance doc path: exact match or suffix.
		if sc.Path != "" && (changed == sc.Path || strings.HasSuffix(changed, "/"+sc.Path) || strings.HasSuffix(changed, sc.Path)) {
			return true
		}
		// Code-scope globs.
		for _, pattern := range sc.Paths {
			if vdGlobMatch(pattern, changed) {
				return true
			}
		}
	}
	return false
}

// vdGlobMatch matches a path against a glob pattern that may contain **.
// ** matches zero or more path segments; * matches within a single segment.
func vdGlobMatch(pattern, path string) bool {
	if !strings.Contains(pattern, "**") {
		ok, _ := filepath.Match(pattern, path)
		return ok
	}
	// Split on the first ** and match prefix / suffix.
	idx := strings.Index(pattern, "**")
	prefix := pattern[:idx]
	rest := pattern[idx+2:]
	rest = strings.TrimPrefix(rest, "/")

	candidate := path
	if prefix != "" {
		if !strings.HasPrefix(candidate, prefix) {
			return false
		}
		candidate = candidate[len(prefix):]
	}

	if rest == "" {
		return true
	}
	// Try matching the suffix at every sub-path of candidate.
	for {
		ok, _ := filepath.Match(rest, candidate)
		if ok {
			return true
		}
		slash := strings.Index(candidate, "/")
		if slash < 0 {
			break
		}
		candidate = candidate[slash+1:]
	}
	return false
}

// vdBuildDispatch applies the ADR-037 shape-selection rule to each directive.  // edikt-guard:allow
func vdBuildDispatch(entries []topicEntry) []captureDirective {
	var result []captureDirective
	idx := 0
	for _, e := range entries {
		for _, d := range e.directives {
			did := fmt.Sprintf("%s.directive[%d]", e.artifactID, idx)
			intentGated := vdGateField(d.Intent)
			falsifyingGated := vdGateField(d.FalsifyingObservation)
			hasIntent := intentGated != "" && falsifyingGated != ""

			var cd captureDirective
			cd.DirectiveID = did
			if hasIntent {
				cd.Shape = "intent"
				cd.Intent = intentGated
				cd.FalsifyingObservation = falsifyingGated
				cd.SourceText = d.Text // preserved for AC-7.3 no-text-leak check
			} else {
				cd.Shape = "text"
				cd.Text = d.Text
			}
			result = append(result, cd)
			idx++
		}
	}
	return result
}

// vdGateField applies NFKC normalization + casefold + strip and rejects
// strings containing control characters or exceeding 300 chars (INV-006).  // edikt-guard:allow
// Returns the stripped original on success, "" on rejection.
func vdGateField(v string) string {
	if v == "" {
		return ""
	}
	normalized := norm.NFKC.String(v)
	// Validate: reject control characters.
	for _, r := range normalized {
		if unicode.IsControl(r) {
			return ""
		}
	}
	stripped := strings.TrimSpace(normalized)
	if len([]rune(strings.ToLower(stripped))) > 300 {
		return ""
	}
	if stripped == "" {
		return ""
	}
	// Return the stripped original (preserve case; normalization is for gating only).
	return strings.TrimSpace(v)
}

// vdWriteCapture serialises the payload as indented JSON to the given path.
func vdWriteCapture(path string, payload capturePayload) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// vdWriteStub writes a canned PASS verdict to .edikt/state/gov-verify/.
func vdWriteStub(projectRoot, topic string, dispatched []captureDirective) error {
	ts := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(projectRoot, ".edikt", "state", "gov-verify")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	verdicts := make([]stubVerdictEntry, 0, len(dispatched))
	for _, d := range dispatched {
		verdicts = append(verdicts, stubVerdictEntry{
			DirectiveID: d.DirectiveID,
			Status:      "PASS",
			Rationale:   "stub mode — no diff evaluated",
		})
	}
	sv := stubVerdictDoc{
		Verdicts: verdicts,
		Meta: stubVerdictMeta{
			Topic:        topic,
			RanAt:        time.Now().UTC().Format(time.RFC3339),
			AgentVersion: verifyDiffAgentVersion,
			Stub:         true,
			StubReason:   "EDIKT_VERIFIER_STUB=1 — no agent ran and no diff was evaluated; every PASS above is fixture output, not a verdict",
		},
	}
	data, err := json.MarshalIndent(sv, "", "  ")
	if err != nil {
		return err
	}
	// STUB- leads the filename so a directory listing, a glob, and a
	// most-recent-report lookup all carry the label without opening the
	// file. A marker only inside the JSON is one os.ReadDir away from
	// being invisible.
	reportPath := filepath.Join(dir, "STUB-"+topic+"-"+ts+".json")
	return os.WriteFile(reportPath, append(data, '\n'), 0o644)
}

// vdComputeDiff runs `git diff --numstat` and materialises the diff to a
// temp file. Returns changed files and the temp file path (caller removes it).
func vdComputeDiff(rangeNorm string) (changedFiles []string, diffFile string, err error) {
	out, gitErr := exec.Command("git", "diff", "--numstat", rangeNorm).Output()
	if gitErr != nil {
		return nil, "", nil // treat as empty diff
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		if fields[0] == "-" || fields[1] == "-" {
			continue // binary file
		}
		changedFiles = append(changedFiles, strings.TrimSpace(fields[2]))
	}
	if len(changedFiles) == 0 {
		return nil, "", nil
	}
	f, err := os.CreateTemp("", "edikt-verify-diff-*.diff")
	if err != nil {
		return nil, "", fmt.Errorf("create diff temp: %w", err)
	}
	_ = f.Close()
	diffOut, err := exec.Command("git", "diff", rangeNorm, "--").Output()
	if err != nil {
		_ = os.Remove(f.Name())
		return nil, "", fmt.Errorf("git diff: %w", err)
	}
	if err := os.WriteFile(f.Name(), diffOut, 0o644); err != nil {
		_ = os.Remove(f.Name())
		return nil, "", fmt.Errorf("write diff temp: %w", err)
	}
	return changedFiles, f.Name(), nil
}

// vdNormalizeRef applies NFKC + lower + strip for allowlist comparison (INV-006).  // edikt-guard:allow
func vdNormalizeRef(v string) string {
	return strings.TrimSpace(strings.ToLower(norm.NFKC.String(v)))
}

// vdJSONString returns v as a JSON-encoded string.
func vdJSONString(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

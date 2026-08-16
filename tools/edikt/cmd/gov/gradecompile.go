package gov

// gradecompile.go — `bin/edikt gov grade-compile` subcommand.
//
// SPEC-009 Plan H (SR-016), restructured by ADR-044.
//
// This command does NOT dispatch an LLM. Under INV-012 (promoted from
// ADR-030) a tier-2 binary never spawns an LLM CLI: the compile-quality
// grader is dispatched from tier-1 markdown via the host agent's Task
// primitive (commands/gov/grade-compile.md), exactly as the
// governance-verifier is (ADR-035). What remains here is the deterministic
// half — parse the agent-produced JSON, validate it, persist it atomically
// under .edikt/state/compile-quality/, and summarize.
//
// The in-binary `claude -p` dispatcher this replaced is why the
// restructure happened: it passed the agent template's YAML frontmatter
// into the prompt (the leading `---` parsed as a CLI flag), never unwrapped
// `claude --output-format json`'s result envelope — so every grade silently
// parsed as 0/10 — and inherited ANTHROPIC_API_KEY, routing to the metered
// API against ADR-012's session posture. A grader reporting scores it never
// computed is the failure mode this file now cannot have: it records what
// an agent actually returned, or it fails.
//
// Exit-code contract (mirrors gov benchmark cheat-rate):
//   0 — recorded (or stub returned a canned report)
//   1 — parse / validation / state-write error
//   3 — invalid or missing arguments / flags
//
// Stub mode: when EDIKT_GRADE_COMPILE_STUB=1 is set, the command emits the
// canned fixture at test/fixtures/grade-compile/stub-report.json to the
// canonical state path, so integration tests exercise the record pipeline
// without an agent.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/gradecompile"
	"github.com/spf13/cobra"
)

var (
	gradeCompileDir    string
	gradeCompileModel  string
	gradeCompileRecord string
	gradeCompilePrint  bool
)

// stub fixture location, relative to the project root.
const gradeCompileStubFixture = "test/fixtures/grade-compile/stub-report.json"

// Asset-resolution candidate lists, in precedence order. A project
// override under .edikt/templates/ wins over the installed payload
// (ADR-005 template-lookup mechanism).  // edikt-guard:allow
var (
	gradeRubricCandidates = []string{
		".edikt/templates/rubrics/compile-quality.md",
		"templates/rubrics/compile-quality.md",
	}
	gradeSchemaCandidates = []string{
		".edikt/templates/schemas/compile-quality-report.v1.schema.json",
		"templates/schemas/compile-quality-report.v1.schema.json",
	}
	gradeTemplateCandidates = []string{
		".edikt/templates/agents/compile-quality-grader.md",
		"templates/agents/compile-quality-grader.md",
	}
)

// (ref: INV-012 — tier-2 Go binaries must not dispatch an LLM)
var gradeCompileCmd = &cobra.Command{
	Use:   "grade-compile",
	Short: "Grade the editorial quality of compiled governance (LLM-as-judge)",
	Long: `Record a compile-quality grade produced by the compile-quality-grader
agent and persist it under .edikt/state/compile-quality/.

This command does not call an LLM. Tier-2 binaries never dispatch one;
the grader is dispatched from tier-1 by /edikt:gov:grade-compile,
which pipes the agent's JSON report here.

  /edikt:gov:grade-compile          # dispatches the agent, then records
  bin/edikt gov grade-compile --record report.json
  <agent output> | bin/edikt gov grade-compile --record -

Run after 'gov compile'. Grading is post-compile and never affects Phase B
determinism.

Stub mode (testing/CI): EDIKT_GRADE_COMPILE_STUB=1 writes the canned fixture
report instead, exercising the record pipeline without an agent.

Exit codes:
  0 — recorded (or stub report written)
  1 — parse / validation / state-write error
  3 — invalid or missing arguments / flags`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := runGradeCompile(cmd)
		exitFromExitErr(err)
		return err
	},
}

func init() {
	gradeCompileCmd.Flags().StringVar(&gradeCompileDir, "dir", ".claude/rules/governance",
		"compiled governance directory to grade (relative to project root or absolute)")
	gradeCompileCmd.Flags().StringVar(&gradeCompileModel, "model", "claude-opus-4-7",
		"grader model id recorded in the report when the agent omits it")
	gradeCompileCmd.Flags().StringVar(&gradeCompileRecord, "record", "",
		"path to the grader agent's JSON report, or - for stdin")
	gradeCompileCmd.Flags().BoolVar(&gradeCompilePrint, "print-inputs", false,
		"resolve and print the governance dir, rubric, schema, and agent template the tier-1 dispatch needs")
	Cmd.AddCommand(gradeCompileCmd)
}

func runGradeCompile(cmd *cobra.Command) error {
	if gradeCompileDir == "" {
		return &exitErr{code: 3, msg: "grade-compile: --dir must not be empty"}
	}
	if gradeCompileModel == "" {
		return &exitErr{code: 3, msg: "grade-compile: --model must not be empty"}
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("grade-compile: getwd: %v", err)}
	}
	stateDir := filepath.Join(projectRoot, ".edikt", "state")

	if gradeCompilePrint {
		return runGradeCompilePrintInputs(cmd, projectRoot)
	}
	if os.Getenv("EDIKT_GRADE_COMPILE_STUB") == "1" {
		return runGradeCompileStub(cmd, projectRoot, stateDir)
	}
	if gradeCompileRecord == "" {
		// Fail closed and say who does dispatch now. Silently succeeding
		// with no grade — or worse, emitting a zeroed one — is the exact
		// behavior ADR-044 removed.
		// (ref: INV-012 — tier-2 Go binaries must not dispatch an LLM)
		return &exitErr{code: 3, msg: "grade-compile: --record is required.\n" +
			"This binary does not dispatch an LLM. Run /edikt:gov:grade-compile,\n" +
			"which dispatches the compile-quality-grader agent and pipes its JSON here,\n" +
			"or pass a report you already have: --record <file|->"}
	}
	return runGradeCompileRecord(cmd, projectRoot, stateDir)
}

// runGradeCompileStub reads the canned fixture, writes it to the
// canonical state path, and prints a summary — exercising the full
// report pipeline without an LLM call.
func runGradeCompileStub(cmd *cobra.Command, projectRoot, stateDir string) error {
	fixture := filepath.Join(projectRoot, gradeCompileStubFixture)
	raw, err := os.ReadFile(fixture)
	if err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("grade-compile stub: read fixture %s: %v", fixture, err)}
	}
	report, err := gradecompile.ParseReport(raw)
	if err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("grade-compile stub: %v", err)}
	}
	if report.GradedAt == "" {
		report.GradedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return writeAndSummarize(cmd, stateDir, report, true)
}

// runGradeCompileRecord reads the grader agent's JSON report from a file or
// stdin, parses it (ParseReport tolerates both a bare object and a Claude
// result envelope), fills in provenance the agent may have omitted, and
// persists it.
//
// Every field the binary fills is provenance, never a score: if the agent
// did not produce scores, parsing fails and nothing is written. That is the
// invariant the old dispatcher violated by defaulting an unparsed envelope
// to zeros.
func runGradeCompileRecord(cmd *cobra.Command, projectRoot, stateDir string) error {
	var raw []byte
	var err error
	if gradeCompileRecord == "-" {
		raw, err = io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return &exitErr{code: 1, msg: fmt.Sprintf("grade-compile: read stdin: %v", err)}
		}
	} else {
		raw, err = os.ReadFile(gradeCompileRecord)
		if err != nil {
			return &exitErr{code: 1, msg: fmt.Sprintf("grade-compile: read %s: %v", gradeCompileRecord, err)}
		}
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		// (ref: INV-011 — absence of inspectable input is not a pass)
		return &exitErr{code: 1, msg: "grade-compile: empty grader report — nothing recorded.\n" +
			"An absent report is a failure, not a zero score."}
	}

	report, err := gradecompile.ParseReport(raw)
	if err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("grade-compile: %v\ngrader output:\n%s", err, truncateForError(string(raw)))}
	}
	// Validate the RAW document, not the decoded struct. Report's fields are
	// plain ints, so an absent score and a real 0 decode identically — which
	// is how the old dispatcher persisted all-zero grades. Reject before
	// anything reaches disk.
	if err := gradecompile.ValidateRaw(gradecompile.UnwrapForValidation(raw)); err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("grade-compile: %v\ngrader output:\n%s", err, truncateForError(string(raw)))}
	}
	if report.GradedAt == "" {
		report.GradedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if report.GraderModel == "" {
		report.GraderModel = gradeCompileModel
	}
	if report.TargetDir == "" {
		report.TargetDir = gradeCompileDir
	}
	return writeAndSummarize(cmd, stateDir, report, false)
}

// truncateForError caps echoed agent output so a runaway response cannot
// bury the actual parse error.
func truncateForError(s string) string {
	const cap = 2000
	if len(s) <= cap {
		return s
	}
	return s[:cap] + "\n… (truncated)"
}

// runGradeCompilePrintInputs resolves the four paths the tier-1 dispatch
// needs and prints them. This is the deterministic half ADR-044 §2 keeps in
// tier-2: path resolution honours the .edikt/templates/ project override
// (ADR-005) that a markdown command would otherwise have to reimplement.
//
// Per ADR-029 Rule 2 the caller gates on the EXIT CODE, not this text: a
// missing asset exits non-zero and names what is missing, so tier-1 refuses
// to dispatch a grader with no rubric rather than dispatching a blind one.
func runGradeCompilePrintInputs(cmd *cobra.Command, projectRoot string) error {
	govDir := gradeCompileDir
	if !filepath.IsAbs(govDir) {
		govDir = filepath.Join(projectRoot, govDir)
	}
	if info, err := os.Stat(govDir); err != nil || !info.IsDir() {
		return &exitErr{code: 1, msg: fmt.Sprintf(
			"grade-compile: governance dir %q not found (run 'gov compile' first?)", govDir)}
	}
	rubricPath, err := resolveAsset(projectRoot, gradeRubricCandidates)
	if err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("grade-compile: %v", err)}
	}
	schemaPath, err := resolveAsset(projectRoot, gradeSchemaCandidates)
	if err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("grade-compile: %v", err)}
	}
	templatePath, err := resolveAsset(projectRoot, gradeTemplateCandidates)
	if err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("grade-compile: %v", err)}
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "governance_dir: %s\n", govDir)
	fmt.Fprintf(out, "rubric:         %s\n", rubricPath)
	fmt.Fprintf(out, "schema:         %s\n", schemaPath)
	fmt.Fprintf(out, "agent_template: %s\n", templatePath)
	return nil
}

// writeAndSummarize persists the report and prints a human summary.
// writeAndSummarize persists the report and prints its scores.
//
// stub marks fixture output. A stub report is byte-identical in SHAPE to a
// graded one, so once it lands in .edikt/state/ nothing downstream can tell
// them apart — the same defect that let the code-reviewer's canned fixture
// fail builds and post a fabricated CRITICAL. Here the label rides on the
// FILENAME rather than inside the JSON, because the report schema is a
// consumed contract and a directory listing must carry the warning without
// anyone opening the file.
func writeAndSummarize(cmd *cobra.Command, stateDir string, report *gradecompile.Report, stub bool) error {
	path, err := gradecompile.WriteReport(stateDir, report)
	if err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("grade-compile: %v", err)}
	}
	if stub {
		stubPath := filepath.Join(filepath.Dir(path), "STUB-"+filepath.Base(path))
		if rerr := os.Rename(path, stubPath); rerr != nil {
			// Refuse rather than leave an unlabelled stub report on disk:
			// an unmarked fixture is the failure this branch exists to
			// prevent, so failing to mark it is a failure, not a warning.
			return &exitErr{code: 1, msg: fmt.Sprintf(
				"grade-compile stub: could not label the report as a stub (%v); refusing to leave %s indistinguishable from a real grade", rerr, path)}
		}
		path = stubPath
	}
	out := cmd.OutOrStdout()
	if stub {
		fmt.Fprintf(out, "STUB — canned fixture, NOT a grade of this corpus. No grader ran.\n")
	}
	fmt.Fprintf(out, "Compiled-governance quality: overall %d/10\n", report.Overall)
	fmt.Fprintf(out, "  coherence %d  conciseness %d  signal-to-noise %d  description %d  tiering %d  no-double-loading %d\n",
		report.Scores.Coherence, report.Scores.Conciseness,
		report.Scores.SignalToNoise, report.Scores.DescriptionQuality,
		report.Scores.TierAssignment, report.Scores.NoDoubleLoading)
	fmt.Fprintf(out, "  %d finding(s); report: %s\n", len(report.Findings), path)
	return nil
}

// resolveAsset returns the first candidate (joined against projectRoot)
// that exists on disk, or an error naming all candidates tried.
func resolveAsset(projectRoot string, candidates []string) (string, error) {
	for _, c := range candidates {
		p := filepath.Join(projectRoot, c)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("none of the expected asset paths exist: %v", candidates)
}

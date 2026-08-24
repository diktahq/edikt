package gov

// verify_prompt.go — `bin/edikt gov verify-prompt`, replacing the prompt and
// capture builders in commands/gov/verify-diff.md (PLAN-v080 Phase 24, FR-010).
//
// ONE RULE, ONE PLACE. Both the prompt and the capture file choose between two
// per-directive shapes, and the rule is the same: INTENT shape iff BOTH intent
// and falsifying_observation are present, TEXT shape otherwise. It was written
// twice in python, in two adjacent heredocs, which is two chances to disagree
// about which directives got which treatment — and the capture file exists
// precisely so a test can check what was dispatched.
//
// The intent shape STRIPS text from the prompt. That is deliberate and is what
// AC-7.3 checks: a verifier given both the intent and the original wording
// tends to grade the wording. The capture keeps `source_text` so the test can
// prove the text was withheld from the prompt rather than never present.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type promptDirective struct {
	DirectiveID           string `json:"directive_id"`
	Text                  string `json:"text,omitempty"`
	Intent                string `json:"intent,omitempty"`
	FalsifyingObservation string `json:"falsifying_observation,omitempty"`
}

// usesIntentShape is THE rule. Both fields or neither — a directive with an
// intent but no falsifying observation cannot be graded on intent, because
// there is nothing stated that would disprove it.
func (d promptDirective) usesIntentShape() bool {
	return d.Intent != "" && d.FalsifyingObservation != ""
}

type captureEntry struct {
	DirectiveID           string `json:"directive_id"`
	Shape                 string `json:"shape"`
	Text                  string `json:"text,omitempty"`
	Intent                string `json:"intent,omitempty"`
	FalsifyingObservation string `json:"falsifying_observation,omitempty"`
	SourceText            string `json:"source_text,omitempty"`
}

func init() {
	var diffFile, topic, directivesJSON, agentVersion, capturePath string
	cmd := &cobra.Command{
		Use:   "verify-prompt",
		Short: "Compose the governance-verifier prompt for one topic",
		Long: `Print the verifier prompt body for a topic's directives.

Each directive is rendered in the INTENT shape when both intent and
falsifying_observation are present, and the TEXT shape otherwise. The intent
shape omits the directive's original wording on purpose: a verifier shown both
tends to grade the wording rather than the behaviour.

With --capture, also writes the dispatched shape per directive as JSON,
including source_text, so a test can prove the wording was WITHHELD rather than
absent.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var ds []promptDirective
			if err := json.Unmarshal([]byte(directivesJSON), &ds); err != nil {
				return fmt.Errorf("--directives is not a JSON array of directives: %w", err)
			}

			var b strings.Builder
			fmt.Fprintf(&b, "You are evaluating diff at: %s\n\nTopic: %s\nDirectives:\n", diffFile, topic)
			capture := make([]captureEntry, 0, len(ds))
			for _, d := range ds {
				fmt.Fprintf(&b, "  - directive_id: %s\n", d.DirectiveID)
				if d.usesIntentShape() {
					fmt.Fprintf(&b, "    intent: %s\n", d.Intent)
					fmt.Fprintf(&b, "    falsifying_observation: %s\n", d.FalsifyingObservation)
					capture = append(capture, captureEntry{
						DirectiveID: d.DirectiveID, Shape: "intent",
						Intent: d.Intent, FalsifyingObservation: d.FalsifyingObservation,
						SourceText: d.Text,
					})
					continue
				}
				fmt.Fprintf(&b, "    text: %s\n", d.Text)
				capture = append(capture, captureEntry{
					DirectiveID: d.DirectiveID, Shape: "text", Text: d.Text,
				})
			}
			fmt.Fprintf(&b, "\nRead the diff file. For each directive, emit one verdict "+
				"(PASS / FAIL / NEEDS_REVIEW) per the governance-verifier contract. Emit one JSON "+
				"object conforming to templates/agents/governance-verifier-verdict.schema.json.\n\n")
			fmt.Fprintf(&b, "meta.topic = %s\n", topic)
			fmt.Fprintf(&b, "meta.agent_version = %q\n", agentVersion)

			if capturePath != "" {
				payload := map[string]any{
					"topic":      topic,
					"directives": capture,
					"meta":       map[string]any{"topic": topic, "agent_version": agentVersion},
				}
				enc, err := json.MarshalIndent(payload, "", "  ")
				if err != nil {
					return err
				}
				if err := os.WriteFile(capturePath, append(enc, '\n'), 0o644); err != nil { //nolint:gosec // test artefact
					return err
				}
			}
			_, err := fmt.Fprint(cmd.OutOrStdout(), b.String())
			return err
		},
	}
	cmd.Flags().StringVar(&diffFile, "diff-file", "", "path of the diff under evaluation (required)")
	cmd.Flags().StringVar(&topic, "topic", "", "topic name (required)")
	cmd.Flags().StringVar(&directivesJSON, "directives", "", "directives JSON array (required)")
	cmd.Flags().StringVar(&agentVersion, "agent-version", "1.1.0", "verifier agent version")
	cmd.Flags().StringVar(&capturePath, "capture", "", "also write the dispatched shape here as JSON")
	for _, f := range []string{"diff-file", "topic", "directives"} {
		_ = cmd.MarkFlagRequired(f)
	}
	Cmd.AddCommand(cmd)

	// ── stub verdict ────────────────────────────────────────────────────
	var tplPath, stubTopic string
	stub := &cobra.Command{
		Use:   "verify-stub-verdict",
		Short: "Emit a canned verdict from a fixture, with meta.topic and meta.ran_at set",
		Long: `Read a verdict fixture and re-emit it with meta.topic and meta.ran_at set.

For EDIKT_VERIFIER_STUB, so the persistence path can be exercised without
dispatching an agent. The timestamp is generated here rather than passed in,
matching the predecessor — a stub verdict claiming a caller-supplied time would
be a fixture pretending to be a run.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(tplPath) //nolint:gosec // operator-supplied fixture
			if err != nil {
				return err
			}
			var doc map[string]any
			if err := json.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("%s is not valid JSON: %w", tplPath, err)
			}
			meta, ok := doc["meta"].(map[string]any)
			if !ok {
				// The fixture is the contract; inventing a meta block would
				// produce a verdict shaped unlike anything the verifier emits.
				return fmt.Errorf("%s has no meta object to stamp", tplPath)
			}
			meta["topic"] = stubTopic
			meta["ran_at"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
			enc, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(enc))
			return nil
		},
	}
	stub.Flags().StringVar(&tplPath, "fixture", "", "verdict fixture path (required)")
	stub.Flags().StringVar(&stubTopic, "topic", "", "topic to stamp (required)")
	_ = stub.MarkFlagRequired("fixture")
	_ = stub.MarkFlagRequired("topic")
	Cmd.AddCommand(stub)
}

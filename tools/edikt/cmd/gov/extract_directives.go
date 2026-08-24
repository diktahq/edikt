package gov

// extract_directives.go — `bin/edikt gov extract-directives`, replacing the
// inline extractor in commands/gov/verify-diff.md (PLAN-v080 Phase 24, FR-010).
//
// WHAT IT READS: the region of a compiled topic file between
// `[edikt:directives:start]: #` and `[edikt:directives:end]: #`, in either of
// the two formats that region has carried — a bare `- text` bullet, and the
// Phase B extension where `intent` and `falsifying_observation` follow as
// indented sub-fields under `- text:`.
//
// THE GATE IS A SECURITY CONTROL, NOT VALIDATION. `intent` and
// `falsifying_observation` come from artifact prose and end up in a prompt sent
// to a verifier. Each is NFKC-normalised, casefolded and stripped, then
// REJECTED if it contains a Unicode control character or exceeds the length
// cap. A rejected field is dropped rather than the directive — the directive is
// still worth verifying without its intent, and refusing the whole entry would
// let one bad field silently remove a rule from the run.
//
// THE GATE RETURNS THE ORIGINAL, NOT THE NORMALISED FORM. Normalisation
// decides ADMISSIBILITY; the text delivered is what the author wrote. Emitting
// the casefolded version would put a mangled sentence in front of the verifier
// and call it the directive's intent.

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const directiveFieldMaxLen = 300

var directivesRegionRe = regexp.MustCompile(
	`(?s)\[edikt:directives:start\]: #(.*?)\[edikt:directives:end\]: #`)

// gateField mirrors the python `_gate`: normalise to decide, return the
// original stripped. Returns "" when the value is inadmissible.
func gateField(val string) string {
	if val == "" {
		return ""
	}
	n := strings.TrimSpace(cases.Fold().String(norm.NFKC.String(val)))
	for _, r := range n {
		if unicode.IsControl(r) {
			return ""
		}
	}
	if len([]rune(n)) > directiveFieldMaxLen {
		return ""
	}
	return strings.TrimSpace(val)
}

type extractedDirective struct {
	DirectiveID           string `json:"directive_id"`
	Text                  string `json:"text"`
	Intent                string `json:"intent,omitempty"`
	FalsifyingObservation string `json:"falsifying_observation,omitempty"`
}

func init() {
	govExtract := &cobra.Command{
		Use:   "extract-directives",
		Short: "Emit a topic file's compiled directives as JSON",
		Long: `Read the [edikt:directives:start]/[edikt:directives:end] region of a
compiled topic file and emit one JSON array of
{directive_id, text, intent?, falsifying_observation?}.

directive_id is <ref>.directive[<index>], indexed over the directives actually
found so the ids are stable for a given file.

An ABSENT region yields an empty array and exit 0 — a topic with no compiled
directives is a real answer. An unreadable FILE is an error, so a caller can
tell "nothing to verify" from "could not look".`,
		Args: cobra.NoArgs,
	}
	var topicFile, ref string
	govExtract.Flags().StringVar(&topicFile, "topic-file", "", "compiled topic file (required)")
	govExtract.Flags().StringVar(&ref, "ref", "", "artifact ref used to build directive_id (required)")
	_ = govExtract.MarkFlagRequired("topic-file")
	_ = govExtract.MarkFlagRequired("ref")
	govExtract.RunE = func(cmd *cobra.Command, args []string) error {
		b, err := os.ReadFile(topicFile) //nolint:gosec // operator-supplied path
		if err != nil {
			return err
		}
		out := extractDirectives(string(b), ref)
		enc, err := json.Marshal(out)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(enc))
		return nil
	}
	Cmd.AddCommand(govExtract)
}

func extractDirectives(body, ref string) []extractedDirective {
	// Never nil: a nil slice marshals to `null`, and a caller doing
	// `len(json.loads(...))` on null gets an error rather than zero.
	out := []extractedDirective{}
	m := directivesRegionRe.FindStringSubmatch(body)
	if m == nil {
		return out
	}
	lines := strings.Split(m[1], "\n")
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "- ") {
			i++
			continue
		}
		content := strings.TrimSpace(line[2:])
		e := extractedDirective{DirectiveID: fmt.Sprintf("%s.directive[%d]", ref, len(out))}
		if strings.HasPrefix(content, "text: ") {
			e.Text = strings.TrimSpace(content[len("text: "):])
			j := i + 1
			for j < len(lines) {
				sub := lines[j]
				// A non-indented line ends this entry's sub-fields. An EMPTY
				// line does not: the python treated "" as continuing, and a
				// blank line between sub-fields is ordinary formatting.
				if sub != "" && !strings.HasPrefix(sub, " ") && !strings.HasPrefix(sub, "\t") {
					break
				}
				s := strings.TrimSpace(sub)
				switch {
				case strings.HasPrefix(s, "intent: "):
					if g := gateField(strings.TrimSpace(s[len("intent: "):])); g != "" {
						e.Intent = g
					}
				case strings.HasPrefix(s, "falsifying_observation: "):
					if g := gateField(strings.TrimSpace(s[len("falsifying_observation: "):])); g != "" {
						e.FalsifyingObservation = g
					}
				}
				j++
			}
			i = j
		} else {
			e.Text = content
			i++
		}
		out = append(out, e)
	}
	return out
}

package gov

// directivecheck.go — `bin/edikt gov directive-check` subcommand.
//
// Pure Go port of the python heredoc previously embedded in
// commands/gov/_shared-directive-checks.md (lines 89-140 pre-Phase 11.5).
// Three checks (FR-003a, FR-003b, AC-003c) — see internal/dircheck for
// the implementation. This wrapper handles I/O only.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/diktahq/edikt/tools/edikt/internal/dircheck"
	"github.com/spf13/cobra"
)

var directiveCheckCmd = &cobra.Command{
	Use:   "directive-check",
	Short: "Run the three directive-quality checks (FR-003a, FR-003b, AC-003c)",
	Long: `Reads a JSON payload from stdin and emits one warning line per
triggered condition to stdout. The payload contract matches the
heredoc this command replaces:

  {
    "adr_id": "ADR-NNN",
    "directive_body": "All DB access MUST go through ...",
    "canonical_phrases": ["repository layer", ...],
    "no_directives_reason": null
  }

Exit 0 always — this command never blocks a caller (AC-021 grace
period). A clean directive emits no output.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := runDirectiveCheck(cmd, args)
		exitFromExitErr(err)
		return err
	},
}

func init() {
	Cmd.AddCommand(directiveCheckCmd)
}

func runDirectiveCheck(cmd *cobra.Command, args []string) error {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return &exitErr{code: 2, msg: fmt.Sprintf("read stdin: %v", err)}
	}
	if len(raw) == 0 {
		return &exitErr{code: 2, msg: "empty stdin payload"}
	}

	var in dircheck.Input
	dec := json.NewDecoder(bytesReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return &exitErr{code: 2, msg: fmt.Sprintf("parse stdin payload: %v", err)}
	}

	for _, w := range dircheck.Check(in) {
		fmt.Println(w)
	}
	return nil
}

// bytesReader is a tiny helper to avoid importing "bytes" twice.
func bytesReader(b []byte) io.Reader {
	return &byteSliceReader{b: b}
}

type byteSliceReader struct {
	b []byte
	i int
}

func (r *byteSliceReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

package cmd

// hook_event.go — `bin/edikt hook event`, the verb that replaced
// templates/hooks/event-log.sh (PLAN-v080-reliability-core Phase 23).
//
// WHAT IT REPLACED, AND WHY A VERB RATHER THAN A SHIM. event-log.sh was not a
// hook: it was a shell FUNCTION other hooks sourced, so it had no hook
// protocol, no stdin payload and nothing for the FR-004 exec shim to wrap.
// Every hook that used it is Go now and writes the ledger in-process, which
// left exactly two consumers — two command bodies that sourced it to record a
// drift report and a compile. A verb is what those two actually need: one
// argv-safe entry point that builds the record through a serializer.
//
// INV-003 IS THE REASON THE OLD ONE HAD python3 AT ALL. Building the record
// meant merging caller-supplied JSON with three computed fields, and doing
// that in bash means string concatenation, which INV-003 forbids. So the shim
// carried an interpreter for one json.dumps call — the last python3 in
// templates/hooks/, and the whole of FR-008's remaining work.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newHookEventCmd() *cobra.Command {
	var eventType, data string
	var fields []string

	cmd := &cobra.Command{
		Use:   "event",
		Short: "Append one structured event to ~/.edikt/events.jsonl",
		Long: `Append a structured event to the ledger at ~/.edikt/events.jsonl.

Replaces the sourced shell helper templates/hooks/event-log.sh. The caller
supplies a type and a JSON object of its own fields; this adds "type", "at"
and "by" and appends one line.

The ledger is created 0600 and re-chmodded on every call (MED-8): it carries
git email, gate findings and session metadata, and the default umask would
leave it world-readable.

FAILURE IS REPORTED, NEVER SILENT. The helper this replaces ended in
` + "`|| return 0`" + `, so a malformed payload or a missing interpreter lost the
record with nothing anywhere saying so. This exits non-zero and says what
happened on stderr.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(eventType) == "" {
				return errors.New("--type is required and must not be blank")
			}
			if data == "" {
				data = "{}"
			}

			// The caller's fields, decoded rather than concatenated. A
			// non-object is refused by name: merging computed fields into an
			// array or a scalar would silently produce a record shaped
			// nothing like every other line in the ledger.
			var base map[string]any
			if err := json.Unmarshal([]byte(data), &base); err != nil {
				return fmt.Errorf("--data is not a JSON object: %w", err)
			}
			if base == nil {
				base = map[string]any{}
			}

			// --field key=value exists so a SHELL caller never has to build
			// JSON. INV-003 forbids shell-concatenated JSON, and the caller
			// this replaced worked around it by shelling out to python3 for
			// one json.dumps — which is how the last interpreter dependency
			// survived in templates/hooks/pre-push, a file the migration's own
			// `*.sh` glob never counted.
			//
			// The value is taken verbatim as a STRING and never re-parsed: a
			// finding containing a brace or a quote is data, not structure.
			for _, kv := range fields {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return fmt.Errorf("--field %q is not key=value", kv)
				}
				base[k] = v
			}

			// Field-insertion order matches the helper's exactly: the
			// caller's keys first, then type, at, by. Go maps marshal sorted,
			// so the ORDER is not preserved in the output either way — what
			// matters is that a caller key named "type" is overwritten by the
			// computed one, as it was before.
			base["type"] = eventType
			base["at"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
			base["by"] = gitEmailOrUnknown()

			line, err := json.Marshal(base)
			if err != nil {
				return fmt.Errorf("encoding the event: %w", err)
			}

			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("resolving the home directory: %w", err)
			}
			logFile := filepath.Join(home, ".edikt", "events.jsonl")
			if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
				return fmt.Errorf("creating %s: %w", filepath.Dir(logFile), err)
			}
			f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // MED-8
			if err != nil {
				return fmt.Errorf("opening %s: %w", logFile, err)
			}
			defer func() { _ = f.Close() }()
			// Re-assert the mode: OpenFile leaves an EXISTING file's
			// permissions alone, so a ledger created loosely by an older
			// version would stay world-readable forever without this.
			_ = os.Chmod(logFile, 0o600) //nolint:gosec // MED-8
			if _, err := f.Write(append(line, '\n')); err != nil {
				return fmt.Errorf("appending to %s: %w", logFile, err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&eventType, "type", "", "event type, e.g. drift_report or compile (required)")
	cmd.Flags().StringVar(&data, "data", "{}", "a JSON object of caller fields")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "key=value, repeatable; the value is taken verbatim as a string so a shell caller never builds JSON")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

// gitEmailOrUnknown mirrors `git config user.email 2>/dev/null || echo unknown`.
// The fallback fires on a NON-ZERO exit only; a successful lookup returning
// empty stays empty rather than being upgraded to "unknown", because "the user
// has no email configured" and "git could not be asked" are different facts.
func gitEmailOrUnknown() string {
	out, err := exec.Command("git", "config", "user.email").Output() //nolint:gosec // fixed argv
	if err != nil {
		return "unknown"
	}
	return strings.TrimRight(string(out), "\r\n")
}

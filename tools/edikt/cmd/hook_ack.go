package cmd

// hook_ack.go — ADR-050 §2: the ack surface for the surfaced-ledger.
//
// Hooks append typed emissions to .edikt/state/hook-ledger.jsonl and consult
// .edikt/state/hook-acks.json before surfacing a known fingerprint. This verb
// group manages the acks file: `hook ack` records a visible, reasoned,
// expiring suppression; `hook held` lists; `hook unack` clears. --why is
// mandatory — an ack without a reason is the silent suppression the ledger
// exists to prevent. Event-based expiry (commit-touching:<path>) records the
// current HEAD for the path; hooks treat the ack as expired once
// `git log -1 --format=%H -- <path>` differs.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	hookAckUntil string
	hookAckWhy   string
)

// INV-006: fingerprints are hex tokens produced by the hooks' sha256; anything
// else is refused before it can become a JSON key or filesystem-adjacent value.
var hookFingerprintRe = regexp.MustCompile(`^[a-f0-9]{6,64}$`)

// untilSpecRe: ISO date, RFC3339, or the event forms.
var hookUntilDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}Z)?$`)

const hookAcksRelPath = ".edikt/state/hook-acks.json"

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage the surfaced-ledger hook channel (ack, held, unack)",
}

// (ref: ADR-050 — held items recorded in the hook ledger must never vanish)
var hookAckCmd = &cobra.Command{
	Use:   "ack <fingerprint>",
	Short: "Hold a known hook emission until an event or date, with a mandatory reason",
	Long: `Records a visible suppression for a hook-emission fingerprint in
.edikt/state/hook-acks.json. Held items still render as a count line on
every firing — they never vanish.

--until accepts:
  <YYYY-MM-DD> or RFC3339        date expiry
  commit-touching:<path>          expires when a new commit touches <path>
  compile-clean                   expires when gov compile reports 0 stale
--why is mandatory.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		fp := strings.TrimSpace(args[0])
		if !hookFingerprintRe.MatchString(fp) {
			// (ref: INV-006 — allowlist-validate externally supplied values)
			return &exitCodeError{code: 3, msg: fmt.Sprintf("hook ack: fingerprint %q is not a hex token", fp)}
		}
		if strings.TrimSpace(hookAckWhy) == "" {
			return &exitCodeError{code: 3, msg: "hook ack: --why is mandatory — an ack without a reason is silent suppression"}
		}
		until := strings.TrimSpace(hookAckUntil)
		head := ""
		switch {
		case hookUntilDateRe.MatchString(until):
			// date expiry — fine as-is
		case strings.HasPrefix(until, "commit-touching:"):
			p := strings.TrimPrefix(until, "commit-touching:")
			if p == "" || strings.Contains(p, "..") || filepath.IsAbs(p) {
				return &exitCodeError{code: 3, msg: "hook ack: commit-touching path must be a relative project path"}
			}
			out, err := exec.Command("git", "log", "-1", "--format=%H", "--", p).Output()
			if err == nil {
				head = strings.TrimSpace(string(out))
			}
			if head == "" {
				// No history for the path yet — record the repo HEAD so any
				// first commit touching it expires the ack.
				if out, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
					head = strings.TrimSpace(string(out))
				}
			}
		case until == "compile-clean":
			// event known to hooks; nothing to record
		default:
			return &exitCodeError{code: 3, msg: fmt.Sprintf("hook ack: --until %q is not a date, commit-touching:<path>, or compile-clean", until)}
		}

		acks, err := loadHookAcks()
		if err != nil {
			return &exitCodeError{code: 1, msg: err.Error()}
		}
		entry := map[string]string{
			"until": until,
			"why":   hookAckWhy,
			"at":    time.Now().UTC().Format(time.RFC3339),
		}
		if head != "" {
			entry["head"] = head
		}
		acks[fp] = entry
		if err := saveHookAcks(acks); err != nil {
			return &exitCodeError{code: 1, msg: err.Error()}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "held %s until %s — %s\n", fp, until, hookAckWhy)
		return nil
	},
}

var hookHeldCmd = &cobra.Command{
	Use:          "held",
	Short:        "List held hook emissions with their reasons and expiries",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		acks, err := loadHookAcks()
		if err != nil {
			return &exitCodeError{code: 1, msg: err.Error()}
		}
		if len(acks) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no held hook emissions")
			return nil
		}
		for fp, e := range acks {
			fmt.Fprintf(cmd.OutOrStdout(), "%s  until=%s  why=%s\n", fp, e["until"], e["why"])
		}
		return nil
	},
}

var hookUnackCmd = &cobra.Command{
	Use:          "unack <fingerprint>",
	Short:        "Clear a held hook emission",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		fp := strings.TrimSpace(args[0])
		if !hookFingerprintRe.MatchString(fp) {
			// (ref: INV-006 — allowlist-validate externally supplied values)
			return &exitCodeError{code: 3, msg: fmt.Sprintf("hook unack: fingerprint %q is not a hex token", fp)}
		}
		acks, err := loadHookAcks()
		if err != nil {
			return &exitCodeError{code: 1, msg: err.Error()}
		}
		if _, ok := acks[fp]; !ok {
			return &exitCodeError{code: 2, msg: fmt.Sprintf("hook unack: %s is not held", fp)}
		}
		delete(acks, fp)
		if err := saveHookAcks(acks); err != nil {
			return &exitCodeError{code: 1, msg: err.Error()}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "cleared %s\n", fp)
		return nil
	},
}

func loadHookAcks() (map[string]map[string]string, error) {
	acks := map[string]map[string]string{}
	raw, err := os.ReadFile(hookAcksRelPath)
	if err != nil {
		if os.IsNotExist(err) {
			return acks, nil
		}
		return nil, fmt.Errorf("read %s: %w", hookAcksRelPath, err)
	}
	if err := json.Unmarshal(raw, &acks); err != nil {
		return nil, fmt.Errorf("parse %s: %w", hookAcksRelPath, err)
	}
	return acks, nil
}

func saveHookAcks(acks map[string]map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(hookAcksRelPath), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(acks, "", "  ")
	if err != nil {
		return err
	}
	tmp := hookAcksRelPath + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, hookAcksRelPath)
}

func init() {
	hookAckCmd.Flags().StringVar(&hookAckUntil, "until", "", "expiry: <date>, commit-touching:<path>, or compile-clean (required)")
	hookAckCmd.Flags().StringVar(&hookAckWhy, "why", "", "reason for holding this emission (required)")
	_ = hookAckCmd.MarkFlagRequired("until")
	hookCmd.AddCommand(hookAckCmd, hookHeldCmd, hookUnackCmd)
	hookCmd.AddCommand(newHookMatchCmd(), newHookProbeCmd(), newHookReportCmd())
	rootCmd.AddCommand(hookCmd)
}

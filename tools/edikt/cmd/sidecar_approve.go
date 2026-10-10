package cmd

// sidecar_approve.go — `edikt sidecar approve` subcommand.
//
// Phase 3 of SPEC-009 Plan B. Promotes a pending behavioral verify proposal  // edikt-guard:allow
// from .edikt/state/pending-verifies/<id>.yaml into the originating sidecar's
// directives[<index>] entry, setting verify_kind: behavioral and
// human_approved_at: <RFC3339 UTC now>.
//
// Per ADR-039:  // edikt-guard:allow
//   - The binary is args-driven and non-interactive. No stdin prompts. No
//     $EDITOR. No TTY assumption. The human decision is captured by the
//     tier-1 wrapper (commands/sidecar/approve.md) and delivered as the
//     --decision flag value.
//   - Exit codes:
//       0 — success (or intentional no-op for --decision=defer)
//       1 — validation or IO error
//       2 — pending-id not found in .edikt/state/pending-verifies/
//       3 — invalid args (missing --decision, bad enum value, malformed
//           --edited-content path, or --edited-content combined with a
//           --kind that has no single value for it to replace)
//   - Per ADR-030, this file MUST stay LLM-agnostic — covered by the  // edikt-guard:allow
//     no-llm-in-tier-2 grep gate in .github/workflows/sidecar-checks.yml.

import (
	"bufio"
	"fmt"
	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// approve flags
var (
	approveDecision      string
	approveEditedContent string
	// approvePositiveFixture / approveNegativeFixture override the
	// conventional test/fixtures/behavioral/<artifact-id>/{positive,negative}.sh
	// pair. Only used with --decision=approve.
	approvePositiveFixture string
	approveNegativeFixture string
	// approveKind selects which ceremony this invocation runs. "verify" is
	// the ADR-039 original and stays the default, so every pre-SPEC-011  // edikt-guard:allow
	// invocation keeps its exact meaning. See sidecar_approve_kinds.go.
	approveKind string
	// approveList switches to read-only batch review: emit every pending
	// proposal of the selected kind as JSON and mutate nothing.
	approveList    bool
	approveRemoval bool
)

// pendingVerify is the on-disk shape of a single pending-verify proposal
// emitted by the sidecar-extractor under .edikt/state/pending-verifies/<id>.yaml.
type pendingVerify struct {
	ID                    string `yaml:"id"`
	SidecarPath           string `yaml:"sidecar_path"`
	DirectiveIndex        int    `yaml:"directive_index"`
	EntryKind             string `yaml:"entry_kind,omitempty"` // "" | "directive" | "prohibition" (D47)
	ProposedVerify        string `yaml:"proposed_verify"`
	Intent                string `yaml:"intent,omitempty"`
	FalsifyingObservation string `yaml:"falsifying_observation,omitempty"`
	ProposedAt            string `yaml:"proposed_at,omitempty"`
}

var approveCmd = &cobra.Command{
	Use:   "approve <pending-id>",
	Short: "Promote, reject, or defer a pending behavioral verify proposal",
	Long: `Promote, reject, or defer a pending behavioral verify proposal.

Reads a pending verify entry at .edikt/state/pending-verifies/<pending-id>.yaml
and either writes it into the originating sidecar's directives[] entry
(approve), discards it (reject), or leaves state unchanged (defer).

The decision is consumed via --decision; the binary is args-driven and
non-interactive (no stdin prompts, no $EDITOR, no TTY assumption). The
tier-1 wrapper (commands/sidecar/approve.md) is responsible for the human
UX.

Invocation forms:
  edikt sidecar approve <id> --decision=approve
  edikt sidecar approve <id> --decision=reject
  edikt sidecar approve <id> --decision=defer
  edikt sidecar approve <id> --decision=approve --edited-content=<path>

On --decision=approve the sidecar's directives[<directive_index>] entry gets:
  - verify         := <proposed_verify> from pending file
                      (or contents of --edited-content if supplied)
  - verify_kind    := "behavioral"
  - human_approved_at := <ISO8601 UTC now>
and the pending file is removed.

--edited-content is honoured by --kind=verify (it replaces the verify command)
and by --kind=topic-description (it replaces the description). It is REFUSED
with exit 3 for --kind=paths: that kind proposes a list — proposed_paths plus
proposed_removals — with no single value for a file's contents to replace. The
refusal is deliberate; accepting and ignoring the flag discarded a correction
the human had made while still reporting success. To correct a paths proposal,
edit .edikt/state/pending-paths/<pending-id>.yaml directly and then approve it.

Exit codes:
  0 — success (or intentional no-op for --decision=defer)
  1 — validation or IO error
  2 — pending-id not found in .edikt/state/pending-verifies/
  3 — invalid args (missing --decision, bad enum value, malformed
      --edited-content path, or --edited-content on a kind that
      cannot honour it)`,
	// MaximumNArgs, not ExactArgs: --list is a whole-directory read and takes
	// no pending-id. The zero-arg case without --list is still rejected below
	// with exit 3, so the contract for a decision invocation is unchanged.
	Args: cobra.MaximumNArgs(1),
	RunE: runSidecarApprove,
}

func init() {
	approveCmd.Flags().StringVar(&approveDecision, "decision", "",
		"decision: approve | reject | defer (required)")
	// The kind restriction belongs in the flag's own description, not only in
	// the Long text: a reader who greps the flag list is the reader most
	// likely to reach for it on the kind that refuses it.
	approveCmd.Flags().StringVar(&approveEditedContent, "edited-content", "",
		"path to a file whose contents replace the proposed value (only with --decision=approve, "+
			"and only for --kind=verify or --kind=topic-description; unsupported for --kind=paths, "+
			"which proposes a list rather than a single value — that combination exits 3)")
	approveCmd.Flags().StringVar(&approvePositiveFixture, "positive-fixture", "",
		"path to the positive bidirectional fixture (default: test/fixtures/behavioral/<artifact-id>/positive.sh)")
	approveCmd.Flags().StringVar(&approveNegativeFixture, "negative-fixture", "",
		"path to the negative bidirectional fixture (default: test/fixtures/behavioral/<artifact-id>/negative.sh)")
	approveCmd.Flags().StringVar(&approveKind, "kind", kindVerify,
		"what is being approved: verify | paths | topic-description")
	approveCmd.Flags().BoolVar(&approveRemoval, "removal", false,
		"with --kind=topic-description --decision=reject: remove the topic's APPROVED registry entry "+
			"(for an orphan whose topic no longer exists in the corpus). Never automatic — a topic absent "+
			"from one compile may be a source artifact mid-rename")
	approveCmd.Flags().BoolVar(&approveList, "list", false,
		"batch review: emit every pending proposal of --kind as JSON and exit; mutates nothing")
	// Don't use MarkFlagRequired — cobra's default error path exits with code 1
	// from the framework, but the ADR-039 contract demands exit 3 for missing  // edikt-guard:allow
	// --decision. We validate manually below and return exitCodeError{code: 3}.

	sidecarCmd.AddCommand(approveCmd)
}

func runSidecarApprove(cmd *cobra.Command, args []string) error {
	// ── Validate --kind before anything else: it selects which contract the
	// rest of this invocation is under. ──────────────────────────────────────
	if _, ok := pendingDirForKind[approveKind]; !ok {
		return &exitCodeError{code: 3, msg: fmt.Sprintf(
			"--kind: invalid value %q (allowed: %s, %s, %s)",
			approveKind, kindVerify, kindPaths, kindTopicDescription)}
	}

	// ── --list is read-only batch review; no pending-id, no decision. ───────
	if approveList {
		if len(args) > 0 {
			return &exitCodeError{code: 3, msg: "--list takes no pending-id (it reports every pending proposal of --kind)"}
		}
		if approveDecision != "" {
			return &exitCodeError{code: 3, msg: "--list is read-only and cannot be combined with --decision"}
		}
		return runApproveList(approveKind)
	}

	if len(args) == 0 {
		return &exitCodeError{code: 3, msg: "pending-id is required (or pass --list for batch review)"}
	}
	pendingID := strings.TrimSpace(args[0])

	// ── Validate --decision (exit 3 on missing or invalid value) ─────────────
	if approveDecision == "" {
		return &exitCodeError{code: 3, msg: "--decision is required (approve | reject | defer)"}
	}
	switch approveDecision {
	case "approve", "reject", "defer":
		// ok
	default:
		return &exitCodeError{code: 3, msg: fmt.Sprintf(
			"--decision: invalid value %q (allowed: approve, reject, defer)", approveDecision)}
	}

	// --edited-content only valid with --decision=approve
	if approveEditedContent != "" && approveDecision != "approve" {
		return &exitCodeError{code: 3, msg: "--edited-content is only valid with --decision=approve"}
	}

	// ── Validate pending-id (basic charset; no traversal) ────────────────────
	if pendingID == "" {
		return &exitCodeError{code: 3, msg: "pending-id is required"}
	}
	if strings.ContainsAny(pendingID, "/\\") || strings.Contains(pendingID, "..") {
		return &exitCodeError{code: 3, msg: fmt.Sprintf("pending-id %q: invalid (no path separators or traversal)", pendingID)}
	}

	// --edited-content is a replace-ONE-STRING mechanism, and only two of the
	// three kinds have a single string for it to replace: verify replaces the
	// verify command, topic-description replaces the description. A paths
	// proposal carries a LIST — proposed_paths plus proposed_removals — so
	// there is no scalar for a file's contents to become.
	//
	// Until this guard the flag was accepted, ignored, and the approval still
	// reported success: state mutated, pending file removed, and the human's
	// correction gone with no signal. That is absence of effect reported as
	// success, which INV-011 names a failure state and never a silent  // edikt-guard:allow
	// success. The check runs BEFORE dispatch, so a refused invocation leaves
	// the sidecar and the pending file exactly as it found them.
	//
	// Refusing honestly is not the same as solving. What the correction
	// ceremony for a disputed paths proposal should be is a separate, unruled
	// question; the interim answer this message gives — edit the pending file
	// — is safe because the glob validation re-runs against the live tree at
	// approval time and never trusts the proposal's own matched_example.
	if approveEditedContent != "" && approveKind == kindPaths {
		return &exitCodeError{code: 3, msg: fmt.Sprintf(
			"--edited-content is not supported for --kind=%s: a paths proposal carries a list "+
				"(proposed_paths + proposed_removals), not one value for a file's contents to replace. "+
				"Refusing rather than discarding your edit. To correct a paths proposal, edit "+
				".edikt/state/pending-paths/%s.yaml directly and then approve it — the glob validation "+
				"re-runs against the live tree at approval time, so an edited proposal is still checked. "+
				"--edited-content is honoured by --kind=%s and --kind=%s.",
			kindPaths, pendingID, kindVerify, kindTopicDescription)}
	}

	// ── Dispatch the non-verify kinds. Everything below this point is the
	// ADR-039 verify ceremony, unchanged. ────────────────────────────────────  // edikt-guard:allow
	// Removal of an APPROVED registry entry, on the same gate as approving
	// one. It takes no pending file: an orphan has none — the compile stopped
	// producing the topic, which is why the entry is orphaned.
	if approveRemoval {
		if approveKind != kindTopicDescription || approveDecision != "reject" {
			return &exitCodeError{code: 3, msg: "--removal applies only to --kind=topic-description --decision=reject"}
		}
		root, rootErr := os.Getwd()
		if rootErr != nil {
			return &exitCodeError{code: 1, msg: fmt.Sprintf("resolve project root: %v", rootErr)}
		}
		return removeRegistryEntry(root, pendingID)
	}

	switch approveKind {
	case kindPaths:
		return runApprovePaths(pendingID, approveDecision)
	case kindTopicDescription:
		return runApproveTopicDescription(pendingID, approveDecision)
	}

	// ── Resolve pending file path ────────────────────────────────────────────
	pendingDir, err := resolvePendingVerifiesDir()
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("resolve pending-verifies dir: %v", err)}
	}
	pendingPath := filepath.Join(pendingDir, pendingID+".yaml")

	// ── Defer is a no-op — short-circuit before any disk reads of the
	// pending file. Per ADR-039: defer leaves state unchanged. We still  // edikt-guard:allow
	// require the pending file to exist for `defer` to mean anything;
	// otherwise the caller is referencing a missing entity and the
	// exit-2 contract should fire.
	if _, statErr := os.Stat(pendingPath); os.IsNotExist(statErr) {
		return &exitCodeError{code: 2, msg: fmt.Sprintf("pending-id not found: %s", pendingPath)}
	} else if statErr != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("stat pending file: %v", statErr)}
	}

	// DEFER IS NOT CAPTURED, deliberately. The phase-5 ruling names approve /
	// reject / edit; defer is an intentional no-op that does not even read the
	// pending file. Capturing it would mean loading and strict-decoding first,
	// so a malformed pending file would start failing a command that currently
	// succeeds — changing defer's contract to record a non-decision.
	if approveDecision == "defer" {
		fmt.Fprintf(os.Stdout, "ok: deferred %s (no state change)\n", pendingID)
		return nil
	}

	// ── Load pending file (strict decode) ────────────────────────────────────
	pv, err := loadPendingVerify(pendingPath)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("load pending file: %v", err)}
	}

	// ── Reject: remove the pending file, no sidecar mutation ────────────────
	if approveDecision == "reject" {
		// Capture BEFORE the delete. This is the labelled negative — the most
		// informative outcome of the ceremony and, until now, the only one
		// that left no trace (phase 5).
		recordApprovalDiff(pendingPath, approvalDiff{
			PendingID:             pendingID,
			Decision:              "reject",
			DecidedAt:             approvalNow(),
			SidecarPath:           pv.SidecarPath,
			DirectiveIndex:        pv.DirectiveIndex,
			ProposedVerify:        pv.ProposedVerify,
			Intent:                pv.Intent,
			FalsifyingObservation: pv.FalsifyingObservation,
			ProposedAt:            pv.ProposedAt,
		})
		if err := os.Remove(pendingPath); err != nil {
			return &exitCodeError{code: 1, msg: fmt.Sprintf("remove pending file: %v", err)}
		}
		fmt.Fprintf(os.Stdout, "ok: rejected %s (pending file removed)\n", pendingID)
		return nil
	}

	// ── Approve flow ─────────────────────────────────────────────────────────

	if pv.SidecarPath == "" {
		return &exitCodeError{code: 1, msg: "pending file missing sidecar_path"}
	}
	if pv.DirectiveIndex < 0 {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("pending file directive_index %d: must be >= 0", pv.DirectiveIndex)}
	}

	// Resolve sidecar path. Allow either absolute or repo-relative; reject
	// traversal patterns up-front.
	if strings.Contains(pv.SidecarPath, "..") {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("sidecar_path %q: contains traversal", pv.SidecarPath)}
	}
	sidecarPath := pv.SidecarPath
	if !filepath.IsAbs(sidecarPath) {
		// Resolve relative to CWD (the same anchor used by other sidecar
		// subcommands). Absolute paths from the pending file are honored
		// as-is.
		abs, absErr := filepath.Abs(sidecarPath)
		if absErr != nil {
			return &exitCodeError{code: 1, msg: fmt.Sprintf("resolve sidecar_path: %v", absErr)}
		}
		sidecarPath = abs
	}

	if _, statErr := os.Stat(sidecarPath); os.IsNotExist(statErr) {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("sidecar not found: %s", sidecarPath)}
	}

	// Determine the verify command body. --edited-content overrides
	// proposed_verify when supplied.
	verifyCmd := pv.ProposedVerify
	if approveEditedContent != "" {
		// Validate path: must exist, must not traverse, must be a regular
		// file. We do not bound the size — the verify command can be a
		// multi-line script; the sidecar's directives[].text limit is 500
		// chars but verify has no such cap in the schema.
		if strings.Contains(approveEditedContent, "..") {
			return &exitCodeError{code: 3, msg: fmt.Sprintf("--edited-content %q: contains traversal", approveEditedContent)}
		}
		absEdit, absErr := filepath.Abs(approveEditedContent)
		if absErr != nil {
			return &exitCodeError{code: 3, msg: fmt.Sprintf("--edited-content: %v", absErr)}
		}
		info, statErr := os.Lstat(absEdit)
		if os.IsNotExist(statErr) {
			return &exitCodeError{code: 3, msg: fmt.Sprintf("--edited-content not found: %s", absEdit)}
		}
		if statErr != nil {
			return &exitCodeError{code: 3, msg: fmt.Sprintf("--edited-content: %v", statErr)}
		}
		if !info.Mode().IsRegular() {
			return &exitCodeError{code: 3, msg: fmt.Sprintf("--edited-content %s: not a regular file", absEdit)}
		}
		body, readErr := os.ReadFile(absEdit)
		if readErr != nil {
			return &exitCodeError{code: 1, msg: fmt.Sprintf("read --edited-content: %v", readErr)}
		}
		// Trim trailing newlines/whitespace — a verify command body is a
		// shell script-like scalar; trailing whitespace produces noisy
		// canonical YAML diffs.
		verifyCmd = strings.TrimRight(string(body), "\n\r\t ")
	}

	if strings.TrimSpace(verifyCmd) == "" {
		return &exitCodeError{code: 1, msg: "proposed_verify is empty (nothing to approve)"}
	}

	// ── Bidirectional fixture gate, BEFORE any mutation ─────────────────────
	//
	// Phase B compile refuses any behavioral directive whose
	// positive_fixture_path / negative_fixture_path is empty (merge.go
	// validatePhaseBConstraints). Approve used to set verify_kind: behavioral
	// unconditionally and say nothing about this — the first approval of a
	// batch of 17 broke `gov compile` downstream, discovered only by running
	// it, not by approve refusing. This check makes that refusal happen HERE,
	// with a named reason, instead of silently accepting a proposal the
	// corpus cannot compile.
	posFixture, negFixture, fixtureErr := resolveBidirectionalFixtures(sidecarPath, pendingID)
	if fixtureErr != nil {
		return &exitCodeError{code: 1, msg: fixtureErr.Error()}
	}

	// ── Load sidecar, mutate, marshal, atomic write ─────────────────────────
	sc, err := sidecar.Load(sidecarPath)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("load sidecar: %v", err)}
	}

	approvedAt := time.Now().UTC().Format(time.RFC3339)
	switch pv.EntryKind {
	case "prohibition":
		if pv.DirectiveIndex >= len(sc.Prohibitions) {
			return &exitCodeError{code: 1, msg: fmt.Sprintf(
				"directive_index %d out of range (sidecar has %d prohibitions)",
				pv.DirectiveIndex, len(sc.Prohibitions))}
		}
		pr := &sc.Prohibitions[pv.DirectiveIndex]
		pr.Verify, pr.VerifyKind, pr.HumanApprovedAt = verifyCmd, "behavioral", approvedAt
		pr.PositiveFixturePath, pr.NegativeFixturePath = posFixture, negFixture
		if pr.FalsifyingObservation == "" && pv.FalsifyingObservation != "" {
			pr.FalsifyingObservation = pv.FalsifyingObservation
		}
		if pr.Intent == "" && pv.Intent != "" {
			pr.Intent = pv.Intent
		}
	case "", "directive":
		if pv.DirectiveIndex >= len(sc.Directives) {
			return &exitCodeError{code: 1, msg: fmt.Sprintf(
				"directive_index %d out of range (sidecar has %d directives)",
				pv.DirectiveIndex, len(sc.Directives))}
		}
		d := &sc.Directives[pv.DirectiveIndex]
		d.Verify, d.VerifyKind, d.HumanApprovedAt = verifyCmd, "behavioral", approvedAt
		d.PositiveFixturePath, d.NegativeFixturePath = posFixture, negFixture
		if d.FalsifyingObservation == "" && pv.FalsifyingObservation != "" {
			d.FalsifyingObservation = pv.FalsifyingObservation
		}
		if d.Intent == "" && pv.Intent != "" {
			d.Intent = pv.Intent
		}
	default:
		return &exitCodeError{code: 1, msg: fmt.Sprintf("pending file entry_kind %q: must be directive or prohibition", pv.EntryKind)}
	}

	if err := sc.Validate(); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("validate after approve: %v", err)}
	}

	// Use Marshal: sc was loaded then mutated, so the load-time
	// cache is stale (Marshal would emit pre-mutation bytes).
	out, err := sidecar.Marshal(sc)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("marshal sidecar: %v", err)}
	}
	if err := migrate.AtomicWriteNoFollow(sidecarPath, out, 0o644); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("write sidecar: %v", err)}
	}

	// Remove the pending file only after the sidecar write succeeded —
	// if write fails we want the pending entry to survive so the
	// operation can be retried.
	if err := os.Remove(pendingPath); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("remove pending file (sidecar already updated): %v", err)}
	}

	accepted := sc.Directives[pv.DirectiveIndex].Verify
	recordApprovalDiff(pendingPath, approvalDiff{
		PendingID:             pendingID,
		Decision:              "approve",
		DecidedAt:             approvalNow(),
		SidecarPath:           pv.SidecarPath,
		DirectiveIndex:        pv.DirectiveIndex,
		ProposedVerify:        pv.ProposedVerify,
		Intent:                pv.Intent,
		FalsifyingObservation: pv.FalsifyingObservation,
		ProposedAt:            pv.ProposedAt,
		AcceptedVerify:        accepted,
		// The edit is the richest signal: "nearly, but not that".
		Edited: accepted != pv.ProposedVerify,
	})

	fmt.Fprintf(os.Stdout, "ok: approved %s\n", pendingID)
	fmt.Fprintf(os.Stdout, "    sidecar:         %s\n", sidecarPath)
	fmt.Fprintf(os.Stdout, "    directive_index: %d\n", pv.DirectiveIndex)
	fmt.Fprintf(os.Stdout, "    verify_kind:     behavioral\n")
	fmt.Fprintf(os.Stdout, "    human_approved_at: %s\n", sc.Directives[pv.DirectiveIndex].HumanApprovedAt)
	return nil
}

// approveArtifactIDRe extracts an artifact id from a sidecar filename or
// pending-id. Mirrors govIDRe's shape (verify_gov.go) without the anchors,
// since this is a FindString over a larger string, not a full-string match.
// SR-005 (SPEC-018): id segments are uppercase-or-digit, not digits only.
// `ADR-GOLD-001` is a valid id; the old `\d{3,}` form rejected or truncated it,
// and fixing only discover.go would relocate the collision to this consumer.
var approveArtifactIDRe = regexp.MustCompile(`(?:ADR|INV)(?:-[A-Z0-9]+)*-\d{3,}|GL-\d{3,}-[a-z][a-z0-9-]*`)

// resolveBidirectionalFixtures computes the positive/negative fixture paths
// for a directive being approved and REFUSES (non-nil err) if either is
// missing on disk. Phase B compile (phaseb/merge.go validatePhaseBConstraints)
// requires both path fields to be non-empty for verify_kind: behavioral, and
// the corpus's own established convention is
// test/fixtures/behavioral/<artifact-id>/{positive,negative}.sh, already
// used by several other artifacts' approved directives.  edikt-guard:allow
// Before this,
// approve set verify_kind: behavioral unconditionally and said nothing about
// the fixture requirement — the gap surfaced only on the next `gov compile`,
// downstream of the decision that created it, discovered by breaking a
// batch of 17 trial approvals rather than by the tool refusing up front.
func resolveBidirectionalFixtures(sidecarPath, pendingID string) (positive, negative string, err error) {
	root, rootErr := findProjectRootForVerify()
	if rootErr != nil {
		return "", "", fmt.Errorf("resolve project root for fixture check: %w", rootErr)
	}

	artifactID := approveArtifactIDRe.FindString(filepath.Base(sidecarPath))
	if artifactID == "" {
		artifactID = approveArtifactIDRe.FindString(pendingID)
	}
	if artifactID == "" {
		return "", "", fmt.Errorf(
			"could not derive an artifact id from sidecar path %q or pending-id %q to locate its bidirectional fixtures",
			sidecarPath, pendingID)
	}

	positive = approvePositiveFixture
	if positive == "" {
		positive = filepath.Join("test", "fixtures", "behavioral", artifactID, "positive.sh")
	}
	negative = approveNegativeFixture
	if negative == "" {
		negative = filepath.Join("test", "fixtures", "behavioral", artifactID, "negative.sh")
	}

	var missing []string
	for _, rel := range []string{positive, negative} {
		abs := rel
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, rel)
		}
		info, statErr := os.Stat(abs)
		if statErr != nil || !info.Mode().IsRegular() {
			missing = append(missing, rel)
		}
	}
	if len(missing) > 0 {
		verb := "is"
		if len(missing) > 1 {
			verb = "are"
		}
		return "", "", fmt.Errorf(
			"refusing to approve %s: verify_kind would be behavioral, which Phase B compile requires a bidirectional fixture pair for, but %s %s missing. "+
				"Write the fixture(s) (a positive.sh that exits 0 when the property holds, a negative.sh that exits non-zero when it is violated), "+
				"or pass --positive-fixture/--negative-fixture to point at existing ones, or --decision=defer to leave this proposal for later",
			pendingID, strings.Join(missing, " and "), verb)
	}
	return positive, negative, nil
}

// resolvePendingVerifiesDir finds .edikt/state/pending-verifies/ by walking up
// from CWD looking for a .edikt/ directory. Falls back to ./.edikt if none
// found in ancestors — the err path is reserved for filesystem failure, not
// "no .edikt found".
func resolvePendingVerifiesDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; {
		candidate := filepath.Join(dir, ".edikt", "state", "pending-verifies")
		// Use the parent .edikt/ marker as the anchor — the
		// pending-verifies subdir may not yet exist on a fresh project.
		ediktMarker := filepath.Join(dir, ".edikt")
		if info, statErr := os.Stat(ediktMarker); statErr == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root with no .edikt/ found. Fall back to
			// CWD-relative; the missing pending file produces exit 2 down
			// the line.
			return filepath.Join(cwd, ".edikt", "state", "pending-verifies"), nil
		}
		dir = parent
	}
}

// loadPendingVerify decodes a pending-verify file with strict (KnownFields)
// decoding so unknown fields trip an error rather than silently dropping.
func loadPendingVerify(path string) (*pendingVerify, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := yaml.NewDecoder(bufio.NewReader(f))
	dec.KnownFields(true)
	var pv pendingVerify
	if err := dec.Decode(&pv); err != nil {
		return nil, err
	}
	return &pv, nil
}

package cmd

// prd.go — `bin/edikt prd {ship,close,link-spec}`, the PRD lifecycle
// mutations that replaced inline python in commands/sdlc/{prd,spec}.md
// (PLAN-v080 Phase 24, FR-010).
//
// WHY THESE ARE VERBS AND NOT A GENERIC "SET THIS YAML PATH".
//
// A generic setter would have removed the python faster and been the wrong
// tool: every one of these operations mutates a sidecar AND appends a
// revision_history entry recording who changed what and why. A caller that
// could set a field without the history could write an unaudited change to a
// sidecar, which is precisely what revision_history exists to prevent.
//
// So the history append is not a flag. It is unconditional, and there is no
// verb here that mutates without it.
//
// _sync IS CLEARED, NEVER RECOMPUTED HERE. The hashes describe agreement
// between the .md and the .yaml, and this only touches the .yaml — computing
// them would assert an agreement this command cannot see. The caller
// recomputes after it has both halves, and an empty _sync is the honest
// intermediate state rather than a stale-but-plausible one.

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func init() {
	prdCmd := &cobra.Command{Use: "prd", Short: "PRD sidecar lifecycle operations"}
	prdCmd.AddCommand(newPRDShipCmd(), newPRDCloseCmd(), newPRDLinkSpecCmd(), newPRDSupersedeCmd())
	rootCmd.AddCommand(prdCmd)
}

// loadSidecar reads a sidecar as a yaml.Node so key order AND comments survive
// the round trip. The python this replaces used safe_dump, which preserved
// order and dropped every comment in the file.
func loadSidecar(path string) (*yaml.Node, *yaml.Node, error) {
	b, err := os.ReadFile(path) //nolint:gosec // operator-supplied path
	if err != nil {
		return nil, nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, nil, fmt.Errorf("%s is not valid YAML: %w", path, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%s is not a YAML mapping", path)
	}
	return &doc, doc.Content[0], nil
}

func saveSidecar(path string, doc *yaml.Node) error {
	b, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644) //nolint:gosec // sidecars are world-readable by design
}

func mapGet(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mapSet(m *yaml.Node, key, value string) {
	if v := mapGet(m, key); v != nil {
		v.Kind, v.Tag, v.Value, v.Content = yaml.ScalarNode, "!!str", value, nil
		return
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

func mapEnsureSeq(m *yaml.Node, key string) *yaml.Node {
	if v := mapGet(m, key); v != nil {
		return v
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, seq)
	return seq
}

// appendHistory records the change. Called by every verb in this file, with no
// way to opt out — see the package comment.
func appendHistory(root *yaml.Node, at, author, action, note string, affected []string) {
	entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	add := func(k, v string) {
		entry.Content = append(entry.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
	}
	add("at", at)
	add("author", author)
	add("action", action)
	add("note", note)
	if len(affected) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, a := range affected {
			seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: a})
		}
		entry.Content = append(entry.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "affected"}, seq)
	}
	mapEnsureSeq(root, "revision_history").Content = append(
		mapEnsureSeq(root, "revision_history").Content, entry)
}

// clearSync empties the .md/.yaml agreement hashes. See the package comment.
func clearSync(root *yaml.Node) {
	sync := mapGet(root, "_sync")
	if sync == nil || sync.Kind != yaml.MappingNode {
		sync = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "_sync"}, sync)
	}
	for _, k := range []string{"md_hash", "yaml_hash", "synced_at"} {
		mapSet(sync, k, "")
	}
}

type auditFlags struct{ author, at string }

func (a *auditFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&a.author, "author", "", "who is making the change (required)")
	cmd.Flags().StringVar(&a.at, "at", "", "RFC3339 UTC timestamp (required)")
	_ = cmd.MarkFlagRequired("author")
	_ = cmd.MarkFlagRequired("at")
}

func newPRDShipCmd() *cobra.Command {
	var file, frs, acs, note string
	audit := &auditFlags{}
	cmd := &cobra.Command{
		Use:   "ship",
		Short: "Mark requirements shipped and roll up the PRD status",
		Long: `Mark the named requirements shipped.

Only requirements NOT already shipped are affected, so re-running is a no-op
rather than a second history entry claiming work that already happened. The
top-level status rolls up to shipped only when EVERY requirement is shipped and
there is at least one — an empty requirements list must not read as complete.

Prints the affected ids, comma-separated.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, root, err := loadSidecar(file)
			if err != nil {
				return err
			}
			want := map[string]bool{}
			for _, id := range strings.Split(frs, ",") {
				if id = strings.TrimSpace(id); id != "" {
					want[id] = true
				}
			}
			reqs := mapGet(root, "requirements")
			var affected []string
			total, shipped := 0, 0
			if reqs != nil && reqs.Kind == yaml.SequenceNode {
				for _, r := range reqs.Content {
					if r.Kind != yaml.MappingNode {
						continue
					}
					total++
					idNode, statusNode := mapGet(r, "id"), mapGet(r, "status")
					cur := ""
					if statusNode != nil {
						cur = statusNode.Value
					}
					if idNode != nil && want[idNode.Value] && cur != "shipped" {
						mapSet(r, "status", "shipped")
						mapSet(r, "shipped_at", audit.at)
						affected = append(affected, idNode.Value)
						cur = "shipped"
					}
					if cur == "shipped" {
						shipped++
					}
				}
			}
			// --note carries the EVIDENCE. "Marked shipped: FR-005" records that
			// someone said so; a note citing the phase and commit records why
			// anyone should believe it, which is the difference between an
			// audit trail and a log.
			msg := "Marked shipped: " + strings.Join(affected, ", ")
			if len(affected) == 0 {
				// The history append stays unconditional — running the verb is
				// itself a fact worth recording, and a note may be the whole
				// point of the invocation (closing a PRD whose requirements
				// were all shipped earlier). But the MESSAGE must not claim a
				// ship that did not happen: "Marked shipped: " with an empty id
				// list reads as a ship of nothing and is indistinguishable, on
				// later review, from a ship whose ids were lost.
				msg = "No requirements changed — every id named was already shipped"
			}
			if note != "" {
				msg += " — " + note
			}
			// Acceptance criteria travel with their requirement. Reconciling
			// them through the SAME verb keeps the history append
			// unconditional — a separate script that set AC statuses directly
			// would be exactly the unaudited mutation these verbs exist to
			// make impossible.
			if acs != "" {
				wantAC := map[string]bool{}
				for _, id := range strings.Split(acs, ",") {
					if id = strings.TrimSpace(id); id != "" {
						wantAC[id] = true
					}
				}
				if list := mapGet(root, "acceptance_criteria"); list != nil && list.Kind == yaml.SequenceNode {
					for _, a := range list.Content {
						if a.Kind != yaml.MappingNode {
							continue
						}
						idNode := mapGet(a, "id")
						if idNode == nil || !wantAC[idNode.Value] {
							continue
						}
						if st := mapGet(a, "status"); st == nil || st.Value != "shipped" {
							mapSet(a, "status", "shipped")
							affected = append(affected, idNode.Value)
						}
					}
				}
			}
			appendHistory(root, audit.at, audit.author, "ship", msg, affected)
			// An EMPTY requirements list is not "all shipped". The python
			// guarded this with `if all_shipped and data.get("requirements")`
			// and the guard is the load-bearing half.
			if total > 0 && shipped == total {
				mapSet(root, "status", "shipped")
			}
			clearSync(root)
			if err := saveSidecar(file, doc); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), strings.Join(affected, ","))
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "PRD sidecar path (required)")
	cmd.Flags().StringVar(&frs, "frs", "", "comma-separated requirement ids (required)")
	cmd.Flags().StringVar(&acs, "acs", "", "comma-separated acceptance-criterion ids to reconcile alongside")
	cmd.Flags().StringVar(&note, "note", "", "evidence for the closure — the phase and commit that closed it")
	_ = cmd.MarkFlagRequired("file")

	audit.bind(cmd)
	return cmd
}

func newPRDCloseCmd() *cobra.Command {
	var file, status, reason string
	audit := &auditFlags{}
	cmd := &cobra.Command{
		Use:   "close",
		Short: "Cancel or deprecate a PRD, recording the reason",
		Long: `Set the PRD's terminal status and record why.

` + "`cancelled`" + ` and ` + "`deprecated`" + ` are the same operation with a different word, so
they are one verb with a closed --status enum rather than two that drift.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if status != "cancelled" && status != "deprecated" {
				return fmt.Errorf("--status must be cancelled or deprecated, got %q", status)
			}
			if strings.TrimSpace(reason) == "" {
				// A terminal status with no reason is the shape someone reads
				// six months later and cannot act on.
				return fmt.Errorf("--reason is required: a PRD closed without one cannot be understood later")
			}
			doc, root, err := loadSidecar(file)
			if err != nil {
				return err
			}
			mapSet(root, "status", status)
			mapSet(root, status+"_at", audit.at)
			mapSet(root, status+"_reason", reason)
			// An explicit map, not suffix-trimming. The clever version got
			// "cancelled" -> "cancel" right and "deprecated" -> "deprecated"
			// wrong, writing a history action the python never produced — the
			// kind of near-miss that survives review because the common case
			// works.
			action := map[string]string{"cancelled": "cancel", "deprecated": "deprecate"}[status]
			appendHistory(root, audit.at, audit.author, action,
				strings.ToUpper(status[:1])+status[1:]+": "+reason, nil)
			clearSync(root)
			return saveSidecar(file, doc)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "PRD sidecar path (required)")
	cmd.Flags().StringVar(&status, "status", "", "cancelled | deprecated (required)")
	cmd.Flags().StringVar(&reason, "reason", "", "why (required)")
	_ = cmd.MarkFlagRequired("file")
	_ = cmd.MarkFlagRequired("status")
	_ = cmd.MarkFlagRequired("reason")
	audit.bind(cmd)
	return cmd
}

func newPRDLinkSpecCmd() *cobra.Command {
	var file, spec string
	audit := &auditFlags{}
	cmd := &cobra.Command{
		Use:   "link-spec",
		Short: "Add a spec back-reference to a PRD, idempotently",
		Long: `Append a spec id to source_specs if it is not already there.

Idempotent: re-running adds neither a duplicate id nor a second history entry,
so a retried command does not fabricate a second linking event.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, root, err := loadSidecar(file)
			if err != nil {
				return err
			}
			seq := mapEnsureSeq(root, "source_specs")
			for _, e := range seq.Content {
				if e.Value == spec {
					return nil // already linked; no history entry
				}
			}
			seq.Content = append(seq.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: spec})
			appendHistory(root, audit.at, audit.author, "edited",
				"Back-reference added: source_specs += "+spec, []string{spec})
			clearSync(root)
			return saveSidecar(file, doc)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "PRD sidecar path (required)")
	cmd.Flags().StringVar(&spec, "spec", "", "spec id (required)")
	_ = cmd.MarkFlagRequired("file")
	_ = cmd.MarkFlagRequired("spec")
	audit.bind(cmd)
	return cmd
}

// newPRDSupersedeCmd links two PRDs, writing an audited entry on BOTH.
//
// TWO FILES, TWO HISTORIES. A supersession that recorded itself only on the
// old PRD would leave the new one claiming no lineage — and the new one is the
// document people read. Both sides get an entry, and both get their _sync
// cleared, because both were changed.
//
// The old sidecar is written FIRST and its failure aborts before the new one
// is touched. That is not atomicity — nothing here is atomic — it is the
// cheaper failure: an old PRD marked superseded with no new one pointing back
// is a visible inconsistency, while a new PRD claiming to supersede a PRD that
// never heard about it reads as complete.
func newPRDSupersedeCmd() *cobra.Command {
	var oldFile, newFile, oldID, newID string
	var forced bool
	audit := &auditFlags{}
	cmd := &cobra.Command{
		Use:   "supersede",
		Short: "Mark one PRD superseded by another, recording it on both",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			oldDoc, oldRoot, err := loadSidecar(oldFile)
			if err != nil {
				return err
			}
			newDoc, newRoot, err := loadSidecar(newFile)
			if err != nil {
				return err
			}
			mapSet(oldRoot, "status", "superseded")
			mapSet(oldRoot, "superseded_by", newID)
			note := "Superseded by " + newID
			if forced {
				// The override is recorded IN the history rather than only in
				// the operator's memory: a supersession that skipped the gate
				// should be readable as such later.
				note += " (supersede gate overridden with --force)"
			}
			appendHistory(oldRoot, audit.at, audit.author, "supersede", note, nil)
			clearSync(oldRoot)
			if err := saveSidecar(oldFile, oldDoc); err != nil {
				return err
			}

			mapSet(newRoot, "supersedes", oldID)
			appendHistory(newRoot, audit.at, audit.author, "supersede", "Supersedes "+oldID, nil)
			clearSync(newRoot)
			return saveSidecar(newFile, newDoc)
		},
	}
	cmd.Flags().StringVar(&oldFile, "old-file", "", "superseded PRD sidecar path (required)")
	cmd.Flags().StringVar(&newFile, "new-file", "", "superseding PRD sidecar path (required)")
	cmd.Flags().StringVar(&oldID, "old-id", "", "superseded PRD id (required)")
	cmd.Flags().StringVar(&newID, "new-id", "", "superseding PRD id (required)")
	cmd.Flags().BoolVar(&forced, "forced", false, "record that the supersede gate was overridden")
	for _, f := range []string{"old-file", "new-file", "old-id", "new-id"} {
		_ = cmd.MarkFlagRequired(f)
	}
	audit.bind(cmd)
	return cmd
}

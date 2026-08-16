package cmd

// doctor_topic_descriptions.go — durability backstop for the topic-
// description approval ceremony (RouteProposals in internal/govrun/
// proposals.go).
//
// RouteProposals already refuses to invent a description: an unapproved
// topic gets a pending-queue entry, and render (topic.md.tmpl,
// phaseb/skillpackage.go) substitutes an explicit "no approved topic
// description" placeholder rather than emitting a blank string. That
// placeholder is not silent — but it is not a routing signal either: a
// skill package whose frontmatter description says "no approved
// description — run `sidecar approve ...`" cannot be matched to any real
// task, so a project that accumulates pending topics quietly loses
// routing coverage for exactly those topics. The one place this backlog
// is surfaced today is the one-shot compile summary line
// (ProposalRoutingReport.Report(), printed once per `gov compile` run) —
// it does not persist, so a project that compiled once and then sat idle
// has no standing way to rediscover how many topics are still unrouted.
//
// This check makes that backlog a durable, repeatable `doctor` signal,
// the same way schema-pin drift and orphaned surfaces are.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type pendingTopicDescriptionEntry struct {
	Topic       string `yaml:"topic"`
	Description string `yaml:"description"`
}

// runTopicDescriptionsCheck reports how many topics in
// .edikt/state/pending-topic-descriptions/ still lack an approved
// description, split into "proposed, awaiting approval" (a human
// suggestion exists) vs "empty slot" (nothing proposed at all — the
// weaker case, since even the extractor found nothing to suggest).
// Returns the warning count and whether the check ran (false when the
// directory doesn't exist — nothing pending, not a failure).
func runTopicDescriptionsCheck(projectRoot string, out io.Writer) (warnCount int, ran bool) {
	dir := filepath.Join(projectRoot, ".edikt", "state", "pending-topic-descriptions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, false
	}

	var proposed, empty []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var p pendingTopicDescriptionEntry
		if err := yaml.Unmarshal(body, &p); err != nil {
			continue
		}
		topic := p.Topic
		if topic == "" {
			topic = strings.TrimSuffix(e.Name(), ".yaml")
		}
		if strings.TrimSpace(p.Description) == "" {
			empty = append(empty, topic)
		} else {
			proposed = append(proposed, topic)
		}
	}
	if len(proposed) == 0 && len(empty) == 0 {
		return 0, true
	}
	sort.Strings(proposed)
	sort.Strings(empty)

	total := len(proposed) + len(empty)
	fmt.Fprintf(out, "  WARN: %d topic(s) have no approved description — their skill package and topic-file routing signal is a placeholder, not a real match target:\n", total)
	if len(proposed) > 0 {
		fmt.Fprintf(out, "    proposed, awaiting approval (%d): %s — run: edikt sidecar approve --kind topic-description --list\n", len(proposed), strings.Join(proposed, ", "))
	}
	if len(empty) > 0 {
		fmt.Fprintf(out, "    empty slot, nothing proposed (%d): %s — needs a human-written description before it can be approved\n", len(empty), strings.Join(empty, ", "))
	}
	return 1, true
}

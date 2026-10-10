package cmd

import (
	"fmt"
	"os"

	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
	"github.com/diktahq/edikt/tools/edikt/internal/topicregistry"
)

// THE REGISTRY HAD NO REMOVAL PATH.
//
// `.edikt/topics.yaml` holds one approved description per topic. When a
// topic's last source artifact is archived, the compile stops producing that
// topic and deletes its skill package — and the registry entry stays,
// describing a topic nothing carries. The coverage report has always counted
// those as orphans; nothing could act on the count.
//
// Removal runs on the SAME human gate as approval, because withdrawing a
// description is as much a decision as writing one. It is never automatic: a
// topic absent from one compile may be a source artifact mid-rename, and a
// registry that pruned itself would erase a human's sentence on the strength
// of a transient absence.

// removeRegistryEntry deletes one topic's approved description, refusing when
// the entry is not there. Refusing rather than succeeding silently: "removed"
// for an entry that never existed reads as a completed action and hides a
// mistyped topic name.
func removeRegistryEntry(root, topic string) error {
	path := topicregistry.PathFor(root)
	reg, err := topicregistry.LoadOrEmpty(path)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("load topic registry: %v", err)}
	}
	if _, ok := reg[topic]; !ok {
		known := make([]string, 0, len(reg))
		for t := range reg {
			known = append(known, t)
		}
		return &exitCodeError{code: 2, msg: fmt.Sprintf(
			"no registry entry for topic %q — nothing to remove.\n"+
				"  The registry currently describes: %v\n"+
				"  A removal that reports success for an entry that was never there hides a typo.",
			topic, known)}
	}
	delete(reg, topic)
	out, err := topicregistry.Marshal(reg)
	if err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("marshal topic registry: %v", err)}
	}
	if err := migrate.AtomicWriteNoFollow(path, out, 0o644); err != nil {
		return &exitCodeError{code: 1, msg: fmt.Sprintf("write topic registry: %v", err)}
	}
	fmt.Fprintf(os.Stdout, "ok: removed the registry entry for %q (%d topic(s) remain described)\n", topic, len(reg))
	fmt.Fprintln(os.Stdout, "    the topic's directives are unaffected — this withdraws the sentence that said when to load them")
	return nil
}

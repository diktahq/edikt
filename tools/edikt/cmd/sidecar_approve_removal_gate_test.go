package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/topicregistry"
)

// The registry had no removal path: an entry describing a topic the corpus no
// longer carries was counted as an orphan for releases, with nothing a reader
// could do about it.

func plantRegistry(t *testing.T, root string, topics map[string]string) {
	t.Helper()
	reg := topicregistry.Registry{}
	for topic, desc := range topics {
		reg[topic] = topicregistry.Entry{Description: desc}
	}
	out, err := topicregistry.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	p := topicregistry.PathFor(root)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveRegistryEntry_WithdrawsOneDescription(t *testing.T) {
	root := t.TempDir()
	plantRegistry(t, root, map[string]string{
		"hooks":    "Choosing which hook carries a rule.",
		"archived": "A topic whose last artifact was archived.",
	})

	if err := removeRegistryEntry(root, "archived"); err != nil {
		t.Fatalf("removal must succeed for an existing entry: %v", err)
	}
	reg, err := topicregistry.LoadOrEmpty(topicregistry.PathFor(root))
	if err != nil {
		t.Fatal(err)
	}
	if _, gone := reg["archived"]; gone {
		t.Error("the entry was not removed")
	}
	if _, kept := reg["hooks"]; !kept {
		t.Error("removal must touch exactly one entry")
	}
}

// A removal that reports success for an entry that was never there reads as a
// completed action and hides a mistyped topic name.
func TestRemoveRegistryEntry_RefusesWhenThereIsNothingToRemove(t *testing.T) {
	root := t.TempDir()
	plantRegistry(t, root, map[string]string{"hooks": "Choosing which hook carries a rule."})

	err := removeRegistryEntry(root, "hoks")
	if err == nil {
		t.Fatal("a removal for an absent entry must be refused")
	}
	if !strings.Contains(err.Error(), "nothing to remove") || !strings.Contains(err.Error(), "hooks") {
		t.Errorf("the refusal must say nothing matched and show what the registry does describe: %v", err)
	}
}

// The orphan report carries the remedy, not just the count.
func TestRegistryCoverage_OrphanReportNamesTheRemedy(t *testing.T) {
	cov := topicregistry.Coverage{
		CorpusTopics: []string{"hooks"},
		Approved:     []string{"hooks"},
		Orphans:      []string{"archived"},
	}
	line := cov.Report()
	for _, want := range []string{"orphan", "archived", "--removal", "never automatic"} {
		if !strings.Contains(line, want) {
			t.Errorf("the orphan report must carry %q:\n%s", want, line)
		}
	}
}

// The flag is narrow on purpose: it removes an APPROVED description, which is
// not what rejecting a pending proposal means.
func TestApproveRemoval_OnlyForTopicDescriptionReject(t *testing.T) {
	for _, c := range []struct{ kind, decision string }{
		{"verify", "reject"},
		{"paths", "reject"},
		{"topic-description", "approve"},
	} {
		out, err := runCmd(t, "sidecar", "approve", "some-topic",
			"--kind="+c.kind, "--decision="+c.decision, "--removal")
		if err == nil {
			t.Errorf("--removal with --kind=%s --decision=%s must be refused:\n%s", c.kind, c.decision, out)
		}
	}
}

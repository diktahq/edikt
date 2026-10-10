package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A COMMAND BODY DRIFTING BEHIND ITS OWN BINARY.
//
// The verb has supported `--kind paths` and `--kind topic-description` for two
// releases. The command that wraps it documented behavioral verifies
// exclusively: its description, its empty-state line, every prompt. So a
// project on the front-door rule — invoke commands, not the binary — could not
// describe a topic at all, and the compile's own remediation named a binary
// call the skill did not wrap. Measured on a consumer: two topics pending
// after one compile, carrying 43 and 14 directives, compiled and ambient and
// unroutable, because the line that says when to load a topic is the missing
// line.
//
// This test is the general fix for that class, not a patch for those two
// kinds: the command's documented kinds must EQUAL the verb's supported kinds,
// so adding a kind to the verb turns it red until the front door catches up.

func TestSidecarApprove_DocumentedKindsEqualSupportedKinds(t *testing.T) {
	supported := make([]string, 0, len(pendingDirForKind))
	for k := range pendingDirForKind {
		supported = append(supported, k)
	}
	sort.Strings(supported)

	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "commands", "sidecar", "approve.md"))
	if err != nil {
		t.Fatalf("read the command body: %v", err)
	}
	body := string(raw)

	var undocumented []string
	for _, k := range supported {
		// The kind must appear as a value the command actually passes, not
		// merely as prose about it: `--kind=verify`, `--kind <verify|...>`, or
		// a table row naming the store it reads.
		if !strings.Contains(body, k) {
			undocumented = append(undocumented, k)
			continue
		}
		// And its pending store must be named, because that is what the
		// listing step reads.
		if dir := pendingDirForKind[k]; !strings.Contains(body, dir) {
			undocumented = append(undocumented, k+" (store "+dir+" not named)")
		}
	}
	if len(undocumented) > 0 {
		t.Errorf("the verb supports kinds the command does not reach: %s\n"+
			"  A project on the front-door rule can only reach what this command wraps, so an\n"+
			"  undocumented kind is an unreachable one.",
			strings.Join(undocumented, ", "))
	}

	// The other direction: a kind the command offers that the verb does not
	// support would exit 3 on every invocation.
	for _, tok := range []string{"--kind=", "--kind "} {
		for _, frag := range strings.Split(body, tok)[1:] {
			name := strings.TrimSpace(frag)
			name = strings.TrimLeft(name, "<")
			cut := strings.IndexAny(name, " \n\t>|`")
			if cut > 0 {
				name = name[:cut]
			}
			if name == "" || strings.HasPrefix(name, "$") || strings.Contains(name, "kind") {
				continue // a placeholder, not a literal kind
			}
			if _, ok := pendingDirForKind[name]; !ok {
				t.Errorf("the command dispatches --kind=%q, which the verb does not support (exit 3 every time)", name)
			}
		}
	}
}

// The empty-state line must name every kind. "No pending behavioral verifies"
// was TRUE and misleading in a project whose pending topic descriptions were
// the reason its governance would not route: a report scoped to one kind reads
// as a report about the queue.
func TestSidecarApprove_EmptyStateNamesEveryKind(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "commands", "sidecar", "approve.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	idx := strings.Index(body, "Nothing to approve")
	if idx < 0 {
		t.Fatal("the command has no empty-state line")
	}
	// Look at the sentence, not the whole file.
	start := idx - 200
	if start < 0 {
		start = 0
	}
	sentence := body[start : idx+len("Nothing to approve")]
	for k := range pendingDirForKind {
		if !strings.Contains(sentence, k) {
			t.Errorf("the empty-state line does not name kind %q — it reports on the queue while covering one store:\n%s", k, sentence)
		}
	}
}

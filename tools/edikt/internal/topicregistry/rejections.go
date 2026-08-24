package topicregistry

// rejections.go — durable record that a human declined a topic description.
//
// Rejecting a pending description used to remove the pending file and record
// nothing. The next extraction re-proposed the same description, and the loop
// `reject → compile → re-proposed` had no exit. A consumer project reported
// the only escape they found was APPROVING a description for a topic they had
// already decided was dead — which writes a registry row for a dissolved
// subsystem to satisfy a loop rather than resolve it.
//
// Why not a row in topics.yaml: that file's schema requires
// description + description_hash + approved_at with additionalProperties
// false, and its own contract is "there are no unapproved rows in this file".
// A rejection is the opposite of an approval and does not belong there.
//
// Why not .edikt/state/: that directory is gitignored. A rejection recorded
// there would be invisible to every teammate, and the loop would return for
// the next person to compile. This file sits beside topics.yaml and is
// committed, because the decision is the team's, not the machine's.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// RejectionsRelPath is the file's location relative to a project root.
const RejectionsRelPath = ".edikt/topic-rejections.yaml"

// Rejection is one declined topic.
type Rejection struct {
	RejectedAt string `yaml:"rejected_at"`
	Reason     string `yaml:"reason,omitempty"`
}

// Rejections is the whole file: topic id -> rejection.
type Rejections map[string]Rejection

// RejectionsPathFor returns the rejections path for a project root.
func RejectionsPathFor(root string) string { return filepath.Join(root, RejectionsRelPath) }

// LoadRejections reads the file, returning an empty set when it does not
// exist. A malformed file is an error rather than an empty set: silently
// treating unreadable rejections as "none" would re-open the loop this file
// closes, and do it invisibly.
func LoadRejections(path string) (Rejections, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Rejections{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	out := Rejections{}
	if err := yaml.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}

// SaveRejections writes the file atomically, sorted, with a header naming what
// wrote it and how to undo it. Undoing must be obvious: a rejection that
// cannot be reversed without reading source is a trap, not a decision.
func SaveRejections(path string, r Rejections) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("# Topic descriptions a human declined.\n")
	sb.WriteString("# Written by `bin/edikt sidecar approve --kind topic-description --decision=reject`.\n")
	sb.WriteString("# A topic listed here is not re-proposed on the next compile.\n")
	sb.WriteString("# To undo a rejection, delete its entry — the next compile proposes again.\n")
	for _, k := range keys {
		e := r[k]
		sb.WriteString(fmt.Sprintf("%s:\n", k))
		sb.WriteString(fmt.Sprintf("  rejected_at: %q\n", e.RejectedAt))
		if strings.TrimSpace(e.Reason) != "" {
			sb.WriteString(fmt.Sprintf("  reason: %q\n", e.Reason))
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// IsRejected reports whether a topic carries a recorded rejection.
func (r Rejections) IsRejected(topic string) bool {
	_, ok := r[topic]
	return ok
}

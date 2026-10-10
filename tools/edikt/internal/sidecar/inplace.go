package sidecar

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// KeyRemoval names keys to delete from one mapping in a sidecar file: the
// top-level mapping (List == "") or item Index of the `directives` /
// `prohibitions` sequence.
type KeyRemoval struct {
	List  string // "" | "directives" | "prohibitions"
	Index int
	Keys  []string
}

// RemoveKeysInPlace deletes the named keys from the file's bytes without
// reserialising it: every other byte is preserved, including quoting, key
// order, comments and wrapping. It exists because the proposal router used
// to rewrite a routed sidecar through Marshal, and a consumer's compile then
// changed 316 lines of a committed sidecar to remove two keys (D51 (a)).
// Phase B is read-only except this edit, and this edit is the minimum.
//
// A key's span runs from its own line to the line before the next key of the
// same mapping (or the mapping's last line), so a multi-line scalar goes with
// its key. Returns the number of keys removed; a key that is absent is not
// an error (the caller already cleared it in memory).
func RemoveKeysInPlace(path string, removals []KeyRemoval) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return 0, fmt.Errorf("%s: not a mapping document", path)
	}
	root := doc.Content[0]
	lines := bytes.Split(raw, []byte("\n"))
	drop := map[int]bool{} // 1-indexed lines to delete
	removed := 0

	// nextStart is the first line of whatever follows a node at the same
	// level; the node's span ends the line before it.
	for _, r := range removals {
		m, after := locateMapping(root, r.List, r.Index)
		if m == nil {
			continue
		}
		type kv struct {
			key  string
			line int
		}
		var keys []kv
		for i := 0; i+1 < len(m.Content); i += 2 {
			keys = append(keys, kv{m.Content[i].Value, m.Content[i].Line})
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].line < keys[j].line })
		for i, k := range keys {
			if !contains(r.Keys, k.key) {
				continue
			}
			end := after - 1
			if i+1 < len(keys) {
				end = keys[i+1].line - 1
			}
			for l := k.line; l <= end && l <= len(lines); l++ {
				drop[l] = true
			}
			removed++
		}
	}
	if removed == 0 {
		return 0, nil
	}
	var out [][]byte
	for i, l := range lines {
		if !drop[i+1] {
			out = append(out, l)
		}
	}
	if err := os.WriteFile(path, bytes.Join(out, []byte("\n")), 0o644); err != nil {
		return 0, err
	}
	return removed, nil
}

// locateMapping returns the target mapping node and the 1-indexed line on
// which the next node at its level begins (or one past the file's last line).
func locateMapping(root *yaml.Node, list string, index int) (*yaml.Node, int) {
	last := lastLine(root) + 1
	if list == "" {
		return root, last
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != list {
			continue
		}
		seq := root.Content[i+1]
		if seq.Kind != yaml.SequenceNode || index >= len(seq.Content) {
			return nil, 0
		}
		item := seq.Content[index]
		if item.Kind != yaml.MappingNode {
			return nil, 0
		}
		after := last
		if index+1 < len(seq.Content) {
			after = seq.Content[index+1].Line
		} else if i+2 < len(root.Content) {
			after = root.Content[i+2].Line
		}
		return item, after
	}
	return nil, 0
}

func lastLine(n *yaml.Node) int {
	l := n.Line
	for _, c := range n.Content {
		if cl := lastLine(c); cl > l {
			l = cl
		}
	}
	return l
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// KeyValue is one top-level scalar key to set on a sidecar file.
type KeyValue struct {
	Key   string
	Value string
}

// InsertKeyInPlace sets top-level scalar keys on the sidecar file's bytes
// without reserialising it, mirroring RemoveKeysInPlace's line-splice
// shape: every other byte is preserved, including quoting, key order,
// comments and wrapping elsewhere in the file.
//
// An existing key's value is replaced on its own line, in place. A key
// that does not yet exist is inserted as a new line immediately after
// body_digest: if the sidecar carries one, else immediately after path: —
// a stable, deterministic anchor so repeated calls (and calls in any
// order across the kvs slice) produce the same file shape.
//
// Values are written as plain YAML scalars (`key: value`, no quoting):
// this function's only callers are phasea verify's extractor_model /
// extractor_contract stamp, and both values are restricted by construction
// to charsets that need no YAML quoting (idvalidate.ModelID's pattern, a
// hex digest, or the literal "unrecorded") — a caller passing anything
// else is a programming error this function does not try to guard against.
func InsertKeyInPlace(path string, kvs []KeyValue) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s: not a mapping document", path)
	}
	root := doc.Content[0]

	lines := bytes.Split(raw, []byte("\n"))

	// Map of existing top-level key -> its value's line (1-indexed).
	existing := map[string]int{}
	var anchorLine int // line after which a NEW key is inserted
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i].Value
		existing[key] = root.Content[i].Line
		if key == "body_digest" || (key == "path" && anchorLine == 0) {
			anchorLine = root.Content[i].Line
		}
	}
	if anchorLine == 0 {
		// Neither body_digest nor path found — anchor after the last
		// top-level scalar key seen, so insertion still lands inside the
		// mapping rather than guessing.
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i].Line > anchorLine {
				anchorLine = root.Content[i].Line
			}
		}
	}

	// Replace existing keys first, tracking which kvs still need insertion.
	replace := map[int]string{} // 1-indexed line -> full replacement line text
	var toInsert []KeyValue
	for _, kv := range kvs {
		if line, ok := existing[kv.Key]; ok {
			replace[line] = kv.Key + ": " + kv.Value
		} else {
			toInsert = append(toInsert, kv)
		}
	}

	out := make([][]byte, 0, len(lines)+len(toInsert))
	for i, l := range lines {
		lineNo := i + 1
		if repl, ok := replace[lineNo]; ok {
			out = append(out, []byte(repl))
		} else {
			out = append(out, l)
		}
		if lineNo == anchorLine {
			for _, kv := range toInsert {
				out = append(out, []byte(kv.Key+": "+kv.Value))
			}
		}
	}

	return os.WriteFile(path, bytes.Join(out, []byte("\n")), 0o644)
}

package sidecar

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConvertDocToV2 rewrites one sidecar YAML document from the v1 single-anchor
// shape to the v2 multi-anchor shape, in place on the node tree.
//
// WHY THE NODE TREE AND NOT THE STRUCT. A decode/encode round-trip through
// Sidecar would reorder every key and silently drop anything the struct does
// not model. This migration must be provably lossless for fields a human
// approved — verify:, human_approved_at, approved paths: — so it edits only the
// two keys it is responsible for (plus the yaml-language-server $schema
// comment, when present and still pointing at v1 — an editor validating v2
// content against the v1 schema is the same half-converted-file defect this
// migration exists to avoid) and leaves every other byte where it was.
//
// Idempotent: a document already carrying source_excerpts is left untouched, so
// a second run is a no-op rather than a re-wrap. Returns whether anything
// changed.
func ConvertDocToV2(root *yaml.Node) (bool, error) {
	if root == nil || len(root.Content) == 0 {
		return false, fmt.Errorf("empty YAML document")
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return false, fmt.Errorf("sidecar root is not a mapping")
	}

	changed := false
	for _, listKey := range []string{"directives", "prohibitions"} {
		list := mapValue(doc, listKey)
		if list == nil || list.Kind != yaml.SequenceNode {
			continue
		}
		for _, item := range list.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			if mapValue(item, "source_excerpts") != nil {
				continue // already v2 — idempotent
			}
			se := mapValue(item, "source_excerpt")
			if se == nil {
				continue
			}
			seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			seq.Content = append(seq.Content, se)
			renameKeyInPlace(item, "source_excerpt", "source_excerpts", seq)
			changed = true
		}
	}

	// Bump last so a document that needed no anchor work is still promoted —
	// but only if it is actually a v1 document.
	if sv := mapValue(doc, "schema_version"); sv != nil && sv.Value == "1" {
		sv.Value = "2"
		changed = true
	}

	// A migration that bumps schema_version but leaves the editor's
	// yaml-language-server $schema hint pointing at the v1 schema file
	// ships the file half-converted: the on-disk shape is v2, but an IDE
	// validates it against v1 and reports source_excerpts as an unknown
	// field. edikt itself ignores the comment, but the user's editor does
	// not — fix it here, in the same pass that changes the shape, rather
	// than as a follow-up.
	//
	// WHERE go-yaml ACTUALLY ATTACHES THIS COMMENT — verified against real
	// sidecar headers, not assumed. Every real sidecar's header is a
	// multi-line `#` block followed by a BLANK line before `schema_version:`,
	// and go-yaml attaches a head comment separated from its following node
	// by a blank line to the DOCUMENT node (`root.HeadComment`), not to the
	// mapping's first key. `doc.Content[0].HeadComment` (the first-key
	// rung below) only holds it when the comment sits directly above
	// `schema_version:` with NO blank line — a shape that does not occur
	// anywhere in a real corpus, only in a naive hand-written test fixture.
	// Checking one location and not the other passed a unit test while
	// doing nothing on every real file — check both, unconditionally.
	if strings.Contains(root.HeadComment, "gov-sidecar.v1.schema.json") {
		root.HeadComment = strings.Replace(
			root.HeadComment,
			"gov-sidecar.v1.schema.json",
			"gov-sidecar.v2.schema.json",
			1,
		)
		changed = true
	}
	if len(doc.Content) > 0 {
		head := doc.Content[0]
		if strings.Contains(head.HeadComment, "gov-sidecar.v1.schema.json") {
			head.HeadComment = strings.Replace(
				head.HeadComment,
				"gov-sidecar.v1.schema.json",
				"gov-sidecar.v2.schema.json",
				1,
			)
			changed = true
		}
	}
	return changed, nil
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// renameKeyInPlace swaps a key's name and value while holding its POSITION, so
// the rewritten file diffs as one changed key rather than a reordered document.
func renameKeyInPlace(m *yaml.Node, oldKey, newKey string, newVal *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == oldKey {
			m.Content[i].Value = newKey
			m.Content[i+1] = newVal
			return
		}
	}
}

// ConvertFileToV2 applies ConvertDocToV2 to a sidecar file. It NEVER touches the
// parent .md (INV-010). Returns whether the file was modified.  edikt-guard:allow
func ConvertFileToV2(sidecarPath string) (bool, error) {
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return false, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return false, fmt.Errorf("parse %s: %w", sidecarPath, err)
	}
	changed, err := ConvertDocToV2(&root)
	if err != nil {
		return false, fmt.Errorf("%s: %w", sidecarPath, err)
	}
	if !changed {
		return false, nil
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return false, fmt.Errorf("encode %s: %w", sidecarPath, err)
	}
	if err := enc.Close(); err != nil {
		return false, fmt.Errorf("close encoder for %s: %w", sidecarPath, err)
	}
	out := buf.Bytes()
	if err := os.WriteFile(sidecarPath, out, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// WouldConvertToV2 reports whether ConvertFileToV2 would modify these bytes,
// without writing anything. Used by --dry-run so the preview and the apply can
// never disagree: both ask the same function the same question.
func WouldConvertToV2(raw []byte) (bool, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return false, fmt.Errorf("parse: %w", err)
	}
	return ConvertDocToV2(&root)
}

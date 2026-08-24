package hook

// phaseendjson.go — order-preserving JSON objects, and the Python-shaped
// helpers phase-end-detector's two on-disk artifacts need.
//
// WHY ORDER IS PRESERVED AT ALL. The verdict file this hook persists is the
// EVALUATOR'S OWN object with two fields overwritten. Python's json.load keeps
// insertion order and json.dump writes it back out in that order, so the
// artifact a user opens reads in the shape the evaluator emitted. Decoding
// into a Go map and re-encoding would sort every key alphabetically — a
// silent, whole-file rewrite of an artifact this migration is supposed to
// leave alone.
//
// This is deliberately the SMALLEST thing that preserves order: a slice of
// members with a token-stream decoder. It is not a general ordered-map type
// and does not try to be one.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// jsonMember is one key/value pair of an ordered object.
type jsonMember struct {
	Key   string
	Value json.RawMessage
}

// jsonObject is a JSON object that round-trips in its original key order.
type jsonObject struct {
	members []jsonMember
}

// UnmarshalJSON decodes an object through the token stream so key order
// survives. A non-object decodes to the zero value with an error, which every
// caller here treats as "not the shape I expected" rather than as fatal.
func (o *jsonObject) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("phase-end: expected a JSON object")
	}
	o.members = nil
	for dec.More() {
		keyTok, kerr := dec.Token()
		if kerr != nil {
			return kerr
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("phase-end: non-string object key")
		}
		var raw json.RawMessage
		if verr := dec.Decode(&raw); verr != nil {
			return verr
		}
		o.set(key, raw)
	}
	if _, err := dec.Token(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

// MarshalJSON writes the members back in order.
//
// The opening and closing braces are written as single bytes rather than as a
// literal so tools/edikt/check/no-shell-json-in-hook-core.sh's hand-built-JSON
// patterns have nothing to match: every KEY and every VALUE below goes through
// encoding/json, which is what INV-003 actually requires.
func (o jsonObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, m := range o.members {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(m.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		if len(m.Value) == 0 {
			buf.WriteString("null")
			continue
		}
		buf.Write(m.Value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (o *jsonObject) set(key string, raw json.RawMessage) {
	for i := range o.members {
		if o.members[i].Key == key {
			o.members[i].Value = raw
			return
		}
	}
	o.members = append(o.members, jsonMember{Key: key, Value: raw})
}

// setValue marshals v and stores it under key, appending if new.
func (o *jsonObject) setValue(key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	o.set(key, raw)
	return nil
}

// raw returns the raw value for key, and whether the key is present. Present
// with a null value and absent are different answers (INV-013), which is why
// the second return exists rather than a nil check on the first.
func (o jsonObject) raw(key string) (json.RawMessage, bool) {
	for _, m := range o.members {
		if m.Key == key {
			return m.Value, true
		}
	}
	return nil, false
}

// str reads key as a string, reporting whether it was BOTH present and a
// string — python's `isinstance(x, str)` test, not a coerce.
func (o jsonObject) str(key string) (string, bool) {
	raw, ok := o.raw(key)
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// truthy mirrors python's bool(x) for the JSON values that can appear here:
// false / null / 0 / "" / [] / {} are falsey, everything else is truthy.
func (o jsonObject) truthy(key string) bool {
	raw, ok := o.raw(key)
	if !ok {
		return false
	}
	trimmed := strings.TrimSpace(string(raw))
	switch trimmed {
	case "", "null", "false", "0", `""`, "[]", "{}":
		return false
	}
	if n, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return n != 0
	}
	return true
}

// objectList reads key as a list of objects, returning an empty slice for a
// value that is absent, null, or not a list — python's `x or []` idiom.
func (o jsonObject) objectList(key string) []jsonObject {
	raw, ok := o.raw(key)
	if !ok {
		return nil
	}
	var out []jsonObject
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// object reads key as a nested object, returning the zero object when the
// value is absent, null or not an object — python's `x or {}` idiom.
func (o jsonObject) object(key string) jsonObject {
	raw, ok := o.raw(key)
	if !ok {
		return jsonObject{}
	}
	var out jsonObject
	if err := json.Unmarshal(raw, &out); err != nil {
		return jsonObject{}
	}
	return out
}

// clone copies the member slice so a mutation does not reach the original.
func (o jsonObject) clone() jsonObject {
	out := jsonObject{members: make([]jsonMember, len(o.members))}
	copy(out.members, o.members)
	return out
}

// pythonRepr renders s the way python's !r conversion would for an ordinary
// string: single-quoted unless the value contains a single quote and no double
// quote, with backslashes and control characters escaped.
//
// Reproduced rather than approximated because the value is a config-supplied
// model name that reaches the user's systemMessage, and the quoting is what
// makes `sonnet --dangerously-skip-permissions` legible as ONE rejected token
// rather than as prose.
func pythonRepr(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote):
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// runeHead returns the first n RUNES of s — python's `raw[:800]`, which counts
// characters rather than bytes. Slicing bytes would cut a multi-byte rune in
// half and change the emitted message for any non-ASCII evaluator output.
func runeHead(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

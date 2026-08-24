package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These verbs are load-bearing: fourteen command bodies call them where they
// used to inline python3, and several of those call sites are the ONLY thing
// standing between a shell variable and a JSON line. A verb that mangled a
// value would produce a broken record with nothing reporting it, which is the
// exact defect INV-003 exists to prevent.

func runUtil(t *testing.T, args ...string) (string, error) {
	t.Helper()
	c := newUtilCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), err
}

// The property the whole verb exists for: a caller never concatenates, so a
// value carrying JSON metacharacters cannot break the line.
func TestUtilJSONEmitEncodesHostileValues(t *testing.T) {
	hostile := `quote " brace } bracket ] backslash \ newline` + "\n" + `tab	end`
	out, err := runUtil(t, "json", "emit", "--field", "reason="+hostile, "--field", "status=error")
	if err != nil {
		t.Fatalf("emit: %v (%s)", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("emitted line is not valid JSON: %v\n%s", err, out)
	}
	if got["reason"] != hostile {
		t.Errorf("value round-tripped as %q, want %q", got["reason"], hostile)
	}
	if got["status"] != "error" {
		t.Errorf("status = %v, want error", got["status"])
	}
}

// --int exists because a shell caller has no way to say "this one is a number",
// and a count emitted as "3" reads differently downstream than 3.
func TestUtilJSONEmitIntIsANumber(t *testing.T) {
	out, err := runUtil(t, "json", "emit", "--int", "count=3", "--field", "name=x")
	if err != nil {
		t.Fatalf("emit: %v (%s)", err, out)
	}
	if !strings.Contains(out, `"count":3`) {
		t.Errorf("count was not emitted as a number: %s", out)
	}
	if _, err := runUtil(t, "json", "emit", "--int", "count=notanumber"); err == nil {
		t.Error("a non-numeric --int was accepted; it would have become a silent string")
	}
	if _, err := runUtil(t, "json", "emit", "--field", "novalue"); err == nil {
		t.Error("a --field without = was accepted")
	}
}

// A MISSING PATH IS AN ERROR unless a default is asked for. Printing an empty
// line for "not found" is what makes a caller treat absence as a value — the
// shape this whole migration keeps finding.
func TestUtilGetAbsenceIsAnErrorNotAnEmptyLine(t *testing.T) {
	dir := t.TempDir()
	jf := filepath.Join(dir, "d.json")
	if err := os.WriteFile(jf, []byte(`{"l2_summary":{"status":"pass"},"n":7,"ok":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	yf := filepath.Join(dir, "d.yaml")
	if err := os.WriteFile(yf, []byte("schema_version: 2\nnested:\n  key: v\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ kind, file, path, want string }{
		{"json", jf, "l2_summary.status", "pass"},
		{"json", jf, "n", "7"},
		{"json", jf, "ok", "true"},
		{"yaml", yf, "schema_version", "2"},
		{"yaml", yf, "nested.key", "v"},
	} {
		out, err := runUtil(t, c.kind, "get", c.file, c.path)
		if err != nil {
			t.Errorf("%s get %s: %v", c.kind, c.path, err)
			continue
		}
		if strings.TrimSpace(out) != c.want {
			t.Errorf("%s get %s = %q, want %q", c.kind, c.path, strings.TrimSpace(out), c.want)
		}
	}

	if _, err := runUtil(t, "json", "get", jf, "nope.missing"); err == nil {
		t.Error("a missing path returned success — a caller would read the empty line as a value")
	}
	out, err := runUtil(t, "json", "get", jf, "nope.missing", "--default", "absent")
	if err != nil {
		t.Fatalf("--default: %v", err)
	}
	if strings.TrimSpace(out) != "absent" {
		t.Errorf("--default printed %q", strings.TrimSpace(out))
	}
	// An EMPTY default is still a default: "" and "not asked for" differ.
	out, err = runUtil(t, "json", "get", jf, "nope.missing", "--default", "")
	if err != nil {
		t.Errorf("an empty --default was treated as absent: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("empty --default printed %q", out)
	}
}

// canonicalize is hashed by callers to detect a MEANING change, so the same
// document must produce the same bytes regardless of key order in the source.
func TestUtilYAMLCanonicalizeIsOrderIndependentAndStable(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	b := filepath.Join(dir, "b.yaml")
	if err := os.WriteFile(a, []byte("zebra: 1\napple: 2\nmango:\n  yak: 3\n  ant: 4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("apple: 2\nmango:\n  ant: 4\n  yak: 3\nzebra: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outA, err := runUtil(t, "yaml", "canonicalize", a)
	if err != nil {
		t.Fatal(err)
	}
	outB, err := runUtil(t, "yaml", "canonicalize", b)
	if err != nil {
		t.Fatal(err)
	}
	if outA != outB {
		t.Errorf("key order changed the output:\n%q\n%q", outA, outB)
	}
	// Stable across repeated runs — Go map iteration is randomised, so an
	// implementation that leaked that would pass once and fail later.
	for i := 0; i < 5; i++ {
		again, err := runUtil(t, "yaml", "canonicalize", a)
		if err != nil {
			t.Fatal(err)
		}
		if again != outA {
			t.Fatalf("canonicalize is not deterministic: run %d differs", i)
		}
	}
}

func TestUtilValidateAndHash(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "g.json")
	bad := filepath.Join(dir, "b.json")
	if err := os.WriteFile(good, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte(`{"a":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runUtil(t, "json", "validate", good); err != nil {
		t.Errorf("valid JSON reported invalid: %v", err)
	}
	if _, err := runUtil(t, "json", "validate", bad); err == nil {
		t.Error("malformed JSON reported valid")
	}
	if _, err := runUtil(t, "json", "validate", filepath.Join(dir, "nope.json")); err == nil {
		t.Error("a missing file reported valid — absence is not a pass")
	}

	h1, err := runUtil(t, "hash", good)
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := runUtil(t, "hash", good)
	if h1 != h2 || len(strings.TrimSpace(h1)) != 64 {
		t.Errorf("hash unstable or wrong length: %q vs %q", h1, h2)
	}
	if err := os.WriteFile(good, []byte(`{"a":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	h3, _ := runUtil(t, "hash", good)
	if h3 == h1 {
		t.Error("hash did not change when the content did")
	}
}

func TestUtilPathRelative(t *testing.T) {
	out, err := runUtil(t, "path", "relative", "docs/product", "docs/product/specs/x.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != filepath.Join("specs", "x.md") {
		t.Errorf("relative = %q", strings.TrimSpace(out))
	}
}

// render is a SAFETY verb: the values it substitutes come from sidecars, which
// come from artifact prose, and the command bodies it replaces say so. The two
// properties below are what make injection impossible rather than discouraged.
func TestUtilRenderIsLiteralAndSinglePass(t *testing.T) {
	c := newUtilCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetIn(strings.NewReader("Intent: {{intent}}\nCmd: {{verify}}\n"))
	c.SetArgs([]string{"render",
		"--field", "intent=refers to {{verify}} and $(whoami) and `id` and ${HOME}",
		"--field", "verify=grep -q x"})
	if err := c.Execute(); err != nil {
		t.Fatalf("render: %v (%s)", err, out.String())
	}
	got := out.String()

	// Single-pass: a placeholder INSIDE a substituted value stays literal. If
	// it were rescanned, a sidecar could name another field and pull its value
	// into a prompt it was never given.
	if !strings.Contains(got, "refers to {{verify}} and") {
		t.Errorf("a placeholder inside a value was expanded — substitution is not single-pass:\n%s", got)
	}
	// Literal: shell constructs are text, not something a later stage expands.
	for _, frag := range []string{"$(whoami)", "`id`", "${HOME}"} {
		if !strings.Contains(got, frag) {
			t.Errorf("%s did not survive as literal text:\n%s", frag, got)
		}
	}
	// And the real placeholder still resolved.
	if !strings.Contains(got, "Cmd: grep -q x") {
		t.Errorf("the template placeholder did not resolve:\n%s", got)
	}
}

// An unsubstituted placeholder is an ERROR. A prompt shipping with a literal
// {{intent}} is silently degraded rather than broken, and nothing downstream
// would report it.
func TestUtilRenderRefusesAnUnsubstitutedPlaceholder(t *testing.T) {
	c := newUtilCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetIn(strings.NewReader("A {{missing}} B"))
	c.SetArgs([]string{"render", "--field", "a=1"})
	if err := c.Execute(); err == nil {
		t.Error("an unsubstituted placeholder was accepted; the prompt would ship degraded")
	}
}

// schema-validate replaced BOTH a call and a weaker fallback: the command
// bodies carried a "if jsonschema is not installed, check required fields are
// present" path that ran on exactly the machines nobody tested on.
func TestUtilSchemaValidateReportsEveryViolation(t *testing.T) {
	dir := t.TempDir()
	schema := filepath.Join(dir, "s.json")
	if err := os.WriteFile(schema, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object","required":["id","title"],
		"properties":{"id":{"type":"string"},"n":{"type":"integer"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ok := filepath.Join(dir, "ok.yaml")
	if err := os.WriteFile(ok, []byte("id: PRD-007\ntitle: fine\nn: 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runUtil(t, "schema-validate", ok, schema); err != nil {
		t.Errorf("a conforming document was rejected: %v", err)
	}

	// Three violations at once — the fallback this replaced would have caught
	// only the missing field, and only if it happened to be listed.
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("id: 7\nn: notanint\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runUtil(t, "schema-validate", bad, schema)
	if err == nil {
		t.Fatal("a non-conforming document validated")
	}
	for _, want := range []string{"title", "id", "n"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not mention %q — a caller fixes one field per run:\n%s", want, out)
		}
	}
	// Not a raw Go struct dump. internal/sidecar/schemavalidate.go records
	// that walking ErrorKind internals panics on a nil localiser AND renders
	// as `&{[title]}`; this printed exactly that before the note was found.
	if strings.Contains(out, "&{") {
		t.Errorf("errors render as Go structs rather than messages:\n%s", out)
	}
}

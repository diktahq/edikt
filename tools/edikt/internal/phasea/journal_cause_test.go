package phasea

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestJournalSchema_CauseEnumCoversConstants — c1-e. Every Cause* constant the
// package can journal must be a member of phasea-journal.v1's `cause` enum,
// or the line it writes is schema-invalid. The constants are read from the
// source by AST rather than listed here, so a constant added later is covered
// without anyone remembering this test exists. `sidecar-absent-on-request`
// (D414) shipped as a constant and a journal line without ever reaching the
// enum.
func TestJournalSchema_CauseEnumCoversConstants(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	pkgDir := filepath.Dir(here)
	repo := filepath.Join(pkgDir, "..", "..", "..", "..")

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(pkgDir, "plan_types.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	consts := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range vs.Names {
			if !strings.HasPrefix(name.Name, "Cause") || i >= len(vs.Values) {
				continue
			}
			lit, ok := vs.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			consts[name.Name] = v
		}
		return true
	})
	if len(consts) < 9 {
		t.Fatalf("found only %d Cause* constants in plan_types.go — the AST walk is not seeing them (INV-013)", len(consts))
	}

	shipped := filepath.Join(repo, "templates", "schemas", "phasea-journal.v1.schema.json")
	contract := filepath.Join(repo, "docs", "product", "specs", "SPEC-013-phasea-dispatch-contract", "contracts", "phasea-journal.v1.schema.json")
	a, err := os.ReadFile(shipped)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(contract)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("the shipped journal schema and SPEC-013's contract copy differ — they must be byte-identical")
	}

	var schema struct {
		Properties struct {
			Cause struct {
				Enum []string `json:"enum"`
			} `json:"cause"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(a, &schema); err != nil {
		t.Fatal(err)
	}
	enum := map[string]bool{}
	for _, e := range schema.Properties.Cause.Enum {
		enum[e] = true
	}
	for name, v := range consts {
		if !enum[v] {
			t.Errorf("%s = %q is not in phasea-journal.v1's cause enum — a refusal journaled under it is schema-invalid", name, v)
		}
	}
}

package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// repoRoot walks up for the registry file.
//
// FAILS rather than skips when it cannot find one. A parity test that skips
// because it could not locate its subject reports green while measuring
// nothing, which is the exact shape INV-013 forbids.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "templates", "hooks", RegistryFileName)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate templates/hooks/%s walking up from the test's cwd — "+
		"the parity gate has no subject and must not report success", RegistryFileName)
	return ""
}

func loadRepoRegistry(t *testing.T) File {
	t.Helper()
	root := repoRoot(t)
	f, err := LoadRegistryFile(filepath.Join(root, "templates", "hooks", RegistryFileName))
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	return f
}

// TestRegistryParity is the SP-007 gate: `status: migrated` rows and
// registered Go handlers are the same set, with any difference NAMED.
func TestRegistryParity(t *testing.T) {
	f := loadRepoRegistry(t)
	migrated := f.MigratedNames()
	registered := Names()

	// The denominator, always. "0 mismatches" over 24 rows and "0 mismatches"
	// over an empty file are the same sentence and different facts.
	t.Logf("registry %s: %d row(s) declared, %d migrated, %d Go handler(s) registered",
		f.Path, len(f.Hooks), len(migrated), len(registered))

	claimed, undeclared := ParityDiff(migrated, registered)
	for _, n := range claimed {
		t.Errorf("registry row %q says status: migrated but no Go handler is registered — "+
			"a shim execing this name runs nothing", n)
	}
	for _, n := range undeclared {
		t.Errorf("Go handler %q is registered but no registry row declares it migrated — "+
			"no shim will ever reach it and the corpus recorder does not know it exists", n)
	}
}

// TestRegistryParityCanFail is the sensitivity control for the gate above.
//
// Phase 6 registers ZERO handlers by design (no hook is migrated here), so the
// real comparison is empty-versus-empty and passes trivially. Without this,
// "TestRegistryParity passes" would be evidence of nothing at all.
func TestRegistryParityCanFail(t *testing.T) {
	claimed, undeclared := ParityDiff([]string{"session-start", "post-compact"}, []string{"post-compact", "verify-gate"})
	if len(claimed) != 1 || claimed[0] != "session-start" {
		t.Fatalf("claimed: got %v, want [session-start]", claimed)
	}
	if len(undeclared) != 1 || undeclared[0] != "verify-gate" {
		t.Fatalf("undeclared: got %v, want [verify-gate]", undeclared)
	}
}

// TestRegistryRowsAreWellFormed checks the facts the JSON schema cannot: that
// every declared file actually exists, that library rows are the only ones
// without an event, and that names match the pattern the Go side enforces.
func TestRegistryRowsAreWellFormed(t *testing.T) {
	root := repoRoot(t)
	f := loadRepoRegistry(t)
	checked := 0
	for _, r := range f.Hooks {
		checked++
		if !NamePattern.MatchString(r.Name) {
			t.Errorf("row %q: name does not match %s — Register would panic on it", r.Name, NamePattern)
		}
		if _, err := os.Stat(filepath.Join(root, "templates", "hooks", r.File)); err != nil {
			t.Errorf("row %q declares file %s which does not exist: %v", r.Name, r.File, err)
		}
		ev, has := r.EventName()
		switch r.Class {
		case ClassLibrary:
			if has {
				t.Errorf("row %q is class library but declares event %q — a sourced helper is never registered", r.Name, ev)
			}
		default:
			if !has || ev == "" {
				t.Errorf("row %q is class %s and must declare an event; a row with none can never fire", r.Name, r.Class)
			}
		}
	}
	if checked == 0 {
		t.Fatal("checked 0 rows — nothing was measured")
	}
	t.Logf("%d row(s) checked", checked)
}

// TestNamePatternMatchesSchema keeps the one rule written in two places from
// drifting: Go's NamePattern and the JSON schema's `name` pattern.
func TestNamePatternMatchesSchema(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "templates", "schemas", "hook-registry.v1.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var doc struct {
		Properties struct {
			Hooks struct {
				Items struct {
					Properties struct {
						Name struct {
							Pattern string `json:"pattern"`
						} `json:"name"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"hooks"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	got := doc.Properties.Hooks.Items.Properties.Name.Pattern
	if got == "" {
		t.Fatal("schema declares no name pattern — nothing to compare against")
	}
	if got != NamePattern.String() {
		t.Fatalf("name pattern drift: schema %q vs Go %q", got, NamePattern.String())
	}
}

// TestLoadRegistryFileRefusesEmpty — an empty registry must be an error, not
// an empty result. Every consumer would otherwise report "nothing to do".
func TestLoadRegistryFileRefusesEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, RegistryFileName)
	if err := os.WriteFile(p, []byte("schema_version: 1\nhooks: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistryFile(p); err == nil {
		t.Fatal("expected an error for a registry declaring zero hooks, got nil")
	}
}

// TestRegisterRejectsBadInput — the three programming errors that would
// otherwise produce a hook silently bound to the wrong handler.
func TestRegisterRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		hook    string
		handler Handler
	}{
		{"invalid name", "Not_A_Name", func(*Invocation) Result { return Result{} }},
		{"nil handler", "some-hook", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected a panic, got none")
				}
			}()
			NewRegistry().Register(tc.hook, tc.handler)
		})
	}
	t.Run("duplicate", func(t *testing.T) {
		r := NewRegistry()
		r.Register("some-hook", func(*Invocation) Result { return Result{} })
		defer func() {
			if recover() == nil {
				t.Fatal("expected a panic on duplicate registration, got none")
			}
		}()
		r.Register("some-hook", func(*Invocation) Result { return Result{} })
	})
}

package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

// These tests build the payload and FIRE the hook (INV-021 Enforcement: the
// exemption is proven by firing, not by reading a matcher list). The project
// is a temp dir the test chdirs into, because the guard reads the file on
// disk relative to the working directory exactly as the dispatcher does.

const managedCLAUDE = "# Project\n\n[edikt:start]: # managed by edikt\n## edikt\nrules\n[edikt:end]: #\n\ntail\n"

func plantManagedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(managedCLAUDE), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs", "internal", "reviews"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Setenv("EDIKT_COMPILE_IN_PROGRESS", "")
	t.Setenv("EDIKT_MIGRATION_IN_PROGRESS", "")
	t.Setenv("_EDIKT_SKIP_SENTINEL_GUARD", "")
	return root
}

// dropRegionWrite is a Write whose new content has no managed region at all —
// the INV-005 shape the guard exists to stop.
func dropRegionWrite(agentType string) *Invocation {
	return &Invocation{Name: "pre-tool-use", Payload: adapter.Payload{
		ToolName:  "Write",
		FilePath:  "CLAUDE.md",
		Content:   "# Project\n\nno region here any more\n",
		AgentType: agentType,
	}}
}

func TestPreToolUseManagedRegionDenies(t *testing.T) {
	plantManagedProject(t)
	res := preToolUse(dropRegionWrite(""))
	if res.Response.Decision != adapter.DecisionDeny {
		t.Fatalf("a Write that drops a managed region must be denied; got %+v", res.Response)
	}
}

func TestPreToolUse_DroppedRegionDenied(t *testing.T) {
	plantManagedProject(t)
	res := preToolUse(dropRegionWrite("governance-verifier"))
	if res.Response.Decision != adapter.DecisionDeny {
		t.Fatalf("an agent that is not on the attested list is governed like any writer; got %+v", res.Response)
	}
}

func TestPreToolUseAttestedWriterExempt(t *testing.T) {
	plantManagedProject(t)
	res := preToolUse(dropRegionWrite("sidecar-extractor"))
	if res.Response.Decision == adapter.DecisionDeny {
		t.Fatalf("the attested governance-producing agent must not be denied by the hook it produces for (INV-021); got %+v", res.Response)
	}
	if !res.Response.ContinueTrue {
		t.Fatalf("exemption must be a plain allow, got %+v", res.Response)
	}
}

// The negative: an env var claiming to be the extractor is the writer
// attesting to itself, and buys nothing. The payload carries no agent_type,
// so the write is governed and the region drop is denied.
func TestPreToolUse_EnvActorIsNotAttestation(t *testing.T) {
	plantManagedProject(t)
	t.Setenv("EDIKT_HOOK_ACTOR", "extractor")
	t.Setenv("EDIKT_AGENT_TYPE", "sidecar-extractor")
	res := preToolUse(dropRegionWrite(""))
	if res.Response.Decision != adapter.DecisionDeny {
		t.Fatalf("an env-declared actor must not exempt a managed-region overwrite; got %+v", res.Response)
	}
}

// D54: the deny that hit a docs/internal review on 2026-09-02 came from the
// retired injection hook matching seven INV-008 ids by glob. With that hook
// gone, a Write to docs/internal/** whose content carries no managed region
// is never denied — by any actor, with or without a URL in it.
func TestPreToolUse_DocsInternalNeverDenied(t *testing.T) {
	plantManagedProject(t)
	for _, content := range []string{
		"# Review\n\nplain prose, no region\n",
		"# Review\n\nsee https://raw.githubusercontent.com/x/y/main/install.sh\n",
	} {
		res := preToolUse(&Invocation{Name: "pre-tool-use", Payload: adapter.Payload{
			ToolName: "Write",
			FilePath: "docs/internal/reviews/REVIEW-2026-09-02.md",
			Content:  content,
		}})
		if res.Response.Decision == adapter.DecisionDeny {
			t.Fatalf("a docs/internal write with no managed region was denied: %+v", res.Response)
		}
	}
}

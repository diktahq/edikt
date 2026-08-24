package hookcorpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNormalizationCoverage — AC-7.4.
//
// Every pass AC-7.4 names must fire on input that carries its subject, exit
// codes must never be touched, and the passes that fired must be what the
// recording reports.
func TestNormalizationCoverage(t *testing.T) {
	cfg := NormalizerConfig{
		SandboxRoot: "/tmp/hookcorpus-XYZ",
		ProjectRoot: "/tmp/hookcorpus-XYZ/project",
		HomeDir:     "/tmp/hookcorpus-XYZ/home",
		TempDir:     "/var/folders/T",
		Username:    "maintainer",
		Hostname:    "laptop.local",
		SessionIDs:  []string{"ses_abc123def"},
	}
	n, err := NewNormalizer(cfg)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		token string
		in    string
	}{
		{"RFC3339 timestamp", PassISO8601, `{"at":"2026-08-20T12:34:56Z"}`},
		{"RFC3339 with offset", PassISO8601, `{"at":"2026-08-20T12:34:56.123+01:00"}`},
		{"absolute home path", PassHomePath, `HOME is /tmp/hookcorpus-XYZ/home/.edikt`},
		{"project root", PassProjectRoot, `cwd /tmp/hookcorpus-XYZ/project/docs`},
		{"tmpdir", PassTmpdir, `scratch under /var/folders/T/x`},
		{"username", PassUsername, `ran_by: maintainer`},
		{"hostname", PassHostname, `host laptop.local`},
		{"session id", PassSessionID, `{"session_id":"ses_abc123def"}`},
		{"unix epoch seconds", PassEpochSec, `since 1755691234`},
		{"unix epoch ms", PassEpochMS, `at 1755691234567`},
		{"random uuid", PassUUID, `id 3f2504e0-4f89-11d3-9a0c-0305e82c3301`},
		{"pid", PassPID, `worker pid=48210 started`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, fired := n.Apply([]byte(tc.in))
			if string(out) == tc.in {
				t.Fatalf("%s pass did not change %q", tc.token, tc.in)
			}
			found := false
			for _, f := range fired {
				if f == tc.token {
					found = true
				}
			}
			if !found {
				t.Fatalf("%q was masked but %s is not in the reported pass list %v",
					tc.in, tc.token, fired)
			}
		})
	}

	t.Run("every AC-7.4 pass exists", func(t *testing.T) {
		want := []string{PassISO8601, PassHomePath, PassUsername, PassHostname, PassSessionID, PassProjectRoot}
		have := strings.Join(n.Tokens(), " ")
		for _, w := range want {
			if !strings.Contains(have, w) {
				t.Errorf("normalizer has no %s pass; it has %v", w, n.Tokens())
			}
		}
	})

	t.Run("exit codes are never normalized", func(t *testing.T) {
		// An exit code is a small integer, and no pass may touch one. F-036 is
		// the finding that a rewrite can preserve stdout exactly while changing
		// exit semantics; a normalizer that masked exit codes would reintroduce
		// that finding as a comparator setting.
		for _, code := range []string{"0", "1", "2", "127", "255"} {
			in := "exit " + code
			out, fired := n.Apply([]byte(in))
			if string(out) != in {
				t.Errorf("exit code text %q was normalized to %q by %v", in, out, fired)
			}
		}
		// And structurally: no pass token names an exit code.
		for _, tok := range KnownPassTokens() {
			if strings.Contains(tok, "exit") {
				t.Errorf("pass token %q looks like an exit-code mask", tok)
			}
		}
	})

	t.Run("clean bytes fire no pass", func(t *testing.T) {
		out, fired := n.Apply([]byte("nothing volatile here"))
		if string(out) != "nothing volatile here" {
			t.Errorf("clean bytes were rewritten to %q", out)
		}
		if len(fired) != 0 {
			t.Errorf("clean bytes reported passes %v — the recording would then overstate what "+
				"the comparison stopped proving", fired)
		}
	})
}

// TestNormalizerPassListIsPinned — the anti-gaming pin.
//
// A stubborn diff can always be made green by adding one more normalization
// pass. This test is what stops that from being a quiet edit: change any pass's
// token, placeholder or pattern and this fails, naming the version bump and the
// re-record that the change forces on every recording in the corpus.
func TestNormalizerPassListIsPinned(t *testing.T) {
	got := PassListDigest()
	want := expectedPassListDigest
	if want == "" {
		t.Fatalf("no pinned digest recorded; current is %s", got)
	}
	if got != want {
		t.Fatalf(`the normalization pass list changed.

  pinned:  %s
  current: %s

Every recording in the corpus was hashed under the OLD pass list, so this change
silently alters what every existing gate does and does not compare. If the change
is intended:

  1. bump Version in normalize.go (currently %q),
  2. update expectedPassListDigest to %s,
  3. re-record every hook's corpus — diff will refuse the stale recordings until
     you do, which is the point.

If it is not intended, revert it: widening a pattern is the cheapest way to make
a failing migration gate go green without fixing anything.`,
			want, got, Version, got)
	}
}

// expectedPassListDigest pins PassListDigest(). See the test above.
const expectedPassListDigest = "57aa8458f322daab906a15efebb13994e12662f0be547b382f3dd5e2333d4372"

// TestNormalizerTokensMatchSchema — the Go pass tokens and the contract's
// closed enum are one vocabulary or they are two, and two would mean the diff
// gate could not mechanically confirm both sides ran the same pass.
func TestNormalizerTokensMatchSchema(t *testing.T) {
	path, err := filepath.Abs("../../../../templates/schemas/hook-corpus-recording.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("schema not readable: %v", err)
	}
	var doc struct {
		Properties struct {
			VolatileFieldsNormalized struct {
				Items struct {
					Enum []string `json:"enum"`
				} `json:"items"`
			} `json:"volatile_fields_normalized"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	enum := doc.Properties.VolatileFieldsNormalized.Items.Enum
	if len(enum) == 0 {
		t.Fatal("read zero enum values out of the schema — this assertion measured nothing")
	}
	inSchema := map[string]bool{}
	for _, e := range enum {
		inSchema[e] = true
	}
	for _, tok := range KnownPassTokens() {
		if !inSchema[tok] {
			t.Errorf("pass token %q is not in the schema's closed enum %v — a recording carrying "+
				"it would fail validation", tok, enum)
		}
	}
	known := map[string]bool{}
	for _, tok := range KnownPassTokens() {
		known[tok] = true
	}
	for _, e := range enum {
		if !known[e] {
			t.Errorf("the schema permits pass %q that this build cannot produce", e)
		}
	}
}

// TestFSDiffNormalizesContent — a hook whose written file carries a timestamp
// must not look different on every run.
func TestFSDiffNormalizesContent(t *testing.T) {
	n, err := NewNormalizer(NormalizerConfig{SandboxRoot: "/tmp/s", ProjectRoot: "/tmp/s/p", HomeDir: "/tmp/s/h"})
	if err != nil {
		t.Fatal(err)
	}
	before := &Snapshot{
		Files: map[string]string{"a.jsonl": "x"},
		Raw:   map[string][]byte{"a.jsonl": []byte(`{"at":"2026-08-20T12:00:00Z"}`)},
	}
	after := &Snapshot{
		Files: map[string]string{"a.jsonl": "y"},
		Raw:   map[string][]byte{"a.jsonl": []byte(`{"at":"2026-08-20T13:45:01Z"}`)},
	}
	d, fired := DiffTrees(before, after, n)
	if len(d.Modified) != 0 {
		t.Errorf("a timestamp-only change registered as modified: %+v", d.Modified)
	}
	if len(fired) == 0 {
		t.Error("no pass was reported for content that clearly carried one")
	}

	// But a real content change still registers.
	after.Raw["a.jsonl"] = []byte(`{"at":"2026-08-20T13:45:01Z","new":"field"}`)
	d2, _ := DiffTrees(before, after, n)
	if len(d2.Modified) != 1 {
		t.Fatalf("a real content change did not register as modified: %+v", d2)
	}
}

package hookcorpus

// normalize.go — the volatile-field masking applied to stdout, stderr and
// fs_diff content BEFORE hashing, on both sides of every comparison.
//
// WHY ANY OF THIS IS NECESSARY. A bash-oracle recording and a later Go run of
// the same scenario legitimately differ in every field carrying a value the
// run itself produced: a timestamp, a pid, a uuid, and — because each scenario
// gets a fresh temporary sandbox — every absolute path in the output. Compare
// raw bytes and no honest run is ever green. That is not a comparison that is
// too strict; it is a comparison of the wrong thing.
//
// WHY IT IS DANGEROUS, AND WHAT BOUNDS IT. Every pass is a hole in the gate:
// bytes inside a mask are no longer compared, so a real regression hiding in a
// masked field passes. Three things bound that, and all three are mechanical:
//
//   - The pass list is CLOSED. Tokens come from
//     hook-corpus-recording.v1.schema.json's enum; there is no free-form pass.
//   - The pass list is PINNED. PassListDigest hashes every token and pattern.
//     TestNormalizerPassListIsPinned fails the moment either changes, and the
//     failure names the remedy: bump Version and re-record. Adding a pass to
//     make a stubborn diff go green therefore cannot be done quietly — it
//     invalidates every recording in the corpus, loudly, in the same commit.
//   - Which passes actually FIRED is recorded per recording, so a reviewer
//     reading a PASS can see exactly what the byte comparison did not prove.
//
// EXIT CODES ARE NOT IN THIS FILE AND NEVER CAN BE. F-036 is the finding that
// a rewrite can preserve stdout bytes exactly and change exit-code semantics —
// 2 blocks at the harness level, 127 does not. Normalizing an exit code would
// be that finding reappearing as a comparator setting.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Version pins the normalization contract.
//
// It pins the same way prompt_version pins the extractor (ADR-055 §4, by
// analogy): a change to what a pass strips or how it matches invalidates every
// recording captured under the prior version, and this string is what lets the
// diff gate REFUSE a stale recording rather than compare two sets of bytes that
// were masked by different rules.
const Version = "v1"

// Normalization pass tokens. Closed enum — mirrors
// templates/schemas/hook-corpus-recording.v1.schema.json's
// volatile_fields_normalized items.
const (
	PassISO8601     = "iso8601-timestamp"
	PassEpochSec    = "unix-epoch-seconds"
	PassEpochMS     = "unix-epoch-ms"
	PassTmpdir      = "tmpdir-path"
	PassUUID        = "random-uuid"
	PassPID         = "pid"
	PassHomePath    = "home-path"
	PassProjectRoot = "project-root"
	PassUsername    = "username"
	PassHostname    = "hostname"
	PassSessionID   = "session-id"
)

// NormalizerConfig carries the per-run values whose literal text must be
// masked. They are per-run by construction (a fresh sandbox per scenario), and
// passing them explicitly rather than reading the ambient environment is what
// makes the normalizer testable without a process.
type NormalizerConfig struct {
	SandboxRoot string   // the temp root — masked as tmpdir-path
	ProjectRoot string   // <sandbox>/project
	HomeDir     string   // <sandbox>/home
	TempDir     string   // the host TMPDIR the child inherits
	Username    string   // the account the recorder runs as
	Hostname    string   // this machine
	SessionIDs  []string // literal session ids the scenario's payloads carry
}

// pass is one masking rule.
type pass struct {
	token       string
	pattern     string // regexp source, or "" for a literal replacement
	literal     string // literal to replace, when pattern is ""
	placeholder string
	re          *regexp.Regexp
}

// Normalizer applies the pass list.
//
// Constructed, never package-level: SSP-002 forbids non-trivial package state
// here, because this package links into the same binary that runs on every hook
// fire and a package-level MustCompile would be paid by every one of them.
type Normalizer struct {
	passes []pass
	cfg    NormalizerConfig
}

// literalPasses are the ones whose subject is a value only the run knows.
// Ordered longest-prefix-first at build time so <sandbox>/project is masked as
// project-root before <sandbox> is masked as tmpdir-path.
func (c NormalizerConfig) literalPasses() []pass {
	// A TempDir of "/tmp" is the same on every machine (Linux), so masking it
	// buys nothing and rewrites fixture payloads' own fixed /tmp paths. Only a
	// per-user temp dir (macOS /var/folders/…/T) varies and needs masking.
	tempDir := c.TempDir
	if filepath.Clean(tempDir) == "/tmp" {
		tempDir = ""
	}
	out := []pass{
		{token: PassProjectRoot, literal: c.ProjectRoot, placeholder: "<PROJECT_ROOT>"},
		{token: PassHomePath, literal: c.HomeDir, placeholder: "<HOME>"},
		{token: PassTmpdir, literal: c.SandboxRoot, placeholder: "<TMPDIR>"},
		{token: PassTmpdir, literal: tempDir, placeholder: "<TMPDIR>"},
	}
	for _, sid := range c.SessionIDs {
		out = append(out, pass{token: PassSessionID, literal: sid, placeholder: "<SESSION_ID>"})
	}
	out = append(out,
		pass{token: PassUsername, literal: c.Username, placeholder: "<USER>"},
		pass{token: PassHostname, literal: c.Hostname, placeholder: "<HOST>"},
	)
	// Longest literal first: masking a prefix before its extension would leave
	// "<TMPDIR>/project" where "<PROJECT_ROOT>" belongs, and the two sides of a
	// comparison could disagree about which they produced.
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].literal) > len(out[j].literal) })
	return out
}

// patternPasses are the shape-matched ones. Order matters and is fixed here:
// epoch-ms before epoch-seconds (a 13-digit number contains a 10-digit prefix),
// and session-id literals (above) before random-uuid, since a session id is
// often itself a uuid and masking it as <UUID> would lose the distinction the
// recording is meant to carry.
func patternPasses() []pass {
	return []pass{
		{token: PassISO8601, placeholder: "<TIMESTAMP>",
			pattern: `\d{4}-\d{2}-\d{2}[Tt ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[Zz]|[+-]\d{2}:?\d{2})?`},
		{token: PassEpochMS, placeholder: "<EPOCH_MS>", pattern: `\b1[0-9]{12}\b`},
		{token: PassEpochSec, placeholder: "<EPOCH_S>", pattern: `\b1[0-9]{9}\b`},
		{token: PassUUID, placeholder: "<UUID>",
			pattern: `\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`},
		{token: PassSessionID, placeholder: "<SESSION_ID>",
			pattern: `\bses[s]?[-_][A-Za-z0-9_-]{6,}\b`},
		{token: PassPID, placeholder: "<PID>", pattern: `(?i)\bpid[=: ]+\d+\b`},
	}
}

// NewNormalizer builds the pass list for one run.
func NewNormalizer(cfg NormalizerConfig) (*Normalizer, error) {
	n := &Normalizer{cfg: cfg}
	for _, p := range cfg.literalPasses() {
		if p.literal == "" || p.literal == "/" {
			// A masking rule whose subject is empty would replace between
			// every byte; one whose subject is "/" would eat every path.
			continue
		}
		n.passes = append(n.passes, p)
	}
	for _, p := range patternPasses() {
		re, err := regexp.Compile(p.pattern)
		if err != nil {
			return nil, fmt.Errorf("normalizer pass %s: %w", p.token, err)
		}
		p.re = re
		n.passes = append(n.passes, p)
	}
	return n, nil
}

// HostNormalizerConfig fills in the host-identity fields (username, hostname,
// TMPDIR) that every run shares, leaving the per-sandbox paths to the caller.
func HostNormalizerConfig() NormalizerConfig {
	cfg := NormalizerConfig{TempDir: os.TempDir()}
	if u, err := user.Current(); err == nil {
		cfg.Username = u.Username
	}
	if h, err := os.Hostname(); err == nil {
		cfg.Hostname = h
	}
	return cfg
}

// Apply masks b and reports which passes actually fired.
//
// "Applied" means CHANGED BYTES, not "was enabled". A recording that listed
// every pass regardless would tell a reviewer nothing about what this
// particular comparison stopped proving — which is the entire reason
// volatile_fields_normalized exists rather than being an implicit property of
// the tool.
func (n *Normalizer) Apply(b []byte) ([]byte, []string) {
	fired := map[string]bool{}
	out := b
	for _, p := range n.passes {
		var next []byte
		if p.re != nil {
			next = p.re.ReplaceAll(out, []byte(p.placeholder))
		} else {
			next = []byte(strings.ReplaceAll(string(out), p.literal, p.placeholder))
		}
		if !sameBytes(next, out) {
			fired[p.token] = true
		}
		out = next
	}
	tokens := make([]string, 0, len(fired))
	for t := range fired {
		tokens = append(tokens, t)
	}
	sort.Strings(tokens)
	return out, tokens
}

// Hash returns the sha256 of b after normalization, plus the fired passes.
func (n *Normalizer) Hash(b []byte) (string, []string) {
	norm, fired := n.Apply(b)
	sum := sha256.Sum256(norm)
	return hex.EncodeToString(sum[:]), fired
}

// Tokens lists every token this normalizer could emit, sorted and deduplicated.
func (n *Normalizer) Tokens() []string {
	seen := map[string]bool{}
	for _, p := range n.passes {
		seen[p.token] = true
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// PassListDigest is the anti-gaming pin.
//
// It hashes every pass's token, placeholder and matching rule — the pattern for
// a shape-matched pass, and the NAME of the run-value for a literal one (the
// value itself is per-run and would make the digest useless). A test compares
// it against a committed constant, so widening a pattern to make a failing diff
// disappear is a test failure that names the version bump and the re-record it
// forces, rather than an invisible loosening of every gate in the corpus.
func PassListDigest() string {
	h := sha256.New()
	fmt.Fprintf(h, "normalizer %s\n", Version)
	// Literal passes are enumerated by token+placeholder only.
	for _, p := range (NormalizerConfig{
		SandboxRoot: "S", ProjectRoot: "P", HomeDir: "H", TempDir: "T",
		Username: "U", Hostname: "N",
	}).literalPasses() {
		fmt.Fprintf(h, "literal %s %s\n", p.token, p.placeholder)
	}
	for _, p := range patternPasses() {
		fmt.Fprintf(h, "pattern %s %s %s\n", p.token, p.placeholder, p.pattern)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// KnownPassTokens is every token the closed enum permits. Asserted against the
// schema file by TestNormalizerTokensMatchSchema, so the Go side and the
// contract cannot drift.
func KnownPassTokens() []string {
	return []string{
		PassEpochMS, PassEpochSec, PassHomePath, PassHostname, PassISO8601,
		PassPID, PassProjectRoot, PassSessionID, PassTmpdir, PassUUID, PassUsername,
	}
}

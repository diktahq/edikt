// Package trust gates execution of repo-defined `verify:` shell commands
// behind an explicit, per-project consent decision (ADR-041; security  // edikt-guard:allow
// review Finding 3).
//
// edikt's verify runner (internal/verify) executes arbitrary shell from a
// repo's sidecar `verify:` fields via `bash -c`. Without a consent step,
// cloning an untrusted repo and running `gov compile` or `bin/edikt verify`
// would run that shell as the user, with their environment and credentials —
// while the user believes they are only "recompiling governance". This
// package records which project roots the user has approved and answers the
// single question the verify commands ask before executing anything: "is this
// project trusted to run its own verify: commands?".
//
// Trust is granted by:
//   - the EDIKT_VERIFY_TRUST=1 environment bypass (ephemeral; CI / one-off), or
//   - a recorded entry in the global trust store (persistent; via `--trust`).
//
// The store is ALWAYS under the user-global edikt home ($EDIKT_HOME or
// $HOME/.edikt) — never under a project root — so a repo cannot ship a file
// that marks itself trusted. The default is fail-closed: an unknown project
// is untrusted.
package trust

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnvBypass grants ephemeral trust for a single process. Intended for CI and
// one-off runs; it does not persist anything.
const EnvBypass = "EDIKT_VERIFY_TRUST"

// EnvMode selects the gate posture for an UNTRUSTED project. It is read ONLY
// from the environment (never from the repo's .edikt/config.yaml) — a posture
// sourced from repo-controlled config would let a hostile repo set "disabled"
// and neuter its own gate.
const EnvMode = "EDIKT_VERIFY_TRUST_MODE"

// Gate postures.
const (
	ModeWarn     = "warn"     // default: trust-on-first-use — run, warn once, record
	ModeBlock    = "block"    // opt-in: refuse untrusted projects until --trust
	ModeDisabled = "disabled" // opt-out: run silently, never warn or record
)

// Decision is the outcome of Evaluate.
type Decision int

const (
	Proceed            Decision = iota // allow, silent
	ProceedWithWarning                 // allow; root now recorded (TOFU); surface Message
	Refuse                             // deny; surface Message and exit non-zero
)

// Mode resolves the gate posture from EnvMode. Unset or unrecognized values
// fall back to the default, ModeWarn.
func Mode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvMode))) {
	case ModeBlock:
		return ModeBlock
	case ModeDisabled:
		return ModeDisabled
	default:
		return ModeWarn
	}
}

// Evaluate decides whether projectRoot may execute its repo-defined verify:
// commands. explicitTrust reflects an explicit --trust flag.
//
//   - explicitTrust or already-trusted (store / EDIKT_VERIFY_TRUST=1) → Proceed.
//   - otherwise, by posture:
//     disabled → Proceed (silent).
//     block    → Refuse (with an actionable message).
//     warn     → ProceedWithWarning: the root is recorded (trust-on-first-use,
//     so the notice fires once) and the warning text is returned.
func Evaluate(projectRoot string, explicitTrust bool) (Decision, string) {
	if explicitTrust {
		_ = Record(projectRoot)
		return Proceed, ""
	}
	if IsTrusted(projectRoot) {
		return Proceed, ""
	}
	switch Mode() {
	case ModeDisabled:
		return Proceed, ""
	case ModeBlock:
		return Refuse, UntrustedMessage(projectRoot)
	default: // warn — trust-on-first-use
		_ = Record(projectRoot)
		return ProceedWithWarning, WarnMessage(projectRoot)
	}
}

type entry struct {
	TrustedAt string `json:"trusted_at"`
}

// Realpath resolves p to an absolute, symlink-free path used as the trust-store
// key. It falls back to a cleaned absolute path when symlink resolution fails
// (e.g. a component does not exist), so it never returns "".
func Realpath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// storePath returns the global trust-store path. It is deliberately resolved
// from the user-global edikt home, never from the project root, so an
// untrusted repo cannot plant a trusted-roots.json that approves itself.
func storePath() (string, error) {
	base := os.Getenv("EDIKT_HOME")
	if base == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return "", fmt.Errorf("cannot resolve trust store: neither EDIKT_HOME nor HOME is set")
		}
		base = filepath.Join(home, ".edikt")
	}
	return filepath.Join(base, "state", "trusted-roots.json"), nil
}

// load reads the trust store. A missing or corrupt store yields an empty map
// (fail-closed: nothing is trusted) rather than an error, so verify never
// crashes on a damaged state file — it just refuses until re-approved.
func load() map[string]entry {
	path, err := storePath()
	if err != nil {
		return map[string]entry{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]entry{}
	}
	m := map[string]entry{}
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]entry{}
	}
	return m
}

// IsTrusted reports whether projectRoot may execute its repo-defined verify:
// commands, either via the EDIKT_VERIFY_TRUST=1 bypass or a recorded store
// entry keyed by the project's realpath.
func IsTrusted(projectRoot string) bool {
	if os.Getenv(EnvBypass) == "1" {
		return true
	}
	m := load()
	_, ok := m[Realpath(projectRoot)]
	return ok
}

// Record persists projectRoot's realpath in the global trust store. It is
// idempotent, creates the state dir (0700) and store file (0600) as needed,
// and writes atomically (temp + rename) so a concurrent reader never sees a
// half-written file.
func Record(projectRoot string) error {
	path, err := storePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	m := load()
	m[Realpath(projectRoot)] = entry{TrustedAt: time.Now().UTC().Format(time.RFC3339)}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".trusted-roots.*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// UntrustedMessage is the actionable refusal printed in block posture when a
// verify command runs in an unapproved project. It names both remediation
// paths (--trust to approve persistently, EDIKT_VERIFY_TRUST=1 for CI/one-off).
func UntrustedMessage(projectRoot string) string {
	return fmt.Sprintf(
		"refusing to run this repo's verify: commands — %s is not an approved edikt project (EDIKT_VERIFY_TRUST_MODE=block).\n"+
			"verify: fields execute arbitrary shell from the repo's sidecars. To approve this repo once:\n"+
			"  bin/edikt verify <id> --trust      (or: bin/edikt gov compile --trust)\n"+
			"For CI or a one-off run, set EDIKT_VERIFY_TRUST=1.",
		Realpath(projectRoot))
}

// WarnMessage is the one-time trust-on-first-use notice printed in the default
// (warn) posture. By the time it is shown the root has been recorded, so the
// notice fires once per repo. It explains what ran and how to harden / revoke.
func WarnMessage(projectRoot string) string {
	return fmt.Sprintf(
		"edikt: running this project's repo-defined verify: commands (%s) and trusting it from now on (trust-on-first-use).\n"+
			"  verify: fields run arbitrary shell from the repo's sidecars. If you do NOT trust this repo, remove its entry\n"+
			"  from ~/.edikt/state/trusted-roots.json. To require explicit approval for unknown repos instead, set\n"+
			"  EDIKT_VERIFY_TRUST_MODE=block (then approve with: bin/edikt gov compile --trust).",
		Realpath(projectRoot))
}

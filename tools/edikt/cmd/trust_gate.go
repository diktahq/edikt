package cmd

import (
	"fmt"
	"os"

	"github.com/diktahq/edikt/tools/edikt/internal/trust"
)

// verifyTrust backs the `--trust` flag shared across the verify command tree
// (verify, verify all, verify gov|prd|spec). It is a persistent flag on
// verifyCmd, so every subcommand inherits it.
var verifyTrust bool

// ensureVerifyTrust gates execution of a project's repo-defined `verify:`
// shell commands (ADR-041; security review Finding 3) via the posture model:  // edikt-guard:allow
//
//   - default (warn): trust-on-first-use — proceed, record the root, and print
//     a one-time notice to stderr.
//   - block (EDIKT_VERIFY_TRUST_MODE=block): refuse an untrusted project with
//     exit code 4 and an actionable message until --trust / EDIKT_VERIFY_TRUST.
//   - disabled: proceed silently.
//
// --trust and the EDIKT_VERIFY_TRUST=1 bypass always proceed.
func ensureVerifyTrust(projectRoot string) error {
	decision, msg := trust.Evaluate(projectRoot, verifyTrust)
	switch decision {
	case trust.Refuse:
		return &exitCodeError{code: 4, msg: msg}
	case trust.ProceedWithWarning:
		fmt.Fprintln(os.Stderr, msg)
	}
	return nil
}

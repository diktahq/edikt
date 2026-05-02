package verify

import "os"

// envWithVerify returns a copy of the current process env, used as the
// base for the child process env (the runner appends EDIKT_VERIFY=1).
// Split out for test substitution.
func envWithVerify() []string {
	return append([]string(nil), os.Environ()...)
}

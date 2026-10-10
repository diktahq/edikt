package adapter

// sessionid.go — SR-005 (AC-4.3): Payload.SessionID is host-supplied, but
// still external input reaching this process (INV-006) — nothing guarantees  edikt-guard:allow
// its shape, and the Stop hook's end-of-session tally line is the first place
// a session id is due to be interpolated into a Claude-facing line (the
// hook-corpus's own session ids are ordinary UUIDs, but a hostile or
// malformed one must never reach model-facing output un-vetted).

import "regexp"

var sessionIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidSessionID reports whether s is safe to render on a Claude-facing
// line: 1-64 bytes, allowlisted to letters, digits, underscore and hyphen.
// An empty, oversized, or otherwise-shaped session id is NOT rendered —
// callers must omit the session id (or the whole line depending on it)
// rather than substitute a guessed value.
func ValidSessionID(s string) bool {
	return sessionIDRe.MatchString(s)
}

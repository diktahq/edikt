package cmd

import "testing"

// TestGovIDRe_AcceptsRealOnDiskConventions pins the exact defect found
// 2026-08-09: `verify gov GL-003-single-dispatch-per-compile` failed
// against a real, on-disk guideline because govIDRe required an
// all-lowercase leading character, rejecting every guideline's actual
// "GL-NNN-slug" naming convention. `verify all` walks the filesystem
// directly and was unaffected -- only this single-id validation path had
// the gap, and nothing had exercised it against a real guideline before.
func TestGovIDRe_AcceptsRealOnDiskConventions(t *testing.T) {
	valid := []string{
		"ADR-001",
		"INV-013",
		"GL-001-capture-gates",
		"GL-003-single-dispatch-per-compile",
		"a-bare-lowercase-slug",
	}
	for _, id := range valid {
		if !govIDRe.MatchString(id) {
			t.Errorf("govIDRe rejected valid id %q", id)
		}
	}
}

// TestGovIDRe_RejectsShellMetacharacters guards INV-006: this id is
// interpolated into a filesystem path, so anything outside the allowed
// shape must be refused before it reaches disk.
func TestGovIDRe_RejectsShellMetacharacters(t *testing.T) {
	invalid := []string{
		"",
		"../../etc/passwd",
		"GL-001; rm -rf /",
		"gl_001_underscores",
		"ADR-1",   // fewer than 3 digits
		"GL-1-x",  // fewer than 3 digits
		"GL-001-", // no slug after the prefix
	}
	for _, id := range invalid {
		if govIDRe.MatchString(id) {
			t.Errorf("govIDRe accepted invalid id %q", id)
		}
	}
}

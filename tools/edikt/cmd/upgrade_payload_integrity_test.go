package cmd

import (
	"fmt"
	"strings"
	"testing"
)

// F-042 — the payload's integrity source.
//
// The launcher fetched an UNSIGNED `<payload>.tar.gz.sha256` sidecar that
// release.yml has never published, cosign-verified SHA256SUMS, and then threw
// the verified document away without reading it. So the default path 404'd
// into an error recommending EDIKT_INSTALL_INSECURE=1, which ALSO downgraded a
// failed cosign verify to a warning. Nothing bound the downloaded bytes to
// anything signed.
//
// These tests pin the two properties that were missing, at the level where the
// decision is actually made. They are deliberately NOT tests that "a lookup
// function exists" — GL-002 calls that cheatable by the generator. Each case
// states an input the release could really produce and the outcome required.

func TestLookupSHA256SUMS(t *testing.T) {
	// A realistic document: the exact asset set release.yml publishes.
	doc := []byte(strings.Join([]string{
		"aaaa000000000000000000000000000000000000000000000000000000000001  install.sh",
		"aaaa000000000000000000000000000000000000000000000000000000000002  edikt-v0.6.0-darwin-arm64.tar.gz",
		"aaaa000000000000000000000000000000000000000000000000000000000003 *edikt-payload-v0.6.0.tar.gz",
		"",
		"# a comment line the format does not define but a human might add",
	}, "\n"))

	t.Run("payload listed in binary-mode form is found", func(t *testing.T) {
		// The `*` prefix is coreutils binary mode. install.sh:530 accepts it,
		// so the Go launcher must too or the two disagree on the same file.
		got, listed := lookupSHA256SUMS(doc, "edikt-payload-v0.6.0.tar.gz")
		if !listed {
			t.Fatal("payload not found; a binary-mode (*) entry must still match")
		}
		if got != "aaaa000000000000000000000000000000000000000000000000000000000003" {
			t.Fatalf("wrong digest returned: %s", got)
		}
	})

	t.Run("an asset absent from the document is reported absent, not empty", func(t *testing.T) {
		// ABSENT MUST BE DISTINGUISHABLE FROM A DIGEST. Returning "" with no
		// flag would let a missing entry compare equal to a missing observed
		// value somewhere downstream — absence rendering as a pass.
		_, listed := lookupSHA256SUMS(doc, "edikt-payload-v9.9.9.tar.gz")
		if listed {
			t.Fatal("an asset the document does not list was reported as listed")
		}
	})

	// BOTH DIRECTIONS OF INEXACT MATCHING. The first version of this test only
	// covered the shorter-than-entry direction, and a mutation to
	// `strings.HasPrefix(name, entry)` sailed through it — the test was green
	// over the case that actually matters.
	t.Run("a name shorter than a listed entry does not match", func(t *testing.T) {
		if _, listed := lookupSHA256SUMS(doc, "edikt-payload-v0.6.0.tar"); listed {
			t.Fatal("a truncated filename matched; the comparison must be exact")
		}
	})

	t.Run("a name EXTENDING a listed entry does not match", func(t *testing.T) {
		// The direction with teeth: an asset named so that a listed entry is
		// its prefix would borrow that entry's digest under a prefix compare.
		if _, listed := lookupSHA256SUMS(doc, "edikt-payload-v0.6.0.tar.gz.evil"); listed {
			t.Fatal("a filename extending a listed entry matched; the comparison must be exact")
		}
	})

	t.Run("a name a listed entry extends does not match", func(t *testing.T) {
		// And the mirror, against `strings.HasPrefix(entry, name)`.
		if _, listed := lookupSHA256SUMS(doc, "edikt-payload"); listed {
			t.Fatal("a filename that a listed entry extends matched; the comparison must be exact")
		}
	})

	t.Run("an empty document lists nothing", func(t *testing.T) {
		// INV-013: zero input must be UNMEASURED, never a pass.
		if _, listed := lookupSHA256SUMS(nil, "edikt-payload-v0.6.0.tar.gz"); listed {
			t.Fatal("an empty SHA256SUMS reported an asset as listed")
		}
	})
}

// TestTamperedPayloadIsRejected is the fixture F-042 asks for: the signed
// document says one thing, the downloaded bytes say another.
//
// This exercises the comparison the installer performs, over a document in the
// real published format, including the case-insensitivity that would otherwise
// make a hex-case difference read as tampering.
func TestTamperedPayloadIsRejected(t *testing.T) {
	const tag = "v0.6.0"
	payloadName := fmt.Sprintf("edikt-payload-%s.tar.gz", tag)
	const genuine = "3b1f8c2d4e5a6b7c8d9e0f1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e"
	const tampered = "deadbeef4e5a6b7c8d9e0f1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e"

	doc := []byte(fmt.Sprintf("%s  %s\n", genuine, payloadName))

	expected, listed := lookupSHA256SUMS(doc, payloadName)
	if !listed {
		t.Fatal("setup is wrong: the payload must be listed for this test to mean anything")
	}

	// SENSITIVITY — tampered bytes must not compare equal.
	if strings.EqualFold(expected, tampered) {
		t.Fatal("a tampered payload digest compared equal to the signed digest")
	}

	// ISOLATION — the genuine bytes must compare equal, in either hex case.
	// Without this the test would also pass against a comparison that rejects
	// everything, which protects nothing and breaks every install.
	if !strings.EqualFold(expected, genuine) {
		t.Fatal("the genuine payload digest did not compare equal to the signed digest")
	}
	if !strings.EqualFold(expected, strings.ToUpper(genuine)) {
		t.Fatal("hex case caused a genuine payload to be treated as tampered")
	}
}

package redact

import (
	"strings"
	"testing"
)

func TestScrub_AWSAccessKey(t *testing.T) {
	in := "error: bad credential AKIAIOSFODNN7EXAMPLE in env"
	got := Scrub(in)
	if strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("AWS access key not scrubbed: %s", got)
	}
	if !strings.Contains(got, "<REDACTED:") {
		t.Fatalf("expected REDACTED placeholder; got: %s", got)
	}
}

func TestScrub_GitHubClassicToken(t *testing.T) {
	in := "auth header: Bearer ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789"
	got := Scrub(in)
	if strings.Contains(got, "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789") {
		t.Fatalf("GitHub token not scrubbed: %s", got)
	}
}

func TestScrub_OpenAIKey(t *testing.T) {
	in := "API_KEY=sk-aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789-_abc"
	got := Scrub(in)
	if strings.Contains(got, "sk-aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789-_abc") {
		t.Fatalf("sk- API key not scrubbed: %s", got)
	}
}

func TestScrub_AnthropicKey(t *testing.T) {
	// Anthropic keys begin with sk-ant-. Covered by the sk- pattern.
	in := "auth: sk-ant-api03-aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789"
	got := Scrub(in)
	if strings.Contains(got, "sk-ant-api03-aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789") {
		t.Fatalf("Anthropic key not scrubbed: %s", got)
	}
}

func TestScrub_JWT(t *testing.T) {
	in := "header: Authorization: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NSJ9.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	got := Scrub(in)
	if strings.Contains(got, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9") {
		t.Fatalf("JWT not scrubbed: %s", got)
	}
}

func TestScrub_LongBase64Catchall(t *testing.T) {
	// Long opaque token of unknown shape. The catch-all matches.
	in := "token=" + strings.Repeat("A", 80) + " expires"
	got := Scrub(in)
	if strings.Contains(got, strings.Repeat("A", 80)) {
		t.Fatalf("long base64-shaped token not scrubbed: %s", got)
	}
}

func TestScrub_PreservesOrdinaryText(t *testing.T) {
	in := "claude exit: exit status 1; output: ADR-027 sidecar generation failed"
	got := Scrub(in)
	if got != in {
		t.Fatalf("ordinary text was modified: in=%q out=%q", in, got)
	}
}

func TestScrub_Idempotent(t *testing.T) {
	in := "key=AKIAIOSFODNN7EXAMPLE more"
	once := Scrub(in)
	twice := Scrub(once)
	if once != twice {
		t.Fatalf("scrub is not idempotent:\n  once:  %q\n  twice: %q", once, twice)
	}
}

func TestScrub_PlaceholderRecordsByteLength(t *testing.T) {
	in := "key=AKIAIOSFODNN7EXAMPLE end"
	got := Scrub(in)
	// AKIAIOSFODNN7EXAMPLE is 20 bytes
	if !strings.Contains(got, "<REDACTED:20 bytes>") {
		t.Fatalf("expected <REDACTED:20 bytes> in output; got: %s", got)
	}
}

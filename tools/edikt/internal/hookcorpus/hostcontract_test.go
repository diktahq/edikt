package hookcorpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRecording(t *testing.T, dir, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "recording.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const recordingHead = `{"schema_version":1,"hook":"post-compact","scenario_id":"baseline","scenario_input_sha256":"a","normalizer_version":"` + Version + `","recorded_at":"2026-09-02T00:00:00Z",`
const recordingTail = `"exit_code":0,"stdout_sha256":"s","stderr_sha256":"e","stdout_bytes":0,"wall_clock_ms":1,"volatile_fields_normalized":[],"fs_diff":{"added":[],"removed":[],"modified":[]}}`

// A host-contract recording must name its contract and the date it was read;
// one that does not is an implementation graded against itself and is
// refused. A bash or go recording must not carry contract fields.
func TestLoadRecording_HostContractRequiresANamedContract(t *testing.T) {
	base := t.TempDir()
	ok := writeRecording(t, filepath.Join(base, "ok"), recordingHead+`"oracle":"host-contract","contract":"claude 2.1.257 reply union","contract_recorded_at":"2026-09-02",`+recordingTail)
	if _, err := LoadRecording(ok); err != nil {
		t.Fatalf("named host contract must load: %v", err)
	}
	unnamed := writeRecording(t, filepath.Join(base, "unnamed"), recordingHead+`"oracle":"host-contract",`+recordingTail)
	if _, err := LoadRecording(unnamed); err == nil || !strings.Contains(err.Error(), "must name its contract") {
		t.Fatalf("unnamed host contract must be refused, got %v", err)
	}
	bashWithContract := writeRecording(t, filepath.Join(base, "bashc"), recordingHead+`"oracle":"bash","contract":"x","contract_recorded_at":"2026-09-02",`+recordingTail)
	if _, err := LoadRecording(bashWithContract); err == nil || !strings.Contains(err.Error(), "must not carry contract fields") {
		t.Fatalf("bash recording with contract fields must be refused, got %v", err)
	}
	unknown := writeRecording(t, filepath.Join(base, "unk"), recordingHead+`"oracle":"vibes",`+recordingTail)
	if _, err := LoadRecording(unknown); err == nil {
		t.Fatal("unknown oracle must be refused")
	}
}

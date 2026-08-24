package phasea

// plan_journal.go — the append-only .edikt/state/phasea-dispatch.jsonl
// writer (AC-018-2). Mirrors hookmatch.Append's own append-only-JSONL
// discipline (internal/hookmatch/journal.go): O_APPEND|O_CREATE is atomic
// enough that two processes appending concurrently cannot corrupt or lose
// either line, without needing a lock file of its own. Unlike the hook
// journal (per-user, under $HOME/.edikt/state/), this journal is
// PROJECT-scoped, at the same .edikt/state/ root plan.json, compile.lock,
// and every other phase-A-and-adjacent state file already live under.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// JournalRelPath is .edikt/state/phasea-dispatch.jsonl, relative to the
// project root.
const JournalRelPath = ".edikt/state/phasea-dispatch.jsonl"

// appendJournal writes one line. rec.Failing is normalized to a non-nil
// empty slice when nil, so the emitted JSON is `"failing":[]` rather than
// `"failing":null` — phasea-journal.v1 declares `failing` as type "array",
// and a `null` there is schema-invalid.
func appendJournal(root string, rec JournalRecord) error {
	if rec.SchemaVersion == 0 {
		rec.SchemaVersion = 1
	}
	if rec.Failing == nil {
		rec.Failing = []string{}
	}
	if rec.TS == "" {
		rec.TS = time.Now().UTC().Format(time.RFC3339)
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	path := filepath.Join(root, filepath.FromSlash(JournalRelPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

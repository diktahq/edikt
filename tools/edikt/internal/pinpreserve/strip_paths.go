package pinpreserve

import (
	"fmt"
	"os"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// StripResult reports what StripUnapprovedPaths removed.
type StripResult struct {
	// Dropped is the artifact-level glob set removed from the sidecar. Empty
	// when nothing was stripped.
	Dropped []string
	// ReceiptDropped is true when an approval receipt the pre-dispatch
	// sidecar never carried was removed along with the globs it described.
	ReceiptDropped bool
}

// StripUnapprovedPaths removes an artifact-level `paths:` the extractor
// introduced: the pre-dispatch sidecar had none (before is nil — no prior
// sidecar — or parsed with empty Paths), the regenerated sidecar at
// sidecarPath has some, and no approval receipt from the pre-dispatch state
// covers them. It rewrites the sidecar when it strips, and reports what went.
//
// WHY. `paths:` is enforced scope. The only way scope is meant to enter a
// sidecar is as a `proposed_paths` inference that a human promotes with
// `edikt sidecar approve --kind paths`, which stamps a receipt. An extractor
// that writes `paths:` directly skips that ceremony, and before this the
// written globs simply became the artifact's scope with nothing saying a
// human never saw them.
//
// A RECEIPT IN THE OUTPUT IS NOT AN APPROVAL. Only a receipt the pre-dispatch
// sidecar already carried can cover the globs, and even then only when its
// hash matches them. A receipt that appears for the first time in the
// regenerated sidecar was written by whatever wrote the sidecar; honouring it
// would let the extractor approve its own scope. Such a receipt is removed
// with the globs it describes.
//
// THE CALLER DECIDES WHEN THIS APPLIES. It is for phasea restore, which knows
// the pre-dispatch state from the plan's pin snapshot. A caller that could
// not read that state must not call this — "no paths in a snapshot I could
// not parse" is not the same claim as "no paths" — and restore notes that
// case instead. `gov reextract restore-pinned` never calls it: it restores
// from a git ref, and a ref without paths is not evidence an extractor wrote
// the live ones.
func StripUnapprovedPaths(before *sidecar.Sidecar, sidecarPath string) (*StripResult, error) {
	res := &StripResult{}
	if before != nil && len(before.Paths) > 0 {
		return res, nil
	}
	after, err := sidecar.Load(sidecarPath)
	if err != nil {
		return res, fmt.Errorf("load regenerated sidecar %s: %w", sidecarPath, err)
	}
	if len(after.Paths) == 0 {
		return res, nil
	}
	var priorReceipt *sidecar.PathsApproval
	if before != nil {
		priorReceipt = before.PathsApproval
	}
	if priorReceipt != nil && after.PathsApproval != nil && *after.PathsApproval == *priorReceipt &&
		sidecar.HashGlobs(after.Paths) == priorReceipt.GlobsSHA256 {
		return res, nil
	}

	// res is filled only once the write lands: a failed strip must not
	// report globs as dropped that are still on disk.
	dropped := append([]string(nil), after.Paths...)
	after.Paths = nil
	receiptDropped := false
	if after.PathsApproval != nil && (priorReceipt == nil || *after.PathsApproval != *priorReceipt) {
		after.PathsApproval = priorReceipt
		receiptDropped = true
	}
	if err := after.Validate(); err != nil {
		return res, fmt.Errorf("sidecar without the introduced paths fails validation: %w", err)
	}
	body, err := sidecar.Marshal(after)
	if err != nil {
		return res, fmt.Errorf("marshal sidecar without the introduced paths: %w", err)
	}
	if err := os.WriteFile(sidecarPath, body, 0o644); err != nil {
		return res, fmt.Errorf("write sidecar without the introduced paths: %w", err)
	}
	res.Dropped, res.ReceiptDropped = dropped, receiptDropped
	return res, nil
}

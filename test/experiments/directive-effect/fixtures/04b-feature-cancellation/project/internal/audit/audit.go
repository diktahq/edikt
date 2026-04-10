// Package audit writes append-only audit records. Every entry records
// WHO did WHAT to WHICH subject within WHICH tenant.
package audit

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Recorder writes audit entries to the append-only audit_log table.
type Recorder struct {
	db *sql.DB
}

// NewRecorder constructs an audit recorder.
func NewRecorder(db *sql.DB) *Recorder {
	return &Recorder{db: db}
}

// Entry is a single audit event.
type Entry struct {
	// TenantID is the tenant the action was performed in. Required.
	TenantID string
	// ActorID is the authenticated user that performed the action.
	ActorID string
	// Action is a stable dotted identifier, e.g. "order.placed".
	Action string
	// Subject is the ID of the entity the action was performed on.
	Subject string
}

// ErrEmptyTenant is returned when Record is called without a tenant ID.
var ErrEmptyTenant = errors.New("audit: empty tenant id")

// Record appends an audit entry.
func (r *Recorder) Record(ctx context.Context, e Entry) error {
	if e.TenantID == "" {
		return ErrEmptyTenant
	}
	const q = `
		INSERT INTO audit_log (tenant_id, actor_id, action, subject, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, q, e.TenantID, e.ActorID, e.Action, e.Subject, time.Now().UTC())
	return err
}

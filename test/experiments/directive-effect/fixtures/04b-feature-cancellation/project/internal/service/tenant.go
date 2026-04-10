package service

import (
	"context"

	"github.com/example/orders-service/internal/ctxkeys"
)

// scope extracts the authoritative tenant and user IDs from the request
// context. Called at the top of every service-layer method. Downstream
// collaborators (repository, audit, events, log calls) take tenant as
// an explicit argument — this function is where the context-to-argument
// translation happens.
func scope(ctx context.Context) (tenantID, userID string, err error) {
	tid, _ := ctx.Value(ctxkeys.TenantID).(string)
	uid, _ := ctx.Value(ctxkeys.UserID).(string)
	if tid == "" || uid == "" {
		return "", "", ErrNoSession
	}
	return tid, uid, nil
}

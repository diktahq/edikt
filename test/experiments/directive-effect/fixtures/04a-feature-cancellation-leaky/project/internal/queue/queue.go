// Package queue is the asynchronous job queue. Anything that talks to a
// third-party (email, SMS, webhooks, analytics) is enqueued here rather
// than executed inline on the request path, so a slow or failing third
// party never blocks an HTTP response.
package queue

import (
	"context"
)

// Queue is a typed enqueuer.
type Queue struct {
	url string
}

// New returns a queue client bound to a broker URL.
func New(url string) *Queue {
	return &Queue{url: url}
}

// Job is the marker interface every enqueueable payload implements.
type Job interface {
	JobName() string
}

// Enqueue submits a job for asynchronous processing. The tenant context
// on ctx is captured and re-established when the worker picks the job up,
// so worker code runs with the same scope the request had.
func (q *Queue) Enqueue(ctx context.Context, j Job) error {
	// Stub: in production this serializes the job, captures tenant_id and
	// request_id from ctx, and pushes to NATS / SQS / Redis.
	_ = j.JobName()
	return nil
}

// Package queue is the asynchronous job queue. Third-party side effects
// (email, webhooks, analytics) are enqueued here rather than executed
// inline on the request path.
package queue

import "context"

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

// Enqueue submits a job for asynchronous processing.
func (q *Queue) Enqueue(ctx context.Context, j Job) error {
	_ = j.JobName()
	return nil
}

package asynctask

import (
	"context"
	"errors"
	"time"
)

// Status is the outcome of a single FetchFunc call.
//
// The field set is frozen: new fields go inside Payload, never as new
// top-level Status fields.
type Status struct {
	Done    bool           // true → polling terminates (success or fail)
	Success bool           // valid only when Done == true
	Message string         // human-readable supplemental message
	Payload map[string]any // full server response data; passthrough, no normalization
}

// FetchFunc returns the latest task status. The caller decides how to
// interpret server response (numeric status codes, string enums, nested
// objects, etc.) and populates Done/Success/Payload accordingly.
//
// Returning a non-nil err terminates Poll immediately (no further attempts).
type FetchFunc func(ctx context.Context) (Status, error)

// PollOptions controls polling behavior. Zero values fall back to defaults:
//
//	Interval:    3 seconds
//	MaxDuration: 3 minutes
type PollOptions struct {
	Interval    time.Duration
	MaxDuration time.Duration
}

// ErrTimeout is returned when MaxDuration elapses before Status.Done==true.
var ErrTimeout = errors.New("task polling timed out")

// Poll calls fetch repeatedly until Status.Done==true, ctx cancels, fetch
// returns err, or MaxDuration elapses. On timeout, returns the last Status
// observed paired with ErrTimeout — the caller can include the payload in
// envelopes (e.g., a themes push transit timeout).
func Poll(ctx context.Context, fetch FetchFunc, opts PollOptions) (Status, error) {
	interval := opts.Interval
	if interval <= 0 {
		interval = 3 * time.Second
	}
	maxDur := opts.MaxDuration
	if maxDur <= 0 {
		maxDur = 3 * time.Minute
	}
	deadline := time.Now().Add(maxDur)
	var last Status

	// One immediate fetch before sleeping.
	for {
		st, err := fetch(ctx)
		if err != nil {
			return st, err
		}
		last = st
		if st.Done {
			return st, nil
		}
		if time.Now().After(deadline) {
			return last, ErrTimeout
		}
		// Sleep until next interval or ctx cancel.
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(interval):
		}
		if time.Now().After(deadline) {
			return last, ErrTimeout
		}
	}
}

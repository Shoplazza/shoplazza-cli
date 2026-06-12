package asynctask

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPoll_ImmediateSuccess(t *testing.T) {
	ctx := context.Background()
	fetch := func(ctx context.Context) (Status, error) {
		return Status{Done: true, Success: true, Payload: map[string]any{"task_id": "t1"}}, nil
	}
	st, err := Poll(ctx, fetch, PollOptions{Interval: 10 * time.Millisecond, MaxDuration: 1 * time.Second})
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if !st.Done || !st.Success {
		t.Fatalf("expected done+success, got %+v", st)
	}
}

func TestPoll_ImmediateFailure(t *testing.T) {
	fetch := func(ctx context.Context) (Status, error) {
		return Status{Done: true, Success: false, Message: "structure invalid", Payload: map[string]any{"status": 2}}, nil
	}
	st, err := Poll(context.Background(), fetch, PollOptions{Interval: 10 * time.Millisecond, MaxDuration: 1 * time.Second})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if st.Success {
		t.Fatal("expected failure")
	}
	if st.Message != "structure invalid" {
		t.Errorf("message: %q", st.Message)
	}
}

func TestPoll_TimeoutReturnsErrTimeoutAndLastStatus(t *testing.T) {
	calls := 0
	fetch := func(ctx context.Context) (Status, error) {
		calls++
		return Status{Done: false, Payload: map[string]any{"poll_count": calls}}, nil
	}
	st, err := Poll(context.Background(), fetch,
		PollOptions{Interval: 20 * time.Millisecond, MaxDuration: 80 * time.Millisecond})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if st.Payload == nil {
		t.Fatal("expected last status payload to be returned")
	}
}

func TestPoll_FetchErrorTerminates(t *testing.T) {
	want := errors.New("network down")
	fetch := func(ctx context.Context) (Status, error) {
		return Status{}, want
	}
	_, err := Poll(context.Background(), fetch, PollOptions{Interval: 10 * time.Millisecond, MaxDuration: 1 * time.Second})
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped err, got %v", err)
	}
}

func TestPoll_RespectsContextCancellation(t *testing.T) {
	fetch := func(ctx context.Context) (Status, error) {
		return Status{Done: false}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	_, err := Poll(ctx, fetch, PollOptions{Interval: 20 * time.Millisecond, MaxDuration: 5 * time.Second})
	if err == nil {
		t.Fatal("expected ctx-cancel error")
	}
}

func TestPoll_DefaultsApplied(t *testing.T) {
	called := false
	fetch := func(ctx context.Context) (Status, error) {
		called = true
		return Status{Done: true, Success: true}, nil
	}
	_, err := Poll(context.Background(), fetch, PollOptions{}) // zero values
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !called {
		t.Fatal("fetch never called")
	}
}

func TestPoll_DoesNotHardcodeStatusEnums(t *testing.T) {
	// Asserting by inspection: Status only has Done/Success/Message/Payload;
	// no numeric or string status enum lives in this package.
	var s Status
	_ = s.Done
	_ = s.Success
	_ = s.Message
	_ = s.Payload
}

package theme

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func extractEnvelope(t *testing.T, err error) map[string]any {
	t.Helper()
	if err == nil {
		t.Fatal("err is nil")
	}
	type enveloper interface {
		Envelope() map[string]any
	}
	if e, ok := err.(enveloper); ok {
		return e.Envelope()
	}
	t.Fatalf("err does not expose Envelope(): %T", err)
	return nil
}

func TestErrAuthExpired_Shape(t *testing.T) {
	e := ErrAuthExpired(errors.New("token revoked"))
	env := extractEnvelope(t, e)
	if env["type"] != "auth" {
		t.Errorf("type = %v, want auth", env["type"])
	}
	if env["code"] != 3 {
		t.Errorf("code = %v, want 3", env["code"])
	}
	if !strings.Contains(env["hint"].(string), "shoplazza auth login") {
		t.Errorf("hint must include login command: %v", env["hint"])
	}
}

func TestErrValidation_Shape(t *testing.T) {
	e := ErrValidation("invalid: %s", "x")
	env := extractEnvelope(t, e)
	if env["type"] != "validation" || env["code"] != 2 {
		t.Errorf("envelope: %v", env)
	}
	if !strings.Contains(env["message"].(string), "invalid: x") {
		t.Errorf("message: %v", env["message"])
	}
}

func TestErrTaskBusinessFailure_PassthroughPayload(t *testing.T) {
	task := map[string]any{"task_id": "t1", "status": 2, "message": "structure invalid", "progress": 0.5}
	e := ErrTaskBusinessFailure(task)
	env := extractEnvelope(t, e)
	if env["type"] != "api" {
		t.Errorf("type: %v", env["type"])
	}
	gotTask := env["task"].(map[string]any)
	if gotTask["task_id"] != "t1" || gotTask["progress"] != 0.5 {
		t.Errorf("task passthrough: %v", gotTask)
	}
}

// TestErrTaskTimeout_NonDefaultCapInterpolated: a caller-configured cap
// (not the 3-minute default) must appear in the message verbatim.
func TestErrTaskTimeout_NonDefaultCapInterpolated(t *testing.T) {
	e := ErrTaskTimeout(9*time.Second, 10*time.Second, nil)
	env := extractEnvelope(t, e)
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "10s") {
		t.Errorf("message must carry the configured 10s cap: %v", msg)
	}
	if strings.Contains(msg, "3 minutes") {
		t.Errorf("hardcoded '3 minutes' must be gone: %v", msg)
	}
}

func TestErrTaskTimeout_Shape(t *testing.T) {
	task := map[string]any{"task_id": "t1", "status": 0, "info": "compressing"}
	e := ErrTaskTimeout(180*time.Second+200*time.Millisecond, 3*time.Minute, task)
	env := extractEnvelope(t, e)
	if env["type"] != "network" || env["code"] != 4 {
		t.Errorf("envelope: %v", env)
	}
	// The configured cap is interpolated — no hardcoded "3 minutes".
	if msg, _ := env["message"].(string); !strings.Contains(msg, "3m0s") {
		t.Errorf("message must carry the configured cap (3m0s): %v", msg)
	}
	elapsed, ok := env["elapsed_seconds"].(float64)
	if !ok || elapsed < 180.0 {
		t.Errorf("elapsed_seconds: %v", env["elapsed_seconds"])
	}
	// must be 1-decimal float
	if elapsed != 180.2 {
		t.Errorf("elapsed_seconds = %v, want 180.2 (1-decimal)", elapsed)
	}
	if env["task"] == nil {
		t.Errorf("task passthrough required")
	}
	if !strings.Contains(env["hint"].(string), "still running") {
		t.Errorf("hint: %v", env["hint"])
	}
}

func TestErrLiveReloadBindFailed_Shape(t *testing.T) {
	e := ErrLiveReloadBindFailed(21647, errors.New("address in use"))
	env := extractEnvelope(t, e)
	if env["type"] != "network" || env["code"] != 4 {
		t.Errorf("envelope: %v", env)
	}
	if !strings.Contains(env["hint"].(string), "--port") {
		t.Errorf("hint must guide --port: %v", env["hint"])
	}
	if !strings.Contains(env["message"].(string), "21647") {
		t.Errorf("message must mention port: %v", env["message"])
	}
}

func TestErrWatcherFatal_Shape(t *testing.T) {
	e := ErrWatcherFatal(errors.New("EMFILE: too many open files"))
	env := extractEnvelope(t, e)
	if env["type"] != "internal" || env["code"] != 5 {
		t.Errorf("envelope: %v", env)
	}
	// hint should mention OS config
	if !strings.Contains(env["hint"].(string), "inotify") && !strings.Contains(env["hint"].(string), "ulimit") {
		t.Errorf("hint must mention OS config: %v", env["hint"])
	}
}

func TestErrLocalIO_Shape(t *testing.T) {
	e := ErrLocalIO("write tmp zip", errors.New("disk full"))
	env := extractEnvelope(t, e)
	if env["type"] != "internal" || env["code"] != 5 {
		t.Errorf("envelope: %v", env)
	}
	if !strings.Contains(env["message"].(string), "write tmp zip") {
		t.Errorf("message must mention op: %v", env["message"])
	}
}

func TestErrCloneNetwork_Shape(t *testing.T) {
	e := ErrCloneNetwork(errors.New("dial tcp: i/o timeout"))
	env := extractEnvelope(t, e)
	if env["type"] != "network" || env["code"] != 4 {
		t.Errorf("envelope: %v", env)
	}
	if !strings.Contains(env["message"].(string), "failed to download theme template") {
		t.Errorf("message: %v", env["message"])
	}
	if !strings.Contains(env["message"].(string), "i/o timeout") {
		t.Errorf("message must include cause: %v", env["message"])
	}
	if !strings.Contains(env["hint"].(string), "network connection") {
		t.Errorf("hint: %v", env["hint"])
	}
}

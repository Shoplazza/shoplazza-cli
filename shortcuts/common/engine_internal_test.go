package common

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// fakeNetErr implements net.Error for classifySendError/classifyExecError tests.
type fakeNetErr struct{ msg string }

func (e *fakeNetErr) Error() string   { return e.msg }
func (e *fakeNetErr) Timeout() bool   { return false }
func (e *fakeNetErr) Temporary() bool { return false }

var _ net.Error = (*fakeNetErr)(nil)

// ── classifySendError ─────────────────────────────────────────────────────────

func TestClassifySendError_HTTPError422(t *testing.T) {
	httpErr := &client.HTTPError{StatusCode: 422, Body: `{"error":"unprocessable"}`, Method: "GET", Path: "/x"}
	got := classifySendError(httpErr)
	var exit *output.ExitError
	if !errors.As(got, &exit) {
		t.Fatalf("expected ExitError, got %T", got)
	}
	if exit.Code != output.ExitAPI {
		t.Errorf("Code: got %v want ExitAPI", exit.Code)
	}
}

func TestClassifySendError_HTTPError403(t *testing.T) {
	httpErr := &client.HTTPError{StatusCode: 403, Body: `{"error":"forbidden"}`, Method: "GET", Path: "/x"}
	got := classifySendError(httpErr)
	var exit *output.ExitError
	if !errors.As(got, &exit) {
		t.Fatalf("expected ExitError, got %T", got)
	}
	// 403 is reclassified as ExitAuth by ErrAPI.
	if exit.Code != output.ExitAuth {
		t.Errorf("Code: got %v want ExitAuth", exit.Code)
	}
}

func TestClassifySendError_NetError(t *testing.T) {
	got := classifySendError(&fakeNetErr{"dial tcp: connection refused"})
	var exit *output.ExitError
	if !errors.As(got, &exit) {
		t.Fatalf("expected ExitError, got %T", got)
	}
	if exit.Code != output.ExitNetwork {
		t.Errorf("Code: got %v want ExitNetwork", exit.Code)
	}
}

func TestClassifySendError_GenericError(t *testing.T) {
	got := classifySendError(errors.New("something weird"))
	var exit *output.ExitError
	if !errors.As(got, &exit) {
		t.Fatalf("expected ExitError, got %T", got)
	}
	if exit.Code != output.ExitInternal {
		t.Errorf("Code: got %v want ExitInternal", exit.Code)
	}
}

// ── classifyExecError ─────────────────────────────────────────────────────────

func TestClassifyExecError_PassthroughExitError(t *testing.T) {
	orig := output.ErrValidation("bad input")
	got := classifyExecError(orig)
	if got != orig {
		t.Errorf("expected the same ExitError to pass through; got %v", got)
	}
}

func TestClassifyExecError_HTTPError(t *testing.T) {
	httpErr := &client.HTTPError{StatusCode: 500, Body: `{"error":"oops"}`, Method: "POST", Path: "/y"}
	got := classifyExecError(httpErr)
	var exit *output.ExitError
	if !errors.As(got, &exit) {
		t.Fatalf("expected ExitError, got %T", got)
	}
	if exit.Code != output.ExitAPI {
		t.Errorf("Code: got %v want ExitAPI", exit.Code)
	}
}

func TestClassifyExecError_NetError(t *testing.T) {
	got := classifyExecError(&fakeNetErr{"timeout"})
	var exit *output.ExitError
	if !errors.As(got, &exit) {
		t.Fatalf("expected ExitError, got %T", got)
	}
	if exit.Code != output.ExitNetwork {
		t.Errorf("Code: got %v want ExitNetwork", exit.Code)
	}
}

func TestClassifyExecError_GenericError(t *testing.T) {
	got := classifyExecError(errors.New("unknown"))
	var exit *output.ExitError
	if !errors.As(got, &exit) {
		t.Fatalf("expected ExitError, got %T", got)
	}
	if exit.Code != output.ExitInternal {
		t.Errorf("Code: got %v want ExitInternal", exit.Code)
	}
}

// ── defaultFloat ──────────────────────────────────────────────────────────────

func TestDefaultFloat_Nil(t *testing.T) {
	if got := defaultFloat(Flag{Name: "f", Type: FlagFloat}); got != 0 {
		t.Errorf("nil Default: got %v want 0", got)
	}
}

func TestDefaultFloat_ValidFloat(t *testing.T) {
	if got := defaultFloat(Flag{Name: "f", Type: FlagFloat, Default: float64(3.14)}); got != 3.14 {
		t.Errorf("got %v want 3.14", got)
	}
}

func TestDefaultFloat_WrongTypePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for wrong Default type")
		}
	}()
	defaultFloat(Flag{Name: "f", Type: FlagFloat, Default: "not-a-float"})
}

// ── defaultStringSlice ────────────────────────────────────────────────────────

func TestDefaultStringSlice_Nil(t *testing.T) {
	if got := defaultStringSlice(Flag{Name: "s", Type: FlagStringSlice}); got != nil {
		t.Errorf("nil Default: got %v want nil", got)
	}
}

func TestDefaultStringSlice_ValidSlice(t *testing.T) {
	want := []string{"a", "b"}
	got := defaultStringSlice(Flag{Name: "s", Type: FlagStringSlice, Default: want})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestDefaultStringSlice_WrongTypePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for wrong Default type")
		}
	}()
	defaultStringSlice(Flag{Name: "s", Type: FlagStringSlice, Default: "not-a-slice"})
}

// ── defaultString ─────────────────────────────────────────────────────────────

func TestDefaultString_Nil(t *testing.T) {
	if got := defaultString(Flag{Name: "s", Type: FlagString}); got != "" {
		t.Errorf("nil Default: got %q want empty", got)
	}
}

func TestDefaultString_ValidString(t *testing.T) {
	if got := defaultString(Flag{Name: "s", Type: FlagString, Default: "hello"}); got != "hello" {
		t.Errorf("got %q want hello", got)
	}
}

func TestDefaultString_WrongTypePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for wrong Default type")
		}
	}()
	defaultString(Flag{Name: "s", Type: FlagString, Default: 42})
}

// ── defaultBool ───────────────────────────────────────────────────────────────

func TestDefaultBool_Nil(t *testing.T) {
	if got := defaultBool(Flag{Name: "b", Type: FlagBool}); got != false {
		t.Errorf("nil Default: got %v want false", got)
	}
}

func TestDefaultBool_ValidBool(t *testing.T) {
	if got := defaultBool(Flag{Name: "b", Type: FlagBool, Default: true}); got != true {
		t.Errorf("got %v want true", got)
	}
}

func TestDefaultBool_WrongTypePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for wrong Default type")
		}
	}()
	defaultBool(Flag{Name: "b", Type: FlagBool, Default: "not-a-bool"})
}

// ── ValidateShortcut ──────────────────────────────────────────────────────────

func TestValidateShortcut_Valid(t *testing.T) {
	s := Shortcut{Service: "orders", Command: "+list", Plan: func(PlanInput) (PlannedRequest, error) { return PlannedRequest{}, nil }}
	if err := ValidateShortcut(s); err != nil {
		t.Errorf("valid shortcut: unexpected error: %v", err)
	}
}

func TestValidateShortcut_EmptyService(t *testing.T) {
	s := Shortcut{Service: "", Command: "+list", Plan: func(PlanInput) (PlannedRequest, error) { return PlannedRequest{}, nil }}
	if err := ValidateShortcut(s); err == nil {
		t.Error("expected error for empty Service")
	}
}

func TestValidateShortcut_EmptyCommand(t *testing.T) {
	s := Shortcut{Service: "orders", Command: "", Plan: func(PlanInput) (PlannedRequest, error) { return PlannedRequest{}, nil }}
	if err := ValidateShortcut(s); err == nil {
		t.Error("expected error for empty Command")
	}
}

func TestValidateShortcut_BothNil(t *testing.T) {
	s := Shortcut{Service: "orders", Command: "+list"}
	if err := ValidateShortcut(s); err == nil {
		t.Error("expected error when both Plan and Execute are nil")
	}
}

func TestValidateShortcut_BothSet(t *testing.T) {
	s := Shortcut{
		Service: "orders", Command: "+list",
		Plan:    func(PlanInput) (PlannedRequest, error) { return PlannedRequest{}, nil },
		Execute: func(_ context.Context, in ExecInput) (ExecResult, error) { return ExecResult{}, nil },
	}
	if err := ValidateShortcut(s); err == nil {
		t.Error("expected error when both Plan and Execute are set")
	}
}

package checkout_test

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"

	checkout "github.com/Shoplazza/shoplazza-cli/v2/cmd/checkout"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func TestParseDevIDs(t *testing.T) {
	got, exitErr := checkout.ParseDevIDs([]string{"a,b", "c"}, false)
	if exitErr != nil {
		t.Fatalf("unexpected: %v", exitErr)
	}
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("got %v", got)
	}
}

func TestParseDevIDs_AllReturnsEmpty(t *testing.T) {
	got, exitErr := checkout.ParseDevIDs(nil, true)
	if exitErr != nil || len(got) != 0 {
		t.Fatalf("--all should yield empty id list (Node reads all), got %v err %v", got, exitErr)
	}
}

func TestParseDevIDs_NeitherIsValidation(t *testing.T) {
	_, exitErr := checkout.ParseDevIDs(nil, false)
	if exitErr == nil || exitErr.Detail.Type != output.TypeValidation {
		t.Fatalf("neither --extension-name nor --all must be type=validation, got %v", exitErr)
	}
}

// waitErrFrom runs a small shell script and returns its Wait error, giving the
// classification tests a real *exec.ExitError (exit code or signal).
func waitErrFrom(t *testing.T, script string) error {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	err := cmd.Run()
	if err == nil {
		t.Fatalf("script %q should have failed", script)
	}
	return err
}

func TestClassifyDevExit_NonZeroExitIsInternal(t *testing.T) {
	exitErr := checkout.ClassifyDevExit(waitErrFrom(t, "exit 7"), nil)
	if exitErr == nil || exitErr.Code != output.ExitInternal || exitErr.Detail.Type != output.TypeInternal {
		t.Fatalf("non-zero child exit must be internal-class, got %v", exitErr)
	}
	if !strings.Contains(exitErr.Detail.Message, "7") {
		t.Errorf("message should carry the exit code, got %q", exitErr.Detail.Message)
	}
}

func TestClassifyDevExit_SignaledIsCleanStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal-driven exit (WaitStatus.Signaled) is a POSIX concept")
	}
	if exitErr := checkout.ClassifyDevExit(waitErrFrom(t, "kill -TERM $$"), nil); exitErr != nil {
		t.Fatalf("signal-driven exit (forwarded Ctrl-C/SIGTERM) must not be a CLI failure, got %v", exitErr)
	}
}

func TestClassifyDevExit_ContextCanceledIsCleanStop(t *testing.T) {
	// With the no-op c.Cancel, Wait can return the absorbed ctx.Err() after a
	// clean exit following Ctrl-C — that is a clean stop.
	if exitErr := checkout.ClassifyDevExit(context.Canceled, nil); exitErr != nil {
		t.Fatalf("absorbed ctx.Err() must not be a failure, got %v", exitErr)
	}
	// Child catches SIGINT and exits non-zero while the root signal ctx is
	// canceled: still user-initiated, still a clean stop.
	if exitErr := checkout.ClassifyDevExit(waitErrFrom(t, "exit 130"), context.Canceled); exitErr != nil {
		t.Fatalf("non-zero exit after ctx cancel must not be a failure, got %v", exitErr)
	}
}

func TestClassifyDevExit_NonExitErrorIsInternal(t *testing.T) {
	exitErr := checkout.ClassifyDevExit(errors.New("wait: boom"), nil)
	if exitErr == nil || exitErr.Code != output.ExitInternal {
		t.Fatalf("unexpected Wait error must be internal-class, got %v", exitErr)
	}
}

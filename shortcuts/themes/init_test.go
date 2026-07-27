package themes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

	"github.com/spf13/cobra"
)

// flagsWithName builds a FlagSet over a freshly-constructed cobra command so
// that GetString("name") returns the supplied value. Empty name means the flag
// is registered but defaulted to "" (mirrors the missing-flag case).
func flagsWithName(name string) common.FlagSet {
	cmd := &cobra.Command{Use: "init"}
	cmd.Flags().String("name", name, "")
	return common.NewCobraFlagSet(cmd)
}

// captureStderr swaps os.Stderr for a pipe while fn runs, returns everything
// written. Restores os.Stderr on exit so other tests are unaffected.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() {
		os.Stderr = old
	}()
	fn()
	_ = w.Close()
	return <-done
}

func TestInit_DryRunPrintsCdHintToStderr(t *testing.T) {
	var res common.ExecResult
	var execErr error
	stderr := captureStderr(t, func() {
		in := common.ExecInput{
			Args:   nil,
			Flags:  flagsWithName("my-shop"),
			Tool:   "init",
			DryRun: true,
		}
		res, execErr = initShortcut.Execute(context.Background(), in)
	})
	if execErr != nil {
		t.Fatalf("Execute err: %v", execErr)
	}
	if !strings.Contains(stderr, "cd my-shop") {
		t.Errorf("stderr missing cd hint: %q", stderr)
	}
	if !strings.Contains(stderr, "shoplazza themes serve") {
		t.Errorf("stderr missing next-step hint: %q", stderr)
	}
	if !strings.Contains(stderr, "(if executed") {
		t.Errorf("stderr missing dry-run preamble: %q", stderr)
	}
	if res.Body == nil {
		t.Fatal("dry-run body should not be nil")
	}
	if res.Body["dry_run"] != true {
		t.Errorf("Body.dry_run = %v, want true", res.Body["dry_run"])
	}
	if res.Body["theme_dir"] != "./my-shop" {
		t.Errorf("Body.theme_dir = %v, want ./my-shop", res.Body["theme_dir"])
	}
	if res.Body["action"] != "clone-template" {
		t.Errorf("Body.action = %v, want clone-template", res.Body["action"])
	}
}

func TestInit_NameRequired(t *testing.T) {
	// Even with stderr capture (in case the shortcut writes anything on the
	// early-return path), assert that error is returned.
	var execErr error
	_ = captureStderr(t, func() {
		in := common.ExecInput{
			Flags:  flagsWithName(""),
			Tool:   "init",
			DryRun: true,
		}
		_, execErr = initShortcut.Execute(context.Background(), in)
	})
	if execErr == nil {
		t.Fatal("expected error when --name missing")
	}
	if !strings.Contains(execErr.Error(), "name") {
		t.Errorf("error should mention --name; got: %v", execErr)
	}
}

// TestInit_RejectsUnsafeNames: --name is used as a directory to extract into;
// separators, "..", "." and absolute paths previously escaped the cwd.
func TestInit_RejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"../evil", "a/b", `a\b`, "..", ".", "/tmp/abs"} {
		var execErr error
		_ = captureStderr(t, func() {
			_, execErr = initShortcut.Execute(context.Background(), common.ExecInput{
				Flags: flagsWithName(name),
				Tool:  "init",
			})
		})
		if execErr == nil {
			t.Errorf("name %q: expected validation error", name)
			continue
		}
		type envelopeCarrier interface{ Envelope() map[string]any }
		var ec envelopeCarrier
		if !errors.As(execErr, &ec) {
			t.Errorf("name %q: error does not implement Envelope(); got %T", name, execErr)
			continue
		}
		if env := ec.Envelope(); env["type"] != output.TypeValidation {
			t.Errorf("name %q: type = %v, want validation", name, env["type"])
		}
	}
}

// TestInit_NonEmptyTargetDirIsValidationError: re-initializing over an
// existing non-empty directory previously truncated its files. The guard
// fires before any download — no network involved.
func TestInit_NonEmptyTargetDirIsValidationError(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "my-shop", "layout"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "my-shop", "layout", "theme.liquid"), []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}

	var execErr error
	_ = captureStderr(t, func() {
		_, execErr = initShortcut.Execute(context.Background(), common.ExecInput{
			Flags: flagsWithName("my-shop"),
			Tool:  "init",
		})
	})
	if execErr == nil {
		t.Fatal("expected validation error for non-empty target dir")
	}
	type envelopeCarrier interface{ Envelope() map[string]any }
	var ec envelopeCarrier
	if !errors.As(execErr, &ec) {
		t.Fatalf("error does not implement Envelope(); got %T: %v", execErr, execErr)
	}
	if env := ec.Envelope(); env["type"] != output.TypeValidation {
		t.Errorf("type = %v, want validation", env["type"])
	}
	got, rerr := os.ReadFile(filepath.Join(tmp, "my-shop", "layout", "theme.liquid"))
	if rerr != nil || string(got) != "precious" {
		t.Errorf("existing file must be untouched; content=%q err=%v", got, rerr)
	}
}

// TestClassifyCloneErr_Classes: the classifier maps each CloneTemplate
// failure family to the right envelope class.
func TestClassifyCloneErr_Classes(t *testing.T) {
	type envelopeCarrier interface{ Envelope() map[string]any }
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"non-empty target", fmt.Errorf("%w: ./x", pack.ErrTargetDirNotEmpty), output.TypeValidation},
		{"non-200 download", fmt.Errorf("%w: HTTP 502", pack.ErrTemplateDownload), output.TypeNetwork},
		{"url error", &url.Error{Op: "Get", URL: "https://x", Err: errors.New("dial tcp: i/o timeout")}, output.TypeNetwork},
		{"local io", errors.New("tar read: unexpected EOF"), output.TypeInternal},
	}
	for _, c := range cases {
		got := classifyCloneErr(c.err)
		var ec envelopeCarrier
		if !errors.As(got, &ec) {
			t.Errorf("%s: classifyCloneErr did not produce an envelope: %T", c.name, got)
			continue
		}
		if env := ec.Envelope(); env["type"] != c.want {
			t.Errorf("%s: type = %v, want %v", c.name, env["type"], c.want)
		}
	}
}

// TestInit_DeclaresAuthFreeAndLocal: init performs zero Shoplazza API calls —
// it must run without login and print its result without the {ok,data}
// API success envelope.
func TestInit_DeclaresAuthFreeAndLocal(t *testing.T) {
	if !initShortcut.AuthFree {
		t.Error("themes init must be AuthFree (purely local)")
	}
	if !initShortcut.Local {
		t.Error("themes init must be Local (no API success envelope)")
	}
	if !packageShortcut.AuthFree {
		t.Error("themes package must be AuthFree (purely local)")
	}
	if !packageShortcut.Local {
		t.Error("themes package must be Local (no API success envelope)")
	}
	// All other themes shortcuts stay gated and envelope-wrapped.
	for _, s := range []common.Shortcut{pullShortcut, pushShortcut, shareShortcut, serveShortcut} {
		if s.AuthFree {
			t.Errorf("themes %s must remain auth-gated", s.Command)
		}
		if s.Local {
			t.Errorf("themes %s must keep the API success envelope", s.Command)
		}
	}
}

func TestInit_LiveModeClonesAndPrintsCdHint(t *testing.T) {
	if testing.Short() {
		t.Skip("skip clone in -short mode (needs GitHub network)")
	}
	tmp := t.TempDir()
	t.Chdir(tmp) // Go 1.24+

	var res common.ExecResult
	var execErr error
	stderr := captureStderr(t, func() {
		in := common.ExecInput{
			Flags:  flagsWithName("my-shop"),
			Tool:   "init",
			DryRun: false,
		}
		res, execErr = initShortcut.Execute(context.Background(), in)
	})
	if execErr != nil {
		t.Skipf("live clone failed (offline or rate-limited): %v", execErr)
	}
	if _, err := os.Stat(filepath.Join(tmp, "my-shop", "config")); err != nil {
		t.Errorf("expected my-shop/config to exist after clone: %v", err)
	}
	if !strings.Contains(stderr, "cd my-shop") {
		t.Errorf("stderr missing cd hint: %q", stderr)
	}
	if !strings.Contains(stderr, "shoplazza themes serve") {
		t.Errorf("stderr missing next-step hint: %q", stderr)
	}
	if strings.Contains(stderr, "(if executed") {
		t.Errorf("live mode should NOT show dry-run preamble: %q", stderr)
	}
	if res.Body == nil {
		t.Fatal("live body should not be nil")
	}
	if res.Body["theme_dir"] != "./my-shop" {
		t.Errorf("Body.theme_dir = %v, want ./my-shop", res.Body["theme_dir"])
	}
	if res.Body["status"] != "initialized" {
		t.Errorf("Body.status = %v, want initialized", res.Body["status"])
	}
}

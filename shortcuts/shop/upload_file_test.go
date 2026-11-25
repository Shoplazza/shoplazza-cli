package shop

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
	"github.com/spf13/cobra"
)

func newShopExecInputWithClient(t *testing.T, values map[string]string, c *client.Client) common.ExecInput {
	t.Helper()
	in := newShopExecInput(t, values, false)
	in.Client = c
	return in
}

func newShopExecInput(t *testing.T, values map[string]string, dryRun bool) common.ExecInput {
	t.Helper()
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.Flags().StringSlice("source-url", nil, "")
	var args []string
	for name, val := range values {
		args = append(args, "--"+name+"="+val)
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}
	return common.ExecInput{Flags: common.NewCobraFlagSet(cmd), DryRun: dryRun}
}

func TestUploadFileShortcut_DeclarativeShape(t *testing.T) {
	if uploadFileShortcut.Service != "shop" || uploadFileShortcut.Command != "+upload-file" {
		t.Errorf("identity wrong")
	}
	if uploadFileShortcut.Execute == nil {
		t.Fatal("+upload-file requires Execute (POST + single GET)")
	}
	if err := common.ValidateShortcut(uploadFileShortcut); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestExtractTaskID_OK(t *testing.T) {
	resp := map[string]any{"task_id": "task-123"}
	got := extractTaskID(resp)
	if got != "task-123" {
		t.Errorf("got %q want task-123", got)
	}
}

func TestExtractTaskID_Missing(t *testing.T) {
	resp := map[string]any{"foo": "bar"}
	got := extractTaskID(resp)
	if got != "" {
		t.Errorf("expected empty string for missing task_id; got %q", got)
	}
}

func TestUploadFileShortcut_HasSourceURLFlag(t *testing.T) {
	var hasSourceURL bool
	for _, f := range uploadFileShortcut.Flags {
		if f.Name == "source-url" {
			hasSourceURL = true
			if !f.Required {
				t.Error("--source-url must be Required")
			}
			if f.Type != common.FlagStringSlice {
				t.Error("--source-url must be FlagStringSlice (can repeat)")
			}
		}
		if f.Name == "file" || f.Name == "folder" {
			t.Errorf("--%s flag should not exist", f.Name)
		}
	}
	if !hasSourceURL {
		t.Error("--source-url flag missing")
	}
}

func TestUploadFileShortcutExecute_DryRun(t *testing.T) {
	cases := []struct {
		name     string
		urls     string
		wantErr  string
		wantHint string
	}{
		{name: "https", urls: "https://example.com/img.jpg"},
		{name: "http and https", urls: "http://example.com/a.jpg,HTTPS://example.com/b.jpg"},
		{name: "local path", urls: "/tmp/a.jpg", wantErr: `"/tmp/a.jpg"`, wantHint: "public link"},
		{name: "file scheme", urls: "file:///tmp/a.jpg", wantErr: `"file:///tmp/a.jpg"`},
		{name: "no scheme", urls: "example.com/a.jpg", wantErr: `"example.com/a.jpg"`},
		{name: "ftp", urls: "ftp://example.com/a.jpg", wantErr: `"ftp://example.com/a.jpg"`},
		{name: "no host", urls: "https:///a.jpg", wantErr: `"https:///a.jpg"`},
		{name: "one bad among good", urls: "https://example.com/a.jpg,./b.jpg", wantErr: `"./b.jpg"`},
		{name: "data uri", urls: "data:image/png;base64,iVBORw0KGgo=", wantErr: `"data:image/png;base64"`, wantHint: "shop files upload --data"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := newShopExecInput(t, map[string]string{"source-url": c.urls}, true)
			result, err := uploadFileShortcut.Execute(context.Background(), in)
			if c.wantErr != "" {
				var exitErr *output.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("want validation error, got %v", err)
				}
				if exitErr.Code != output.ExitValidation || exitErr.Detail.Param != "--source-url" ||
					exitErr.Detail.Subtype != output.SubtypeInvalidFlagValue || !strings.Contains(exitErr.Detail.Message, c.wantErr) {
					t.Errorf("got code=%d param=%q subtype=%q message=%q", exitErr.Code, exitErr.Detail.Param, exitErr.Detail.Subtype, exitErr.Detail.Message)
				}
				if !strings.Contains(exitErr.Detail.Hint, c.wantHint) {
					t.Errorf("hint: got %q want it to mention %q", exitErr.Detail.Hint, c.wantHint)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if len(result.Plans) != 2 {
				t.Fatalf("dry-run should return 2 plans (POST + GET), got %d", len(result.Plans))
			}
			body, _ := result.Plans[0].Body.(map[string]any)
			if len(body) != 1 || body["original_source_list"] == nil {
				t.Errorf("POST body: got %v want only original_source_list", body)
			}
		})
	}
}

// TestUploadFileShortcutExecute_LivePostThenGet covers the non-dry-run POST-then-GET path.
func TestUploadFileShortcutExecute_LivePostThenGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"task_id": "tid-1"}})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"status": "done", "task_id": "tid-1"}})
		}
	}))
	defer srv.Close()

	in := newShopExecInputWithClient(t, map[string]string{"source-url": "https://example.com/a.jpg"}, client.New(srv.URL))
	result, err := uploadFileShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Body == nil {
		t.Error("expected a non-nil body from live execute")
	}
}

// TestUploadFileShortcutExecute_LiveNoTaskID covers the path where POST returns no task_id.
func TestUploadFileShortcutExecute_LiveNoTaskID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
			"data": map[string]any{"url": "https://cdn.example.com/a.jpg"}})
	}))
	defer srv.Close()

	in := newShopExecInputWithClient(t, map[string]string{"source-url": "https://example.com/a.jpg"}, client.New(srv.URL))
	result, err := uploadFileShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Body == nil {
		t.Error("expected a non-nil body")
	}
}

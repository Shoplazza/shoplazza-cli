package appcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func TestRunVersionsList_Paginates(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
			"data": map[string]any{
				"versions": []map[string]any{{"id": 1, "version": "1.0.0"}, {"id": 2, "version": "1.1.0"}},
				"has_more": true}})
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	var buf bytes.Buffer
	if err := runVersionsList(context.Background(), d, "p1", "cid_1", 0, 10, &buf, "json", ""); err != nil {
		t.Fatalf("runVersionsList: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "1.0.0") || !strings.Contains(out, "1.1.0") {
		t.Fatalf("output missing versions: %s", out)
	}
	if !strings.Contains(gotQuery, "offset=0") || !strings.Contains(gotQuery, "limit=10") {
		t.Fatalf("query missing offset/limit: %s", gotQuery)
	}
}

// TestRunVersionsList_ServerError_RoutesViaErrAPI locks the critical contract:
// a Dashboard 5xx is routed through output.ErrAPI (exit ExitAPI, server message
// masked) rather than ErrInternal. Guards against a future refactor that wraps
// the Dashboard error and silently drops back to exit 5.
func TestRunVersionsList_ServerError_RoutesViaErrAPI(t *testing.T) {
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})
	var buf bytes.Buffer
	err := runVersionsList(context.Background(), d, "p1", "cid_1", 0, 20, &buf, "json", "")
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if ee.Code != output.ExitAPI {
		t.Fatalf("expected ExitAPI (%d), got %d", output.ExitAPI, ee.Code)
	}
}

// TestVersions_RunE_UnreadableConfig_SurfacesCause: when no flag fills the gap
// AND ActiveConfig() itself failed (here: no toml at all), the error names the
// unreadable config — not a misleading "no client_id".
func TestVersions_RunE_UnreadableConfig_SurfacesCause(t *testing.T) {
	// Temp dir with no shoplazza.app.toml → ActiveConfig() read fails.
	cmd := newCmdVersions(&cmdutil.Factory{})
	if err := cmd.Flags().Set("path", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	err := cmd.RunE(cmd, nil)
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
	if !strings.Contains(ee.Error(), "cannot read active config") {
		t.Fatalf("message = %q, want the read-failure cause surfaced", ee.Error())
	}
}

// TestVersions_RunE_ConfigReadableButNoClientID keeps the genuinely-absent
// branch on the original "no client_id" message.
func TestVersions_RunE_ConfigReadableButNoClientID(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shoplazza.app.toml"),
		[]byte("partner_id = \"p1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newCmdVersions(&cmdutil.Factory{})
	if err := cmd.Flags().Set("path", root); err != nil {
		t.Fatal(err)
	}
	err := cmd.RunE(cmd, nil)
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
	if !strings.Contains(ee.Error(), "no client_id") {
		t.Fatalf("message = %q, want the no-client_id wording", ee.Error())
	}
}

// TestRunVersionsList_Forbidden_RoutesToAuth verifies the 403 branch of apiError
// reclassifies to auth-class (ExitAuth) via output.ErrAPI.
func TestRunVersionsList_Forbidden_RoutesToAuth(t *testing.T) {
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	})
	var buf bytes.Buffer
	err := runVersionsList(context.Background(), d, "p1", "cid_1", 0, 20, &buf, "json", "")
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if ee.Code != output.ExitAuth {
		t.Fatalf("expected ExitAuth (%d) for 403, got %d", output.ExitAuth, ee.Code)
	}
}

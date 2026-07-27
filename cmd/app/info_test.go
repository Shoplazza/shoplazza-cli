package appcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/app/project"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func TestRunInfo_AppAndExtensions(t *testing.T) {
	root := t.TempDir()
	// active config
	os.WriteFile(filepath.Join(root, "shoplazza.app.toml"), []byte("client_id = \"cid_1\"\n"), 0o644)
	// two extensions
	mkExt := func(dir, name, typ string) {
		d := filepath.Join(root, "extensions", dir)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "shoplazza.extension.toml"),
			[]byte("name = \""+name+"\"\ntype = \""+typ+"\"\n"), 0o644)
	}
	mkExt("co", "Checkout Ext", "checkout")
	mkExt("th", "Theme Ext", "theme")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
			"data": map[string]any{
				"user":    map[string]any{"shoplazza_account": "a@x.com"},
				"partner": map[string]any{"id": "p1", "business_name": "Acme"},
				"app":     map[string]any{"client_id": "cid_1", "name": "MyApp", "scopes": []string{"read"}}}})
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	p, _ := project.Open(root)
	var buf bytes.Buffer
	if err := runInfo(context.Background(), d, p, "", &buf, io.Discard, "json", ""); err != nil {
		t.Fatalf("runInfo: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"cid_1", "MyApp", "Checkout Ext", "checkout", "Theme Ext", "theme"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

// TestRunInfo_ClientIDOverride locks the --client-id path: the lookup targets
// the supplied client_id (not the active config), p may be nil, and the local
// extensions list is omitted (it belongs to the project's app, not this one).
func TestRunInfo_ClientIDOverride(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("app_client_id")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
			"data": map[string]any{"app": map[string]any{"client_id": "cid_other", "name": "OtherApp"}}})
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	var buf bytes.Buffer
	// p is nil — the override path must not dereference it.
	if err := runInfo(context.Background(), d, nil, "cid_other", &buf, io.Discard, "json", ""); err != nil {
		t.Fatalf("runInfo with --client-id: %v", err)
	}
	if gotQuery != "cid_other" {
		t.Fatalf("queried app_client_id = %q, want cid_other", gotQuery)
	}
	out := buf.String()
	if !strings.Contains(out, "OtherApp") {
		t.Fatalf("output missing queried app: %s", out)
	}
	if strings.Contains(out, "extensions") {
		t.Fatalf("extensions must be omitted on --client-id override: %s", out)
	}
}

// TestRunInfo_ScopesFromLocalConfig locks the v1-parity fix: the /info endpoint
// returns no scopes, so info surfaces the scopes recorded in the local config
// (space-joined string → array), not the API's null.
func TestRunInfo_ScopesFromLocalConfig(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "shoplazza.app.toml"),
		[]byte("client_id = \"cid_1\"\nscopes = \"read_customer write_cart_transform\"\n"), 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// /info has no "scopes" field, as the real backend returns.
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
			"data": map[string]any{"app": map[string]any{"client_id": "cid_1", "name": "MyApp"}}})
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	p, _ := project.Open(root)
	var buf bytes.Buffer
	if err := runInfo(context.Background(), d, p, "", &buf, io.Discard, "json", ""); err != nil {
		t.Fatalf("runInfo: %v", err)
	}
	var got struct {
		Data struct {
			App struct {
				Scopes []string `json:"scopes"`
			} `json:"app"`
		} `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	want := []string{"read_customer", "write_cart_transform"}
	if len(got.Data.App.Scopes) != 2 || got.Data.App.Scopes[0] != want[0] || got.Data.App.Scopes[1] != want[1] {
		t.Fatalf("scopes = %v, want %v", got.Data.App.Scopes, want)
	}
}

func TestRunInfo_NoClientIDErrors(t *testing.T) {
	root := t.TempDir()
	// no shoplazza.app.toml → empty config → client_id=""
	p, _ := project.Open(root)
	var buf bytes.Buffer
	err := runInfo(context.Background(), nil, p, "", &buf, io.Discard, "json", "")
	if err == nil {
		t.Fatal("expected error when active config has no client_id")
	}
}

// TestRunInfo_NoExtensionsDir tolerates a project without extensions/ (info now
// reuses internal/app.ScanLocalExtensions; a missing dir is not an error).
func TestRunInfo_NoExtensionsDir(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "shoplazza.app.toml"), []byte("client_id = \"cid_1\"\n"), 0o644)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
			"data": map[string]any{"app": map[string]any{"client_id": "cid_1"}}})
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	p, _ := project.Open(root)
	var buf bytes.Buffer
	if err := runInfo(context.Background(), d, p, "", &buf, io.Discard, "json", ""); err != nil {
		t.Fatalf("runInfo without extensions/ should succeed: %v", err)
	}
}

// TestRunInfo_MalformedExtensionToml_Validation: on the info path, a present
// but unparseable extension toml is a validation error naming the file, not a
// silently skipped dir.
func TestRunInfo_MalformedExtensionToml_Validation(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "shoplazza.app.toml"), []byte("client_id = \"cid_1\"\n"), 0o644)
	extDir := filepath.Join(root, "extensions", "broken")
	os.MkdirAll(extDir, 0o755)
	os.WriteFile(filepath.Join(extDir, "shoplazza.extension.toml"), []byte("name = \"oops\ntype =\n"), 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
			"data": map[string]any{"app": map[string]any{"client_id": "cid_1"}}})
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	p, _ := project.Open(root)
	var buf bytes.Buffer
	err := runInfo(context.Background(), d, p, "", &buf, io.Discard, "json", "")
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if ee.Code != output.ExitValidation {
		t.Fatalf("exit code = %d, want ExitValidation (%d)", ee.Code, output.ExitValidation)
	}
	if !strings.Contains(ee.Error(), "extensions/broken/shoplazza.extension.toml") {
		t.Fatalf("error should name the malformed file, got %q", ee.Error())
	}
}

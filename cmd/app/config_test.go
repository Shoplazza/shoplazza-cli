package appcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/app"
	"shoplazza-cli-v2/internal/app/project"
	"shoplazza-cli-v2/internal/client"
)

func TestRunConfigUse_ValidatesThenSwitches(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "shoplazza.app.staging.toml"), []byte("client_id = \"cid_staging\"\n"), 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// /api/cli/v2/info?app_client_id=cid_staging
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
			"data": map[string]any{"app": map[string]any{"client_id": "cid_staging"}}})
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	p, _ := project.Open(root)
	var buf bytes.Buffer
	if err := runConfigUse(context.Background(), d, p, "shoplazza.app.staging.toml", &buf, "json", ""); err != nil {
		t.Fatalf("runConfigUse: %v", err)
	}
	name, _ := p.ActiveConfigName()
	if name != "shoplazza.app.staging.toml" {
		t.Fatalf("active = %q", name)
	}
}

func TestRunConfigUse_ValidationFails_DoesNotSwitch(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "shoplazza.app.bad.toml"), []byte("client_id = \"cid_bad\"\n"), 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"NotFound","message":"no such app"}`))
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	p, _ := project.Open(root)
	var buf bytes.Buffer
	if err := runConfigUse(context.Background(), d, p, "shoplazza.app.bad.toml", &buf, "json", ""); err == nil {
		t.Fatal("expected error on validation failure")
	}
	// active pointer must remain default (state file not written)
	if _, statErr := os.Stat(filepath.Join(root, ".shoplazza", "app-state.json")); !os.IsNotExist(statErr) {
		t.Fatalf("state file should NOT exist after failed validation, err=%v", statErr)
	}
}

func TestRunConfigUse_MissingClientIDErrors(t *testing.T) {
	root := t.TempDir()
	// Config file exists but has no client_id.
	os.WriteFile(filepath.Join(root, "shoplazza.app.empty.toml"), []byte("scopes = []\n"), 0o644)
	p, _ := project.Open(root)
	var buf bytes.Buffer
	if err := runConfigUse(context.Background(), nil, p, "shoplazza.app.empty.toml", &buf, "json", ""); err == nil {
		t.Fatal("expected error when config has no client_id")
	}
}

func dashFor(t *testing.T, h http.HandlerFunc) *app.Dashboard {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return app.NewDashboard(client.New(srv.URL), "ptok")
}

func TestRunConfigLink_LinkExisting(t *testing.T) {
	root := t.TempDir()
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			// Link path derives partner from client_id via /info.
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{
					"partner": map[string]any{"id": "p1", "name": "Acme"},
					"app":     map[string]any{"client_id": "cid_x", "name": "ProdApp", "scopes": []string{"read"}}}})
		case strings.Contains(r.URL.Path, "/apps/cid_x"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"app": map[string]any{"client_id": "cid_x", "id": 1, "name": "ProdApp", "scopes": []string{"read"}}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	p, _ := project.Open(root)
	var buf bytes.Buffer
	// --config is a NAME segment now → shoplazza.app.prod.toml.
	err := runConfigLink(context.Background(), d, p, linkOpts{ClientID: "cid_x", ConfigName: "prod"}, &buf, "json", "")
	if err != nil {
		t.Fatalf("runConfigLink: %v", err)
	}
	cfg, err := p.ReadConfig("shoplazza.app.prod.toml")
	if err != nil || cfg.ClientID != "cid_x" {
		t.Fatalf("written config = %+v, %v", cfg, err)
	}
	if cfg.PartnerID != "p1" {
		t.Fatalf("partner_id not persisted into config: got %q, want p1", cfg.PartnerID)
	}
}

func TestRunConfigLink_CreateNew(t *testing.T) {
	root := t.TempDir()
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/partners"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"partners": []map[string]any{{"id": "p1"}}}})
		case strings.HasSuffix(r.URL.Path, "/apps") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"app": map[string]any{"client_id": "cid_new", "id": 2, "name": "DevApp", "scopes": []string{}}}})
		default:
			t.Fatalf("unexpected path %s %s", r.Method, r.URL.Path)
		}
	})
	p, _ := project.Open(root)
	var buf bytes.Buffer
	err := runConfigLink(context.Background(), d, p, linkOpts{Create: true, Name: "DevApp", ConfigName: "dev"}, &buf, "json", "")
	if err != nil {
		t.Fatalf("runConfigLink: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "shoplazza.app.dev.toml")); statErr != nil {
		t.Fatalf("config file not written: %v", statErr)
	}
}

func TestRunConfigLink_NoSelector_Validation(t *testing.T) {
	root := t.TempDir()
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {})
	p, _ := project.Open(root)
	var buf bytes.Buffer
	if err := runConfigLink(context.Background(), d, p, linkOpts{}, &buf, "json", ""); err == nil {
		t.Fatal("expected validation error when neither --client-id nor --create given")
	}
}

// ── sanitizeConfigName ────────────────────────────────────────────────────────

func TestSanitizeConfigName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"My App Config", "my-app-config"},
		{"hello_world", "hello_world"},
		{"  spaces  ", "spaces"},
		{"MiXeD-CaSe_123", "mixed-case_123"},
		{"---leading-trailing---", "leading-trailing"},
		{"", "app"},
		{"!!!", "app"},
		{"a!!b", "a-b"},
	}
	for _, c := range cases {
		if got := sanitizeConfigName(c.in); got != c.want {
			t.Errorf("sanitizeConfigName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestConfigFileForName locks the Shopify-style name→filename mapping: --config
// takes a NAME segment, not a full filename.
func TestConfigFileForName(t *testing.T) {
	cases := []struct{ name, want string }{
		{"prod", "shoplazza.app.prod.toml"},
		{"staging", "shoplazza.app.staging.toml"},
		{"My App", "shoplazza.app.my-app.toml"},
	}
	for _, c := range cases {
		if got := configFileForName(c.name); got != c.want {
			t.Errorf("configFileForName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

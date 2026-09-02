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

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/app/project"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
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
	// link auto-activates the linked config.
	if active, _ := p.ActiveConfigName(); active != "shoplazza.app.prod.toml" {
		t.Fatalf("link should activate the config, got active=%q", active)
	}
	if !strings.Contains(buf.String(), "active_config") {
		t.Fatalf("output should report active_config, got %s", buf.String())
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
	if active, _ := p.ActiveConfigName(); active != "shoplazza.app.dev.toml" {
		t.Fatalf("create+link should activate the config, got active=%q", active)
	}
}

// When the Dashboard returns no scopes and the target config has none, link
// fills the template default (mirrors `app init`).
func TestRunConfigLink_EmptyScopes_FillsTemplateDefault(t *testing.T) {
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
	if err := runConfigLink(context.Background(), d, p, linkOpts{Create: true, Name: "DevApp", ConfigName: "dev"}, &buf, "json", ""); err != nil {
		t.Fatalf("runConfigLink: %v", err)
	}
	cfg, err := p.ReadConfig("shoplazza.app.dev.toml")
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if cfg.Scopes != project.DefaultScopes {
		t.Errorf("empty scopes should fall back to template default; got %q want %q", cfg.Scopes, project.DefaultScopes)
	}
}

// Empty Dashboard scopes must NOT overwrite scopes already in the target config.
func TestRunConfigLink_EmptyScopes_PreservesExisting(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "shoplazza.app.dev.toml"),
		[]byte("client_id = \"old\"\nscopes = \"read_product\"\n"), 0o644)
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
	if err := runConfigLink(context.Background(), d, p, linkOpts{Create: true, Name: "DevApp", ConfigName: "dev"}, &buf, "json", ""); err != nil {
		t.Fatalf("runConfigLink: %v", err)
	}
	cfg, _ := p.ReadConfig("shoplazza.app.dev.toml")
	if cfg.Scopes != "read_product" {
		t.Errorf("must not clobber existing scopes; got %q want read_product", cfg.Scopes)
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

// ── link: [dashboard] ─────────────────────────────────────────────────────────

// linkDash serves /info + get-config for cid_x with the given dashboard fields.
func linkDash(t *testing.T, appFields map[string]any) *app.Dashboard {
	return dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{
					"partner": map[string]any{"id": "p1", "name": "Acme"},
					"app":     map[string]any{"client_id": "cid_x", "name": "ProdApp"}}})
		case strings.Contains(r.URL.Path, "/apps/cid_x") && r.Method == http.MethodGet:
			body := map[string]any{"client_id": "cid_x", "id": 1, "name": "ProdApp", "scopes": []string{"read"}}
			for k, v := range appFields {
				body[k] = v
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"app": body}})
		default:
			t.Fatalf("unexpected path %s %s", r.Method, r.URL.Path)
		}
	})
}

func TestRunConfigLink_WritesDashboardFields(t *testing.T) {
	root := t.TempDir()
	d := linkDash(t, map[string]any{"app_url": "https://x.dev/auth", "redirect_url": "https://x.dev/cb", "embed": false, "status": "draft"})
	p, _ := project.Open(root)
	var buf bytes.Buffer
	if err := runConfigLink(context.Background(), d, p, linkOpts{ClientID: "cid_x"}, &buf, "json", ""); err != nil {
		t.Fatalf("runConfigLink: %v", err)
	}
	cfg, err := p.ReadConfig("shoplazza.app.prodapp.toml")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	dash := cfg.Dashboard
	if dash.Name != "ProdApp" || dash.AppURL != "https://x.dev/auth" || dash.RedirectURL != "https://x.dev/cb" {
		t.Fatalf("dashboard = %+v", dash)
	}
	if dash.Embed == nil || *dash.Embed {
		t.Fatalf("embed false from the backend must be written, got %v", dash.Embed)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "shoplazza.app.prodapp.toml"))
	if !strings.Contains(string(raw), project.DashboardComment) {
		t.Fatalf("dashboard comment missing:\n%s", raw)
	}
}

// Linking the active app writes back to the active file, not a new one.
func TestRunConfigLink_WritesBackToActiveConfig(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "shoplazza.app.toml"), []byte("client_id = \"cid_x\"\npartner_id = \"p1\"\nscopes = \"read\"\n"), 0o644)
	p, _ := project.Open(root)
	_ = p.SetActiveConfig("shoplazza.app.toml", "cid_x")
	d := linkDash(t, map[string]any{"app_url": "https://x.dev/auth"})
	var buf bytes.Buffer
	if err := runConfigLink(context.Background(), d, p, linkOpts{ClientID: "cid_x"}, &buf, "json", ""); err != nil {
		t.Fatalf("runConfigLink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "shoplazza.app.prodapp.toml")); !os.IsNotExist(err) {
		t.Fatalf("must not create a second config for the active app (err=%v)", err)
	}
	cfg, _ := p.ReadConfig("shoplazza.app.toml")
	if cfg.Dashboard.AppURL != "https://x.dev/auth" {
		t.Fatalf("active config not updated: %+v", cfg)
	}
	if active, _ := p.ActiveConfigName(); active != "shoplazza.app.toml" {
		t.Fatalf("active = %q", active)
	}
}

// A different app than the active one still gets its own file (existing behavior).
func TestRunConfigLink_OtherAppGetsOwnFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "shoplazza.app.toml"), []byte("client_id = \"cid_other\"\n"), 0o644)
	p, _ := project.Open(root)
	_ = p.SetActiveConfig("shoplazza.app.toml", "cid_other")
	d := linkDash(t, nil)
	var buf bytes.Buffer
	if err := runConfigLink(context.Background(), d, p, linkOpts{ClientID: "cid_x"}, &buf, "json", ""); err != nil {
		t.Fatalf("runConfigLink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "shoplazza.app.prodapp.toml")); err != nil {
		t.Fatalf("expected shoplazza.app.prodapp.toml: %v", err)
	}
	other, _ := p.ReadConfig("shoplazza.app.toml")
	if other.ClientID != "cid_other" {
		t.Fatalf("base config must be untouched: %+v", other)
	}
}

// Empty remote fields leave the local value alone (merge, not replace).
func TestRunConfigLink_MergeKeepsLocalWhenBackendEmpty(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "shoplazza.app.prodapp.toml"),
		[]byte("client_id = \"cid_x\"\n[dashboard]\n  app_url = \"https://local.dev/auth\"\n  embed = true\n"), 0o644)
	p, _ := project.Open(root)
	// Backend: redirect_url set, app_url empty, no embed key.
	d := linkDash(t, map[string]any{"app_url": "", "redirect_url": "https://x.dev/cb"})
	var buf bytes.Buffer
	if err := runConfigLink(context.Background(), d, p, linkOpts{ClientID: "cid_x"}, &buf, "json", ""); err != nil {
		t.Fatalf("runConfigLink: %v", err)
	}
	cfg, _ := p.ReadConfig("shoplazza.app.prodapp.toml")
	if cfg.Dashboard.AppURL != "https://local.dev/auth" {
		t.Errorf("empty backend app_url must not blank the local value, got %q", cfg.Dashboard.AppURL)
	}
	if cfg.Dashboard.RedirectURL != "https://x.dev/cb" {
		t.Errorf("backend redirect_url should be written, got %q", cfg.Dashboard.RedirectURL)
	}
	if cfg.Dashboard.Embed == nil || !*cfg.Dashboard.Embed {
		t.Errorf("local embed must survive when the backend sends no embed key, got %v", cfg.Dashboard.Embed)
	}
}

// Create mode: a new app has only a name → [dashboard] gets just name.
func TestRunConfigLink_CreateWritesOnlyName(t *testing.T) {
	root := t.TempDir()
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/partners"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"partners": []map[string]any{{"id": "p1"}}}})
		case strings.HasSuffix(r.URL.Path, "/apps") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"app": map[string]any{"client_id": "cid_new", "id": 2, "name": "DevApp"}}})
		default:
			t.Fatalf("unexpected path %s %s", r.Method, r.URL.Path)
		}
	})
	p, _ := project.Open(root)
	var buf bytes.Buffer
	if err := runConfigLink(context.Background(), d, p, linkOpts{Create: true, Name: "DevApp"}, &buf, "json", ""); err != nil {
		t.Fatalf("runConfigLink: %v", err)
	}
	cfg, _ := p.ReadConfig("shoplazza.app.devapp.toml")
	if cfg.Dashboard.Name != "DevApp" || cfg.Dashboard.AppURL != "" || cfg.Dashboard.Embed != nil {
		t.Fatalf("create mode dashboard = %+v, want only name", cfg.Dashboard)
	}
}

// ── push ──────────────────────────────────────────────────────────────────────

// pushProject writes an active shoplazza.app.toml with the given body.
func pushProject(t *testing.T, toml string) *project.Project {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shoplazza.app.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	p, _ := project.Open(root)
	return p
}

// pushDash serves GET/PATCH for cid_x; patchStatus 0 echoes the body back.
func pushDash(t *testing.T, status string, patched *map[string]any, patchStatus int, patchBody string) *app.Dashboard {
	return dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/apps/cid_x") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"app": map[string]any{"client_id": "cid_x", "name": "ProdApp", "status": status}}})
		case strings.Contains(r.URL.Path, "/apps/cid_x") && r.Method == http.MethodPatch:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if patched != nil {
				*patched = body
			}
			if patchStatus != 0 {
				w.WriteHeader(patchStatus)
				_, _ = w.Write([]byte(patchBody))
				return
			}
			echo := map[string]any{"client_id": "cid_x", "name": "ProdApp", "status": status,
				"app_url": body["app_url"], "redirect_url": "https://kept.dev/cb", "embed": body["embed"]}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"app": echo}})
		default:
			t.Fatalf("unexpected path %s %s", r.Method, r.URL.Path)
		}
	})
}

const pushToml = "client_id = \"cid_x\"\npartner_id = \"p1\"\n\n[dashboard]\n  name = \"\"\n  app_url = \"https://b01.example/auth\"\n  redirect_url = \"\"\n  embed = false\n"

func TestRunConfigPush_SendsOnlyValuedFields(t *testing.T) {
	p := pushProject(t, pushToml)
	var patched map[string]any
	d := pushDash(t, "draft", &patched, 0, "")
	var buf bytes.Buffer
	if err := runConfigPush(context.Background(), d, p, false, &buf, "json", ""); err != nil {
		t.Fatalf("runConfigPush: %v", err)
	}
	// Empty strings are dropped; embed = false is a value and travels.
	if len(patched) != 2 || patched["app_url"] != "https://b01.example/auth" || patched["embed"] != false {
		t.Fatalf("PATCH body = %v, want exactly {app_url, embed:false}", patched)
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output not json: %s", buf.String())
	}
	got := out["data"].(map[string]any)["app"].(map[string]any)
	if got["redirect_url"] != "https://kept.dev/cb" || got["status"] != "draft" {
		t.Fatalf("output must echo the backend's stored app, got %v", got)
	}
}

// Deleting the embed line means "skip" — the key must not be in the PATCH.
func TestRunConfigPush_EmbedMissingNotSent(t *testing.T) {
	p := pushProject(t, "client_id = \"cid_x\"\npartner_id = \"p1\"\n[dashboard]\n  app_url = \"https://b.example/auth\"\n")
	var patched map[string]any
	d := pushDash(t, "draft", &patched, 0, "")
	var buf bytes.Buffer
	if err := runConfigPush(context.Background(), d, p, false, &buf, "json", ""); err != nil {
		t.Fatalf("runConfigPush: %v", err)
	}
	if _, has := patched["embed"]; has || len(patched) != 1 {
		t.Fatalf("PATCH body = %v, embed must be absent", patched)
	}
}

func TestRunConfigPush_EmptyDashboard_Validation(t *testing.T) {
	for _, toml := range []string{
		"client_id = \"cid_x\"\npartner_id = \"p1\"\n",
		"client_id = \"cid_x\"\npartner_id = \"p1\"\n[dashboard]\n  name = \"\"\n  app_url = \"\"\n",
	} {
		p := pushProject(t, toml)
		d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("no request expected, got %s %s", r.Method, r.URL.Path)
		})
		var buf bytes.Buffer
		err := runConfigPush(context.Background(), d, p, false, &buf, "json", "")
		var ee *output.ExitError
		if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
			t.Fatalf("toml %q: want validation error, got %v", toml, err)
		}
	}
}

func TestRunConfigPush_InvalidURL_FailsBeforeNetwork(t *testing.T) {
	// Scheme-less / host-less / non-http values the OAuth flow cannot use.
	for _, bad := range []string{"not-a-url", "localhost:3000/auth", "/auth/callback", "ftp://x.dev/auth"} {
		p := pushProject(t, "client_id = \"cid_x\"\npartner_id = \"p1\"\n[dashboard]\n  redirect_url = \""+bad+"\"\n")
		d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("no request expected for %q, got %s %s", bad, r.Method, r.URL.Path)
		})
		var buf bytes.Buffer
		err := runConfigPush(context.Background(), d, p, false, &buf, "json", "")
		var ee *output.ExitError
		if !errors.As(err, &ee) || ee.Code != output.ExitValidation || !strings.Contains(err.Error(), "redirect_url") {
			t.Errorf("%q: want validation error naming redirect_url, got %v", bad, err)
		}
	}
}

// --yes gate: only draft/rejected pass without it; blocked runs send no PATCH.
func TestRunConfigPush_StatusGate(t *testing.T) {
	cases := []struct {
		status   string
		needsYes bool
	}{
		{"draft", false}, {"rejected", false},
		{"submitted", true}, {"in-review", true}, {"published", true}, {"unpublished", true},
		{"", true}, {"in_review", true},
	}
	for _, c := range cases {
		for _, yes := range []bool{false, true} {
			p := pushProject(t, pushToml)
			var patched map[string]any
			d := pushDash(t, c.status, &patched, 0, "")
			var buf bytes.Buffer
			err := runConfigPush(context.Background(), d, p, yes, &buf, "json", "")
			wantBlock := c.needsYes && !yes
			if wantBlock {
				shown := c.status
				if shown == "" {
					shown = "unknown"
				}
				var ee *output.ExitError
				if !errors.As(err, &ee) || ee.Code != output.ExitValidation || !strings.Contains(err.Error(), shown) {
					t.Errorf("status=%q yes=%v: want gate error naming the status, got %v", c.status, yes, err)
				}
				if patched != nil {
					t.Errorf("status=%s yes=%v: PATCH must not be sent when blocked, body=%v", c.status, yes, patched)
				}
				continue
			}
			if err != nil {
				t.Errorf("status=%s yes=%v: unexpected error %v", c.status, yes, err)
			}
			if patched == nil {
				t.Errorf("status=%s yes=%v: PATCH should have been sent", c.status, yes)
			}
		}
	}
}

func TestRunConfigPush_404_HintsClientID(t *testing.T) {
	p := pushProject(t, pushToml)
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"ResourceNotFound","message":"The app does not exist or does not belong to the currently logged account"}`))
	})
	var buf bytes.Buffer
	err := runConfigPush(context.Background(), d, p, false, &buf, "json", "")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitAPI {
		t.Fatalf("want api error, got %v", err)
	}
	if ee.Detail == nil || !strings.Contains(ee.Detail.Hint, "client_id") {
		t.Fatalf("404 hint should point at client_id, got %+v", ee.Detail)
	}
	if !strings.Contains(ee.Detail.Message, "does not belong") {
		t.Fatalf("backend message should pass through, got %+v", ee.Detail)
	}
}

// 422 (duplicate name / bad URL / too long) surfaces the backend message as-is.
func TestRunConfigPush_422_PassesMessage(t *testing.T) {
	p := pushProject(t, "client_id = \"cid_x\"\npartner_id = \"p1\"\n[dashboard]\n  name = \"taken\"\n")
	d := pushDash(t, "draft", nil, http.StatusUnprocessableEntity, `{"code":"UnprocessableEntity","message":"app name is already exists"}`)
	var buf bytes.Buffer
	err := runConfigPush(context.Background(), d, p, false, &buf, "json", "")
	if err == nil || !strings.Contains(err.Error(), "app name is already exists") {
		t.Fatalf("want backend 422 message, got %v", err)
	}
}

// --yes skips the status GET entirely: only the PATCH is sent.
func TestRunConfigPush_Yes_SkipsStatusRead(t *testing.T) {
	p := pushProject(t, pushToml)
	var methods []string
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
			"data": map[string]any{"app": map[string]any{"client_id": "cid_x", "status": "published"}}})
	})
	var buf bytes.Buffer
	if err := runConfigPush(context.Background(), d, p, true, &buf, "json", ""); err != nil {
		t.Fatalf("runConfigPush --yes: %v", err)
	}
	if len(methods) != 1 || methods[0] != http.MethodPatch {
		t.Fatalf("requests = %v, want exactly one PATCH", methods)
	}
}

// 401 is auth-class with a re-login hint.
func TestRunConfigPush_401_IsAuthClass(t *testing.T) {
	p := pushProject(t, pushToml)
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"Unauthorized","message":"token expired"}`))
	})
	var buf bytes.Buffer
	err := runConfigPush(context.Background(), d, p, false, &buf, "json", "")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitAuth || ee.Detail == nil || ee.Detail.Type != output.TypeAuth {
		t.Fatalf("want auth-class error, got %+v", err)
	}
	if !strings.Contains(ee.Detail.Hint, "auth login") {
		t.Fatalf("401 hint should point at re-login, got %+v", ee.Detail)
	}
}

func TestRunConfigPush_NoClientID_Validation(t *testing.T) {
	p := pushProject(t, "[dashboard]\n  app_url = \"https://b.example/auth\"\n")
	var buf bytes.Buffer
	err := runConfigPush(context.Background(), nil, p, false, &buf, "json", "")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("want validation error, got %v", err)
	}
}

func TestNewCmdConfig_HasPush(t *testing.T) {
	cmd := newCmdConfig(&cmdutil.Factory{})
	var push *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "push" {
			push = c
		}
	}
	if push == nil {
		t.Fatal("missing 'app config push'")
	}
	for _, name := range []string{"path", "yes"} {
		if push.Flags().Lookup(name) == nil {
			t.Errorf("push missing flag --%s", name)
		}
	}
	if push.Flags().Lookup("config") != nil {
		t.Error("push must not take --config (it acts on the active config only)")
	}
	if !strings.Contains(push.Long, "does NOT clear") {
		t.Error("push --help must spell out the patch semantics")
	}
}

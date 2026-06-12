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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/app"
	"shoplazza-cli-v2/internal/app/project"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
)

func makeTemplateRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"tmpl"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Mirror the real app template: a v1-format toml with default scopes.
	if err := os.WriteFile(filepath.Join(dir, "shoplazza.app.toml"),
		[]byte("client_id= \"\"\nscopes = \"read_customer write_cart_transform\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestRunInit_LinkExisting(t *testing.T) {
	tmpl := makeTemplateRepo(t)
	dest := filepath.Join(t.TempDir(), "myproj")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/partners"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"partners": []map[string]any{{"id": "p1"}}}})
		case strings.HasSuffix(r.URL.Path, "/info"):
			// Link path derives partner from client_id via /info.
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{
					"partner": map[string]any{"id": "p1", "name": "Acme"},
					"app":     map[string]any{"client_id": "cid_x", "name": "MyApp", "scopes": []string{"read"}}}})
		case strings.HasSuffix(r.URL.Path, "/template"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"template_type": "app", "https": tmpl}})
		case strings.Contains(r.URL.Path, "/apps/cid_x"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"app": map[string]any{"client_id": "cid_x", "id": 3, "name": "MyApp", "scopes": []string{"read"}}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	p, _ := project.Open(dest)
	var buf bytes.Buffer
	if err := runInit(context.Background(), d, p, initOpts{ClientID: "cid_x"}, &buf, io.Discard, "json", ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	// v1 subdir mode: files land in <path>/<slug(app name)>; "MyApp" → "myapp".
	sub := filepath.Join(dest, "myapp")
	// cloned file present, .git removed
	if _, err := os.Stat(filepath.Join(sub, "package.json")); err != nil {
		t.Fatalf("template not cloned into subdir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git should be removed, err=%v", err)
	}
	// config injected + active (in the subdir project)
	subP, _ := project.Open(sub)
	cfg, err := subP.ReadConfig("shoplazza.app.toml")
	if err != nil || cfg.ClientID != "cid_x" {
		t.Fatalf("config = %+v, %v", cfg, err)
	}
	if cfg.PartnerID != "p1" {
		t.Fatalf("partner_id not persisted into config: got %q, want p1", cfg.PartnerID)
	}
	// The printed result echoes the resolved partner_id (no extra API call — it's
	// the partner /info already derived during link).
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal init output: %v", err)
	}
	if out["partner_id"] != "p1" {
		t.Fatalf("init output partner_id = %v, want p1", out["partner_id"])
	}
	// The mock app has configured scopes ("read") — real data overrides the
	// template default.
	if cfg.Scopes != "read" {
		t.Fatalf("scopes = %q, want API-configured \"read\"", cfg.Scopes)
	}
	name, _ := subP.ActiveConfigName()
	if name != "shoplazza.app.toml" {
		t.Fatalf("active = %q", name)
	}
	// .gitignore has .shoplazza/
	gi, _ := os.ReadFile(filepath.Join(sub, ".gitignore"))
	if !strings.Contains(string(gi), ".shoplazza/") {
		t.Fatalf(".gitignore missing .shoplazza/: %q", gi)
	}
}

// initDashServer returns a Dashboard whose mock serves partners + the cid_x app
// (name "MyApp") + the app template. Shared by the v1-subdir-mode init tests.
func initDashServer(t *testing.T, tmpl string) *app.Dashboard {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/partners"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"partners": []map[string]any{{"id": "p1"}}}})
		case strings.HasSuffix(r.URL.Path, "/info"):
			// Link path derives partner from client_id via /info.
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{
					"partner": map[string]any{"id": "p1", "name": "Acme"},
					"app":     map[string]any{"client_id": "cid_x", "name": "MyApp", "scopes": []string{"read"}}}})
		case strings.HasSuffix(r.URL.Path, "/template"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"template_type": "app", "https": tmpl}})
		case strings.Contains(r.URL.Path, "/apps/cid_x"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"app": map[string]any{"client_id": "cid_x", "id": 3, "name": "MyApp", "scopes": []string{"read"}}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return app.NewDashboard(client.New(srv.URL), "ptok")
}

// TestRunInit_NonEmptyParent_OK locks the v1-subdir-mode contract: init creates a
// sub-dir named after the app under --path, so a NON-EMPTY parent is fine and its
// existing files are left untouched.
func TestRunInit_NonEmptyParent_OK(t *testing.T) {
	tmpl := makeTemplateRepo(t) // skips if no git
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := initDashServer(t, tmpl)
	p, _ := project.Open(dest)
	var buf bytes.Buffer
	if err := runInit(context.Background(), d, p, initOpts{ClientID: "cid_x"}, &buf, io.Discard, "json", ""); err != nil {
		t.Fatalf("runInit into a non-empty parent should succeed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "myapp", "package.json")); err != nil {
		t.Fatalf("subdir 'myapp' not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "existing.txt")); err != nil {
		t.Fatalf("pre-existing parent file was disturbed: %v", err)
	}
}

// TestRunInit_EmptyAPIScopes_KeepsTemplateDefaults: when the app has no scopes
// configured on the dashboard (every fresh app), init must NOT blank out the
// template's default scopes — the template toml is the onboarding baseline and
// the first `app dev` authorization depends on it.
func TestRunInit_EmptyAPIScopes_KeepsTemplateDefaults(t *testing.T) {
	tmpl := makeTemplateRepo(t)
	dest := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{
					"partner": map[string]any{"id": "p1", "name": "Acme"},
					"app":     map[string]any{"client_id": "cid_fresh", "name": "FreshApp", "scopes": []string{}}}})
		case strings.HasSuffix(r.URL.Path, "/template"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"template_type": "app", "https": tmpl}})
		case strings.Contains(r.URL.Path, "/apps/cid_fresh"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"app": map[string]any{"client_id": "cid_fresh", "id": 9, "name": "FreshApp", "scopes": []string{}}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	p, _ := project.Open(dest)
	var buf bytes.Buffer
	if err := runInit(context.Background(), d, p, initOpts{ClientID: "cid_fresh"}, &buf, io.Discard, "json", ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	subP, _ := project.Open(filepath.Join(dest, "freshapp"))
	cfg, err := subP.ReadConfig("shoplazza.app.toml")
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if cfg.ClientID != "cid_fresh" || cfg.PartnerID != "p1" {
		t.Fatalf("identity fields not injected: %+v", cfg)
	}
	if cfg.Scopes != "read_customer write_cart_transform" {
		t.Fatalf("scopes = %q, want template defaults preserved", cfg.Scopes)
	}
}

// TestEnsureGitignore_AlreadyPresent locks the idempotency contract: calling
// ensureGitignore twice with the same entry must not duplicate the line.
func TestEnsureGitignore_AlreadyPresent(t *testing.T) {
	root := t.TempDir()
	// Write a .gitignore that already contains the entry.
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".shoplazza/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitignore(root, ".shoplazza/"); err != nil {
		t.Fatalf("ensureGitignore: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	count := strings.Count(string(data), ".shoplazza/")
	if count != 1 {
		t.Errorf(".shoplazza/ appears %d times, want 1", count)
	}
}

// TestNewCmdInit_PartnerNotRequired: --partner is not a required flag (create
// mode auto-selects a single partner; link mode derives the partner from the
// app).
func TestNewCmdInit_PartnerNotRequired(t *testing.T) {
	cmd := newCmdInit(&cmdutil.Factory{})
	flag := cmd.Flags().Lookup("partner")
	if flag == nil {
		t.Fatal("--partner flag missing")
	}
	if _, required := flag.Annotations[cobra.BashCompOneRequiredFlag]; required {
		t.Fatal("--partner must not be marked required")
	}
}

// TestRunInit_LinkMode_PartnerFlagWarns: in link mode the partner comes from
// the app's /info, so a --partner value is ignored — with a visible one-line
// warning rather than silently.
func TestRunInit_LinkMode_PartnerFlagWarns(t *testing.T) {
	tmpl := makeTemplateRepo(t)
	dest := t.TempDir()
	d := initDashServer(t, tmpl)
	p, _ := project.Open(dest)
	var out, errOut bytes.Buffer
	if err := runInit(context.Background(), d, p, initOpts{ClientID: "cid_x", Partner: "p999"}, &out, &errOut, "json", ""); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if !strings.Contains(errOut.String(), "--partner is ignored") {
		t.Fatalf("expected a --partner-ignored warning on stderr, got %q", errOut.String())
	}
	// The persisted partner must still be the one derived from /info.
	subP, _ := project.Open(filepath.Join(dest, "myapp"))
	cfg, err := subP.ReadConfig("shoplazza.app.toml")
	if err != nil || cfg.PartnerID != "p1" {
		t.Fatalf("partner_id = %q (%v), want p1 from /info", cfg.PartnerID, err)
	}
}

// TestRunInit_CreateMode_MultiplePartnersNoFlag_Validation: with --partner gone
// as a hard requirement, create mode against an account with several partners
// still fails actionably via selectPartner.
func TestRunInit_CreateMode_MultiplePartnersNoFlag_Validation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/partners") {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"partners": []map[string]any{{"id": "p1"}, {"id": "p2"}}}})
			return
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
	}))
	defer srv.Close()

	d := app.NewDashboard(client.New(srv.URL), "ptok")
	p, _ := project.Open(t.TempDir())
	var buf bytes.Buffer
	err := runInit(context.Background(), d, p, initOpts{Create: true, Name: "NewApp"}, &buf, io.Discard, "json", "")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("expected a validation error from selectPartner, got %v", err)
	}
}

// TestCloneTemplate_FailureNamesCause: the error carries the exec error (exit
// status) alongside git's own output.
func TestCloneTemplate_FailureNamesCause(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
	dest := filepath.Join(t.TempDir(), "clone-target")
	err := cloneTemplate(context.Background(), filepath.Join(t.TempDir(), "no-such-repo"), dest)
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if ee.Code != output.ExitInternal {
		t.Fatalf("exit code = %d, want ExitInternal (%d)", ee.Code, output.ExitInternal)
	}
	msg := ee.Error()
	if !strings.Contains(msg, "git clone failed:") || !strings.Contains(msg, "exit status") {
		t.Fatalf("message should carry the exec error, got %q", msg)
	}
}

// TestNewCmdInit_MutuallyExclusiveFlags verifies cobra's flag-group validation:
// --name (create) and --client-id (link) cannot be combined. The validation runs
// in Execute (ValidateFlagGroups), before PreRunE, so no auth is reached.
func TestNewCmdInit_MutuallyExclusiveFlags(t *testing.T) {
	cmd := newCmdInit(&cmdutil.Factory{})
	cmd.SetArgs([]string{"--name", "myapp", "--client-id", "cid123"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when both --name and --client-id are set")
	}
}

// TestNewCmdInit_OneModeRequired verifies cobra requires at least one of
// --name / --client-id (MarkFlagsOneRequired).
func TestNewCmdInit_OneModeRequired(t *testing.T) {
	cmd := newCmdInit(&cmdutil.Factory{})
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when neither --name nor --client-id is set")
	}
}

// TestRunInit_TargetExists_Validation verifies the only hard precondition now: the
// target sub-dir (slug of the app name) must not already exist.
func TestRunInit_TargetExists_Validation(t *testing.T) {
	dest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, "myapp"), 0o755); err != nil { // slug("MyApp")
		t.Fatal(err)
	}
	d := initDashServer(t, "unused")
	p, _ := project.Open(dest)
	var buf bytes.Buffer
	err := runInit(context.Background(), d, p, initOpts{ClientID: "cid_x"}, &buf, io.Discard, "json", "")
	if err == nil {
		t.Fatal("expected validation error when the target sub-dir already exists")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if ee.Code != output.ExitValidation {
		t.Fatalf("expected validation exit code %d, got %d", output.ExitValidation, ee.Code)
	}
	if !strings.Contains(ee.Error(), "already exists") {
		t.Fatalf("expected 'already exists', got %q", ee.Error())
	}
}

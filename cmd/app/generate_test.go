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
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/app"
	"github.com/Shoplazza/shoplazza-cli/internal/app/project"
	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// makeThemeTemplateRepo builds a real local git repo holding the theme template
// layout, committed (git accepts a local path as a clone URL).
func makeThemeTemplateRepo(t *testing.T) string {
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
	write := func(rel, body string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init")
	write("package.json", `{"name":"tmpl","version":"1.0.0"}`)
	write("shoplazza.extension.toml", "name = \"tmpl\"\ntype = \"theme\"\n")
	write("blocks/index-basic.liquid", "basic {{projectName}}\n")
	write("blocks/index-embed.liquid", "embed {{projectName}}\n")
	write("snippets/index.liquid", "snippet\n")
	write("snippets/index_css.liquid", "css\n")
	write("assets/index.css", ".x{}\n")
	write("locales/en-US.json", `{"label":"{{type}}"}`)
	write("locales/zh-CN.json", `{"label":"{{type}}"}`)
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func themeDashboard(t *testing.T, tmplRepo string) *app.Dashboard {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
			"data": map[string]any{"template_type": "ext_thm", "https": tmplRepo}})
	}))
	t.Cleanup(srv.Close)
	return app.NewDashboard(client.New(srv.URL), "ptok")
}

func TestRunGenerateExtension_Theme(t *testing.T) {
	tmpl := makeThemeTemplateRepo(t)
	d := themeDashboard(t, tmpl)
	root := t.TempDir()

	var buf bytes.Buffer
	if err := runGenerateExtension(context.Background(), d, root, "theme", "mytheme", "embed", &buf, io.Discard, "json", ""); err != nil {
		t.Fatalf("runGenerateExtension: %v", err)
	}
	extDir := filepath.Join(root, project.ExtensionsDir, "mytheme")
	if _, err := os.Stat(filepath.Join(extDir, "blocks", "mytheme.liquid")); err != nil {
		t.Fatalf("scaffolded block missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(extDir, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git should not exist, err=%v", err)
	}
}

// ── templateTypeFor ───────────────────────────────────────────────────────────

func TestTemplateTypeFor(t *testing.T) {
	cases := []struct {
		extType string
		want    string
		ok      bool
	}{
		{"theme", "ext_thm", true},
		{"checkout", "ext_co", true},
		{"function", "ext_func", true},
		{"unknown", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := templateTypeFor(c.extType)
		if got != c.want || ok != c.ok {
			t.Errorf("templateTypeFor(%q) = (%q, %v), want (%q, %v)", c.extType, got, ok, c.want, c.ok)
		}
	}
}

// ── runGenerateExtension validation ──────────────────────────────────────────

func TestRunGenerateExtension_EmptyNameErrors(t *testing.T) {
	err := runGenerateExtension(context.Background(), nil, t.TempDir(), "theme", "", "basic", nil, io.Discard, "json", "")
	if err == nil {
		t.Error("expected error when --name is empty")
	}
}

// TestRunGenerateExtension_InvalidNameRejected: the extension name becomes a
// directory under extensions/ and a remote identifier, so anything outside the
// conservative slug pattern — path traversal in particular — is REJECTED with a
// validation error (never silently sanitized).
func TestRunGenerateExtension_InvalidNameRejected(t *testing.T) {
	for _, name := range []string{
		"../../x", "a/b", "..", "My Theme", "UPPER", ".hidden", "-leading", "name!",
	} {
		err := runGenerateExtension(context.Background(), nil, t.TempDir(), "theme", name, "basic", nil, io.Discard, "json", "")
		var ee *output.ExitError
		if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
			t.Errorf("name %q: expected validation error, got %v", name, err)
		}
	}
}

func TestRunGenerateExtension_ValidNamePassesValidation(t *testing.T) {
	tmpl := makeThemeTemplateRepo(t)
	d := themeDashboard(t, tmpl)
	for _, name := range []string{"my-theme_2", "0kay"} {
		var buf bytes.Buffer
		if err := runGenerateExtension(context.Background(), d, t.TempDir(), "theme", name, "embed", &buf, io.Discard, "json", ""); err != nil {
			t.Errorf("name %q should be accepted: %v", name, err)
		}
	}
}

func TestRunGenerateExtension_InvalidTypeErrors(t *testing.T) {
	err := runGenerateExtension(context.Background(), nil, t.TempDir(), "widget", "myext", "", nil, io.Discard, "json", "")
	if err == nil {
		t.Error("expected error for invalid extension type")
	}
}

func TestRunGenerateExtension_ThemeRequiresThemeType(t *testing.T) {
	err := runGenerateExtension(context.Background(), nil, t.TempDir(), "theme", "myext", "invalid", nil, io.Discard, "json", "")
	if err == nil {
		t.Error("expected error when --theme-type is invalid for theme extension")
	}
}

func TestRunGenerateExtension_DirExists_Validation(t *testing.T) {
	tmpl := makeThemeTemplateRepo(t)
	d := themeDashboard(t, tmpl)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, project.ExtensionsDir, "mytheme"), 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runGenerateExtension(context.Background(), d, root, "theme", "mytheme", "embed", &buf, io.Discard, "json", "")
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %v", err)
	}
	if ee.Code != output.ExitValidation {
		t.Fatalf("expected validation exit code %d, got %d", output.ExitValidation, ee.Code)
	}
}

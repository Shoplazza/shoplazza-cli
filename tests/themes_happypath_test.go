// Binary-level happy-path tests for the themes push/pull workflows: the real
// compiled CLI runs the full business chain (pack → upload → poll / download →
// unpack) against a mock server, and the tests assert the business outcome.
package tests_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

type fixtureFile struct {
	path    string
	content string
}

// writeThemeDir creates a minimal valid theme directory (readThemeInfo needs
// config/settings_schema.json with a theme_info block).
func writeThemeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := []fixtureFile{
		{"config/settings_schema.json", `[{"name":"theme_info","theme_name":"e2e-theme","theme_version":"1.0.0"}]`},
		{"layout/theme.liquid", "<html></html>"},
	}
	for _, f := range files {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// buildZip returns zip bytes holding the given entries, in order.
func buildZip(t *testing.T, files []fixtureFile) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// unwrapStrict asserts stdout is exactly the {ok:true,data:{...}} success
// envelope and returns data (unwrapAPISuccess would silently fall back to the
// raw map, letting an envelope regression pass).
func unwrapStrict(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if ok, _ := raw["ok"].(bool); !ok {
		t.Fatalf("stdout missing ok=true envelope: %s", stdout)
	}
	data, isMap := raw["data"].(map[string]any)
	if !isMap {
		t.Fatalf("envelope missing data object: %s", stdout)
	}
	return data
}

// zipEntryNames lists entry names of a zip archive.
func zipEntryNames(data []byte) ([]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names, nil
}

func hasSuffixEntry(names []string, suffix string) bool {
	for _, n := range names {
		if strings.HasSuffix(n, suffix) {
			return true
		}
	}
	return false
}

func TestThemesPushHappyPath(t *testing.T) {
	testenv.IsolateConfigDir(t)

	var (
		mu         sync.Mutex
		zipEntries []string
		uploadErr  string
		taskPolls  int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/themes/upload"):
			// The uploaded multipart "file" part must be a real zip carrying
			// the packed theme files.
			mu.Lock()
			file, _, err := r.FormFile("file")
			if err != nil {
				uploadErr = "missing multipart file part: " + err.Error()
			} else {
				data, rerr := io.ReadAll(file)
				file.Close()
				if rerr != nil {
					uploadErr = "read file part: " + rerr.Error()
				} else if names, zerr := zipEntryNames(data); zerr != nil {
					uploadErr = "file part is not a valid zip: " + zerr.Error()
				} else {
					zipEntries = names
				}
			}
			mu.Unlock()
			// Real upload shape: task id double-nested at task.task.id.
			_, _ = w.Write([]byte(`{"task":{"task":{"id":"task-xyz","status":"0"}}}`))
		case strings.Contains(r.URL.Path, "/themes/task/"):
			mu.Lock()
			taskPolls++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"task":{"status":1}}`))
		default:
			// PlanDetail existence check.
			_, _ = w.Write([]byte(`{"name":"Nova","id":"abc123"}`))
		}
	}))
	defer srv.Close()

	bin := sharedBinary(t)
	themeDir := writeThemeDir(t)

	stdout, stderr, code := runCLIDir(t, bin, themeDir, contractEnv(srv.URL),
		"themes", "push", "--theme-id", "abc123")
	if code != 0 {
		t.Fatalf("push exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	data := unwrapStrict(t, stdout)
	if data["theme_id"] != "abc123" {
		t.Errorf("theme_id = %v, want abc123", data["theme_id"])
	}
	if _, ok := data["task"].(map[string]any); !ok {
		t.Errorf("success body missing task object: %v", data)
	}

	mu.Lock()
	defer mu.Unlock()
	if uploadErr != "" {
		t.Error(uploadErr)
	}
	if zipEntries == nil {
		t.Fatal("upload endpoint never received a valid zip")
	}
	// The pack step must ship the actual theme files, not just any zip.
	for _, want := range []string{"config/settings_schema.json", "layout/theme.liquid"} {
		if !hasSuffixEntry(zipEntries, want) {
			t.Errorf("uploaded zip missing %s; entries: %v", want, zipEntries)
		}
	}
	if taskPolls < 1 {
		t.Error("task endpoint was never polled")
	}
}

func TestThemesPullHappyPath(t *testing.T) {
	testenv.IsolateConfigDir(t)

	zipBytes := buildZip(t, []fixtureFile{
		{"Nova-1.0/assets/main.css", "css-content"},
		{"Nova-1.0/layout/theme.liquid", "<html/>"},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBytes)
			return
		}
		// PlanDetail name lookup (best-effort).
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"Nova","id":"abc123"}`))
	}))
	defer srv.Close()

	bin := sharedBinary(t)
	workDir := t.TempDir()

	stdout, stderr, code := runCLIDir(t, bin, workDir, contractEnv(srv.URL),
		"themes", "pull", "--theme-id", "abc123")
	if code != 0 {
		t.Fatalf("pull exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	data := unwrapStrict(t, stdout)
	if data["theme_id"] != "abc123" {
		t.Errorf("theme_id = %v, want abc123", data["theme_id"])
	}
	if data["theme_name"] != "Nova" {
		t.Errorf("theme_name = %v, want Nova", data["theme_name"])
	}
	if data["target"] != "./" {
		t.Errorf("target = %v, want ./", data["target"])
	}

	// StripTopDir: files land in cwd without the Nova-1.0/ wrapper.
	css, err := os.ReadFile(filepath.Join(workDir, "assets", "main.css"))
	if err != nil {
		t.Fatalf("assets/main.css not unpacked: %v", err)
	}
	if string(css) != "css-content" {
		t.Errorf("main.css content = %q, want css-content", css)
	}
	if _, err := os.Stat(filepath.Join(workDir, "layout", "theme.liquid")); err != nil {
		t.Errorf("layout/theme.liquid not unpacked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "Nova-1.0")); !os.IsNotExist(err) {
		t.Error("top-level dir Nova-1.0 was not stripped")
	}
}

package themes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

	"github.com/spf13/cobra"
)

// shareFlags builds an empty FlagSet for the share shortcut, which has no
// flags of its own — it always uploads the cwd as a fresh temporary theme.
func shareFlags() common.FlagSet {
	return common.NewCobraFlagSet(&cobra.Command{Use: "share"})
}

// TestShare_DryRunEmitsBothV1Plans: dry-run must emit exactly 2 planned
// requests, both on the v1 /openapi/2020-07/ tree, with theme_id="" in the
// upload query (share always uploads as a fresh temporary theme).
func TestShare_DryRunEmitsBothV1Plans(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	res, err := shareShortcut.Execute(context.Background(), common.ExecInput{
		DryRun: true,
		Flags:  shareFlags(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Plans) != 2 {
		t.Fatalf("expected 2 plans (PlanShareShop + PlanShareUpload), got %d", len(res.Plans))
	}
	p1 := res.Plans[0]
	p2 := res.Plans[1]
	if !strings.Contains(p1.Path, "/openapi/2020-07/shop") {
		t.Errorf("PlanShareShop must hit v1 path; got %s", p1.Path)
	}
	if !strings.Contains(p2.Path, "/openapi/2020-07/themes/upload") {
		t.Errorf("PlanShareUpload must hit v1 path; got %s", p2.Path)
	}
	if v := p2.Query["theme_id"]; v != "" {
		t.Errorf("theme_id must be empty (fresh temporary theme); got %v", v)
	}
}

// TestShare_NoPlannedRequestHasShareEndpoint: locked contract — share never
// emits a `/share` URL (no such endpoint exists); the upload rides
// /themes/upload, not the tempting `/themes/{id}/share` shape.
func TestShare_NoPlannedRequestHasShareEndpoint(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	res, err := shareShortcut.Execute(context.Background(), common.ExecInput{
		DryRun: true,
		Flags:  shareFlags(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Plans {
		if strings.Contains(p.Path, "/share") {
			t.Fatalf("share MUST NOT emit a /share endpoint; got: %s", p.Path)
		}
	}
}

// TestShare_LiveModePrintsPreviewURL: end-to-end with httptest. Mocks the
// shop endpoint returning data.shop.domain and the upload endpoint
// returning data.theme_id. Asserts the resulting preview_url stitches both
// together correctly.
func TestShare_LiveModePrintsPreviewURL(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/openapi/2020-07/shop"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"shop": map[string]any{"domain": "demo.myshoplaza.com"},
				},
			})
		case strings.Contains(r.URL.Path, "/openapi/2020-07/themes/upload"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"theme_id": "tmp-xyz"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := shareShortcut.Execute(context.Background(), common.ExecInput{
		Client: client.New(srv.URL),
		Flags:  shareFlags(),
	})
	if err != nil {
		t.Fatalf("live mode err: %v", err)
	}
	url, _ := res.Body["preview_url"].(string)
	if !strings.Contains(url, "demo.myshoplaza.com") || !strings.Contains(url, "preview_theme_id=tmp-xyz") {
		t.Errorf("preview_url: %q", url)
	}
	if got := res.Body["theme_id"]; got != "tmp-xyz" {
		t.Errorf("theme_id in body = %v, want tmp-xyz", got)
	}
	if got := res.Body["store_domain"]; got != "demo.myshoplaza.com" {
		t.Errorf("store_domain in body = %v, want demo.myshoplaza.com", got)
	}
}

// TestShare_AsyncTaskResolvesThemeID: when the upload endpoint returns only
// a task_id (async, no synchronous theme_id), share must poll the task and
// read theme_id out of the task's info JSON (v1 parity). The resulting
// preview URL must carry that resolved id, not an empty one.
func TestShare_AsyncTaskResolvesThemeID(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	// Real wire shapes: /shop and /task come back with ok:true (so the client
	// unwraps the data envelope → resp.shop / resp.task); the upload nests the
	// id at task.task.id; theme_id lives in the completed task's info JSON.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/openapi/2020-07/shop"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"data": map[string]any{"shop": map[string]any{"domain": "demo.test"}},
			})
		case strings.Contains(r.URL.Path, "/themes/task/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{"task": map[string]any{
					"status":  1,
					"message": "success",
					"info":    `{"name":"X","theme_id":"async-id"}`,
				}},
			})
		case strings.Contains(r.URL.Path, "/themes/upload"):
			// Async upload: only a (double-nested) task id, no theme_id.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"task": map[string]any{"task": map[string]any{"id": "task-1", "status": "0"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := shareShortcut.Execute(context.Background(), common.ExecInput{
		Client: client.New(srv.URL),
		Flags:  shareFlags(),
	})
	if err != nil {
		t.Fatalf("async share err: %v", err)
	}
	if got := res.Body["theme_id"]; got != "async-id" {
		t.Errorf("theme_id = %v, want async-id (resolved from task info)", got)
	}
	if got := res.Body["store_domain"]; got != "demo.test" {
		t.Errorf("store_domain = %v, want demo.test (unwrapped shop shape)", got)
	}
	if url, _ := res.Body["preview_url"].(string); !strings.Contains(url, "preview_theme_id=async-id") {
		t.Errorf("preview_url = %q, want preview_theme_id=async-id", url)
	}
}

// TestShare_NoTaskPolling: the upload endpoint returns share-ready data
// immediately — share MUST NOT make any /task/ calls. Counter any task
// hit; the test fails if so.
func TestShare_NoTaskPolling(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	var sawTaskCall atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/task/") {
			sawTaskCall.Store(true)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/openapi/2020-07/shop"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"shop": map[string]any{"domain": "demo.test"},
				},
			})
		case strings.Contains(r.URL.Path, "/openapi/2020-07/themes/upload"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"theme_id": "tx"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, err := shareShortcut.Execute(context.Background(), common.ExecInput{
		Client: client.New(srv.URL),
		Flags:  shareFlags(),
	})
	if err != nil {
		t.Fatalf("share err: %v", err)
	}
	if sawTaskCall.Load() {
		t.Fatal("share MUST NOT trigger task polling")
	}
}

// TestShare_EmptyResolvedThemeIDErrors: a server that neither echoes a
// theme_id nor runs an async task previously "succeeded" with theme_id:"" and
// a broken ?preview_theme_id= URL. It must now error instead of reporting a
// hollow success.
func TestShare_EmptyResolvedThemeIDErrors(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/openapi/2020-07/shop"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"shop": map[string]any{"domain": "demo.test"}},
			})
		case strings.Contains(r.URL.Path, "/openapi/2020-07/themes/upload"):
			// Neither theme_id nor task_id — contract violation.
			_, _ = w.Write([]byte(`{"data":{}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, err := shareShortcut.Execute(context.Background(), common.ExecInput{
		Client: client.New(srv.URL),
		Flags:  shareFlags(),
	})
	if err == nil {
		t.Fatal("expected error when no theme id can be resolved (broken preview URL otherwise)")
	}
	if !strings.Contains(err.Error(), "theme id") {
		t.Errorf("error should explain the missing theme id: %v", err)
	}
}

// ── asMap ────────────────────────────────────────────────────────────────────

func TestAsMap_NonMap(t *testing.T) {
	if got := asMap("not a map"); got != nil {
		t.Errorf("asMap(string) must return nil, got %v", got)
	}
	if got := asMap(nil); got != nil {
		t.Errorf("asMap(nil) must return nil")
	}
}

// ── extractStoreDomain ───────────────────────────────────────────────────────

func TestExtractStoreDomain_StoreDomainKey(t *testing.T) {
	resp := map[string]any{"store_domain": "mystore.com"}
	if got := extractStoreDomain(resp); got != "mystore.com" {
		t.Errorf("expected mystore.com, got %q", got)
	}
}

func TestExtractStoreDomain_WrappedDomain(t *testing.T) {
	resp := map[string]any{"data": map[string]any{"domain": "wrapped.com"}}
	if got := extractStoreDomain(resp); got != "wrapped.com" {
		t.Errorf("expected wrapped.com, got %q", got)
	}
}

func TestExtractStoreDomain_Empty(t *testing.T) {
	if got := extractStoreDomain(map[string]any{}); got != "" {
		t.Errorf("expected empty for no domain, got %q", got)
	}
}

// ── themeIDFromTask ───────────────────────────────────────────────────────────

func TestThemeIDFromTask_FromInfoJSON(t *testing.T) {
	info := `{"theme_id":"info-tid-99"}`
	task := map[string]any{"info": info}
	if got := themeIDFromTask(task); got != "info-tid-99" {
		t.Errorf("expected info-tid-99 from info JSON, got %q", got)
	}
}

func TestThemeIDFromTask_Empty(t *testing.T) {
	if got := themeIDFromTask(map[string]any{}); got != "" {
		t.Errorf("expected empty for missing theme_id, got %q", got)
	}
}

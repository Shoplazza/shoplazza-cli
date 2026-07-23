package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

func TestUpsertCheckout_Create(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"extension":{"extension_id":"e1","id":"v1","name":"X"}},"status":"ok"}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	extID, versionID, err := upsertCheckout(context.Background(), c, map[string]any{"resource_url": "u", "version": "1.0.0"}, "")
	if err != nil {
		t.Fatalf("upsertCheckout: %v", err)
	}
	if extID != "e1" || versionID != "v1" {
		t.Fatalf("extID=%q versionID=%q", extID, versionID)
	}
	if !strings.HasSuffix(gotPath, "/openapi/checkout_extensions/create") {
		t.Fatalf("path = %s (want create)", gotPath)
	}
}

func TestUpsertCheckout_EmptyBody_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	extID, versionID, err := upsertCheckout(context.Background(), c, map[string]any{"resource_url": "u", "version": "1.0.0"}, "")
	if err == nil {
		t.Fatalf("upsertCheckout: expected error for empty body, got extID=%q versionID=%q", extID, versionID)
	}
	if extID != "" || versionID != "" {
		t.Fatalf("expected empty ids on error, got extID=%q versionID=%q", extID, versionID)
	}
	if err.Code != output.ExitInternal {
		t.Fatalf("expected ExitInternal (%d), got code=%d message=%q", output.ExitInternal, err.Code, err.Error())
	}
}

func TestUpsertCheckout_Commit(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"extension":{"extension_id":"e9","id":"v2"}}}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	extID, versionID, err := upsertCheckout(context.Background(), c, map[string]any{"resource_url": "u"}, "e9")
	if err != nil {
		t.Fatalf("upsertCheckout: %v", err)
	}
	if extID != "e9" || versionID != "v2" {
		t.Fatalf("extID=%q versionID=%q", extID, versionID)
	}
	if !strings.HasSuffix(gotPath, "/openapi/checkout_extensions/commit") {
		t.Fatalf("path = %s (want commit)", gotPath)
	}
	// commit body must carry the existing extension_id inside "extension"
	ext, _ := gotBody["extension"].(map[string]any)
	if ext == nil || ext["extension_id"] != "e9" {
		t.Fatalf("commit body missing extension_id: %v", gotBody)
	}
}

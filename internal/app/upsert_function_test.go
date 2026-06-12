package app

import (
	"context"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/client"
)

// parseFunctionForm parses the multipart body of a functions/create|commit
// request into string fields and file parts (by form name).
func parseFunctionForm(t *testing.T, r *http.Request) (fields map[string]string, files map[string][]byte) {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse request Content-Type: %v", err)
	}
	mr := multipart.NewReader(r.Body, params["boundary"])
	fields = map[string]string{}
	files = map[string][]byte{}
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		buf := make([]byte, 0)
		tmp := make([]byte, 512)
		for {
			n, rerr := part.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if rerr != nil {
				break
			}
		}
		if part.FileName() == "" {
			fields[part.FormName()] = string(buf)
		} else {
			files[part.FormName()] = buf
		}
	}
	return fields, files
}

// writeFuncFixtures writes a known index.js + .wasm into a temp dir and returns
// their paths plus the index.js content.
func writeFuncFixtures(t *testing.T) (jsPath, wasmPath, jsContent string) {
	t.Helper()
	dir := t.TempDir()
	jsContent = "export default function(input){ return input; }\n"
	jsPath = filepath.Join(dir, "index.js")
	if err := os.WriteFile(jsPath, []byte(jsContent), 0o644); err != nil {
		t.Fatal(err)
	}
	wasmPath = filepath.Join(dir, "function.wasm")
	if err := os.WriteFile(wasmPath, []byte("\x00asm\x01\x00\x00\x00WASMBYTES"), 0o644); err != nil {
		t.Fatal(err)
	}
	return jsPath, wasmPath, jsContent
}

func newFunctionPartner(t *testing.T, srvURL string) *client.Client {
	t.Helper()
	c := client.New(srvURL)
	c.Headers["app-client-id"] = "cid"
	c.SetBearerToken("apptok")
	return c
}

func TestUpsertFunction_CreatePath(t *testing.T) {
	jsPath, wasmPath, jsContent := writeFuncFixtures(t)
	wasmBytes, _ := os.ReadFile(wasmPath)

	var sawClientID, sawToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/openapi/2025-06/functions/create") {
			t.Errorf("path = %q, want .../functions/create", r.URL.Path)
		}
		sawClientID = r.Header.Get("app-client-id")
		sawToken = r.Header.Get("Access-Token")
		fields, files := parseFunctionForm(t, r)
		if fields["name"] != "fn" {
			t.Errorf("name = %q, want fn", fields["name"])
		}
		if fields["namespace"] != "cart_transform" {
			t.Errorf("namespace = %q, want cart_transform", fields["namespace"])
		}
		if fields["source_code"] != jsContent {
			t.Errorf("source_code = %q, want index.js content %q", fields["source_code"], jsContent)
		}
		if string(files["file"]) != string(wasmBytes) {
			t.Errorf("file part bytes mismatch: got %d bytes", len(files["file"]))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"SUCCESS","data":{"function_id":"fn1","version":"1.0.0","version_id":"fv1"}}`))
	}))
	defer srv.Close()

	partner := newFunctionPartner(t, srv.URL)
	got, exErr := UpsertFunction(context.Background(), Extension{ExtensionName: "fn"}, partner, jsPath, wasmPath)
	if exErr != nil {
		t.Fatalf("upsertFunction: %v", exErr)
	}
	want := UpsertResult{ExtensionID: "fn1", ExtensionVersion: "1.0.0", ExtensionVersionID: "fv1"}
	if got != want {
		t.Errorf("result = %+v, want %+v", got, want)
	}
	if sawClientID != "cid" {
		t.Errorf("server saw app-client-id = %q, want cid", sawClientID)
	}
	if sawToken != "apptok" {
		t.Errorf("server saw Access-Token = %q, want apptok", sawToken)
	}
}

func TestUpsertFunction_CommitPath(t *testing.T) {
	jsPath, wasmPath, _ := writeFuncFixtures(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/openapi/2025-06/functions/commit") {
			t.Errorf("path = %q, want .../functions/commit", r.URL.Path)
		}
		fields, _ := parseFunctionForm(t, r)
		if fields["function_id"] != "fn1" {
			t.Errorf("function_id = %q, want fn1", fields["function_id"])
		}
		if fields["version"] != "2.0.0" {
			t.Errorf("version = %q, want 2.0.0", fields["version"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"SUCCESS","data":{"function_id":"fn1","version":"2.0.0","version_id":"fv2"}}`))
	}))
	defer srv.Close()

	partner := newFunctionPartner(t, srv.URL)
	ext := Extension{ExtensionName: "fn", ExtensionID: "fn1", ExtensionVersion: "2.0.0"}
	got, exErr := UpsertFunction(context.Background(), ext, partner, jsPath, wasmPath)
	if exErr != nil {
		t.Fatalf("upsertFunction: %v", exErr)
	}
	want := UpsertResult{ExtensionID: "fn1", ExtensionVersion: "2.0.0", ExtensionVersionID: "fv2"}
	if got != want {
		t.Errorf("result = %+v, want %+v", got, want)
	}
}

func TestUpsertFunction_NonSuccessIsError(t *testing.T) {
	jsPath, wasmPath, _ := writeFuncFixtures(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"FAILED","message":"bad"}`))
	}))
	defer srv.Close()

	partner := newFunctionPartner(t, srv.URL)
	_, exErr := UpsertFunction(context.Background(), Extension{ExtensionName: "fn"}, partner, jsPath, wasmPath)
	if exErr == nil {
		t.Fatal("expected error for non-success code, got nil")
	}
	if !strings.Contains(exErr.Error(), "bad") {
		t.Errorf("error %q should surface the message %q", exErr.Error(), "bad")
	}
}

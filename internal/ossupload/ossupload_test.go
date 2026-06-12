package ossupload

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/client"
)

func TestBuildOSSForm_FieldOrderFileLast(t *testing.T) {
	sr := signResp{Policy: "P", AccessID: "AK", Sign: "SG"}
	body, _, err := buildOSSForm(sr, "chick-extension/demo.js", "demo.js", strings.NewReader("contents"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(body)
	s := string(raw)
	for _, name := range []string{"policy", "OSSAccessKeyId", "success_action_status", "signature", "x-oss-forbid-overwrite", "key"} {
		if !strings.Contains(s, `name="`+name+`"`) {
			t.Fatalf("missing form field %q", name)
		}
		if strings.Index(s, `name="`+name+`"`) > strings.Index(s, `name="file"`) {
			t.Fatalf("field %q must appear before the file part", name)
		}
	}
	if !strings.Contains(s, `x-oss-forbid-overwrite`) || !strings.Contains(s, "true") {
		t.Fatal("x-oss-forbid-overwrite=true must be present (triggers 409 instead of silent overwrite)")
	}
}

func TestOSSPostURL(t *testing.T) {
	if got := ossPostURL("//oss-cn.example/bucket"); got != "https://oss-cn.example/bucket" {
		t.Fatalf("protocol-relative write_host: got %q", got)
	}
	if got := ossPostURL("http://127.0.0.1:9/upload"); got != "http://127.0.0.1:9/upload" {
		t.Fatalf("absolute write_host must pass through: got %q", got)
	}
}

func TestUpload_Success_BypassesStoreAuth(t *testing.T) {
	var signSawToken bool
	var ossSawToken string
	var ossUploadURL string

	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		signSawToken = r.Header.Get("Access-Token") != ""
		if r.URL.Query().Get("key") != "chick-extension/demo.js" {
			t.Errorf("sign key = %q", r.URL.Query().Get("key"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossUploadURL, "read_host": "https://read.example/",
			"policy": "P", "access_id": "AK", "sign": "SG",
		})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		ossSawToken = r.Header.Get("Access-Token")
		_ = r.ParseMultipartForm(1 << 20)
		if r.MultipartForm.Value["x-oss-forbid-overwrite"][0] != "true" {
			t.Error("x-oss-forbid-overwrite must be true")
		}
		if _, ok := r.MultipartForm.File["file"]; !ok {
			t.Error("file part missing")
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossUploadURL = srv.URL + "/upload"

	dir := t.TempDir()
	artifact := filepath.Join(dir, "demo.js")
	_ = os.WriteFile(artifact, []byte("built-bundle"), 0o644)

	storeClient := client.New(srv.URL)
	storeClient.SetBearerToken("store-token")
	u := &Uploader{Client: storeClient, HTTPClient: srv.Client()}

	resourceURL, exitErr := u.Upload(context.Background(), artifact)
	if exitErr != nil {
		t.Fatalf("Upload: %v", exitErr)
	}
	if resourceURL != "https://read.example/chick-extension/demo.js" {
		t.Fatalf("resource_url = %q", resourceURL)
	}
	if !signSawToken {
		t.Error("sign GET MUST carry the store Access-Token")
	}
	if ossSawToken != "" {
		t.Errorf("OSS POST MUST NOT carry the store Access-Token, got %q", ossSawToken)
	}
}

func TestResourceURL_NoTrailingSlash(t *testing.T) {
	if got := resourceURL("https://read.example", "path/file.js"); got != "https://read.example/path/file.js" {
		t.Errorf("expected trailing slash added; got %q", got)
	}
}

func TestUpload_EmptyWriteHostErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sign returns no write_host.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": "", "read_host": "https://read.example/",
			"policy": "P", "access_id": "AK", "sign": "SG",
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	artifact := filepath.Join(dir, "demo.js")
	_ = os.WriteFile(artifact, []byte("x"), 0o644)
	u := &Uploader{Client: client.New(srv.URL), HTTPClient: srv.Client()}

	_, err := u.Upload(context.Background(), artifact)
	if err == nil {
		t.Fatal("expected error when write_host is empty")
	}
}

func TestUpload_FileOpenError(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": srv.URL + "/upload", "read_host": "https://read.example/",
			"policy": "P", "access_id": "AK", "sign": "SG",
		})
	}))
	defer srv.Close()

	u := &Uploader{Client: client.New(srv.URL), HTTPClient: srv.Client()}
	_, err := u.Upload(context.Background(), "/no/such/file/demo.js")
	if err == nil {
		t.Fatal("expected error when artifact file does not exist")
	}
}

// TestUpload_InvalidWriteHostErrors: a write_host that cannot form a valid
// request URL must return an internal error, not panic on a nil request.
func TestUpload_InvalidWriteHostErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": "//bad host/upload", "read_host": "https://read.example/",
			"policy": "P", "access_id": "AK", "sign": "SG",
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	artifact := filepath.Join(dir, "demo.js")
	_ = os.WriteFile(artifact, []byte("x"), 0o644)
	u := &Uploader{Client: client.New(srv.URL), HTTPClient: srv.Client()}

	_, exitErr := u.Upload(context.Background(), artifact)
	if exitErr == nil {
		t.Fatal("expected error for an unparsable write_host")
	}
	if !strings.Contains(exitErr.Detail.Message, "write_host") {
		t.Errorf("error should name the bad write_host, got %q", exitErr.Detail.Message)
	}
}

// TestUpload_SignHTTPErrorCarriesEndpoint: a non-2xx sign response must name
// the failing method+path in error.detail.
func TestUpload_SignHTTPErrorCarriesEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad sign request"}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	artifact := filepath.Join(dir, "demo.js")
	_ = os.WriteFile(artifact, []byte("x"), 0o644)
	u := &Uploader{Client: client.New(srv.URL), HTTPClient: srv.Client()}

	_, exitErr := u.Upload(context.Background(), artifact)
	if exitErr == nil || exitErr.Detail == nil || exitErr.Detail.Detail == nil {
		t.Fatalf("expected error with endpoint detail, got %v", exitErr)
	}
	if exitErr.Detail.Detail.Method != "GET" || exitErr.Detail.Detail.Path != "/openapi/checkout_extensions/file/sign" {
		t.Fatalf("endpoint = %s %s", exitErr.Detail.Detail.Method, exitErr.Detail.Detail.Path)
	}
}

func TestUpload_NonSuccessStatus(t *testing.T) {
	var ossUploadURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossUploadURL, "read_host": "https://read.example/",
			"policy": "P", "access_id": "AK", "sign": "SG",
		})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossUploadURL = srv.URL + "/upload"

	dir := t.TempDir()
	artifact := filepath.Join(dir, "demo.js")
	_ = os.WriteFile(artifact, []byte("x"), 0o644)
	u := &Uploader{Client: client.New(srv.URL), HTTPClient: srv.Client()}

	_, err := u.Upload(context.Background(), artifact)
	if err == nil {
		t.Fatal("expected error for non-2xx OSS upload status")
	}
}

func TestUpload_409IsGraceful(t *testing.T) {
	var ossUploadURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossUploadURL, "read_host": "https://read.example/",
			"policy": "P", "access_id": "AK", "sign": "SG",
		})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("<Error><Code>FileAlreadyExists</Code></Error>"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossUploadURL = srv.URL + "/upload"

	dir := t.TempDir()
	artifact := filepath.Join(dir, "demo.js")
	_ = os.WriteFile(artifact, []byte("x"), 0o644)
	u := &Uploader{Client: client.New(srv.URL), HTTPClient: srv.Client()}

	resourceURL, exitErr := u.Upload(context.Background(), artifact)
	if exitErr != nil {
		t.Fatalf("409/FileAlreadyExists must be graceful, got error: %v", exitErr)
	}
	if resourceURL != "https://read.example/chick-extension/demo.js" {
		t.Fatalf("resource_url after 409 = %q", resourceURL)
	}
}

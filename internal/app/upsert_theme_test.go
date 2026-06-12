package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/output"
)

// themeStoreServer is a fake store-openapi (2020-07) server. taskStates is the
// sequence of {state, version_id} returned by successive GET version-tasks
// calls (the last entry repeats if polled beyond the slice).
type themeStoreServer struct {
	versionID  string
	taskStates []string // raw JSON bodies for GET version-tasks
	getCalls   int
	mu         sync.Mutex
}

func newThemeStore(versionID string, taskStates ...string) *themeStoreServer {
	return &themeStoreServer{versionID: versionID, taskStates: taskStates}
}

func (s *themeStoreServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/theme-extensions"):
			_, _ = w.Write([]byte(`{"extension_id":"ext1"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/version-tasks"):
			_, _ = w.Write([]byte(`{"task_id":"t1"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/version-tasks/"):
			s.mu.Lock()
			i := s.getCalls
			if i >= len(s.taskStates) {
				i = len(s.taskStates) - 1
			}
			body := s.taskStates[i]
			s.getCalls++
			s.mu.Unlock()
			_, _ = w.Write([]byte(body))
		default:
			http.Error(w, "unexpected request "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}
}

func TestUpsertTheme_CreatePath(t *testing.T) {
	store := newThemeStore("v1",
		`{"task_id":"t1","state":0}`,
		`{"task_id":"t1","state":1,"version_id":"v1"}`,
	)
	storeSrv := httptest.NewServer(store.handler())
	defer storeSrv.Close()

	var mu sync.Mutex
	var connBody map[string]any // nil until the connection endpoint is hit
	partnerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		connBody = body
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"Success"}`))
	}))
	defer partnerSrv.Close()

	storeClient := client.New(storeSrv.URL)
	partnerClient := client.New(partnerSrv.URL)

	ext := Extension{ExtensionName: "th", ResourceURL: "http://x/a.zip"}
	res, err := upsertTheme(context.Background(), ext, storeClient, partnerClient, time.Millisecond, 10)
	if err != nil {
		t.Fatalf("upsertTheme: %v", err)
	}
	if res.ExtensionID != "ext1" || res.ExtensionVersion != "1.0.0" || res.ExtensionVersionID != "v1" {
		t.Fatalf("result = %+v, want {ext1, 1.0.0, v1}", res)
	}
	mu.Lock()
	defer mu.Unlock()
	if connBody == nil {
		t.Fatalf("connection (partner) was not called on create path")
	}
	if connBody["extension_id"] != "ext1" || connBody["type"] != "link" {
		t.Fatalf("connection body = %v, want {extension_id:ext1, type:link}", connBody)
	}
}

func TestUpsertTheme_UpdatePath(t *testing.T) {
	store := newThemeStore("v2",
		`{"task_id":"t1","state":0}`,
		`{"task_id":"t1","state":1,"version_id":"v2"}`,
	)
	storeSrv := httptest.NewServer(store.handler())
	defer storeSrv.Close()

	partnerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("connection (partner) must NOT be called on update path; got %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"Success"}`))
	}))
	defer partnerSrv.Close()

	storeClient := client.New(storeSrv.URL)
	partnerClient := client.New(partnerSrv.URL)

	ext := Extension{ExtensionID: "ext1", ExtensionVersion: "2.0.0", ExtensionName: "th", ResourceURL: "http://x/a.zip"}
	res, err := upsertTheme(context.Background(), ext, storeClient, partnerClient, time.Millisecond, 10)
	if err != nil {
		t.Fatalf("upsertTheme: %v", err)
	}
	if res.ExtensionID != "ext1" || res.ExtensionVersion != "2.0.0" || res.ExtensionVersionID != "v2" {
		t.Fatalf("result = %+v, want {ext1, 2.0.0, v2}", res)
	}
}

func TestUpsertTheme_TaskFailed(t *testing.T) {
	store := newThemeStore("",
		`{"task_id":"t1","state":2,"message":"boom"}`,
	)
	storeSrv := httptest.NewServer(store.handler())
	defer storeSrv.Close()

	partnerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"Success"}`))
	}))
	defer partnerSrv.Close()

	storeClient := client.New(storeSrv.URL)
	partnerClient := client.New(partnerSrv.URL)

	ext := Extension{ExtensionName: "th", ResourceURL: "http://x/a.zip"}
	_, err := upsertTheme(context.Background(), ext, storeClient, partnerClient, time.Millisecond, 10)
	if err == nil {
		t.Fatalf("upsertTheme: expected error on task state=2")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error %q does not mention task failure message 'boom'", err.Error())
	}
	// A state-2 task is a SERVER-reported failure → API-class, not internal.
	if err.Code != output.ExitAPI {
		t.Fatalf("state-2 task failure: exit %d, want %d (api)", err.Code, output.ExitAPI)
	}
}

// TestPollThemeVersionTask_TimeoutIsAPIClass: exhausting the polls means
// the SERVER never finished the accepted task — API-class, not a CLI bug.
func TestPollThemeVersionTask_TimeoutIsAPIClass(t *testing.T) {
	store := newThemeStore("", `{"task_id":"t1","state":0}`)
	storeSrv := httptest.NewServer(store.handler())
	defer storeSrv.Close()

	_, err := PollThemeVersionTask(context.Background(), client.New(storeSrv.URL), "th", "t1", time.Millisecond, 2)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if err.Code != output.ExitAPI {
		t.Fatalf("poll timeout: exit %d, want %d (api)", err.Code, output.ExitAPI)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout message, got %q", err.Error())
	}
}

func TestRegisterThemeExtensionDoesNotConnect(t *testing.T) {
	connectHit := false
	storeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/openapi/2020-07/theme-extensions":
			_, _ = w.Write([]byte(`{"extension_id":"tex_new"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks":
			_, _ = w.Write([]byte(`{"task_id":"task_1"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks/task_1":
			_, _ = w.Write([]byte(`{"task_id":"task_1","state":1,"version_id":"ver_1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer storeSrv.Close()
	partnerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectHit = true
		_, _ = w.Write([]byte(`{"code":"Success"}`))
	}))
	defer partnerSrv.Close()

	ext := Extension{ExtensionName: "thm-x", ExtensionType: "theme", ResourceURL: "https://cdn/x.zip"}
	res, err := RegisterThemeExtension(context.Background(), ext, client.New(storeSrv.URL), time.Millisecond, 5)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if connectHit {
		t.Fatal("RegisterThemeExtension must NOT call the connection endpoint")
	}
	if res.ExtensionVersionID == "" {
		t.Fatal("expected a version id from the poll")
	}
}

func TestRegisterThemeExtensionHonorsExplicitCreateVersion(t *testing.T) {
	var sentVersion string
	storeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/openapi/2020-07/theme-extensions":
			_, _ = w.Write([]byte(`{"extension_id":"tex_new"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			sentVersion, _ = body["version"].(string)
			_, _ = w.Write([]byte(`{"task_id":"t1"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks/t1":
			_, _ = w.Write([]byte(`{"task_id":"t1","state":1,"version_id":"ver_x"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer storeSrv.Close()
	// Fresh extension (no ExtensionID) but explicit version "2.3.0" → create path must send 2.3.0, not 1.0.0.
	ext := Extension{ExtensionName: "thm-x", ExtensionType: "theme", ResourceURL: "https://cdn/x.zip", ExtensionVersion: "2.3.0"}
	if _, err := RegisterThemeExtension(context.Background(), ext, client.New(storeSrv.URL), time.Millisecond, 5); err != nil {
		t.Fatal(err)
	}
	if sentVersion != "2.3.0" {
		t.Fatalf("explicit create version dropped: version-task sent %q, want 2.3.0", sentVersion)
	}
}

func TestConnectThemeAcceptsBothSuccessMarkers(t *testing.T) {
	cases := []struct {
		name, body string
		wantErr    bool
	}{
		{"int 200 (v1 te)", `{"code":200}`, false},
		{"string Success (v1 app-deploy)", `{"code":"Success"}`, false},
		{"string success lowercase", `{"code":"success"}`, false},
		// Live app-deploy shape: enveloped with a data key. The client unwraps
		// {"code":"Success","data":{}} to {} BEFORE we decode, so code arrives
		// empty — must still be treated as success (regression: this 200'd
		// connection was reported as "connection failed").
		{"enveloped Success with data (live)", `{"code":"Success","data":{}}`, false},
		{"empty body", `{}`, false},
		{"failure code", `{"code":4001,"message":"boom"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			partner := client.New(srv.URL)
			err := ConnectTheme(context.Background(), partner, "ext-x", "tex_123", ThemeConnectionPathApp)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for body %s, got nil", tc.body)
			}
			if tc.wantErr && err.Code != output.ExitAPI {
				// A non-success code in a 2xx body is server-reported → API-class.
				t.Fatalf("connect failure: exit %d, want %d (api)", err.Code, output.ExitAPI)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected success for body %s, got %v", tc.body, err)
			}
		})
	}
}

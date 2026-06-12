package dynamic

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/registry"

	"github.com/spf13/cobra"
)

func newRunFactory(t *testing.T, srv *httptest.Server) (*cmdutil.Factory, *strings.Builder, *strings.Builder) {
	t.Helper()
	out := &strings.Builder{}
	errOut := &strings.Builder{}
	c := client.New(srv.URL)
	c.SetBearerToken("dev")
	return &cmdutil.Factory{
		IOStreams: cmdutil.IOStreams{In: strings.NewReader(""), Out: out, ErrOut: errOut},
		Client:    c,
	}, out, errOut
}

func TestRunE_InvalidParamsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("server must not be hit on validation error")
	}))
	defer srv.Close()
	f, _, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"list"}, HTTP: registry.HTTP{Method: "GET", Path: "/x"}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	leaf.SetArgs([]string{"--params", "not-json"})
	err := leaf.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != "validation" {
		t.Fatalf("expected validation ExitError, got %T %v", err, err)
	}
}

func TestRunE_MissingPathParam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("server must not be hit on missing path param")
	}))
	defer srv.Close()
	f, _, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"get"}, HTTP: registry.HTTP{Method: "GET", Path: "/x/{id}"}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	leaf.SetArgs([]string{"--params", "{}"})
	err := leaf.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != "validation" {
		t.Fatalf("expected validation, got %v", err)
	}
}

func TestRunE_DryRunDoesNotSendRequest(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	f, out, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"list"}, HTTP: registry.HTTP{Method: "GET", Path: "/orders"}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	// --dry-run is inherited from the root's PersistentFlags in production;
	// wrap the leaf in a stub parent that mirrors that setup.
	parent := &cobra.Command{Use: "shoplazza", SilenceUsage: true, SilenceErrors: true}
	parent.PersistentFlags().Bool("dry-run", false, "")
	parent.AddCommand(leaf)
	parent.SetArgs([]string{"list", "--params", `{"page_size":10}`, "--dry-run"})
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hit {
		t.Fatal("dry-run must not call backend")
	}
	if !strings.Contains(out.String(), "dry_run") {
		t.Fatalf("expected dry_run marker in output, got: %s", out.String())
	}
}

func TestRunE_HappyPathListGET(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"discounts":[{"id":"d1"}]}`))
	}))
	defer srv.Close()
	f, out, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"list"}, HTTP: registry.HTTP{Method: "GET", Path: "/openapi/2026-01/discounts"}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	leaf.SetArgs([]string{"--params", `{"page_size":10}`})
	if err := leaf.Execute(); err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(gotURL, "page_size=10") {
		t.Fatalf("expected page_size in URL, got %q", gotURL)
	}
	if !strings.Contains(out.String(), "discounts") {
		t.Fatalf("expected response in stdout, got %q", out.String())
	}
}

func TestRunE_PathParamSubstituted(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	f, _, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"get"}, HTTP: registry.HTTP{Method: "GET", Path: "/openapi/2026-01/discounts/{id}"}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	leaf.SetArgs([]string{"--params", `{"id":"d001"}`})
	if err := leaf.Execute(); err != nil {
		t.Fatalf("error: %v", err)
	}
	if gotPath != "/openapi/2026-01/discounts/d001" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestRunE_HTTPErrorBecomesAPIExitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"bad"}`))
	}))
	defer srv.Close()
	f, _, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{Path: []string{"list"}, HTTP: registry.HTTP{Method: "GET", Path: "/x"}}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	err := leaf.Execute()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != "api" {
		t.Fatalf("expected api ExitError, got %v", err)
	}
}

func TestRunE_BodyPOSTWithData(t *testing.T) {
	var gotBody json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"new"}`))
	}))
	defer srv.Close()
	f, _, _ := newRunFactory(t, srv)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "dev")

	cmd := registry.Command{
		Path: []string{"create"},
		HTTP: registry.HTTP{Method: "POST", Path: "/openapi/2026-01/coupons", Body: "*"},
	}
	leaf := buildLeafCommand(cmd, &registry.Spec{}, f, "testmodule")
	leaf.SetArgs([]string{"--data", `{"coupon":{"code":"X"}}`})
	if err := leaf.Execute(); err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(string(gotBody), `"code":"X"`) {
		t.Fatalf("body = %q", gotBody)
	}
}

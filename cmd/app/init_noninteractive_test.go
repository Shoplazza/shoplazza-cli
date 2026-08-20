package appcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// These tests pin the non-interactive path: with no terminal, `app init` asks
// nothing and keeps its output, exit code and request set.

// initServer is the Dashboard mock for the init flow, recording the /api/cli/v2
// paths it is asked for so a case can assert the request set. partnerIDs sets the
// account's partners; tmpl is the git repo the template points at.
type initServer struct {
	*httptest.Server
	mu   sync.Mutex
	seen []string
}

func newInitServer(t *testing.T, tmpl string, partnerIDs ...string) *initServer {
	t.Helper()
	is := &initServer{}
	is.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/cli/v2") {
			is.mu.Lock()
			is.seen = append(is.seen, r.Method+" "+r.URL.Path)
			is.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		ok := func(data map[string]any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": data})
		}
		switch {
		case r.URL.Path == "/api/saiga/cli/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u1"})
		case strings.HasSuffix(r.URL.Path, "/partners"):
			ps := make([]map[string]any, 0, len(partnerIDs))
			for _, id := range partnerIDs {
				ps = append(ps, map[string]any{"id": id, "business_name": "Biz " + id})
			}
			ok(map[string]any{"partners": ps})
		case strings.HasSuffix(r.URL.Path, "/apps") && r.Method == http.MethodPost:
			ok(map[string]any{"app": map[string]any{"client_id": "cid_new", "id": 7, "name": "My App", "scopes": []string{"read"}}})
		case strings.HasSuffix(r.URL.Path, "/apps"):
			ok(map[string]any{"apps": []map[string]any{{"id": 3, "client_id": "cid_x", "name": "MyApp"}}, "total": 1})
		case strings.HasSuffix(r.URL.Path, "/info"):
			ok(map[string]any{
				"partner": map[string]any{"id": "p1", "business_name": "Biz p1"},
				"app":     map[string]any{"client_id": "cid_x", "name": "MyApp", "scopes": []string{"read"}}})
		case strings.HasSuffix(r.URL.Path, "/template"):
			ok(map[string]any{"template_type": "app", "https": tmpl})
		case strings.Contains(r.URL.Path, "/apps/cid_x"):
			ok(map[string]any{"app": map[string]any{"client_id": "cid_x", "id": 3, "name": "MyApp", "scopes": []string{"read"}}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(is.Close)
	return is
}

func (is *initServer) requests() []string {
	is.mu.Lock()
	defer is.mu.Unlock()
	return append([]string(nil), is.seen...)
}

// initFactory builds a logged-in factory whose buffer streams keep the
// interactive gate closed, as under a pipe.
func initFactory(t *testing.T, srvURL string) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	dir := seedLoginKeychain(t)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	f := &cmdutil.Factory{
		IOStreams:  cmdutil.IOStreams{In: strings.NewReader(""), Out: out, ErrOut: errOut},
		ConfigPath: filepath.Join(dir, "config.json"),
		Config:     core.CliConfig{Accounts: []core.AccountConfig{{Name: "alice@co.com"}}},
		Client:     client.New(srvURL),
		AuthClient: client.New(srvURL),
	}
	return f, out, errOut
}

// runInitOffTTY executes `app init` with the given args, failing the test if it
// blocks past the deadline instead of returning.
func runInitOffTTY(t *testing.T, f *cmdutil.Factory, out, errOut *bytes.Buffer, args ...string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		cmd := newCmdInit(f)
		cmd.SetOut(out)
		cmd.SetErr(errOut)
		cmd.SetArgs(args)
		cmd.SetContext(context.Background())
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		done <- cmd.Execute()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(20 * time.Second):
		t.Fatalf("app init %v blocked without a terminal — the gate must never prompt", args)
		return nil
	}
}

// TestInit_OffTTY_BareRunRejectedBeforeAnyRequest pins that a bare run is
// rejected, with cobra's wording, before any request.
func TestInit_OffTTY_BareRunRejectedBeforeAnyRequest(t *testing.T) {
	t.Chdir(t.TempDir())
	is := newInitServer(t, "unused", "p1")
	f, out, errOut := initFactory(t, is.URL)

	err := runInitOffTTY(t, f, out, errOut)
	if err == nil {
		t.Fatal("a bare run must still be rejected without a terminal")
	}
	const want = "at least one of the flags in the group [client-id name] is required"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	// A plain error, not an ExitError: that is what keeps the usage block and exit 2.
	var ee *output.ExitError
	if errors.As(err, &ee) {
		t.Errorf("error must stay a plain error, got *output.ExitError %+v", ee)
	}
	if reqs := is.requests(); len(reqs) != 0 {
		t.Errorf("sent %v before failing, want no request", reqs)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", out.String())
	}
}

// TestInit_OffTTY_MultiPartnerCreateStillErrors pins that --name on a
// multi-partner account still fails through selectPartner, pointing at --partner.
func TestInit_OffTTY_MultiPartnerCreateStillErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	is := newInitServer(t, "unused", "p1", "p2")
	f, out, errOut := initFactory(t, is.URL)

	err := runInitOffTTY(t, f, out, errOut, "--name", "My App")
	if err == nil {
		t.Fatal("multiple partners with no --partner must still error")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("want a validation ExitError, got %v", err)
	}
	if !strings.Contains(ee.Error(), "multiple partners") {
		t.Errorf("message = %q, want selectPartner's wording", ee.Error())
	}
	if ee.Detail == nil || !strings.Contains(ee.Detail.Hint, "--partner") {
		t.Errorf("hint = %+v, want it to point at --partner", ee.Detail)
	}
	// One partner list, read by resolveAppRef; no app list, which is the picker's read.
	want := []string{"GET /api/cli/v2/partners"}
	if reqs := is.requests(); !slices.Equal(reqs, want) {
		t.Errorf("requests = %v, want %v", reqs, want)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", out.String())
	}
}

// TestInit_OffTTY_FullySpecifiedModesRun pins the request set of each
// fully-specified mode.
func TestInit_OffTTY_FullySpecifiedModesRun(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		partners []string
		dir      string
		wantReqs []string
	}{
		{
			name: "link by client-id", args: []string{"--client-id", "cid_x"},
			partners: []string{"p1", "p2"}, dir: "myapp",
			wantReqs: []string{
				"GET /api/cli/v2/info",
				"GET /api/cli/v2/partners/p1/apps/cid_x",
				"GET /api/cli/v2/template",
			},
		},
		{
			name: "create by name, sole partner", args: []string{"--name", "My App"},
			partners: []string{"p1"}, dir: "my-app",
			wantReqs: []string{
				"GET /api/cli/v2/partners",
				"POST /api/cli/v2/partners/p1/apps",
				"GET /api/cli/v2/template",
			},
		},
		{
			name: "create by name and partner", args: []string{"--name", "My App", "--partner", "p2"},
			partners: []string{"p1", "p2"}, dir: "my-app",
			wantReqs: []string{
				"GET /api/cli/v2/partners",
				"POST /api/cli/v2/partners/p2/apps",
				"GET /api/cli/v2/template",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := makeTemplateRepo(t) // skips if git is missing
			root := t.TempDir()
			t.Chdir(root)
			is := newInitServer(t, tmpl, tc.partners...)
			f, out, errOut := initFactory(t, is.URL)

			if err := runInitOffTTY(t, f, out, errOut, tc.args...); err != nil {
				t.Fatalf("app init %v: %v\nstderr: %s", tc.args, err, errOut.String())
			}
			var env map[string]any
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatalf("stdout is not the result body: %v\n%s", err, out.String())
			}
			if got := env["project"]; got != filepath.Join(root, tc.dir) {
				t.Errorf("project = %v, want the %s sub-dir", got, tc.dir)
			}
			if reqs := is.requests(); !slices.Equal(reqs, tc.wantReqs) {
				t.Errorf("requests = %v, want %v", reqs, tc.wantReqs)
			}
		})
	}
}

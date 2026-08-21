package appcmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// wizardServer records every request path and serves the two list endpoints the
// pickers read; status != 200 makes both fail. Any other path fails the test.
type wizardServer struct {
	*httptest.Server
	mu   sync.Mutex
	seen []string
}

func newWizardServer(t *testing.T, status int, partners []map[string]any, apps []map[string]any) *wizardServer {
	t.Helper()
	ws := &wizardServer{}
	ws.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws.mu.Lock()
		ws.seen = append(ws.seen, r.Method+" "+r.URL.Path)
		ws.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"service unavailable"}`))
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/apps"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"apps": apps, "total": len(apps)}})
		case strings.HasSuffix(r.URL.Path, "/partners"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"partners": partners}})
		default:
			t.Errorf("wizard sent an unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(ws.Close)
	return ws
}

func (ws *wizardServer) requests() []string {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return append([]string(nil), ws.seen...)
}

func (ws *wizardServer) dashboard() *app.Dashboard {
	return app.NewDashboard(client.New(ws.URL), "ptok")
}

// TestWizardInit_ClientIDIsZeroInteractionAndZeroRequests pins that link mode
// leaves the flags alone and sends no request.
func TestWizardInit_ClientIDIsZeroInteractionAndZeroRequests(t *testing.T) {
	ws := newWizardServer(t, http.StatusOK, nil, nil)
	in := initFlags{clientID: "cid_x", partner: "p1"}
	got, err := wizardInit(context.Background(), ws.dashboard(), t.TempDir(), in)
	if err != nil {
		t.Fatalf("wizardInit: %v", err)
	}
	if got != in {
		t.Errorf("flags = %+v, want them untouched: %+v", got, in)
	}
	if reqs := ws.requests(); len(reqs) != 0 {
		t.Errorf("link mode sent %v, want no request at all", reqs)
	}
}

// TestWizardInit_SinglePartnerAutoSelectedBeforePlanning pins that the sole
// partner is filled in before stepsFor, so `--name X` prompts nothing and reads
// only the partner list.
func TestWizardInit_SinglePartnerAutoSelectedBeforePlanning(t *testing.T) {
	ws := newWizardServer(t, http.StatusOK, []map[string]any{{"id": "p1", "business_name": "Acme"}}, nil)
	got, err := wizardInit(context.Background(), ws.dashboard(), t.TempDir(), initFlags{name: "My App"})
	if err != nil {
		t.Fatalf("wizardInit: %v", err)
	}
	if got.partner != "p1" || got.name != "My App" || got.clientID != "" {
		t.Errorf("flags = %+v, want the sole partner filled in and --name kept", got)
	}
	want := []string{"GET /api/cli/v2/partners"}
	if reqs := ws.requests(); len(reqs) != 1 || reqs[0] != want[0] {
		t.Errorf("requests = %v, want exactly %v", reqs, want)
	}
}

// TestWizardInit_NameAndPartnerSendNoRequest pins that --name plus --partner
// leaves nothing to ask and nothing to read.
func TestWizardInit_NameAndPartnerSendNoRequest(t *testing.T) {
	ws := newWizardServer(t, http.StatusOK, nil, nil)
	in := initFlags{name: "My App", partner: "p1"}
	got, err := wizardInit(context.Background(), ws.dashboard(), t.TempDir(), in)
	if err != nil {
		t.Fatalf("wizardInit: %v", err)
	}
	if got != in {
		t.Errorf("flags = %+v, want them untouched: %+v", got, in)
	}
	if reqs := ws.requests(); len(reqs) != 0 {
		t.Errorf("fully specified run sent %v, want no request", reqs)
	}
}

// TestWizardInit_ListFailureErrorsWithFlagHint pins that a list that will not
// load fails the command with a hint naming the equivalent flags.
func TestWizardInit_ListFailureErrorsWithFlagHint(t *testing.T) {
	for _, tc := range []struct {
		name      string
		flags     initFlags
		wantReq   string
		wantFlags []string
	}{
		{"partner list down", initFlags{}, "GET /api/cli/v2/partners", []string{"--partner", "--client-id"}},
		{"app list down", initFlags{partner: "p1"}, "GET /api/cli/v2/partners/p1/apps", []string{"--client-id", "--name"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := newWizardServer(t, http.StatusServiceUnavailable, nil, nil)
			_, err := wizardInit(context.Background(), ws.dashboard(), t.TempDir(), tc.flags)
			if err == nil {
				t.Fatal("a failed list must fail the command")
			}
			var ee *output.ExitError
			if !errors.As(err, &ee) || ee.Detail == nil {
				t.Fatalf("error is not a detailed *output.ExitError: %v", err)
			}
			for _, flag := range tc.wantFlags {
				if !strings.Contains(ee.Detail.Hint, flag) {
					t.Errorf("hint = %q, want it to name %s", ee.Detail.Hint, flag)
				}
			}
			if reqs := ws.requests(); len(reqs) != 1 || reqs[0] != tc.wantReq {
				t.Errorf("requests = %v, want exactly [%s]", reqs, tc.wantReq)
			}
		})
	}
}

// TestWizardInit_NoPartnersIsAValidationError pins that an account with no
// partners gets selectPartner's validation error, not an empty picker.
func TestWizardInit_NoPartnersIsAValidationError(t *testing.T) {
	ws := newWizardServer(t, http.StatusOK, []map[string]any{}, nil)
	_, err := wizardInit(context.Background(), ws.dashboard(), t.TempDir(), initFlags{name: "My App"})
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("want a validation error, got %v", err)
	}
	if !strings.Contains(ee.Error(), "no partners available") {
		t.Errorf("message = %q, want selectPartner's wording", ee.Error())
	}
}

// TestValidateAppName_ReusesTargetDirFor pins that the name screen rejects what
// runInit would reject, through targetDirFor ("already exists at" is its wording).
func TestValidateAppName_ReusesTargetDirFor(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "my-app"), 0o755); err != nil { // slug("My App")
		t.Fatal(err)
	}
	validate := validateAppName(root)

	if err := validate("My App"); err == nil {
		t.Error("a name whose slug already exists must be rejected")
	} else if !strings.Contains(err.Error(), "already exists at") {
		t.Errorf("error = %q, want targetDirFor's message", err.Error())
	}
	// Whitespace is trimmed before slugging, so " My App " collides too.
	if err := validate("  My App  "); err == nil {
		t.Error("the trimmed name must be validated, not the raw one")
	}
	if err := validate("Other App"); err != nil {
		t.Errorf("a free name must pass: %v", err)
	}
	if err := validate("   "); err == nil {
		t.Error("a blank name must be rejected")
	}
}

// TestAppOptions_CreateFirstThenTwoColumns pins the create sentinel as the first
// row, then the apps as name + client_id, with no header row.
func TestAppOptions_CreateFirstThenTwoColumns(t *testing.T) {
	opts := appOptions([]app.App{
		{ClientID: "c_aaa111", Name: "order-sync"},
		{ClientID: "c_bbb222", Name: "loyalty-widget"},
	})
	if len(opts) != 3 {
		t.Fatalf("got %d options, want create + 2 apps", len(opts))
	}
	if opts[0].Value != createNewApp || !strings.Contains(opts[0].Key, "Create a new app") {
		t.Errorf("first option = %q/%q, want the create sentinel", opts[0].Key, opts[0].Value)
	}
	for i, want := range []struct{ key, value string }{
		{"order-sync     c_aaa111", "c_aaa111"},
		{"loyalty-widget c_bbb222", "c_bbb222"},
	} {
		if got := opts[i+1]; got.Key != want.key || got.Value != want.value {
			t.Errorf("option %d = %q/%q, want %q/%q", i+1, got.Key, got.Value, want.key, want.value)
		}
	}
}

// TestPartnerOptions_FallsBackToID pins that a partner with no business_name
// gets a row carrying the id alone.
func TestPartnerOptions_FallsBackToID(t *testing.T) {
	opts := partnerOptions([]app.Partner{{ID: "p_1001", BusinessName: "Acme"}, {ID: "p_2002"}})
	if len(opts) != 2 {
		t.Fatalf("got %d options, want 2", len(opts))
	}
	if opts[0].Key != "Acme p_1001" || opts[0].Value != "p_1001" {
		t.Errorf("option 0 = %q/%q", opts[0].Key, opts[0].Value)
	}
	if opts[1].Key != "p_2002" || opts[1].Value != "p_2002" {
		t.Errorf("option 1 = %q/%q, want the bare id", opts[1].Key, opts[1].Value)
	}
}

// TestOptions_AlignTheIDColumnInCells pins that the id column lands on the same
// terminal cell in every row. Names come from the API, so they carry CJK and
// other double-width characters; a byte or rune count skews the column.
func TestOptions_AlignTheIDColumnInCells(t *testing.T) {
	t.Run("partners", func(t *testing.T) {
		opts := partnerOptions([]app.Partner{
			{ID: "3634", BusinessName: "212"},
			{ID: "3665", BusinessName: "1"},
			{ID: "34578", BusinessName: "店匠"},
			{ID: "9", BusinessName: "café"},
		})
		assertOneIDColumn(t, opts)
	})
	t.Run("apps", func(t *testing.T) {
		opts := appOptions([]app.App{
			{ClientID: "c_aaa111", Name: "order-sync"},
			{ClientID: "c_bbb222", Name: "订单同步"},
			{ClientID: "c_ccc333", Name: "loyalty"},
		})
		if opts[0].Value != createNewApp {
			t.Fatalf("first option = %q, want the create sentinel", opts[0].Value)
		}
		assertOneIDColumn(t, opts[1:]) // the sentinel row has no id column
	})
}

// assertOneIDColumn checks every "name  id" label starts its id at one cell.
func assertOneIDColumn(t *testing.T, opts []huh.Option[string]) {
	t.Helper()
	want := -1
	for _, o := range opts {
		at := lipgloss.Width(o.Key) - lipgloss.Width(o.Value)
		if want == -1 {
			want = at
		}
		if at != want {
			t.Errorf("%q starts its id at cell %d, want %d — the column is skewed", o.Key, at, want)
		}
	}
}

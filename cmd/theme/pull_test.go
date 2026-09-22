package themecmd

import (
	"errors"
	"net"
	"path/filepath"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/env"
)

// confirmYes / confirmNo are canned interactive answers for recordThemeEnvironment.
func confirmYes(string) bool { return true }
func confirmNo(string) bool  { return false }

// TestRecordPullEnvironment_CreatesWhenAbsent: a bare theme dir gets a fresh
// shoplazza.theme.toml with a default environment describing the pull.
func TestRecordPullEnvironment_CreatesWhenAbsent(t *testing.T) {
	dir := t.TempDir()

	p, action, err := recordThemeEnvironment(dir, env.DefaultEnvironment, "myshop.myshoplaza.com", "123456", "default", nil)
	if err != nil {
		t.Fatalf("recordThemeEnvironment: %v", err)
	}
	if action != envWriteCreated {
		t.Fatalf("action = %q, want created", action)
	}
	if want := filepath.Join(dir, env.FileName); p != want {
		t.Fatalf("path = %q, want %q", p, want)
	}

	file, err := env.Load(p)
	if err != nil {
		t.Fatalf("load written file: %v", err)
	}
	e, ok := file.Environment(env.DefaultEnvironment)
	if !ok {
		t.Fatalf("default environment not written")
	}
	if e.Store != "myshop.myshoplaza.com" || e.Theme != "123456" || e.Profile != "default" {
		t.Fatalf("default env = %+v, want store/theme/profile populated", e)
	}
}

// TestRecordPullEnvironment_SkipsExistingWhenNonInteractive: an agent (confirm
// nil) must never rewrite a user's existing config.
func TestRecordPullEnvironment_SkipsExistingWhenNonInteractive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, env.FileName)
	seed := env.File{Environments: map[string]env.Environment{
		"default": {Store: "old.myshoplaza.com", Theme: "111"},
		"staging": {Store: "staging.myshoplaza.com", Theme: "222"},
	}}
	if err := env.Save(p, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, action, err := recordThemeEnvironment(dir, env.DefaultEnvironment, "new.myshoplaza.com", "999", "default", nil)
	if err != nil {
		t.Fatalf("recordThemeEnvironment: %v", err)
	}
	if action != envWriteSkipped {
		t.Fatalf("action = %q, want skipped", action)
	}
	// File untouched: default still points at the old theme.
	file, _ := env.Load(p)
	if e, _ := file.Environment("default"); e.Theme != "111" {
		t.Fatalf("default theme = %q, want untouched 111", e.Theme)
	}
}

// TestRecordPullEnvironment_UpdatesOnConfirmPreservingOthers: a confirmed human
// overwrite rewrites default's store/theme/profile but leaves other environments
// and default's hand-set fields intact.
func TestRecordPullEnvironment_UpdatesOnConfirmPreservingOthers(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, env.FileName)
	seed := env.File{Environments: map[string]env.Environment{
		"default": {Store: "old.myshoplaza.com", Theme: "111", Ignore: []string{"config/settings_data.json"}},
		"staging": {Store: "staging.myshoplaza.com", Theme: "222"},
	}}
	if err := env.Save(p, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, action, err := recordThemeEnvironment(dir, env.DefaultEnvironment, "new.myshoplaza.com", "999", "prod", confirmYes)
	if err != nil {
		t.Fatalf("recordThemeEnvironment: %v", err)
	}
	if action != envWriteUpdated {
		t.Fatalf("action = %q, want updated", action)
	}

	file, _ := env.Load(p)
	d, _ := file.Environment("default")
	if d.Store != "new.myshoplaza.com" || d.Theme != "999" || d.Profile != "prod" {
		t.Fatalf("default not updated: %+v", d)
	}
	if len(d.Ignore) != 1 || d.Ignore[0] != "config/settings_data.json" {
		t.Fatalf("default ignore not preserved: %+v", d.Ignore)
	}
	if s, ok := file.Environment("staging"); !ok || s.Theme != "222" {
		t.Fatalf("staging environment not preserved: %+v", s)
	}
}

// TestRecordThemeEnvironment_TargetsNamedEnv: with -e, the record goes to the
// named environment (not default), leaving other environments untouched.
func TestRecordThemeEnvironment_TargetsNamedEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, env.FileName)
	seed := env.File{Environments: map[string]env.Environment{
		"default": {Store: "d.myshoplaza.com", Theme: "1"},
		"prod":    {Store: "prod.myshoplaza.com", Theme: "old"},
	}}
	if err := env.Save(p, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, action, err := recordThemeEnvironment(dir, "prod", "prod.myshoplaza.com", "999", "prodprofile", confirmYes)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if action != envWriteUpdated {
		t.Fatalf("action = %q, want updated", action)
	}
	file, _ := env.Load(p)
	if pr, _ := file.Environment("prod"); pr.Theme != "999" || pr.Profile != "prodprofile" {
		t.Fatalf("prod not updated: %+v", pr)
	}
	if d, _ := file.Environment("default"); d.Theme != "1" {
		t.Fatalf("default must be untouched: %+v", d)
	}
}

// TestRecordPullEnvironment_UnchangedWhenMatching: re-pulling the same store/
// theme/profile is a no-op even without confirmation.
func TestRecordPullEnvironment_UnchangedWhenMatching(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, env.FileName)
	seed := env.File{Environments: map[string]env.Environment{
		"default": {Store: "myshop.myshoplaza.com", Theme: "123456", Profile: "default"},
	}}
	if err := env.Save(p, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, action, err := recordThemeEnvironment(dir, env.DefaultEnvironment, "myshop.myshoplaza.com", "123456", "default", confirmNo)
	if err != nil {
		t.Fatalf("recordThemeEnvironment: %v", err)
	}
	if action != envWriteUnchanged {
		t.Fatalf("action = %q, want unchanged", action)
	}
}

// TestRecordPullEnvironment_NoneWhenNothingBindable: a CI pull with neither store
// nor profile records nothing.
func TestRecordPullEnvironment_NoneWhenNothingBindable(t *testing.T) {
	dir := t.TempDir()

	_, action, err := recordThemeEnvironment(dir, env.DefaultEnvironment, "", "123456", "", nil)
	if err != nil {
		t.Fatalf("recordThemeEnvironment: %v", err)
	}
	if action != envWriteNone {
		t.Fatalf("action = %q, want none", action)
	}
	// No file created.
	if _, lerr := env.Load(filepath.Join(dir, env.FileName)); lerr == nil {
		t.Fatalf("expected no file to be written")
	}
}

func TestClassifyPullDownloadErr_EnvelopeClass(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		wantType string
		wantCode int
	}{
		{"404 is the caller's theme id", &client.HTTPError{StatusCode: 404}, output.TypeValidation, output.ExitValidation},
		{"401 needs a re-login", &client.HTTPError{StatusCode: 401}, output.TypeAuth, output.ExitAuth},
		// 429 also guards the dropped `>= 500` gate, which let every other
		// status fall through to the bare-error path.
		{"other statuses stay api", &client.HTTPError{StatusCode: 429, Body: `{"message":"slow down"}`}, output.TypeAPI, output.ExitAPI},
		{"unreachable network", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, output.TypeNetwork, output.ExitNetwork},
		{"anything else is ours", errors.New("boom"), output.TypeInternal, output.ExitInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ee *output.ExitError
			if !errors.As(classifyPullDownloadErr(tc.err, "t_1"), &ee) {
				t.Fatal("a bare error escapes RunE and is reported as a usage error")
			}
			if ee.Detail.Type != tc.wantType || ee.Code != tc.wantCode {
				t.Errorf("got %s/exit %d, want %s/exit %d", ee.Detail.Type, ee.Code, tc.wantType, tc.wantCode)
			}
		})
	}
}

// A 5xx carries the handles triage needs: the request id feeds the server-side
// log lookup, the endpoint says which call produced it.
func TestClassifyPullDownloadErr_APICarriesTriageContext(t *testing.T) {
	he := &client.HTTPError{
		StatusCode: 503, Body: `{"message":"upstream unavailable"}`,
		Method: "GET", Path: "/themes/t_1/download", RequestID: "req-1",
	}
	var ee *output.ExitError
	if !errors.As(classifyPullDownloadErr(he, "t_1"), &ee) {
		t.Fatal("want an *output.ExitError")
	}
	if ee.Detail.Type != output.TypeAPI {
		t.Errorf("a 5xx is api-class, got %s", ee.Detail.Type)
	}
	got := ee.Detail.Detail
	if got == nil || got.RequestID != "req-1" || got.StatusCode != 503 ||
		got.Method != "GET" || got.Path != "/themes/t_1/download" {
		t.Errorf("triage context = %+v, want request id, status and endpoint", got)
	}
}

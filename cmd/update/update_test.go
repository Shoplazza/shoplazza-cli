package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestNewCmdUpdate_Structure(t *testing.T) {
	cmd := NewCmdUpdate(nil)
	if cmd.Use != "update" {
		t.Errorf("Use = %q, want update", cmd.Use)
	}
	if cmd.Flags().Lookup("check") == nil {
		t.Error("expected --check flag")
	}
}

func TestUpToDate(t *testing.T) {
	cases := []struct {
		latest, current string
		err             error
		want            bool
	}{
		{"2.0.1", "2.0.1", nil, true},         // equal → up to date
		{"2.0.2", "2.0.1", nil, false},        // newer available
		{"2.0.1", "2.0.2", nil, true},         // local ahead → up to date
		{"v2.0.1", "2.0.1", nil, true},        // v-prefix handled by IsNewer
		{"", "2.0.1", errors.New("x"), false}, // lookup failed → allow update
		{"", "2.0.1", nil, false},             // empty latest → allow update
	}
	for _, c := range cases {
		if got := upToDate(c.latest, c.current, c.err); got != c.want {
			t.Errorf("upToDate(%q,%q,%v)=%v want %v", c.latest, c.current, c.err, got, c.want)
		}
	}
}

// fakeOps records install invocations and serves canned latest-version results.
type fakeOps struct {
	latestVer     string
	latestErr     error
	installCalled bool
	installErr    error
}

func (f *fakeOps) build() npmOps {
	return npmOps{
		lookPath: func() (string, error) { return "npm", nil },
		latest:   func(context.Context, string) (string, error) { return f.latestVer, f.latestErr },
		install: func(_ context.Context, _ string, out io.Writer) error {
			f.installCalled = true
			io.WriteString(out, "changed 1 package\n")
			return f.installErr
		},
	}
}

func decodeBody(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("stdout is not JSON: %v (%s)", err, b)
	}
	return m
}

func TestRunUpdate_AlreadyLatest_SkipsInstall(t *testing.T) {
	f := &fakeOps{latestVer: "2.0.1"}
	var out, errW bytes.Buffer
	if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.installCalled {
		t.Error("install must not run when already up to date")
	}
	if !strings.Contains(errW.String(), "up to date") {
		t.Errorf("stderr should report up to date; got %q", errW.String())
	}
	if decodeBody(t, out.Bytes())["updated"] != false {
		t.Errorf("body.updated should be false; got %s", out.String())
	}
}

func TestRunUpdate_UpdateAvailable_RunsInstallAndReports(t *testing.T) {
	f := &fakeOps{latestVer: "2.0.2"}
	var out, errW bytes.Buffer
	if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.installCalled {
		t.Error("install must run when an update is available")
	}
	es := errW.String()
	if !strings.Contains(es, "Updating "+npmPackage) {
		t.Errorf("stderr should show the spinner label; got %q", es)
	}
	if !strings.Contains(es, "Updated") || !strings.Contains(es, "2.0.2") {
		t.Errorf("stderr should report the new version; got %q", es)
	}
	body := decodeBody(t, out.Bytes())
	if body["updated"] != true || body["latest"] != "2.0.2" || body["previous"] != "2.0.1" {
		t.Errorf("body mismatch: %s", out.String())
	}
}

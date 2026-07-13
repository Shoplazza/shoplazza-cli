package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/cmdutil"
)

// runAuthCmd runs the auth command tree with args, capturing stdout, and
// fails the test on any RunE error.
func runAuthCmd(t *testing.T, f *cmdutil.Factory, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	cmd := NewCmdAuth(f)
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth %v: unexpected error: %v", args, err)
	}
	return buf.String()
}

// GATE-09 (display surface): auth status's tokenStatus tri-state, for the
// current profile.
func TestStatus_TokenStates(t *testing.T) {
	f := seedLoggedInWithProfiles(t, "alice@co.com", "us")
	for _, tc := range []struct {
		name, wantStatus string
		expiresAt        time.Time
		seedToken        bool
	}{
		{"valid", "valid", time.Now().Add(time.Hour), true},
		{"expired", "expired", time.Now().Add(-time.Hour), true},
		{"absent", "absent", time.Time{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.seedToken {
				seedProfileToken(t, internalauth.AuthDir(f.ConfigPath), "us", "at-x", tc.expiresAt)
			} else {
				_ = internalauth.RemoveProfileMeta(internalauth.AuthDir(f.ConfigPath), "us")
			}
			out := runAuthCmd(t, f, "status")
			var got map[string]any
			_ = json.Unmarshal([]byte(out), &got)
			if got["token_status"] != tc.wantStatus || got["profile"] != "us" {
				t.Fatalf("status = %v", got)
			}
		})
	}
}

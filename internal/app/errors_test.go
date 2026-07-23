package app

import (
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func TestApiOrInternal_HTTPError_RoutesToErrAPI_WithEndpoint(t *testing.T) {
	err := apiOrInternal(&client.HTTPError{
		StatusCode: 500, Body: `{"message":"boom"}`, Method: "GET", Path: "/api/cli/v2/partners",
	})
	if err.Code != output.ExitAPI {
		t.Fatalf("exit code = %d, want ExitAPI (%d)", err.Code, output.ExitAPI)
	}
	if err.Detail == nil || err.Detail.Detail == nil {
		t.Fatalf("expected endpoint detail, got %+v", err.Detail)
	}
	if err.Detail.Detail.Method != "GET" || err.Detail.Detail.Path != "/api/cli/v2/partners" {
		t.Fatalf("endpoint = %q %q, want GET /api/cli/v2/partners",
			err.Detail.Detail.Method, err.Detail.Detail.Path)
	}
}

// TestApiOrInternal_Forbidden_RoutesToAuth pins the 403→auth reclassification
// passing through ErrAPI (exit 3).
func TestApiOrInternal_Forbidden_RoutesToAuth(t *testing.T) {
	err := apiOrInternal(&client.HTTPError{StatusCode: 403, Body: `{"message":"forbidden"}`})
	if err.Code != output.ExitAuth {
		t.Fatalf("exit code = %d, want ExitAuth (%d)", err.Code, output.ExitAuth)
	}
}

// TestApiOrInternal_NetError_RoutesToErrNetwork: a transport-level
// failure (refused dial, DNS, timeout) must classify as network (exit 4), not
// internal (exit 5). Both the bare *net.OpError and the *url.Error wrapper the
// http client actually returns must match.
func TestApiOrInternal_NetError_RoutesToErrNetwork(t *testing.T) {
	opErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	for name, cause := range map[string]error{
		"bare op error":     opErr,
		"url.Error wrapper": &url.Error{Op: "Get", URL: "https://x.example.com", Err: opErr},
		"wrapped via %w":    &url.Error{Op: "Post", URL: "https://x.example.com", Err: opErr},
	} {
		err := apiOrInternal(cause)
		if err.Code != output.ExitNetwork {
			t.Errorf("%s: exit code = %d, want ExitNetwork (%d)", name, err.Code, output.ExitNetwork)
		}
		if err.Detail == nil || err.Detail.Type != output.TypeNetwork {
			t.Errorf("%s: type = %+v, want %q", name, err.Detail, output.TypeNetwork)
		}
	}
}

func TestApiOrInternal_OtherError_RoutesToErrInternal(t *testing.T) {
	err := apiOrInternal(errors.New("json: cannot unmarshal"))
	if err.Code != output.ExitInternal {
		t.Fatalf("exit code = %d, want ExitInternal (%d)", err.Code, output.ExitInternal)
	}
}

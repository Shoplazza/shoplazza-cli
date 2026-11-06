package docs

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

func TestMain(m *testing.M) { testenv.RunMainIsolated(m) }

// testClient sends every request, search and page reads alike, to srv.
func testClient(srv *httptest.Server) *Client {
	c := NewClient("v9.9.9")
	c.SearchAPI = srv.URL
	c.ClientID = "11111111-1111-4111-8111-111111111111"
	c.HTTP = &http.Client{Transport: testenv.RewriteTransport(srv)}
	return c
}

func TestNewClient(t *testing.T) {
	t.Setenv(EnvSearchAPI, "")
	c := NewClient("v2.3.0")
	if c.SearchAPI != DefaultSearchAPI || c.UserAgent != "shoplazza-cli/2.3.0" || c.HTTP.Timeout == 0 {
		t.Errorf("client = %+v", c)
	}
	t.Setenv(EnvSearchAPI, " https://docs.example.com/ ")
	if got := NewClient("dev").SearchAPI; got != "https://docs.example.com" {
		t.Errorf("env override = %q", got)
	}
}

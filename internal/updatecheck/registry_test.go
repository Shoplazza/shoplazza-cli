package updatecheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLatest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"2.5.0","name":"shoplazza-cli"}`))
	}))
	defer ts.Close()
	origURL, origClient := registryURL, DefaultClient
	registryURL, DefaultClient = ts.URL, ts.Client()
	defer func() { registryURL, DefaultClient = origURL, origClient }()

	got, err := fetchLatest()
	if err != nil {
		t.Fatalf("fetchLatest error: %v", err)
	}
	if got != "2.5.0" {
		t.Errorf("got %q want 2.5.0", got)
	}
}

func TestFetchLatest_Errors(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"non-200":   func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"bad-json":  func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not json")) },
		"empty-ver": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"version":""}`)) },
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			ts := httptest.NewServer(h)
			defer ts.Close()
			origURL, origClient := registryURL, DefaultClient
			registryURL, DefaultClient = ts.URL, ts.Client()
			defer func() { registryURL, DefaultClient = origURL, origClient }()
			if _, err := fetchLatest(); err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}

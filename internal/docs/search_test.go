package docs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeAPIVersion(t *testing.T) {
	for in, want := range map[string]string{
		"2026-07": "202607", "202607": "202607", " 2026-01 ": "202601", "": "", "latest": "latest",
	} {
		if got := NormalizeAPIVersion(in); got != want {
			t.Errorf("NormalizeAPIVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSearch(t *testing.T) {
	var header http.Header
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, `{"locale":"zh-CN","results":[
			{"url":"/zh-CN/docs/app/oauth","title":"OAuth","content":"a","score":0.86},
			{"url":"/zh-CN/docs/app/oauth","title":"OAuth","content":"b","score":0.8},
			{"url":"https://elsewhere.example/x","title":"T","content":"c","score":0.5}]}`)
	}))
	defer srv.Close()

	res, err := testClient(srv).Search(context.Background(), SearchRequest{Query: "oauth 授权", APIVersion: "202601"})
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"X-Client-Id": "11111111-1111-4111-8111-111111111111", "X-Client-Surface": "cli",
		"User-Agent": "shoplazza-cli/9.9.9", "Content-Type": "application/json",
	} {
		if header.Get(k) != want {
			t.Errorf("header %s = %q, want %q", k, header.Get(k), want)
		}
	}
	// Unset fields stay off the wire so the service applies its defaults.
	if len(body) != 2 || body["query"] != "oauth 授权" || body["api_version"] != "202601" {
		t.Errorf("body = %v", body)
	}

	results := res["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("passages of one page must stay separate, got %d results", len(results))
	}
	first := results[0].(map[string]any)
	if first["url"] != SiteURL+"/zh-CN/docs/app/oauth" || first["score"] != json.Number("0.86") {
		t.Errorf("first = %v", first)
	}
	if results[2].(map[string]any)["url"] != "https://elsewhere.example/x" {
		t.Errorf("absolute url must pass through: %v", results[2])
	}
}

func TestSearch_MissingResultsBecomesEmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"locale":"en"}`)
	}))
	defer srv.Close()
	res, err := testClient(srv).Search(context.Background(), SearchRequest{Query: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if list, ok := res["results"].([]any); !ok || len(list) != 0 {
		t.Errorf("results = %#v, want an empty list", res["results"])
	}
}

func TestSearch_HTTPError(t *testing.T) {
	cases := []struct {
		name, header, body, wantTrace string
	}{
		{"trace from header", "h-1", `{"code":"rate_limited","trace_id":"b-1"}`, "h-1"},
		{"trace from body", "", `{"code":"rate_limited","trace_id":"b-1"}`, "b-1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if c.header != "" {
					w.Header().Set("X-Trace-Id", c.header)
				}
				w.Header().Set("Retry-After", "17")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, c.body)
			}))
			defer srv.Close()
			_, err := testClient(srv).Search(context.Background(), SearchRequest{Query: "x"})
			var he *HTTPError
			if !errors.As(err, &he) {
				t.Fatalf("err = %v, want *HTTPError", err)
			}
			if he.StatusCode != 429 || he.RetryAfter != "17" || he.TraceID != c.wantTrace || he.Method != "POST" || he.Path != SearchPath {
				t.Errorf("HTTPError = %+v", he)
			}
		})
	}
}

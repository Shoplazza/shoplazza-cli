package docscmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/docs"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

func TestMain(m *testing.M) { testenv.RunMainIsolated(m) }

// withServer points the commands at handler and returns its request count.
func withServer(t *testing.T, handler http.HandlerFunc) *int {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	setClient(t, func(c *docs.Client) { c.HTTP = &http.Client{Transport: testenv.RewriteTransport(srv)} })
	return &hits
}

func setClient(t *testing.T, tweak func(*docs.Client)) {
	t.Helper()
	prev := newClient
	newClient = func() *docs.Client {
		c := docs.NewClient("1.0.0")
		c.ClientID = "11111111-1111-4111-8111-111111111111"
		tweak(c)
		return c
	}
	t.Cleanup(func() { newClient = prev })
}

// run executes `docs <args>` under a root carrying the global --format flag.
func run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := &cobra.Command{Use: "shoplazza", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("format", "json", "")
	root.AddCommand(NewCmdDocs())
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"docs"}, args...))
	err = root.Execute()
	return out.String(), errOut.String(), err
}

// fail returns err's envelope and error.detail.
func fail(t *testing.T, err error) (map[string]any, *output.ErrorContext) {
	t.Helper()
	env := testenv.ErrEnvelope(t, err)
	var ctx *output.ErrorContext
	if e, ok := err.(*output.ExitError); ok && e.Detail != nil {
		ctx = e.Detail.Detail
	}
	return env, ctx
}

func data(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var env struct {
		OK   bool
		Data map[string]any
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil || !env.OK {
		t.Fatalf("stdout is not a success envelope: %v\n%s", err, stdout)
	}
	return env.Data
}

const searchOK = `{"locale":"en","results":[
	{"url":"/api/product-create","title":"Create product","content":"{\"part\":\"request\"}","score":0.9},
	{"url":"/api/product-create","title":"Create product","content":"{\"part\":\"response200\"}","score":0.8}]}`

func TestSearch(t *testing.T) {
	var body map[string]any
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, searchOK)
	})
	out, stderr, err := run(t, "search", "--query", " product-create ", "--locale", "zh-CN", "--api-version", "2026-01", "--limit", "3")
	if err != nil {
		t.Fatal(err)
	}
	if body["query"] != "product-create" || body["locale"] != "zh-CN" || body["api_version"] != "202601" || body["limit"] != float64(3) {
		t.Errorf("request body = %v", body)
	}
	results := data(t, out)["results"].([]any)
	if len(results) != 2 || results[0].(map[string]any)["url"] != docs.SiteURL+"/api/product-create" {
		t.Errorf("results = %v", results)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty on a hit", stderr)
	}

	// ndjson streams one passage per line, same-page passages included.
	out, _, err = run(t, "search", "--query", "x", "--format", "ndjson")
	if lines := strings.Split(strings.TrimSpace(out), "\n"); err != nil || len(lines) != 2 {
		t.Errorf("ndjson: %v, %d line(s):\n%s", err, len(lines), out)
	}
}

func TestSearch_NoResults(t *testing.T) {
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"locale":"en","results":[]}`)
	})
	out, stderr, err := run(t, "search", "--query", "zzz")
	if err != nil || !strings.Contains(out, `"results": []`) || !strings.Contains(stderr, "hint: nothing relevant found") {
		t.Errorf("err %v\nstdout %s\nstderr %s", err, out, stderr)
	}
}

func TestSearch_LocalValidation(t *testing.T) {
	hits := withServer(t, func(w http.ResponseWriter, r *http.Request) {})
	cases := []struct {
		args          []string
		subtype, hint string
	}{
		{[]string{"--query", "x", "--locale", "zh"}, output.SubtypeInvalidFlagValue, "en, zh-CN"},
		{[]string{"--query", "x", "--limit", "0"}, output.SubtypeInvalidFlagValue, "1 to 20"},
		{[]string{"--query", "x", "--limit", "21"}, output.SubtypeInvalidFlagValue, "1 to 20"},
		{[]string{"--query", "   "}, output.SubtypeInvalidFlagValue, "--query"},
		{[]string{"oauth"}, output.SubtypeInvalidArgument, `--query "oauth"`},
	}
	for _, c := range cases {
		_, _, err := run(t, append([]string{"search"}, c.args...)...)
		env, _ := fail(t, err)
		if env["type"] != output.TypeValidation || env["subtype"] != c.subtype || !strings.Contains(env["hint"].(string), c.hint) {
			t.Errorf("%v → %v", c.args, env)
		}
	}
	if *hits != 0 {
		t.Errorf("%d request(s) sent for invalid flags", *hits)
	}
}

func TestSearch_ErrorMapping(t *testing.T) {
	cases := []struct {
		name, body, retryAfter string
		status                 int
		wantType, wantHint     string
	}{
		{"invalid version", `{"code":"invalid_request","message":"api_version must be one of 202201, 202607","trace_id":"t-1"}`, "", 400, output.TypeValidation, "accepted values"},
		{"rate limited", `{"code":"rate_limited","message":"too many requests","trace_id":"t-1"}`, "42", 429, output.TypeAPI, "42 seconds"},
		{"server error", `{"code":"internal","message":"internal error","trace_id":"t-1"}`, "", 500, output.TypeAPI, ""},
		{"forbidden needs no login", `{"code":"blocked","message":"blocked","trace_id":"t-1"}`, "", 403, output.TypeAPI, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			withServer(t, func(w http.ResponseWriter, r *http.Request) {
				if c.retryAfter != "" {
					w.Header().Set("Retry-After", c.retryAfter)
				}
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, c.body)
			})
			_, _, err := run(t, "search", "--query", "x")
			env, ctx := fail(t, err)
			hint, _ := env["hint"].(string)
			if env["type"] != c.wantType || !strings.Contains(hint, c.wantHint) {
				t.Errorf("envelope = %v", env)
			}
			if ctx == nil || ctx.StatusCode != c.status || ctx.RequestID != "t-1" || ctx.Path != docs.SearchPath {
				t.Errorf("detail = %+v", ctx)
			}
		})
	}
}

func TestSearch_Unreachable(t *testing.T) {
	setClient(t, func(c *docs.Client) { c.SearchAPI = "http://127.0.0.1:1" })
	_, _, err := run(t, "search", "--query", "x")
	if env, _ := fail(t, err); env["code"] != output.ExitNetwork {
		t.Errorf("envelope = %v, want network-class", env)
	}
}

const pageMD = "**Version: 202607**\n\n# Create product\n\nline one\nline two\nline three\n"

func pageServer(t *testing.T) *int {
	return withServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/product-create.md":
			_, _ = io.WriteString(w, pageMD)
		case "/docs/moved.md":
			http.Redirect(w, r, "https://evil.example.com/x.md", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	})
}

func TestGet(t *testing.T) {
	pageServer(t)
	out, stderr, err := run(t, "get", "--url", "https://www.shoplazza.dev/api/product-create/#request")
	if err != nil {
		t.Fatal(err)
	}
	d := data(t, out)
	want := map[string]any{
		"url": docs.SiteURL + "/api/product-create", "title": "Create product", "locale": "en",
		"markdown": pageMD, "total_chars": float64(len(pageMD)), "truncated": false,
	}
	for k, v := range want {
		if d[k] != v {
			t.Errorf("data.%s = %#v, want %#v", k, d[k], v)
		}
	}
	if stderr != "" {
		t.Errorf("stderr = %q", stderr)
	}

	out, _, _ = run(t, "get", "--url", "/api/product-create.md", "--format", "pretty")
	if out != pageMD {
		t.Errorf("pretty = %q, want the raw Markdown", out)
	}
}

func TestGet_Truncates(t *testing.T) {
	pageServer(t)
	out, stderr, err := run(t, "get", "--url", "/api/product-create", "--max-chars", "50")
	if err != nil {
		t.Fatal(err)
	}
	d := data(t, out)
	md, _ := d["markdown"].(string)
	if len(md) > 50 || !strings.HasSuffix(md, "\n") || d["truncated"] != true || d["total_chars"] != float64(len(pageMD)) {
		t.Errorf("data = %v", d)
	}
	if !strings.Contains(stderr, "--output") {
		t.Errorf("stderr = %q, want the --output hint", stderr)
	}
}

func TestGet_Output(t *testing.T) {
	pageServer(t)
	file := filepath.Join(t.TempDir(), "page.md")
	// --max-chars neither cuts nor is checked with --output.
	out, stderr, err := run(t, "get", "--url", "/api/product-create", "--output", file, "--max-chars", "0")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); string(b) != pageMD {
		t.Errorf("file = %q, want the whole page", b)
	}
	d := data(t, out)
	if _, has := d["markdown"]; has || d["file"] != file || d["total_chars"] != float64(len(pageMD)) || stderr != "" {
		t.Errorf("data = %v, stderr = %q", d, stderr)
	}
}

func TestGet_Errors(t *testing.T) {
	hits, sent := pageServer(t), 0
	cases := []struct {
		args              []string
		wantType, contain string
		sends             bool
	}{
		{[]string{"--url", "https://evil.example.com/docs/x"}, output.TypeValidation, "not on www.shoplazza.dev", false},
		{[]string{"--url", "docs/x"}, output.TypeValidation, "site path", false},
		{[]string{"--url", "/docs/x", "--max-chars", "0"}, output.TypeValidation, "--max-chars", false},
		{[]string{"--url", "/docs/moved"}, output.TypeValidation, "evil.example.com", true},
		{[]string{"--url", "/docs/gone"}, output.TypeAPI, "page not found", true},
	}
	for _, c := range cases {
		_, _, err := run(t, append([]string{"get"}, c.args...)...)
		env, ctx := fail(t, err)
		if env["type"] != c.wantType || !strings.Contains(env["message"].(string), c.contain) {
			t.Errorf("%v → %v", c.args, env)
		}
		if c.wantType == output.TypeAPI && (ctx == nil || ctx.StatusCode != 404 || !strings.Contains(env["hint"].(string), "docs search")) {
			t.Errorf("404 detail %+v, envelope %v", ctx, env)
		}
		if c.sends {
			sent++
		}
	}
	if *hits != sent {
		t.Errorf("%d request(s) reached the site, want %d: invalid input must fail before sending", *hits, sent)
	}
}

func TestDocsHelpPointsAtSchema(t *testing.T) {
	if !strings.Contains(NewCmdDocs().Long, "shoplazza schema") {
		t.Error("docs --help must point at schema")
	}
}

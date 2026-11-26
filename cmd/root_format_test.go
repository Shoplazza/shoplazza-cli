package cmd

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

// TestDefaultOutputFormat: SHOPLAZZA_CLI_FORMAT sets the --format default when
// valid; unset or invalid falls back to json.
func TestDefaultOutputFormat(t *testing.T) {
	t.Setenv("SHOPLAZZA_CLI_FORMAT", "")
	if got := defaultOutputFormat(); got != output.FormatJSON {
		t.Errorf("unset = %q, want json", got)
	}
	t.Setenv("SHOPLAZZA_CLI_FORMAT", "pretty")
	if got := defaultOutputFormat(); got != output.FormatPretty {
		t.Errorf("pretty env = %q, want pretty", got)
	}
	t.Setenv("SHOPLAZZA_CLI_FORMAT", "bogus")
	if got := defaultOutputFormat(); got != output.FormatJSON {
		t.Errorf("invalid env = %q, want json fallback", got)
	}
}

func TestNameMissingScope(t *testing.T) {
	const generic = "the access token lacks a scope this endpoint needs"
	cases := []struct {
		name, envToken, method, path string
		wantScopes                   []string
		wantMessage, wantHint        string
	}{
		{"saved login, mapped", "", "POST", "/openapi/2026-01/redirects/detail",
			[]string{"read_shop_navigation", "write_shop_navigation"},
			"the access token lacks read_shop_navigation or write_shop_navigation",
			"run 'shoplazza auth login --scope read_shop_navigation,write_shop_navigation --merge-scopes' to add them"},
		{"injected token, mapped", "tok", "GET", "/openapi/2026-01/discounts",
			[]string{"read_price_rules", "write_price_rules"},
			"the access token lacks read_price_rules or write_price_rules",
			"the token in SHOPLAZZA_ACCESS_TOKEN lacks them: use a token granted read_price_rules and write_price_rules"},
		{"saved login, unmapped", "", "GET", "/openapi/2025-06/script_tags", nil, generic,
			"run 'shoplazza auth scopes' to see the granted scopes, then add the missing one with 'shoplazza auth login --scope <scope> --merge-scopes'"},
		{"injected token, unmapped", "tok", "GET", "/openapi/2025-06/script_tags", nil, generic,
			"the token in SHOPLAZZA_ACCESS_TOKEN lacks it: use a token granted the scope this endpoint needs"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.envToken == "" {
				testenv.Unsetenv(t, cmdutil.EnvAccessToken)
			} else {
				t.Setenv(cmdutil.EnvAccessToken, c.envToken)
			}
			e := output.ErrAPI(403, `{}`, "req-1").WithEndpoint(c.method, c.path)
			nameMissingScope(e)
			got, _ := e.Detail.Extra["required_scopes"].([]string)
			if strings.Join(got, " ") != strings.Join(c.wantScopes, " ") || e.Detail.Message != c.wantMessage || e.Detail.Hint != c.wantHint {
				t.Errorf("required_scopes %q, message %q, hint %q; want %q, %q, %q", got, e.Detail.Message, e.Detail.Hint, c.wantScopes, c.wantMessage, c.wantHint)
			}
		})
	}
}

// TestWantsNoInput: --no-input is detected before the -- terminator.
func TestWantsNoInput(t *testing.T) {
	if !wantsNoInput([]string{"themes", "env", "add", "--no-input"}) {
		t.Error("should detect --no-input")
	}
	if !wantsNoInput([]string{"--no-input=true"}) {
		t.Error("should detect --no-input=true")
	}
	if wantsNoInput([]string{"themes", "list"}) {
		t.Error("absent → false")
	}
	if wantsNoInput([]string{"--", "--no-input"}) {
		t.Error("after -- it is an operand, not a flag")
	}
	// All truthy pflag bool forms enable it.
	for _, tv := range []string{"--no-input=1", "--no-input=t", "--no-input=TRUE", "--no-input=True"} {
		if !wantsNoInput([]string{tv}) {
			t.Errorf("%q should enable no-input", tv)
		}
	}
	// Falsy forms do not.
	for _, fv := range []string{"--no-input=false", "--no-input=0", "--no-input=F"} {
		if wantsNoInput([]string{fv}) {
			t.Errorf("%q must NOT enable no-input", fv)
		}
	}
}

// TestAutoPretty: pretty only when stdout is a terminal and no automation signal.
func TestAutoPretty(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	if !autoPretty(true, none, nil) {
		t.Error("terminal + no signals → pretty")
	}
	if autoPretty(false, none, nil) {
		t.Error("non-terminal → json")
	}
	// An empty CI value must not gate (treated as unset).
	ciEmpty := func(k string) (string, bool) { return "", k == "CI" }
	if !autoPretty(true, ciEmpty, nil) {
		t.Error("empty CI must not gate → pretty")
	}
	ciSet := func(k string) (string, bool) {
		if k == "CI" {
			return "1", true
		}
		return "", false
	}
	if autoPretty(true, ciSet, nil) {
		t.Error("CI=1 → json even on a terminal")
	}
	niSet := func(k string) (string, bool) {
		if k == "SHOPLAZZA_CLI_NO_INTERACTIVE" {
			return "1", true
		}
		return "", false
	}
	if autoPretty(true, niSet, nil) {
		t.Error("SHOPLAZZA_CLI_NO_INTERACTIVE=1 → json")
	}
	if autoPretty(true, none, []string{"--no-input"}) {
		t.Error("--no-input → json")
	}
}

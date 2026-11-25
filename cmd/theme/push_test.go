package themecmd

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/env"
)

// TestResolveThemeID pins the precedence: explicit flag > environment theme= >
// (interactive pick). Non-interactively with nothing to go on it returns "" so
// theme.RequireThemeID emits the structured "required" error.
func TestResolveThemeID(t *testing.T) {
	f := &cmdutil.Factory{} // nil IOStreams → non-interactive

	// explicit flag wins over the environment.
	got, err := resolveThemeID(context.Background(), f, resolvedStore{Env: env.Environment{Theme: "env-theme"}}, "flag-theme")
	if err != nil || got != "flag-theme" {
		t.Fatalf("explicit flag: got %q, %v", got, err)
	}

	// environment theme= fills an unset flag.
	got, err = resolveThemeID(context.Background(), f, resolvedStore{Env: env.Environment{Theme: "env-theme"}}, "")
	if err != nil || got != "env-theme" {
		t.Fatalf("env theme: got %q, %v", got, err)
	}

	// nothing set, non-interactive → "" (no prompt, no network).
	got, err = resolveThemeID(context.Background(), f, resolvedStore{}, "")
	if err != nil || got != "" {
		t.Fatalf("empty non-interactive: got %q, %v", got, err)
	}
}

// TestDigThemes covers the list-response shapes the picker tolerates.
func TestDigThemes(t *testing.T) {
	for _, tc := range []struct {
		name string
		body any
		want []string // ids in order
	}{
		{"top-level themes", map[string]any{"themes": []any{m("1"), m("2")}}, []string{"1", "2"}},
		{"data.themes", map[string]any{"data": map[string]any{"themes": []any{m("3")}}}, []string{"3"}},
		{"data array", map[string]any{"data": []any{m("4")}}, []string{"4"}},
		{"bare array", []any{m("5")}, []string{"5"}},
		{"not a list", map[string]any{"nope": 1}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arr, _ := digThemes(tc.body)
			var ids []string
			for _, r := range arr {
				ids = append(ids, asString(r["id"]))
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Errorf("ids = %v, want %v", ids, tc.want)
			}
		})
	}
}

func m(id string) map[string]any { return map[string]any{"id": id} }

func TestClassifyHTTPErr(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantCode    int
		wantSubtype string
	}{
		{"400 Unauthorized", 400, `"Unauthorized"`, output.ExitAuth, output.SubtypeInvalidToken},
		{"400 bad input", 400, `{"message":"bad input"}`, output.ExitValidation, ""},
		{"403", 403, `{}`, output.ExitAuth, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := classifyHTTPErr(&client.HTTPError{StatusCode: c.status, Body: c.body, Method: "PUT", Path: "/openapi/2026-01/themes/t1"}, "t1")
			var ee *output.ExitError
			if !errors.As(err, &ee) || ee.Code != c.wantCode || ee.Detail.Subtype != c.wantSubtype {
				t.Fatalf("got %#v, want exit %d subtype %q", err, c.wantCode, c.wantSubtype)
			}
		})
	}
}

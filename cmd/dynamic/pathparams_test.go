package dynamic

import (
	"reflect"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
)

func TestPathParamNames(t *testing.T) {
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/openapi/2026-01/themes", nil},
		{"/openapi/2026-01/themes/{theme_id}", []string{"theme_id"}},
		{"/openapi/2026-01/themes/{theme_id}/doc/versions/{version_id}", []string{"theme_id", "version_id"}},
		{"/openapi/2026-01/themes/{theme_id}/publish", []string{"theme_id"}},
	} {
		if got := pathParamNames(tc.path); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("pathParamNames(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestDropHashPrefix(t *testing.T) {
	byNumber := registry.Command{ID: "order-by-number"}
	for _, c := range []struct {
		name     string
		cmd      registry.Command
		value    any
		want     any
		wantHint bool
	}{
		{"display prefix", byNumber, "#1001", "1001", true},
		{"spaces around", byNumber, " # KEX44398 ", "KEX44398", true},
		{"only one # dropped", byNumber, "##1001", "#1001", true},
		{"no prefix", byNumber, "1001", "1001", false},
		{"not a string", byNumber, 1001.0, 1001.0, false},
		{"other command", registry.Command{ID: "discount-get-by-code"}, "#SAVE", "#SAVE", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			params := map[string]any{"number": c.value}
			hint := dropHashPrefix(c.cmd, params)
			if params["number"] != c.want || (hint != "") != c.wantHint {
				t.Errorf("number = %#v, hint = %q; want %#v, hint %v", params["number"], hint, c.want, c.wantHint)
			}
		})
	}
}

// TestFillMissingPathParams_NonInteractiveNoOp pins that without a terminal the
// fill leaves params untouched — a missing {param} must stay the structured
// ResolveTemplatedPath error, not become a prompt (agents/CI never block).
func TestFillMissingPathParams_NonInteractiveNoOp(t *testing.T) {
	params := map[string]any{"theme_id": "123"}
	// nil IOStreams → non-interactive; must return nil and not touch params.
	if err := fillMissingPathParams(nil, &cmdutil.Factory{}, "/themes/{theme_id}/doc/{version_id}", params); err != nil {
		t.Fatalf("non-interactive fill must be a no-op, got %v", err)
	}
	if len(params) != 1 || params["theme_id"] != "123" {
		t.Errorf("params mutated off a non-terminal: %v", params)
	}
}

package dynamic

import (
	"reflect"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
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

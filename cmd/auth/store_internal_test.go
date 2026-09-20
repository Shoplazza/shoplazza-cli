package auth

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
)

// configuredStoreOptions: one option per distinct store domain, the current one
// marked, blanks and duplicates dropped.
func TestConfiguredStoreOptions(t *testing.T) {
	cfg := core.CliConfig{
		CurrentProfile: "b",
		Profiles: []core.ProfileConfig{
			{Name: "a", StoreDomain: "a.myshoplaza.com"},
			{Name: "b", StoreDomain: "b.myshoplaza.com"},
			{Name: "b2", StoreDomain: "b.myshoplaza.com"}, // duplicate domain → dropped
			{Name: "n", StoreDomain: ""},                  // no domain → dropped
		},
	}
	opts := configuredStoreOptions(cfg)
	if len(opts) != 2 {
		t.Fatalf("want 2 distinct stores, got %d: %+v", len(opts), opts)
	}
	got := map[string]string{}
	for _, o := range opts {
		got[o.Value] = o.Label
	}
	if _, ok := got["a.myshoplaza.com"]; !ok {
		t.Errorf("missing store a; got %+v", opts)
	}
	if !strings.Contains(got["b.myshoplaza.com"], "(current)") {
		t.Errorf("current store label = %q, want it marked (current)", got["b.myshoplaza.com"])
	}
}

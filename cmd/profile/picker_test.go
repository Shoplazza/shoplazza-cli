package profile

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
)

func TestConfiguredProfileOptions(t *testing.T) {
	cfg := core.CliConfig{
		CurrentProfile: "prod",
		Profiles: []core.ProfileConfig{
			{Name: "dev", StoreDomain: "dev.myshoplaza.com"},
			{Name: "prod", StoreDomain: "prod.myshoplaza.com"},
		},
	}
	opts := configuredProfileOptions(cfg)
	if len(opts) != 2 {
		t.Fatalf("want 2 options, got %d: %+v", len(opts), opts)
	}
	label := map[string]string{}
	for _, o := range opts {
		label[o.Value] = o.Label
	}
	if !strings.Contains(label["dev"], "dev.myshoplaza.com") {
		t.Errorf("dev label %q should carry its store domain", label["dev"])
	}
	if !strings.Contains(label["prod"], "(current)") {
		t.Errorf("current profile label %q should be marked (current)", label["prod"])
	}
	if strings.Contains(label["dev"], "(current)") {
		t.Errorf("non-current profile must not be marked: %q", label["dev"])
	}
}

// Non-interactively (agent/pipe), use/remove with no --name must fail fast with
// the required-flag error — never open the picker.
func TestUse_NonInteractive_RequiresName(t *testing.T) {
	f := newTestFactory(t, "http://unused")
	if _, err := execProfile(f, "use"); err == nil || !strings.Contains(err.Error(), "--name or --previous is required") {
		t.Errorf("want required error, got %v", err)
	}
}

func TestRemove_NonInteractive_RequiresName(t *testing.T) {
	f := newTestFactory(t, "http://unused")
	if _, err := execProfile(f, "remove"); err == nil || !strings.Contains(err.Error(), "--name is required") {
		t.Errorf("want required error, got %v", err)
	}
}

// Non-interactively, rename with no target must error (never open a picker/input).
func TestRename_NonInteractive_Errors(t *testing.T) {
	f := newTestFactory(t, "http://unused")
	if _, err := execProfile(f, "rename"); err == nil {
		t.Error("non-interactive rename with no target must error, not prompt")
	}
}

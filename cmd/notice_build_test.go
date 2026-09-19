package cmd

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/updatecheck"
)

func TestBuildNotice_OptOut(t *testing.T) {
	t.Setenv(EnvNoNotice, "1")
	if got := buildNotice(&updatecheck.Info{Current: "1.0.0", Latest: "2.0.0"}); got != nil {
		t.Errorf("opt-out must return nil, got %v", got)
	}
}

func TestBuildNotice_UpdateEntry(t *testing.T) {
	t.Setenv(EnvNoNotice, "") // ensure not opted out
	n := buildNotice(&updatecheck.Info{Current: "1.0.0", Latest: "2.0.0"})
	if n == nil {
		t.Fatal("expected a notice with an update entry")
	}
	up, ok := n["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected update block, got %v", n["update"])
	}
	if up["latest"] != "2.0.0" || up["current"] != "1.0.0" {
		t.Errorf("update block = %v", up)
	}
}

func TestBuildNotice_NilUpdate_NoUpdateEntry(t *testing.T) {
	t.Setenv(EnvNoNotice, "")
	n := buildNotice(nil)
	// n may still carry a skills entry depending on host state; it must never
	// carry an update entry when there's no pending update.
	if n != nil {
		if _, ok := n["update"]; ok {
			t.Errorf("no update entry expected, got %v", n["update"])
		}
	}
}

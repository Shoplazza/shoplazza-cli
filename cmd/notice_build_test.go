package cmd

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
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

// The skills entry nudges an install; a plugin install counts as one.
func TestBuildNotice_SkillsEntry(t *testing.T) {
	t.Setenv(EnvNoNotice, "")
	testenv.IsolateSkillsDir(t)
	n := buildNotice(nil)
	sk, ok := n["skills"].(map[string]any)
	if !ok || !strings.Contains(sk["message"].(string), "plugin") {
		t.Fatalf("expected a skills entry naming the plugin install, got %v", n)
	}
	testenv.SeedPluginSkill(t)
	if n := buildNotice(nil); n != nil {
		if _, ok := n["skills"]; ok {
			t.Errorf("a plugin install must silence the skills entry, got %v", n["skills"])
		}
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

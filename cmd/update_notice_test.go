package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

func TestIsUpdateCheckSkippedCommand(t *testing.T) {
	root := NewRootCmd()
	skipped := [][]string{
		{"update"},
		{"update", "--check"},
		{"completion", "bash"},
		{"__complete", "products", ""},
		{"--format", "json", "update"},
	}
	for _, args := range skipped {
		if !isUpdateCheckSkippedCommand(root, args) {
			t.Errorf("args %v should be skipped", args)
		}
	}
	notSkipped := [][]string{
		{"products", "list"},
		{"products", "update", "--params", "{}"}, // dynamic leaf named update
		{"auth", "login"},
		{},
		{"app", "deploy"},
		{"no-such-command"},
	}
	for _, args := range notSkipped {
		if isUpdateCheckSkippedCommand(root, args) {
			t.Errorf("args %v should not be skipped", args)
		}
	}
}

// wantsVersion gates a disk read, so it must not fire for ordinary commands.
func TestWantsVersion(t *testing.T) {
	yes := [][]string{
		{"--version"},
		{"-v"},
		{"--format", "json", "--version"},
	}
	for _, args := range yes {
		if !wantsVersion(args) {
			t.Errorf("args %v should be a version query", args)
		}
	}
	no := [][]string{
		{},
		{"products", "list"},
		{"auth", "login"},
		{"--", "-v"}, // past the terminator these are operands, not flags
	}
	for _, args := range no {
		if wantsVersion(args) {
			t.Errorf("args %v should not be a version query", args)
		}
	}
}

func TestSkillLine(t *testing.T) {
	dir := testenv.IsolateSkillsDir(t)
	if got := skillLine(); got != "skills not installed" {
		t.Errorf("skillLine() = %q on an empty dir", got)
	}
	if err := os.MkdirAll(filepath.Join(dir, "shoplazza-common"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shoplazza-common", "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := skillLine(); got != "skills installed" {
		t.Errorf("skillLine() = %q with a skill present", got)
	}
}

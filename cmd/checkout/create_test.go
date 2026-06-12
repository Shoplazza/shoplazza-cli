package checkout_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"shoplazza-cli-v2/internal/output"
)

func TestCreate_RequiresNameAndExtension(t *testing.T) {
	f, out := tempCheckoutFactory(t, "http://unused")
	err := execCheckout(t, f, out, "init", "--name", "p") // missing --extension
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("missing --extension → type=validation, got %v", err)
	}
}

func TestCreate_WritesProject(t *testing.T) {
	work := t.TempDir()
	old, _ := os.Getwd()
	defer os.Chdir(old)
	_ = os.Chdir(work)

	f, out := tempCheckoutFactory(t, "http://unused")
	if err := execCheckout(t, f, out, "init", "--name", "shop-ext", "--extension", "hello"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "shop-ext", "extensions", "hello", "src", "index.js")); err != nil {
		t.Errorf("project not scaffolded: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "shop-ext", "extension.config.js")); err == nil {
		t.Error("must NOT generate extension.config.js")
	}
}

// TestCreate_RejectsPathTraversalName locks the guard: a --name/--extension
// containing path separators or ".." must be rejected (validation) instead of
// being Cleaned by filepath.Join into a path OUTSIDE the project tree.
func TestCreate_RejectsPathTraversalName(t *testing.T) {
	work := t.TempDir()
	old, _ := os.Getwd()
	defer os.Chdir(old)
	_ = os.Chdir(work)

	cases := [][]string{
		{"init", "--name", "../escape", "--extension", "hello"},
		{"init", "--name", "ok", "--extension", "../../evil"},
		{"init", "--name", "a/b", "--extension", "hello"},
		{"init", "--name", "..", "--extension", "hello"},
	}
	for _, args := range cases {
		f, out := tempCheckoutFactory(t, "http://unused")
		err := execCheckout(t, f, out, args...)
		var ee *output.ExitError
		if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
			t.Fatalf("args %v → want type=validation, got %v", args, err)
		}
	}
	// Nothing should have been scaffolded outside the project dir.
	if _, err := os.Stat(filepath.Join(work, "..", "escape")); err == nil {
		t.Error("traversal name escaped the project tree")
	}
}

func TestCreate_TargetExistsIsValidation(t *testing.T) {
	work := t.TempDir()
	old, _ := os.Getwd()
	defer os.Chdir(old)
	_ = os.Chdir(work)
	_ = os.MkdirAll(filepath.Join(work, "dup"), 0o755)

	f, out := tempCheckoutFactory(t, "http://unused")
	err := execCheckout(t, f, out, "init", "--name", "dup", "--extension", "hello")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("existing target dir → type=validation, got %v", err)
	}
}

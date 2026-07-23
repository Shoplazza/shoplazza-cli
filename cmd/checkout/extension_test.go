package checkout_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

func TestExtension_RequiresExtensionsDir(t *testing.T) {
	work := t.TempDir() // no extensions/ → not an extension project
	old, _ := os.Getwd()
	defer os.Chdir(old)
	_ = os.Chdir(work)
	f, out := tempCheckoutFactory(t, "http://unused")
	err := execCheckout(t, f, out, "create", "--name", "x")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("not an extension project → type=validation, got %v", err)
	}
}

func TestExtension_AddsExtension(t *testing.T) {
	work := t.TempDir()
	_ = os.MkdirAll(filepath.Join(work, "extensions"), 0o755)
	old, _ := os.Getwd()
	defer os.Chdir(old)
	_ = os.Chdir(work)
	f, out := tempCheckoutFactory(t, "http://unused")
	if err := execCheckout(t, f, out, "create", "--name", "fresh"); err != nil {
		t.Fatalf("extension: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "extensions", "fresh", "src", "index.js")); err != nil {
		t.Errorf("extension not scaffolded: %v", err)
	}
}

func TestExtension_DuplicateIsValidation(t *testing.T) {
	work := t.TempDir()
	_ = os.MkdirAll(filepath.Join(work, "extensions", "dup"), 0o755)
	old, _ := os.Getwd()
	defer os.Chdir(old)
	_ = os.Chdir(work)
	f, out := tempCheckoutFactory(t, "http://unused")
	err := execCheckout(t, f, out, "create", "--name", "dup")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("duplicate extension → type=validation, got %v", err)
	}
}

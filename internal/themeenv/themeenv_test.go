package themeenv

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const sample = `
version = 1
unknown_future_key = "ignored"   # forward-compat: unknown keys must not break parsing

[environments.default]
store = "my-dev.myshoplaza.com"

[environments.staging]
store   = "staging.myshoplaza.com"
theme   = "123456"
path    = "./theme"
ignore  = ["config/settings_data.json", "*.map"]
profile = "staging"

[environments.production]
store = "prod.myshoplaza.com"
live  = true
`

func writeFile(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_ParsesEnvironmentsAndIgnoresUnknownKeys(t *testing.T) {
	path := writeFile(t, t.TempDir(), sample)
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Version != 1 {
		t.Errorf("version = %d, want 1", f.Version)
	}
	staging, ok := f.Environment("staging")
	if !ok {
		t.Fatal("staging environment missing")
	}
	want := Environment{
		Store:   "staging.myshoplaza.com",
		Theme:   "123456",
		Path:    "./theme",
		Ignore:  []string{"config/settings_data.json", "*.map"},
		Profile: "staging",
	}
	if !reflect.DeepEqual(staging, want) {
		t.Errorf("staging = %+v, want %+v", staging, want)
	}
	if prod, _ := f.Environment("production"); !prod.Live {
		t.Error("production.live should be true")
	}
}

func TestEnvironment_EmptyNameResolvesToDefault(t *testing.T) {
	path := writeFile(t, t.TempDir(), sample)
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	env, ok := f.Environment("")
	if !ok {
		t.Fatal(`empty name must resolve to "default"`)
	}
	if env.Store != "my-dev.myshoplaza.com" {
		t.Errorf("default store = %q, want the default block's store", env.Store)
	}
	if _, ok := f.Environment("nope"); ok {
		t.Error("a missing environment must report ok=false")
	}
}

func TestNames_Sorted(t *testing.T) {
	path := writeFile(t, t.TempDir(), sample)
	f, _ := Load(path)
	got := f.Names()
	want := []string{"default", "production", "staging"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v (sorted)", got, want)
	}
}

func TestFind_WalksUpAndReportsNotFound(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, sample)
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	// Found from a nested subdirectory (walks up).
	got, err := Find(nested)
	if err != nil {
		t.Fatalf("Find from nested: %v", err)
	}
	if got != filepath.Join(root, FileName) {
		t.Errorf("Find = %q, want the root file", got)
	}

	// Not found under a fresh, file-less tree.
	if _, err := Find(t.TempDir()); !errors.Is(err, ErrNotFound) {
		t.Errorf("Find on a bare dir = %v, want ErrNotFound", err)
	}
}

func TestLoad_MissingFileIsNotFound(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), FileName)); !errors.Is(err, ErrNotFound) {
		t.Errorf("Load of a missing file = %v, want ErrNotFound", err)
	}
}

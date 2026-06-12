package devstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFileReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	id, ok := Load(dir, "shop-a.myshoplaza.com")
	if ok || id != "" {
		t.Fatalf("Load on empty dir = (%q, %v), want (\"\", false)", id, ok)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "shop-a.myshoplaza.com", "12345"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	id, ok := Load(dir, "shop-a.myshoplaza.com")
	if !ok || id != "12345" {
		t.Fatalf("Load = (%q, %v), want (\"12345\", true)", id, ok)
	}
	// Other stores remain unknown.
	if _, ok := Load(dir, "shop-b.myshoplaza.com"); ok {
		t.Fatal("Load for a different store must report not-found")
	}
}

func TestSave_FileLandsAtShoplazzaThemeStateJSON(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "shop-a.myshoplaza.com", "12345"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := filepath.Join(dir, ".shoplazza", "theme-state.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected state file at %s: %v", want, err)
	}
}

func TestSave_MergesAcrossStores(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "shop-a.myshoplaza.com", "111"); err != nil {
		t.Fatalf("Save a: %v", err)
	}
	if err := Save(dir, "shop-b.myshoplaza.com", "222"); err != nil {
		t.Fatalf("Save b: %v", err)
	}
	// Overwrite shop-a; shop-b must survive.
	if err := Save(dir, "shop-a.myshoplaza.com", "333"); err != nil {
		t.Fatalf("Save a2: %v", err)
	}
	if id, ok := Load(dir, "shop-a.myshoplaza.com"); !ok || id != "333" {
		t.Errorf("shop-a = (%q, %v), want (\"333\", true)", id, ok)
	}
	if id, ok := Load(dir, "shop-b.myshoplaza.com"); !ok || id != "222" {
		t.Errorf("shop-b = (%q, %v), want (\"222\", true)", id, ok)
	}
}

func TestLoad_CorruptFileReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".shoplazza"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".shoplazza", "theme-state.json"),
		[]byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if id, ok := Load(dir, "shop-a.myshoplaza.com"); ok || id != "" {
		t.Fatalf("Load on corrupt file = (%q, %v), want not-found (serve recreates)", id, ok)
	}
}

func TestStoreKey_ExtractsHostFromBaseURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://xjn-dev.myshoplaza.com", "xjn-dev.myshoplaza.com"},
		{"http://127.0.0.1:54321", "127.0.0.1:54321"},
		{"", "default"},
		{"not a url ::", "default"},
	}
	for _, c := range cases {
		if got := StoreKey(c.in); got != c.want {
			t.Errorf("StoreKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

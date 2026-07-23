package theme_extension

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := Config{ExtensionID: "tex_1", ClientID: "cid_1", Name: "my-ext", Type: "theme", Subtype: "basic"}
	if err := WriteConfig(dir, c); err != nil {
		t.Fatal(err)
	}
	got, err := ReadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != c {
		t.Fatalf("round-trip mismatch: %+v != %+v", got, c)
	}
	// no client_secret key on disk (the binding chain takes secret via the
	// app-token chain; it must never be persisted)
	raw, _ := os.ReadFile(filepath.Join(dir, configFile))
	if string(raw) == "" {
		t.Fatal("config not written")
	}
	if strings.Contains(string(raw), "client_secret") || strings.Contains(string(raw), "appSecret") {
		t.Error("app/client secret must never be written to disk")
	}
}

func TestReadConfigMissing(t *testing.T) {
	_, err := ReadConfig(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing config")
	}
	// missing keeps its fs.ErrNotExist identity so callers can branch.
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing config should be fs.ErrNotExist, got %v", err)
	}
}

// TestReadConfigMalformed: a present-but-undecodable config is a distinct error
// naming the file — never confusable with "missing".
func TestReadConfigMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFile), []byte("extension_id = [oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadConfig(dir)
	if err == nil {
		t.Fatal("expected error for corrupt config")
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatal("corrupt config must NOT read as missing")
	}
	if !strings.Contains(err.Error(), configFile) || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error should name the file as malformed, got %q", err.Error())
	}
}

// TestRequireExtensionID_MissingVsCorrupt: the missing path keeps the register
// hint; the corrupt path must not carry it (re-registering would orphan the
// extension_id the corrupt file still holds).
func TestRequireExtensionID_MissingVsCorrupt(t *testing.T) {
	// missing → register hint
	_, exErr := RequireExtensionID(t.TempDir())
	if exErr == nil || exErr.Code != output.ExitValidation {
		t.Fatalf("missing: expected validation, got %v", exErr)
	}
	if exErr.Detail == nil || !strings.Contains(exErr.Detail.Hint, "te build") {
		t.Fatalf("missing: expected the register hint, got %+v", exErr.Detail)
	}

	// corrupt → malformed message, no register hint
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFile), []byte("extension_id = [oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, exErr = RequireExtensionID(dir)
	if exErr == nil || exErr.Code != output.ExitValidation {
		t.Fatalf("corrupt: expected validation, got %v", exErr)
	}
	if !strings.Contains(exErr.Error(), "malformed") {
		t.Fatalf("corrupt: expected the malformed message, got %q", exErr.Error())
	}
	if exErr.Detail != nil && strings.Contains(exErr.Detail.Hint, "register first") {
		t.Fatalf("corrupt: must not suggest re-registering, got hint %q", exErr.Detail.Hint)
	}
}

// TestWriteConfigLeavesNoTemp: the atomic write must not leave temp files
// behind (the old fixed-name .tmp could collide across processes).
func TestWriteConfigLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	if err := WriteConfig(dir, Config{ExtensionID: "tex_1", Name: "x"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != configFile {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected only %s, got %v", configFile, names)
	}
}

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestMigrateV1Extension_WritesTomlAndDeprecatesJSON(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "preorder")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(extDir, "extension.config.json")
	if err := os.WriteFile(jsonPath, []byte(`{"extensionId":"old","appId":"ff","partnerId":"3665"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MigrateV1Extension(root, "preorder", "newid", "preorder", "theme", "1.0.0"); err != nil {
		t.Fatalf("MigrateV1Extension: %v", err)
	}

	// v1 json is KEPT, with a _deprecated notice added and original keys intact.
	jb, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("json should be kept: %v", err)
	}
	var jm map[string]any
	if err := json.Unmarshal(jb, &jm); err != nil {
		t.Fatalf("json should stay valid: %v", err)
	}
	if _, ok := jm["_deprecated"]; !ok {
		t.Errorf("json should gain a _deprecated marker, got %v", jm)
	}
	if jm["extensionId"] != "old" {
		t.Errorf("original json keys must be preserved, got %v", jm)
	}
	// v2 toml written with the NEW id and name/type; no appId/partnerId.
	var m map[string]any
	if _, err := toml.DecodeFile(filepath.Join(extDir, "shoplazza.extension.toml"), &m); err != nil {
		t.Fatalf("decode toml: %v", err)
	}
	if m["id"] != "newid" || m["name"] != "preorder" || m["type"] != "theme" {
		t.Errorf("toml = %v, want id=newid name=preorder type=theme", m)
	}
	if _, ok := m["appId"]; ok {
		t.Errorf("v2 toml must not carry appId: %v", m)
	}
	if _, ok := m["partnerId"]; ok {
		t.Errorf("v2 toml must not carry partnerId: %v", m)
	}
}

func TestMigrateV1Extension_Idempotent(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "preorder")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(extDir, "extension.config.json")
	if err := os.WriteFile(jsonPath, []byte(`{"extensionId":"old"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MigrateV1Extension(root, "preorder", "newid", "preorder", "theme", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(jsonPath)
	if err := MigrateV1Extension(root, "preorder", "newid", "preorder", "theme", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(jsonPath)
	if string(first) != string(second) {
		t.Errorf("re-migration should not rewrite an already-deprecated json")
	}
}

func TestMigrateV1Extension_NoJSON_NoOp(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "v2only")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := MigrateV1Extension(root, "v2only", "id", "v2only", "theme", ""); err != nil {
		t.Fatalf("MigrateV1Extension: %v", err)
	}
	if _, err := os.Stat(filepath.Join(extDir, "shoplazza.extension.toml")); !os.IsNotExist(err) {
		t.Errorf("no v1 json → must not create a toml; stat err=%v", err)
	}
}

func TestMigrateV1Extension_TomlExists_KeepsTomlMarksJSON(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "both")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(extDir, "extension.config.json")
	if err := os.WriteFile(jsonPath, []byte(`{"extensionId":"old"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tomlPath := filepath.Join(extDir, "shoplazza.extension.toml")
	if err := os.WriteFile(tomlPath, []byte("id=\"keep\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MigrateV1Extension(root, "both", "newid", "both", "theme", ""); err != nil {
		t.Fatalf("MigrateV1Extension: %v", err)
	}
	// toml present → write-back owns it, so it's not overwritten here.
	var m map[string]any
	if _, err := toml.DecodeFile(tomlPath, &m); err != nil {
		t.Fatal(err)
	}
	if m["id"] != "keep" {
		t.Errorf("existing toml must not be overwritten, got id=%v", m["id"])
	}
	// json still gets the deprecation marker.
	jb, _ := os.ReadFile(jsonPath)
	var jm map[string]any
	_ = json.Unmarshal(jb, &jm)
	if _, ok := jm["_deprecated"]; !ok {
		t.Errorf("json should be marked deprecated even when a toml already exists: %v", jm)
	}
}

package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// makeThemeTemplate builds a fake already-cloned theme template tree.
func makeThemeTemplate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package.json", `{"name":"tmpl","version":"1.0.0"}`)
	write("shoplazza.extension.toml", "name = \"tmpl\"\ntype = \"theme\"\n")
	write("blocks/index-basic.liquid", "basic block for {{projectName}}\n")
	write("blocks/index-embed.liquid", "embed block for {{projectName}}\n")
	write("snippets/index.liquid", "snippet body\n")
	write("snippets/index_css.liquid", "css snippet body\n")
	write("assets/index.css", ".x{}\n")
	write("locales/en-US.json", `{"label":"{{type}} theme"}`)
	write("locales/zh-CN.json", `{"label":"{{type}} 主题"}`)
	return dir
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be gone, stat err=%v", path, err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestScaffoldTheme_Embed(t *testing.T) {
	src := makeThemeTemplate(t)
	dest := filepath.Join(t.TempDir(), "out")
	if err := ScaffoldTheme(src, dest, "mytheme", "embed"); err != nil {
		t.Fatalf("ScaffoldTheme: %v", err)
	}

	// block: renamed + placeholder replaced; both index-*.liquid gone.
	block := readFile(t, filepath.Join(dest, "blocks", "mytheme.liquid"))
	if !strings.Contains(block, "mytheme") {
		t.Fatalf("block missing name: %q", block)
	}
	if strings.Contains(block, "{{projectName}}") {
		t.Fatalf("block still has placeholder: %q", block)
	}
	mustNotExist(t, filepath.Join(dest, "blocks", "index-basic.liquid"))
	mustNotExist(t, filepath.Join(dest, "blocks", "index-embed.liquid"))

	// snippets: index → mytheme; index_css → mytheme_css; originals gone.
	mustExist(t, filepath.Join(dest, "snippets", "mytheme.liquid"))
	mustExist(t, filepath.Join(dest, "snippets", "mytheme_css.liquid"))
	mustNotExist(t, filepath.Join(dest, "snippets", "index.liquid"))
	mustNotExist(t, filepath.Join(dest, "snippets", "index_css.liquid"))

	// embed: assets dir gone entirely.
	mustNotExist(t, filepath.Join(dest, "assets"))

	// locale: EMBED, not {{type}}.
	en := readFile(t, filepath.Join(dest, "locales", "en-US.json"))
	if !strings.Contains(en, "EMBED") || strings.Contains(en, "{{type}}") {
		t.Fatalf("en-US.json not rewritten: %q", en)
	}

	// package.json: name set, version preserved.
	var pkg map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dest, "package.json"))), &pkg); err != nil {
		t.Fatalf("package.json: %v", err)
	}
	if pkg["name"] != "mytheme" {
		t.Fatalf("package.json name = %v", pkg["name"])
	}
	if pkg["version"] != "1.0.0" {
		t.Fatalf("package.json version not preserved: %v", pkg["version"])
	}

	// shoplazza.extension.toml: name=mytheme, type=theme.
	var ext map[string]any
	if _, err := toml.DecodeFile(filepath.Join(dest, "shoplazza.extension.toml"), &ext); err != nil {
		t.Fatalf("toml: %v", err)
	}
	if ext["name"] != "mytheme" || ext["type"] != "theme" {
		t.Fatalf("toml = %v", ext)
	}
}

func TestScaffoldTheme_Basic(t *testing.T) {
	src := makeThemeTemplate(t)
	dest := filepath.Join(t.TempDir(), "out")
	if err := ScaffoldTheme(src, dest, "mytheme", "basic"); err != nil {
		t.Fatalf("ScaffoldTheme: %v", err)
	}

	mustExist(t, filepath.Join(dest, "blocks", "mytheme.liquid"))
	mustNotExist(t, filepath.Join(dest, "blocks", "index-basic.liquid"))
	mustNotExist(t, filepath.Join(dest, "blocks", "index-embed.liquid"))

	mustExist(t, filepath.Join(dest, "snippets", "mytheme.liquid"))
	// basic: index_css deleted, no mytheme_css.
	mustNotExist(t, filepath.Join(dest, "snippets", "index_css.liquid"))
	mustNotExist(t, filepath.Join(dest, "snippets", "mytheme_css.liquid"))

	// basic: assets/index.css → assets/mytheme.css.
	mustExist(t, filepath.Join(dest, "assets", "mytheme.css"))
	mustNotExist(t, filepath.Join(dest, "assets", "index.css"))

	en := readFile(t, filepath.Join(dest, "locales", "en-US.json"))
	zh := readFile(t, filepath.Join(dest, "locales", "zh-CN.json"))
	if !strings.Contains(en, "BASIC") || strings.Contains(en, "{{type}}") {
		t.Fatalf("en-US.json not rewritten: %q", en)
	}
	if !strings.Contains(zh, "BASIC") || strings.Contains(zh, "{{type}}") {
		t.Fatalf("zh-CN.json not rewritten: %q", zh)
	}
}

func TestScaffoldTheme_BadSubtype(t *testing.T) {
	src := makeThemeTemplate(t)
	dest := filepath.Join(t.TempDir(), "out")
	if err := ScaffoldTheme(src, dest, "x", "weird"); err == nil {
		t.Fatal("expected error for invalid theme-type, got nil")
	}
}

func TestScaffoldSimple(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "package.json"), []byte(`{"name":"tmpl","version":"2.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "shoplazza.extension.toml"), []byte("name = \"tmpl\"\ntype = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if err := ScaffoldSimple(src, dest, "mycheckout", "checkout"); err != nil {
		t.Fatalf("ScaffoldSimple: %v", err)
	}
	var pkg map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dest, "package.json"))), &pkg); err != nil {
		t.Fatalf("package.json: %v", err)
	}
	if pkg["name"] != "mycheckout" || pkg["version"] != "2.0.0" {
		t.Fatalf("package.json = %v", pkg)
	}
	var ext map[string]any
	if _, err := toml.DecodeFile(filepath.Join(dest, "shoplazza.extension.toml"), &ext); err != nil {
		t.Fatalf("toml: %v", err)
	}
	if ext["name"] != "mycheckout" || ext["type"] != "checkout" {
		t.Fatalf("toml = %v", ext)
	}
}

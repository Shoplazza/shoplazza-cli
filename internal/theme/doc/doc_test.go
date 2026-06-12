package doc

import (
	"errors"
	"testing"
)

func TestParseThemeFile_SingleLevel(t *testing.T) {
	typ, loc, err := ParseThemeFile("assets/main.css")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if typ != "assets" || loc != "main.css" {
		t.Fatalf("got (%q,%q), want (assets,main.css)", typ, loc)
	}
}

// V1 bug fix point
func TestParseThemeFile_DeepNested_FixesV1Bug(t *testing.T) {
	typ, loc, err := ParseThemeFile("assets/sub/deep/img.png")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if typ != "assets" {
		t.Fatalf("type = %q, want assets (v1 bug returned 'deep')", typ)
	}
	if loc != "sub/deep/img.png" {
		t.Fatalf("location = %q, want sub/deep/img.png (v1 returned 'img.png')", loc)
	}
}

func TestParseThemeFile_NotInTree(t *testing.T) {
	_, _, err := ParseThemeFile("README.md")
	if !errors.Is(err, ErrNotInThemeTree) {
		t.Fatalf("expected ErrNotInThemeTree, got %v", err)
	}
}

func TestParseThemeFile_DirectoryItself(t *testing.T) {
	for _, in := range []string{"assets", "assets/"} {
		_, _, err := ParseThemeFile(in)
		if !errors.Is(err, ErrNotInThemeTree) {
			t.Errorf("for %q: expected ErrNotInThemeTree, got %v", in, err)
		}
	}
}

func TestParseThemeFile_WindowsBackslashes(t *testing.T) {
	typ, loc, err := ParseThemeFile("assets\\sub\\img.png")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if typ != "assets" || loc != "sub/img.png" {
		t.Fatalf("got (%q,%q), want (assets,sub/img.png)", typ, loc)
	}
}

func TestParseThemeFile_EmptyRelPath(t *testing.T) {
	_, _, err := ParseThemeFile("")
	if !errors.Is(err, ErrNotInThemeTree) {
		t.Fatalf("expected ErrNotInThemeTree for empty, got %v", err)
	}
}

func TestParseThemeFile_AllEightStandardDirs(t *testing.T) {
	dirs := []string{"assets", "blocks", "config", "layout", "locales", "sections", "snippets", "templates"}
	for _, d := range dirs {
		typ, loc, err := ParseThemeFile(d + "/x.txt")
		if err != nil || typ != d || loc != "x.txt" {
			t.Errorf("for dir %q: got (%q,%q,%v)", d, typ, loc, err)
		}
	}
}

func TestFileSnapshot_HasAddRemove(t *testing.T) {
	s := FileSnapshot{}
	if s.Has("assets", "main.css") {
		t.Fatalf("empty snapshot should not Has anything")
	}
	s.Add("assets", "main.css")
	if !s.Has("assets", "main.css") {
		t.Fatalf("after Add, Has should be true")
	}
	s.Remove("assets", "main.css")
	if s.Has("assets", "main.css") {
		t.Fatalf("after Remove, Has should be false")
	}
}

func TestFileSnapshot_FromDocTreeResponse(t *testing.T) {
	resp := map[string]any{
		"data": map[string]any{
			"doctree": map[string]any{
				"assets":   []any{"main.css", "sub/img.png"},
				"layout":   []any{"theme.liquid"},
				"sections": []any{},
			},
		},
	}
	s := FromDocTreeResponse(resp)
	if !s.Has("assets", "sub/img.png") {
		t.Errorf("snapshot missing assets/sub/img.png")
	}
	if !s.Has("layout", "theme.liquid") {
		t.Errorf("snapshot missing layout/theme.liquid")
	}
	if s.Has("sections", "anything") {
		t.Errorf("empty sections should not Has any file")
	}
}

// TestFileSnapshot_FromDocTreeResponse_PluralConfigLayout verifies the snapshot
// normalizes the doctree's plural configs/layouts keys to the singular type.
func TestFileSnapshot_FromDocTreeResponse_PluralConfigLayout(t *testing.T) {
	resp := map[string]any{
		"data": map[string]any{
			"configs": []any{map[string]any{"id": "1", "location": "settings_data.json"}},
			"layouts": []any{map[string]any{"id": "2", "location": "theme.liquid"}},
			"assets":  []any{map[string]any{"id": "3", "location": "app.css"}},
		},
	}
	s := FromDocTreeResponse(resp)
	if !s.Has("config", "settings_data.json") {
		t.Errorf("plural 'configs' must normalize to singular 'config' (settings_data.json missing from snapshot)")
	}
	if !s.Has("layout", "theme.liquid") {
		t.Errorf("plural 'layouts' must normalize to singular 'layout' (theme.liquid missing from snapshot)")
	}
	if !s.Has("assets", "app.css") {
		t.Errorf("already-singular 'assets' must still work")
	}
}

func TestFileSnapshot_FromDocTreeResponse_TopLevelDoctree(t *testing.T) {
	// Alt shape: doctree at top level (server may evolve)
	resp := map[string]any{
		"assets": []any{"main.css"},
	}
	s := FromDocTreeResponse(resp)
	if !s.Has("assets", "main.css") {
		t.Errorf("snapshot should handle alternative shape")
	}
}

func TestFileSnapshot_FromDocTreeResponse_ObjectItems(t *testing.T) {
	// Real doctree shape: items are {id, location} objects with theme dirs at
	// the top level. Regression guard for parsing items only as strings.
	resp := map[string]any{
		"assets": []any{
			map[string]any{"id": "1", "location": "app.css"},
			map[string]any{"id": "2", "location": "sub/img.png"},
		},
		"sections": []any{map[string]any{"id": "3", "location": "header.liquid"}},
	}
	s := FromDocTreeResponse(resp)
	if !s.Has("assets", "app.css") || !s.Has("assets", "sub/img.png") {
		t.Errorf("object-item assets not parsed: %v", s["assets"])
	}
	if !s.Has("sections", "header.liquid") {
		t.Errorf("object-item sections not parsed: %v", s["sections"])
	}
}

func TestFileSnapshot_AddDedupes(t *testing.T) {
	s := FileSnapshot{}
	s.Add("assets", "a.css")
	s.Add("assets", "a.css") // dedup
	got := s["assets"]
	count := 0
	for _, v := range got {
		if v == "a.css" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Add should dedup; got %d copies", count)
	}
}

func TestParseThemeFile_NestedNonThemeDir(t *testing.T) {
	// foo/assets/x.css should parse as (assets, x.css) because we find first matching segment
	typ, loc, err := ParseThemeFile("foo/assets/x.css")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if typ != "assets" || loc != "x.css" {
		t.Fatalf("got (%q,%q)", typ, loc)
	}
}

func TestFileSnapshot_FromDocTreeResponse_SkipsNonStringEntries(t *testing.T) {
	resp := map[string]any{"assets": []any{1, true, "ok.css", nil, "main.css"}}
	s := FromDocTreeResponse(resp)
	if !s.Has("assets", "ok.css") {
		t.Errorf("ok.css should be present")
	}
	if !s.Has("assets", "main.css") {
		t.Errorf("main.css should be present")
	}
	if len(s["assets"]) != 2 {
		t.Errorf("non-string entries (int, bool, nil) should be skipped; got %v", s["assets"])
	}
}

// TestDeduper covers the content-change gate serve uses to skip re-pushing
// unchanged bytes on metadata-only (CHMOD) / duplicate fsnotify events.
func TestDeduper(t *testing.T) {
	d := NewDeduper()

	// Never-recorded → treated as changed (so a brand-new file syncs).
	if d.Unchanged("assets/app.css", []byte("a")) {
		t.Fatal("unrecorded file should be treated as changed")
	}

	d.Record("assets/app.css", []byte("a"))
	// Same content (e.g. a CHMOD/Dropbox touch) → unchanged → skip.
	if !d.Unchanged("assets/app.css", []byte("a")) {
		t.Fatal("identical content should be unchanged (skipped)")
	}
	// Different content → changed → sync.
	if d.Unchanged("assets/app.css", []byte("b")) {
		t.Fatal("different content should be changed")
	}

	// Forget (on delete) → next event treated as changed again.
	d.Forget("assets/app.css")
	if d.Unchanged("assets/app.css", []byte("a")) {
		t.Fatal("after Forget, content should be treated as changed")
	}
}

func TestIsEditorTemp(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		{"assets/.DS_Store", true},    // hidden
		{"assets/.#foo.liquid", true}, // emacs lock
		{"assets/foo~", true},         // emacs/vim backup
		{"assets/foo.swp", true},      // vim swap
		{"assets/foo.swo", true},
		{"assets/foo.swx", true},
		{"assets/foo.tmp", true},                 // temp
		{"assets/settings_data.json.sb-x", true}, // atomic-save temp
		{"assets/main.css", false},               // normal file
		{"locales/en-US.json", false},
		{"blocks/index.liquid", false},
	}
	for _, c := range cases {
		got := IsEditorTemp(c.rel)
		if got != c.want {
			t.Errorf("IsEditorTemp(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
}

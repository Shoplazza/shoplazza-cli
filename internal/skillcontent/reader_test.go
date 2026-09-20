package skillcontent

import (
	"testing"
	"testing/fstest"
)

// fakeSkills builds an in-memory skill FS mirroring the embed layout: top-level
// entries are skill dirs, each with a SKILL.md (+ optional references/*).
func fakeSkills() fstest.MapFS {
	return fstest.MapFS{
		"shoplazza-orders/SKILL.md": {Data: []byte(
			"---\nname: shoplazza-orders\ndescription: >-\n  Manage orders:\n  search and refund.\n---\n\n# Orders\n")},
		"shoplazza-orders/references/refunds.md": {Data: []byte("# Refunds\n")},
		"shoplazza-common/SKILL.md": {Data: []byte(
			"---\nname: shoplazza-common\ndescription: Shared foundation.\n---\n\n# Common\n")},
		"not-a-skill/notes.txt": {Data: []byte("ignore me")},
	}
}

func TestListReturnsSkillsSortedWithDescriptions(t *testing.T) {
	got, err := New(fakeSkills()).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 skills (dirs without SKILL.md excluded), got %d: %+v", len(got), got)
	}
	if got[0].Name != "shoplazza-common" || got[1].Name != "shoplazza-orders" {
		t.Fatalf("want sorted [common, orders], got %q, %q", got[0].Name, got[1].Name)
	}
	if got[0].Description != "Shared foundation." {
		t.Errorf("plain description: want %q, got %q", "Shared foundation.", got[0].Description)
	}
	if got[1].Description != "Manage orders: search and refund." {
		t.Errorf("folded description should join to one line, got %q", got[1].Description)
	}
}

func TestListNilFSReturnsEmpty(t *testing.T) {
	got, err := New(nil).List()
	if err != nil {
		t.Fatalf("List on nil FS: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %+v", got)
	}
}

func TestReadSkillReturnsSkillMarkdown(t *testing.T) {
	data, err := New(fakeSkills()).ReadSkill("shoplazza-common")
	if err != nil {
		t.Fatalf("ReadSkill: %v", err)
	}
	if want := "# Common\n"; string(data[len(data)-len(want):]) != want {
		t.Errorf("want body ending in %q, got %q", want, string(data))
	}
}

func TestReadReferenceReturnsFileAndCleanPath(t *testing.T) {
	data, clean, err := New(fakeSkills()).ReadReference("shoplazza-orders", "references/refunds.md")
	if err != nil {
		t.Fatalf("ReadReference: %v", err)
	}
	if clean != "references/refunds.md" {
		t.Errorf("clean path: want %q, got %q", "references/refunds.md", clean)
	}
	if string(data) != "# Refunds\n" {
		t.Errorf("content: got %q", string(data))
	}
}

func TestReadRejectsPathTraversal(t *testing.T) {
	traversals := []string{"../shoplazza-common/SKILL.md", "references/../../x", "/etc/passwd"}
	for _, rel := range traversals {
		if _, _, err := New(fakeSkills()).ReadReference("shoplazza-orders", rel); err == nil {
			t.Errorf("ReadReference(%q) should be rejected", rel)
		}
	}
}

func TestReadRejectsBadSkillName(t *testing.T) {
	for _, name := range []string{"", ".", "..", "foo/bar"} {
		if _, err := New(fakeSkills()).ReadSkill(name); err == nil {
			t.Errorf("ReadSkill(%q) should be rejected", name)
		}
	}
}

func TestReadOnNilFSErrors(t *testing.T) {
	if _, err := New(nil).ReadSkill("shoplazza-common"); err == nil {
		t.Error("ReadSkill on nil FS should error, not panic")
	}
}

func TestListPathListsOneLayer(t *testing.T) {
	entries, dir, err := New(fakeSkills()).ListPath("shoplazza-orders", "")
	if err != nil {
		t.Fatalf("ListPath: %v", err)
	}
	if dir != "shoplazza-orders" {
		t.Errorf("dir: want %q, got %q", "shoplazza-orders", dir)
	}
	// SKILL.md file + references/ dir (dirs get a trailing slash), sorted.
	if len(entries) != 2 || entries[0] != "SKILL.md" || entries[1] != "references/" {
		t.Errorf("entries: got %v", entries)
	}
}

func TestSplitArg(t *testing.T) {
	cases := []struct{ in, name, rel string }{
		{"shoplazza-orders", "shoplazza-orders", ""},
		{"shoplazza-orders/references/refunds.md", "shoplazza-orders", "references/refunds.md"},
	}
	for _, c := range cases {
		name, rel := SplitArg(c.in)
		if name != c.name || rel != c.rel {
			t.Errorf("SplitArg(%q) = (%q, %q), want (%q, %q)", c.in, name, rel, c.name, c.rel)
		}
	}
}

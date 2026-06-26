package products

import (
	"reflect"
	"testing"

	"shoplazza-cli-v2/shortcuts/common"
)

func TestTagShortcut_ValidationFields(t *testing.T) {
	if tagShortcut.Service != "products" || tagShortcut.Command != "+tag" {
		t.Errorf("identity: got %q/%q", tagShortcut.Service, tagShortcut.Command)
	}
	if tagShortcut.Execute == nil {
		t.Fatal("+tag requires Execute (GET current tags, merge, PUT)")
	}
	if err := common.ValidateShortcut(tagShortcut); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestMergeTags(t *testing.T) {
	cases := []struct {
		name                  string
		existing, add, remove []string
		want                  []string
	}{
		{"add new keeps existing", []string{"a", "b"}, []string{"c"}, nil, []string{"a", "b", "c"}},
		{"add existing is no dup", []string{"a", "b"}, []string{"b", "c"}, nil, []string{"a", "b", "c"}},
		{"remove drops one", []string{"a", "b", "c"}, nil, []string{"b"}, []string{"a", "c"}},
		{"remove missing is ignored", []string{"a"}, nil, []string{"z"}, []string{"a"}},
		{"add and remove together", []string{"a", "b"}, []string{"c"}, []string{"a"}, []string{"b", "c"}},
		{"conflict: add wins over remove", []string{"x"}, []string{"y"}, []string{"y"}, []string{"x", "y"}},
		{"trim and drop empties", []string{"a"}, []string{"  b  ", "", "   "}, nil, []string{"a", "b"}},
		{"dedup within add", []string{}, []string{"c", "c"}, nil, []string{"c"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mergeTags(c.existing, c.add, c.remove)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("mergeTags(%v,%v,%v) = %v, want %v", c.existing, c.add, c.remove, got, c.want)
			}
		})
	}
}

func TestNormalizeTags(t *testing.T) {
	got := normalizeTags([]string{"a", "b", "a", "", "  c  "})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalizeTags = %v, want %v", got, want)
	}
}

func TestProductTags(t *testing.T) {
	// Send decodes JSON arrays as []any, so productTags must handle string-valued []any.
	resp := map[string]any{"product": map[string]any{"tags": []any{"a", "b"}}}
	if got := productTags(resp); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("productTags = %v, want [a b]", got)
	}
	if got := productTags(map[string]any{"product": map[string]any{}}); len(got) != 0 {
		t.Errorf("productTags(no tags) = %v, want empty", got)
	}
	if got := productTags(map[string]any{}); len(got) != 0 {
		t.Errorf("productTags(no product) = %v, want empty", got)
	}
}

func TestEqualTags(t *testing.T) {
	if !equalTags([]string{"a", "b"}, []string{"a", "b"}) {
		t.Error("equalTags identical = false")
	}
	if equalTags([]string{"a", "b"}, []string{"a", "c"}) {
		t.Error("equalTags differing = true")
	}
	if equalTags([]string{"a"}, []string{"a", "b"}) {
		t.Error("equalTags different length = true")
	}
}

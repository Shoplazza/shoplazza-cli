package products

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
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

// putTagsRecorder serves GET with the given current tags and records the tags
// array sent by any PUT. The bool reports whether a PUT was received.
func putTagsRecorder(t *testing.T, current []any, gotPut *bool, putTags *[]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			*gotPut = true
			var body struct {
				Product struct {
					Tags []any `json:"tags"`
				} `json:"product"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			*putTags = body.Product.Tags
			_ = json.NewEncoder(w).Encode(map[string]any{"product": map[string]any{"id": "p-1", "tags": body.Product.Tags}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"product": map[string]any{"id": "p-1", "tags": current}})
	}))
}

func tagInputWithClient(t *testing.T, values map[string]string, baseURL string) common.ExecInput {
	in := newProductExecInput(t, tagExecFlags, values, false)
	in.Client = client.New(baseURL)
	return in
}

var tagExecFlags = map[string]string{
	"id": "string", "add": "stringslice", "remove": "stringslice", "set": "stringslice",
}

func TestTagExecute_NoMutationFlagErrors(t *testing.T) {
	in := newProductExecInput(t, tagExecFlags, map[string]string{"id": "p-1"}, false)
	if _, err := tagShortcut.Execute(context.Background(), in); err == nil {
		t.Error("expected error when none of --add/--remove/--set is given")
	}
}

func TestTagExecute_SetWithAddErrors(t *testing.T) {
	in := newProductExecInput(t, tagExecFlags, map[string]string{"id": "p-1", "set": "a", "add": "b"}, false)
	if _, err := tagShortcut.Execute(context.Background(), in); err == nil {
		t.Error("expected error when --set is combined with --add")
	}
}

func TestTagExecute_DryRun_Set(t *testing.T) {
	in := newProductExecInput(t, tagExecFlags, map[string]string{"id": "p-1", "set": "x,y,x"}, true)
	r, err := tagShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 1 || r.Plans[0].Method != "PUT" || !strings.HasSuffix(r.Plans[0].Path, "/products/p-1") {
		t.Fatalf("set should be a single PUT; got %+v", r.Plans)
	}
	body, _ := r.Plans[0].Body.(map[string]any)
	prod, _ := body["product"].(map[string]any)
	if tags, _ := prod["tags"].([]string); !reflect.DeepEqual(tags, []string{"x", "y"}) {
		t.Errorf("set body tags should be deduped [x y]; got %v", prod["tags"])
	}
}

func TestTagExecute_DryRun_AddRemove(t *testing.T) {
	in := newProductExecInput(t, tagExecFlags, map[string]string{"id": "p-1", "add": "c"}, true)
	r, err := tagShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Plans) != 2 || r.Plans[0].Method != "GET" || !strings.HasSuffix(r.Plans[0].Path, "/products/p-1") || r.Plans[1].Method != "PUT" {
		t.Errorf("add/remove should be GET then PUT; got %+v", r.Plans)
	}
}

func TestTagExecute_Set_NoGet(t *testing.T) {
	var gotGet bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gotGet = true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"product": map[string]any{"id": "p-1", "tags": []any{"a"}}})
	}))
	defer srv.Close()
	in := tagInputWithClient(t, map[string]string{"id": "p-1", "set": "x,y"}, srv.URL)
	if _, err := tagShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotGet {
		t.Error("--set must replace without a GET (no read-merge needed)")
	}
}

func TestTagExecute_AddMergesWithExisting(t *testing.T) {
	var gotPut bool
	var putTags []any
	srv := putTagsRecorder(t, []any{"a", "b"}, &gotPut, &putTags)
	defer srv.Close()
	in := tagInputWithClient(t, map[string]string{"id": "p-1", "add": "c"}, srv.URL)
	if _, err := tagShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotPut {
		t.Fatal("expected a PUT after merging")
	}
	if !reflect.DeepEqual(putTags, []any{"a", "b", "c"}) {
		t.Errorf("PUT tags should keep existing and append; got %v", putTags)
	}
}

func TestTagExecute_RemoveDropsTag(t *testing.T) {
	var gotPut bool
	var putTags []any
	srv := putTagsRecorder(t, []any{"a", "b", "c"}, &gotPut, &putTags)
	defer srv.Close()
	in := tagInputWithClient(t, map[string]string{"id": "p-1", "remove": "b"}, srv.URL)
	if _, err := tagShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(putTags, []any{"a", "c"}) {
		t.Errorf("PUT tags should drop removed tag; got %v", putTags)
	}
}

func TestTagExecute_NoChangeSkipsPut(t *testing.T) {
	var gotPut bool
	var putTags []any
	srv := putTagsRecorder(t, []any{"a", "b"}, &gotPut, &putTags)
	defer srv.Close()
	in := tagInputWithClient(t, map[string]string{"id": "p-1", "add": "b"}, srv.URL)
	if _, err := tagShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPut {
		t.Error("adding an already-present tag must not trigger a PUT")
	}
}

package products

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/internal/shortcuttest"
)

var productSearchFlags = map[string]string{
	"keyword": "string", "published": "string", "vendor": "string",
	"collection-id": "string",
	"page-limit":    "int", "fields": "stringslice",
}

func TestProductSearchPlan_DefaultsSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "search", productSearchFlags, nil)
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProductSearchPlan_WithKeywordSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "search", productSearchFlags, map[string]string{
		"keyword": "shirt", "page-limit": "10",
	})
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProductSearchPlan_VendorMapsToVendorsArray(t *testing.T) {
	in := shortcuttest.PlanInput(t, "search", productSearchFlags, map[string]string{"vendor": "Acme"})
	p, err := searchShortcut.Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	vs, _ := p.Query["vendors"].([]string)
	if len(vs) != 1 || vs[0] != "Acme" {
		t.Errorf("vendor should map to vendors=[Acme]; got query=%v", p.Query)
	}
	if _, ok := p.Query["vendor"]; ok {
		t.Error("must not send the invalid singular `vendor` param")
	}
}

func TestProductSearchPlan_PublishedNormalized(t *testing.T) {
	cases := map[string]string{"true": "published", "false": "unpublished", "any": "any", "published": "published"}
	for in, want := range cases {
		pin := shortcuttest.PlanInput(t, "search", productSearchFlags, map[string]string{"published": in})
		p, err := searchShortcut.Plan(pin)
		if err != nil {
			t.Fatalf("--published %q: %v", in, err)
		}
		if got := p.Query["published_status"]; got != want {
			t.Errorf("--published %q -> published_status=%v, want %q", in, got, want)
		}
	}
}

func TestProductSearchPlan_PublishedInvalidErrors(t *testing.T) {
	in := shortcuttest.PlanInput(t, "search", productSearchFlags, map[string]string{"published": "yes"})
	if _, err := searchShortcut.Plan(in); err == nil {
		t.Error("expected error for an invalid --published value")
	}
}

func TestListFieldSelectors(t *testing.T) {
	got := listFieldSelectors([]string{"id", " title ", "primary_image", "image", "origin_price_min", "price_min", ""})
	want := []string{"id", "title", "image", "price_min"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("listFieldSelectors = %v, want %v", got, want)
	}
	if got := listFieldSelectors(nil); got != nil {
		t.Errorf("listFieldSelectors(nil) = %v, want nil", got)
	}
}

// fieldsListServer mimics the list API: it only returns primary_image for the `image` selector.
func fieldsListServer(t *testing.T, gotFields *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotFields = r.URL.Query()["fields"]
		p := map[string]any{"id": "p-1", "title": "Shirt", "created_at": "2026-01-01T00:00:00Z"}
		for _, f := range *gotFields {
			if f == "image" {
				p["primary_image"] = map[string]any{"src": "//img.example/a.png"}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"products": []any{p}, "has_more": false})
	}))
}

func TestProductSearch_FieldsPrimaryImageReturned(t *testing.T) {
	var gotFields []string
	srv := fieldsListServer(t, &gotFields)
	defer srv.Close()

	in := shortcuttest.PlanInput(t, "search", productSearchFlags, map[string]string{"fields": "id,title,primary_image"})
	p, err := searchShortcut.Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := common.Send(context.Background(), client.New(srv.URL), p)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if want := []string{"id", "title", "image"}; !reflect.DeepEqual(gotFields, want) {
		t.Errorf("sent fields = %v, want %v", gotFields, want)
	}
	products, _ := out["products"].([]any)
	if len(products) != 1 {
		t.Fatalf("products = %v", out["products"])
	}
	if _, ok := products[0].(map[string]any)["primary_image"]; !ok {
		t.Errorf("primary_image missing from response: %v", products[0])
	}
}

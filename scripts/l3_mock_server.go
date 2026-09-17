//go:build ignore

// l3_mock_server.go — a throwaway mock API for running the L3 interactive tests
// (docs/M3_INTERACTION_TEST_PLAN.md) without a real store. It returns one canned
// page per resource so the fuzzy pickers have something to show, and a generic
// success for writes so destructive commands complete harmlessly.
//
//	Terminal A:  go run scripts/l3_mock_server.go
//	Terminal B:  export SHOPLAZZA_CLI_API_BASE_URL=http://127.0.0.1:8787
//	             export SHOPLAZZA_ACCESS_TOKEN=test
//	             shoplazza products +unpublish      # picker lists the mock's products
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": data(r.URL.Path)})
	})
	fmt.Println("L3 mock listening on http://127.0.0.1:8787  (ctrl+c to stop)")
	log.Fatal(http.ListenAndServe("127.0.0.1:8787", nil))
}

// data returns the resource list keyed as each picker expects, chosen by path.
func data(path string) map[string]any {
	switch {
	case strings.Contains(path, "checkout_extensions/version/list"):
		return map[string]any{"extensions": []any{
			map[string]any{"version": "1.0", "id": "ver_1"},
			map[string]any{"version": "1.1", "id": "ver_2"},
			map[string]any{"version": "2.0", "id": "ver_3"},
		}}
	case strings.Contains(path, "checkout_extensions"):
		return map[string]any{"extensions": []any{
			map[string]any{"name": "Banner", "extension_id": "ext_1", "publish_status": "published"},
			map[string]any{"name": "Upsell", "extension_id": "ext_2", "publish_status": "draft"},
		}}
	case strings.Contains(path, "/products"):
		return map[string]any{"products": []any{
			map[string]any{"id": "prod_1", "title": "Classic Tee", "status": "active"},
			map[string]any{"id": "prod_2", "title": "Wool Beanie", "status": "draft"},
			map[string]any{"id": "prod_3", "title": "Canvas Tote", "status": "active"},
		}}
	case strings.Contains(path, "/orders"):
		return map[string]any{"orders": []any{
			map[string]any{"id": "ord_1", "number": "1001", "financial_status": "paid", "total_price": "29.99"},
			map[string]any{"id": "ord_2", "number": "1002", "financial_status": "pending", "total_price": "12.00"},
		}}
	case strings.Contains(path, "/themes"):
		return map[string]any{"themes": []any{
			map[string]any{"id": "thm_1", "name": "Dawn", "theme_type": "main"},
			map[string]any{"id": "thm_2", "name": "Dev copy", "theme_type": "unpublished"},
		}}
	default:
		return map[string]any{} // generic success for writes (unpublish/refund/cancel/delete)
	}
}

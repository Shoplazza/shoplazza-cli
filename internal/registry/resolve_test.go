package registry

import (
	"reflect"
	"testing"
)

func newTestSpec() *Spec {
	return &Spec{
		Modules: []Module{{
			Name: "orders",
			Commands: []Command{
				{ID: "order-list", Path: []string{"list"},
					Summary: "List orders",
					HTTP:    HTTP{Method: "GET", Path: "/openapi/2026-01/orders"},
					Parameters: []Parameter{
						{Name: "page_size", In: "query", Type: "integer"},
					},
					ResponseSchema: "ListOrdersResponse",
				},
				{ID: "coupon-create", Path: []string{"coupons", "create"},
					HTTP: HTTP{Method: "POST", Path: "/openapi/2026-01/coupons", Body: "*"},
					Body: &Body{Required: true, Fields: []Field{
						{Name: "coupon", Type: "object", Schema: "CreateCouponParam", Required: true},
					}},
				},
			},
		}},
		Schemas: map[string]ObjectSchema{
			"ListOrdersResponse": {Fields: []Field{
				{Name: "orders", Type: "array", Items: &Field{Type: "object", Schema: "Order"}},
			}},
			"Order": {Fields: []Field{
				{Name: "id", Type: "string"},
				{Name: "parent", Type: "object", Schema: "Order"}, // self-reference
			}},
			"CreateCouponParam": {Fields: []Field{
				{Name: "code", Type: "string", Required: true},
			}},
		},
	}
}

func TestResolveSpecSchema_ModuleList(t *testing.T) {
	spec := newTestSpec()
	payload, ok, err := ResolveSpecSchema(spec, "", "")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	got := payload.(map[string]any)["modules"]
	if !reflect.DeepEqual(got, []string{"orders"}) {
		t.Fatalf("modules = %v", got)
	}
}

func TestResolveSpecSchema_ModuleDetail(t *testing.T) {
	spec := newTestSpec()
	payload, ok, err := ResolveSpecSchema(spec, "orders", "")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	cmds := payload.(map[string]any)["commands"].([]map[string]any)
	if len(cmds) != 2 {
		t.Fatalf("commands count = %d, want 2", len(cmds))
	}
}

func TestResolveSpecSchema_LeafCommand(t *testing.T) {
	spec := newTestSpec()
	payload, ok, err := ResolveSpecSchema(spec, "orders.list", "")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	m := payload.(map[string]any)
	if m["id"] != "order-list" {
		t.Fatalf("id = %v", m["id"])
	}
	if _, hasParams := m["parameters"]; !hasParams {
		t.Fatal("expected parameters in payload")
	}
	resp := m["response"].(map[string]any)
	if _, hasFields := resp["fields"]; !hasFields {
		t.Fatalf("response should expand fields, got %v", resp)
	}
}

func TestResolveSpecSchema_View_Request(t *testing.T) {
	spec := newTestSpec()
	// orders.list has parameters + response_schema (no body).
	payload, ok, _ := ResolveSpecSchema(spec, "orders.list", ViewRequest)
	if !ok {
		t.Fatal("must resolve")
	}
	m := payload.(map[string]any)
	if _, has := m["parameters"]; !has {
		t.Error("ViewRequest should keep parameters")
	}
	if _, has := m["response"]; has {
		t.Error("ViewRequest should drop response")
	}

	// orders.coupons.create has body but no parameters.
	payload, _, _ = ResolveSpecSchema(spec, "orders.coupons.create", ViewRequest)
	m = payload.(map[string]any)
	if _, has := m["body"]; !has {
		t.Error("ViewRequest should keep body for POST commands")
	}
	if _, has := m["response"]; has {
		t.Error("ViewRequest should drop response")
	}
}

func TestResolveSpecSchema_View_Response(t *testing.T) {
	spec := newTestSpec()
	payload, ok, _ := ResolveSpecSchema(spec, "orders.list", ViewResponse)
	if !ok {
		t.Fatal("must resolve")
	}
	m := payload.(map[string]any)
	if _, has := m["parameters"]; has {
		t.Error("ViewResponse should drop parameters")
	}
	if _, has := m["body"]; has {
		t.Error("ViewResponse should drop body")
	}
	if _, has := m["response"]; !has {
		t.Error("ViewResponse should keep response")
	}
}

func TestResolveSpecSchema_View_AllIsDefault(t *testing.T) {
	spec := newTestSpec()
	// Empty view string should equal ViewAll behaviour.
	emptyPayload, _, _ := ResolveSpecSchema(spec, "orders.list", "")
	allPayload, _, _ := ResolveSpecSchema(spec, "orders.list", ViewAll)
	if !reflect.DeepEqual(emptyPayload, allPayload) {
		t.Errorf("empty view should match ViewAll, got %v vs %v", emptyPayload, allPayload)
	}
}

func TestResolveSpecSchema_ThreeLevelCommand(t *testing.T) {
	spec := newTestSpec()
	_, ok, err := ResolveSpecSchema(spec, "orders.coupons.create", "")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestResolveSpecSchema_UnknownPath(t *testing.T) {
	spec := newTestSpec()
	_, ok, _ := ResolveSpecSchema(spec, "no-such-module", "")
	if ok {
		t.Fatal("unknown module must return ok=false")
	}
	_, ok, _ = ResolveSpecSchema(spec, "orders.gibberish", "")
	if ok {
		t.Fatal("unknown command must return ok=false")
	}
}

func TestResolveSpecSchema_CycleDetection(t *testing.T) {
	spec := newTestSpec()
	payload, ok, _ := ResolveSpecSchema(spec, "orders.list", "")
	if !ok {
		t.Fatal("must resolve")
	}
	// Walk the response → ListOrdersResponse → orders.items.Order → parent.Order
	// The deepest parent must be flagged either cycle or truncated, not infinite.
	raw, _ := payload.(map[string]any)
	if raw == nil {
		t.Fatal("nil payload")
	}
	// Just exercising it — non-termination would hang the test.
}

func TestResolveSpecSchema_UnresolvedRef(t *testing.T) {
	spec := &Spec{
		Modules: []Module{{Name: "x", Commands: []Command{{
			Path:           []string{"go"},
			HTTP:           HTTP{Method: "GET", Path: "/x"},
			ResponseSchema: "DoesNotExist",
		}}}},
	}
	payload, ok, _ := ResolveSpecSchema(spec, "x.go", "")
	if !ok {
		t.Fatal("must resolve command itself even with bad ref")
	}
	resp := payload.(map[string]any)["response"].(map[string]any)
	if resp["unresolved"] != true {
		t.Fatalf("expected unresolved marker, got %v", resp)
	}
}

func TestResolveSpecSchema_ScopesStub(t *testing.T) {
	spec := newTestSpec()
	payload, ok, _ := ResolveSpecSchema(spec, "scopes orders.list", "")
	if !ok {
		t.Fatal("scopes must always succeed")
	}
	if got := payload.(map[string]any)["scopes"]; !reflect.DeepEqual(got, []any{}) {
		t.Fatalf("scopes = %v, want []", got)
	}
}

func TestResolveSpecSchema_NilSpec(t *testing.T) {
	_, ok, _ := ResolveSpecSchema(nil, "x", "")
	if ok {
		t.Fatal("nil spec must return ok=false")
	}
}

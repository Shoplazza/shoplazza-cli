package registry

import (
	"encoding/json"
	"testing"
)

func TestSpec_UnmarshalMinimal(t *testing.T) {
	raw := []byte(`{"version":"v202601","modules":[]}`)
	var s Spec
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.Version != "v202601" {
		t.Fatalf("version = %q, want v202601", s.Version)
	}
	if len(s.Modules) != 0 {
		t.Fatalf("modules len = %d, want 0", len(s.Modules))
	}
}

func TestSpec_UnmarshalCommandWithThreeLevelPath(t *testing.T) {
	raw := []byte(`{
      "version":"v202601",
      "modules":[{
        "name":"discounts",
        "commands":[{
          "id":"coupon-create",
          "command_path":["coupons","create"],
          "summary":"Create coupon",
          "http":{"method":"POST","path":"/openapi/2026-01/coupons","body":"*"},
          "body":{"required":true,"fields":[{"name":"coupon","type":"object","schema":"v202601.request.CreateCouponParam","required":true}]}
        }]
      }]
    }`)
	var s Spec
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cmd := s.Modules[0].Commands[0]
	if got, want := cmd.Path, []string{"coupons", "create"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("path = %v, want %v", got, want)
	}
	if cmd.HTTP.Body != "*" {
		t.Fatalf("body marker = %q, want *", cmd.HTTP.Body)
	}
	if cmd.Body == nil || !cmd.Body.Required || len(cmd.Body.Fields) != 1 {
		t.Fatalf("body fields = %#v", cmd.Body)
	}
	if got, want := cmd.Body.Fields[0].Schema, "v202601.request.CreateCouponParam"; got != want {
		t.Fatalf("schema ref = %q, want %q", got, want)
	}
}

func TestSpec_UnmarshalUnknownFieldsIgnored(t *testing.T) {
	raw := []byte(`{"version":"v202601","modules":[],"future_field":42}`)
	var s Spec
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal must ignore unknown fields: %v", err)
	}
}

func TestSpec_HiddenDefaultFalse(t *testing.T) {
	raw := []byte(`{"version":"v","modules":[{"name":"m","commands":[{"id":"x","command_path":["a"],"http":{"method":"GET","path":"/a"}}]}]}`)
	var s Spec
	_ = json.Unmarshal(raw, &s)
	if s.Modules[0].Commands[0].Hidden {
		t.Fatalf("hidden default must be false")
	}
}

func TestObjectSchema_RecursiveReference(t *testing.T) {
	raw := []byte(`{"version":"v","modules":[],"schemas":{
      "Tree":{"fields":[
        {"name":"name","type":"string"},
        {"name":"children","type":"array","items":{"type":"object","schema":"Tree"}}
      ]}
    }}`)
	var s Spec
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	children := s.Schemas["Tree"].Fields[1]
	if children.Items == nil || children.Items.Schema != "Tree" {
		t.Fatalf("recursive schema ref not preserved: %#v", children)
	}
}

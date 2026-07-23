package discounts

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"

	"github.com/spf13/cobra"
)

// newPlanInput builds a PlanInput by registering the given flags on a cobra
// command, parsing args, and wrapping the result in common.NewCobraFlagSet.
// flags maps flag-name → type ("string","int","float","bool","stringslice").
// values maps flag-name → string value to pass on the command line.
func newPlanInput(t *testing.T, tool string, flags map[string]string, values map[string]string) common.PlanInput {
	t.Helper()
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	for name, typ := range flags {
		switch typ {
		case "string":
			cmd.Flags().String(name, "", "")
		case "int":
			cmd.Flags().Int(name, 0, "")
		case "float":
			cmd.Flags().Float64(name, 0, "")
		case "bool":
			cmd.Flags().Bool(name, false, "")
		case "stringslice":
			cmd.Flags().StringSlice(name, nil, "")
		}
	}
	var args []string
	for name, val := range values {
		args = append(args, "--"+name+"="+val)
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}
	return common.PlanInput{Tool: tool, Flags: common.NewCobraFlagSet(cmd)}
}

// ── GenerateCode ──────────────────────────────────────────────────────────────

func TestGenerateCode(t *testing.T) {
	code := GenerateCode()
	if !strings.HasPrefix(code, "CLI-") {
		t.Errorf("GenerateCode = %q, want CLI- prefix", code)
	}
	if len(code) != 10 { // "CLI-" (4) + 6 chars
		t.Errorf("GenerateCode length = %d, want 10 (CLI-XXXXXX)", len(code))
	}
	for _, ch := range code[4:] {
		if !((ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')) {
			t.Errorf("GenerateCode %q contains invalid char %q", code, ch)
		}
	}
}

func TestGenerateCode_Uniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		seen[GenerateCode()] = true
	}
	if len(seen) < 95 {
		t.Errorf("GenerateCode produced too many duplicates in 100 draws: only %d unique", len(seen))
	}
}

// ── ParseRebateType ───────────────────────────────────────────────────────────

func TestParseRebateType(t *testing.T) {
	cases := []struct {
		input        string
		discountType string
		condition    string
		obtain       string
	}{
		{"amount-off", "rebate_cta_otr", "purchase_amount", "fixed_price_reduction"},
		{"amount-percent", "rebate_cta_otp", "purchase_amount", "percent"},
		{"qty-off", "rebate_ctq_otr", "purchase_quantity", "fixed_price_reduction"},
		{"qty-percent", "rebate_ctq_otp", "purchase_quantity", "percent"},
		{"", "rebate_cta_otr", "purchase_amount", "fixed_price_reduction"},
	}
	for _, tc := range cases {
		rt, err := ParseRebateType(tc.input)
		if err != nil {
			t.Fatalf("ParseRebateType(%q): %v", tc.input, err)
		}
		if rt.DiscountType != tc.discountType {
			t.Errorf("[%s] DiscountType = %q, want %q", tc.input, rt.DiscountType, tc.discountType)
		}
		if rt.ConditionType != tc.condition {
			t.Errorf("[%s] ConditionType = %q, want %q", tc.input, rt.ConditionType, tc.condition)
		}
		if rt.ObtainType != tc.obtain {
			t.Errorf("[%s] ObtainType = %q, want %q", tc.input, rt.ObtainType, tc.obtain)
		}
	}
}

func TestParseRebateType_Invalid(t *testing.T) {
	if _, err := ParseRebateType("unknown"); err == nil {
		t.Error("expected error for unknown rebate type")
	}
}

// ── ParseFlashsaleType ────────────────────────────────────────────────────────

func TestParseFlashsaleType(t *testing.T) {
	cases := []struct{ input, want string }{
		{"percent", "percent"},
		{"fixed-price", "fixed_price"},
		{"off", "fixed_price_reduction"},
		{"", "percent"},
	}
	for _, tc := range cases {
		got, err := ParseFlashsaleType(tc.input)
		if err != nil {
			t.Fatalf("ParseFlashsaleType(%q): %v", tc.input, err)
		}
		if got != tc.want {
			t.Errorf("[%s] = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseFlashsaleType_Invalid(t *testing.T) {
	if _, err := ParseFlashsaleType("unknown"); err == nil {
		t.Error("expected error for unknown flashsale type")
	}
}

// ── buildAutoInfoFromFlags ────────────────────────────────────────────────────

func TestBuildAutoInfoFromFlags_Defaults(t *testing.T) {
	in := newPlanInput(t, "rebate", map[string]string{"name": "string", "start": "string", "end": "string"}, nil)
	info, err := buildAutoInfoFromFlags(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if info["discount_name"] == "" {
		t.Error("discount_name should be auto-generated")
	}
	if info["starts_at"] == nil {
		t.Error("starts_at should be set")
	}
	if info["ends_at"] != int64(-1) {
		t.Errorf("ends_at: got %v want -1 (no end)", info["ends_at"])
	}
}

func TestBuildAutoInfoFromFlags_CustomName(t *testing.T) {
	in := newPlanInput(t, "rebate", map[string]string{"name": "string", "start": "string", "end": "string"}, map[string]string{"name": "My Campaign"})
	info, err := buildAutoInfoFromFlags(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if info["discount_name"] != "My Campaign" {
		t.Errorf("discount_name: got %v want My Campaign", info["discount_name"])
	}
}

func TestBuildAutoInfoFromFlags_InvalidStartErrors(t *testing.T) {
	in := newPlanInput(t, "rebate", map[string]string{"name": "string", "start": "string", "end": "string"}, map[string]string{"start": "not-a-time"})
	_, err := buildAutoInfoFromFlags(in)
	if err == nil {
		t.Error("expected error for invalid --start value")
	}
}

// ── resolveLimitMaxUser ───────────────────────────────────────────────────────

func TestResolveLimitMaxUser_BothUnset(t *testing.T) {
	in := newPlanInput(t, "code", map[string]string{"limit-max": "int", "limit-user": "int"}, nil)
	max, user, err := resolveLimitMaxUser(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if max != -1 || user != -1 {
		t.Errorf("unset: got max=%d user=%d want -1 -1", max, user)
	}
}

func TestResolveLimitMaxUser_ValidValues(t *testing.T) {
	in := newPlanInput(t, "code", map[string]string{"limit-max": "int", "limit-user": "int"}, map[string]string{"limit-max": "5", "limit-user": "2"})
	max, user, err := resolveLimitMaxUser(in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if max != 5 || user != 2 {
		t.Errorf("got max=%d user=%d want 5 2", max, user)
	}
}

func TestResolveLimitMaxUser_ZeroMaxErrors(t *testing.T) {
	in := newPlanInput(t, "code", map[string]string{"limit-max": "int", "limit-user": "int"}, map[string]string{"limit-max": "0"})
	_, _, err := resolveLimitMaxUser(in)
	if err == nil {
		t.Error("expected error for limit-max=0")
	}
}

func TestResolveLimitMaxUser_ZeroUserErrors(t *testing.T) {
	in := newPlanInput(t, "code", map[string]string{"limit-max": "int", "limit-user": "int"}, map[string]string{"limit-user": "0"})
	_, _, err := resolveLimitMaxUser(in)
	if err == nil {
		t.Error("expected error for limit-user=0")
	}
}

// ── buildCodeDiscountPayload ──────────────────────────────────────────────────

func discountCodeFlags() map[string]string {
	return map[string]string{
		"target": "string", "products": "string", "collections": "string",
		"variants": "string", "exclude": "bool", "min-amount": "float",
		"min-quantity": "int", "code": "string", "name": "string",
		"start": "string", "end": "string", "combines": "stringslice",
		"limit-max": "int", "limit-user": "int", "customer-segments": "string",
	}
}

func TestBuildCodeDiscountPayload_OrderTarget(t *testing.T) {
	in := newPlanInput(t, "percent-code", discountCodeFlags(), map[string]string{"target": "order"})
	payload, err := buildCodeDiscountPayload(in, "code_percent", "percent", 10)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	disc, _ := payload["discount"].(map[string]any)
	info, _ := disc["discount_info"].(map[string]any)
	if info["discount_target"] != "order" {
		t.Errorf("discount_target: got %v want order", info["discount_target"])
	}
	layer, _ := disc["discount_layer"].(map[string]any)
	if layer["condition_type"] != "no_condition" {
		t.Errorf("condition_type: got %v want no_condition (no min-amount)", layer["condition_type"])
	}
}

func TestBuildCodeDiscountPayload_ProductTarget(t *testing.T) {
	in := newPlanInput(t, "percent-code", discountCodeFlags(), map[string]string{"target": "product", "products": "p-1,p-2"})
	payload, err := buildCodeDiscountPayload(in, "code_percent", "percent", 20)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	disc, _ := payload["discount"].(map[string]any)
	ep, _ := disc["entitled_product"].(map[string]any)
	if ep["selection"] != "entitled" {
		t.Errorf("entitled_product.selection: got %v want entitled", ep["selection"])
	}
}

func TestBuildCodeDiscountPayload_InvalidTargetErrors(t *testing.T) {
	in := newPlanInput(t, "percent-code", discountCodeFlags(), map[string]string{"target": "store"})
	_, err := buildCodeDiscountPayload(in, "code_percent", "percent", 10)
	if err == nil {
		t.Error("expected error for invalid --target")
	}
}

func TestBuildCodeDiscountPayload_ExcludeOnOrderErrors(t *testing.T) {
	in := newPlanInput(t, "percent-code", discountCodeFlags(), map[string]string{"target": "order", "exclude": "true"})
	_, err := buildCodeDiscountPayload(in, "code_percent", "percent", 10)
	if err == nil {
		t.Error("expected error: --exclude only applies to --target=product")
	}
}

func TestBuildCodeDiscountPayload_ProductTargetNoScopeErrors(t *testing.T) {
	in := newPlanInput(t, "percent-code", discountCodeFlags(), map[string]string{"target": "product"})
	_, err := buildCodeDiscountPayload(in, "code_percent", "percent", 10)
	if err == nil {
		t.Error("expected error: product target requires one of --products/--variants/--collections")
	}
}

// ── validateCombines ──────────────────────────────────────────────────────────

func combinesFlags() map[string]string {
	return map[string]string{
		"combines":   "stringslice",
		"limit-max":  "int",
		"limit-user": "int",
		"code":       "string",
		"name":       "string",
		"start":      "string",
		"end":        "string",
	}
}

func TestValidateCombines_Unset(t *testing.T) {
	in := newPlanInput(t, "code", combinesFlags(), nil)
	got, err := validateCombines(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
}

func TestValidateCombines_ValidValues(t *testing.T) {
	in := newPlanInput(t, "code", combinesFlags(), map[string]string{"combines": "order,product"})
	got, err := validateCombines(in)
	if err != nil || len(got) != 2 {
		t.Errorf("got (%v, %v)", got, err)
	}
}

func TestValidateCombines_InvalidValue(t *testing.T) {
	in := newPlanInput(t, "code", combinesFlags(), map[string]string{"combines": "invalid"})
	_, err := validateCombines(in)
	if err == nil {
		t.Error("expected error for invalid --combines value")
	}
}

// ── buildCodeInfoFromFlags ────────────────────────────────────────────────────

func TestBuildCodeInfoFromFlags_Defaults(t *testing.T) {
	in := newPlanInput(t, "code", combinesFlags(), nil)
	info, err := buildCodeInfoFromFlags(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info["discount_name"] == "" {
		t.Error("discount_name should be auto-generated")
	}
	codes, _ := info["discount_codes"].([]string)
	if len(codes) == 0 {
		t.Error("discount_codes should contain at least one code")
	}
}

func TestBuildCodeInfoFromFlags_CustomCode(t *testing.T) {
	in := newPlanInput(t, "code", combinesFlags(), map[string]string{"code": "SAVE20"})
	info, err := buildCodeInfoFromFlags(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	codes, _ := info["discount_codes"].([]string)
	if len(codes) == 0 || codes[0] != "SAVE20" {
		t.Errorf("discount_codes[0]: got %v want SAVE20", codes)
	}
}

func TestBuildCodeInfoFromFlags_InvalidStartErrors(t *testing.T) {
	in := newPlanInput(t, "code", combinesFlags(), map[string]string{"start": "not-a-time"})
	_, err := buildCodeInfoFromFlags(in)
	if err == nil {
		t.Error("expected error for invalid --start")
	}
}

// ── codeRuleFromFlags ─────────────────────────────────────────────────────────

func TestCodeRuleFromFlags_Defaults(t *testing.T) {
	in := newPlanInput(t, "code", combinesFlags(), nil)
	rule, err := codeRuleFromFlags(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule["limit_max_discount"] != -1 || rule["limit_user_discount"] != -1 {
		t.Errorf("unset limits should be -1: %v", rule)
	}
}

func TestCodeRuleFromFlags_InvalidCombinesErrors(t *testing.T) {
	in := newPlanInput(t, "code", combinesFlags(), map[string]string{"combines": "bad"})
	_, err := codeRuleFromFlags(in)
	if err == nil {
		t.Error("expected error for invalid combines")
	}
}

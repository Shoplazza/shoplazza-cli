package checkout

import (
	"encoding/json"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/core"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// ── resolveStore ──────────────────────────────────────────────────────────────

func TestResolveStore_FallsBackToConfig(t *testing.T) {
	f := &cmdutil.Factory{Config: currentStoreConfig("abc.com")}
	got, exitErr := resolveStore(f)
	if exitErr != nil || got != "abc.com" {
		t.Fatalf("got %q err %v", got, exitErr)
	}
}

func TestResolveStore_NoneIsValidation(t *testing.T) {
	f := &cmdutil.Factory{Config: core.CliConfig{}}
	_, exitErr := resolveStore(f)
	if exitErr == nil || exitErr.Detail.Type != output.TypeValidation {
		t.Fatalf("no store → type=validation, got %v", exitErr)
	}
}

func TestResolveStore_NormalizesSchemePrefix(t *testing.T) {
	f := &cmdutil.Factory{Config: currentStoreConfig("https://abc.com/")}
	got, exitErr := resolveStore(f)
	if exitErr != nil || got != "abc.com" {
		t.Fatalf("scheme-prefixed config domain must be normalized, got %q err %v", got, exitErr)
	}
}

func TestResolveStore_SchemeOnlyConfigIsValidation(t *testing.T) {
	f := &cmdutil.Factory{Config: currentStoreConfig("https://")}
	got, exitErr := resolveStore(f)
	if exitErr == nil || exitErr.Detail.Type != output.TypeValidation {
		t.Fatalf("config domain normalizing to empty → type=validation, got %q err %v", got, exitErr)
	}
}

// currentStoreConfig builds a v2 CliConfig whose CurrentStoreDomain() resolves to domain.
func currentStoreConfig(domain string) core.CliConfig {
	return core.CliConfig{
		CurrentProfile: "p",
		Profiles:       []core.ProfileConfig{{Name: "p", StoreDomain: domain}},
	}
}

// ── selectJSArtifact ─────────────────────────────────────────────────────────

func TestSelectJSArtifact_PicksFirstJS(t *testing.T) {
	got, exitErr := selectJSArtifact("demo", []string{"dist/demo.abc.css", "dist/demo.abc.js", "dist/demo.abc.js.map"})
	if exitErr != nil || got != "dist/demo.abc.js" {
		t.Fatalf("got %q err %v, want dist/demo.abc.js", got, exitErr)
	}
}

func TestSelectJSArtifact_NoJSIsValidation(t *testing.T) {
	_, exitErr := selectJSArtifact("demo", []string{"dist/demo.abc.css", "dist/demo.abc.js.map"})
	if exitErr == nil || exitErr.Detail.Type != output.TypeValidation {
		t.Fatalf("no .js artifact must be type=validation, got %v", exitErr)
	}
}

// ── asString ─────────────────────────────────────────────────────────────────

func TestAsString_HandlesJSONNumber(t *testing.T) {
	// 18-digit ids decode as json.Number; asString must stringify, not round.
	if got := asString(json.Number("907123456789012345")); got != "907123456789012345" {
		t.Fatalf("asString(json.Number) = %q", got)
	}
	if got := asString("E1"); got != "E1" {
		t.Fatalf("asString(string) = %q", got)
	}
	if got := asString(nil); got != "" {
		t.Fatalf("asString(nil) = %q", got)
	}
}

func TestAsString_Bool(t *testing.T) {
	if got := asString(true); got != "true" {
		t.Errorf("asString(true) = %q, want true", got)
	}
	if got := asString(false); got != "false" {
		t.Errorf("asString(false) = %q, want false", got)
	}
}

func TestAsString_Default(t *testing.T) {
	if got := asString(42); got != "42" {
		t.Errorf("asString(42) = %q, want 42", got)
	}
}

// ── statusIsZero ─────────────────────────────────────────────────────────────

func TestStatusIsZero_Float64(t *testing.T) {
	if !statusIsZero(float64(0)) {
		t.Error("float64(0) must be zero")
	}
	if statusIsZero(float64(1)) {
		t.Error("float64(1) must not be zero")
	}
}

func TestStatusIsZero_Int(t *testing.T) {
	if !statusIsZero(int(0)) {
		t.Error("int(0) must be zero")
	}
	if statusIsZero(int(3)) {
		t.Error("int(3) must not be zero")
	}
}

func TestStatusIsZero_Int64(t *testing.T) {
	if !statusIsZero(int64(0)) {
		t.Error("int64(0) must be zero")
	}
	if statusIsZero(int64(99)) {
		t.Error("int64(99) must not be zero")
	}
}

func TestStatusIsZero_String(t *testing.T) {
	if !statusIsZero("") {
		t.Error("empty string must be zero")
	}
	if !statusIsZero("0") {
		t.Error(`"0" string must be zero`)
	}
	if statusIsZero("1") {
		t.Error(`"1" string must not be zero`)
	}
}

func TestStatusIsZero_Nil(t *testing.T) {
	if !statusIsZero(nil) {
		t.Error("nil must be zero")
	}
}

func TestStatusIsZero_JSONNumber(t *testing.T) {
	if !statusIsZero(json.Number("0")) {
		t.Error("json.Number(0) must be zero")
	}
	if statusIsZero(json.Number("5")) {
		t.Error("json.Number(5) must not be zero")
	}
}

func TestStatusIsZero_Default(t *testing.T) {
	if statusIsZero([]string{"x"}) {
		t.Error("unexpected type must not be zero")
	}
}

// ── mapField ──────────────────────────────────────────────────────────────────

func TestMapField_NonMap(t *testing.T) {
	if got := mapField("not a map", "key"); got != nil {
		t.Errorf("mapField(string, ...) = %v, want nil", got)
	}
}

func TestMapField_Map(t *testing.T) {
	m := map[string]any{"foo": "bar"}
	if got := mapField(m, "foo"); got != "bar" {
		t.Errorf("mapField(map, foo) = %v, want bar", got)
	}
}

// ── checkoutFailureMessage ───────────────────────────────────────────────────

func TestCheckoutFailureMessage_NoMessage(t *testing.T) {
	body := map[string]any{"status": 3}
	msg := checkoutFailureMessage(body)
	if msg != "request rejected by the server" {
		t.Errorf("expected 'request rejected by the server', got %q", msg)
	}
}

func TestCheckoutFailureMessage_WithMessage(t *testing.T) {
	body := map[string]any{"status": 3, "message": "INVALID_VERSION"}
	msg := checkoutFailureMessage(body)
	if msg != "INVALID_VERSION" {
		t.Errorf("expected INVALID_VERSION, got %q", msg)
	}
}

func TestCheckoutFailureMessage_ZeroStatus(t *testing.T) {
	body := map[string]any{"status": 0, "message": "success"}
	if msg := checkoutFailureMessage(body); msg != "" {
		t.Errorf("status=0 must return empty, got %q", msg)
	}
}

func TestCheckoutFailureMessage_NonMap(t *testing.T) {
	if msg := checkoutFailureMessage("not a map"); msg != "" {
		t.Errorf("non-map must return empty, got %q", msg)
	}
}

func TestCheckoutFailureMessage_NoStatus(t *testing.T) {
	body := map[string]any{"message": "something"}
	if msg := checkoutFailureMessage(body); msg != "" {
		t.Errorf("absent status must return empty, got %q", msg)
	}
}

// ── newCmdDev (internal) ──────────────────────────────────────────────────────

func TestNewCmdDev_Flags(t *testing.T) {
	cmd := newCmdDev(&cmdutil.Factory{})
	for _, name := range []string{"extension-name", "all"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
	}
}

func TestNewCmdDev_RunE_NoSelectionErrors(t *testing.T) {
	cmd := newCmdDev(&cmdutil.Factory{})
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("expected validation error when no extension is selected")
	}
}

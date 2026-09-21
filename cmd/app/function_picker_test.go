package appcmd

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
)

// TestLocalFunctionPickerOptions pins value = dir (compile/release key on it),
// the label's dir·name form, and that non-function extensions are dropped.
func TestLocalFunctionPickerOptions(t *testing.T) {
	locals := []app.LocalExt{
		{Dir: "discount-fn", Name: "Discount", Type: "function"}, // dir + name differ → both
		{Dir: "cart-fn", Name: "cart-fn", Type: "function"},      // name == dir → dir only
		{Dir: "no-name-fn", Name: "", Type: "function"},          // no name → dir only
		{Dir: "my-theme", Name: "Theme", Type: "theme"},          // not a function → dropped
		{Dir: "my-checkout", Name: "Checkout", Type: "checkout"}, // not a function → dropped
	}

	opts := localFunctionPickerOptions(locals)

	if len(opts) != 3 {
		t.Fatalf("got %d options, want 3 function extensions: %+v", len(opts), opts)
	}
	want := []struct{ label, value string }{
		{"discount-fn · Discount", "discount-fn"},
		{"cart-fn", "cart-fn"},
		{"no-name-fn", "no-name-fn"},
	}
	for i, w := range want {
		if opts[i].Label != w.label || opts[i].Value != w.value {
			t.Errorf("opt[%d] = {%q,%q}, want {%q,%q}", i, opts[i].Label, opts[i].Value, w.label, w.value)
		}
	}
}

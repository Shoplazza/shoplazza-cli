package common

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func TestFirstString(t *testing.T) {
	m := map[string]any{
		"id":     json.Number("12345678901234567"), // beyond float64 exact range
		"name":   "",
		"title":  "T-shirt",
		"number": "1001",
	}
	if got := FirstString(m, "id"); got != "12345678901234567" {
		t.Errorf("json.Number id = %q, want full-precision digits", got)
	}
	if got := FirstString(m, "name", "title"); got != "T-shirt" {
		t.Errorf("skip empty then take title, got %q", got)
	}
	if got := FirstString(m, "missing"); got != "" {
		t.Errorf("absent key → empty, got %q", got)
	}
}

func TestListExtractor(t *testing.T) {
	extract := ListExtractor("orders", []string{"id"}, func(o map[string]any) string {
		n := FirstString(o, "number")
		if n == "" {
			return "" // exercise the id fallback for the row that has no number
		}
		return "#" + n
	})
	resp := map[string]any{"orders": []any{
		map[string]any{"id": "o1", "number": "1001"},
		map[string]any{"number": "no-id"},      // skipped: no id
		map[string]any{"id": "o2"},             // empty label → falls back to id
		"not-a-row",                            // skipped: wrong type
	}}
	opts := extract(resp)
	if len(opts) != 2 {
		t.Fatalf("want 2 options (rows without id skipped), got %d: %+v", len(opts), opts)
	}
	if opts[0] != (interact.Option{Label: "#1001", Value: "o1"}) {
		t.Errorf("first option = %+v", opts[0])
	}
	if opts[1].Label != "o2" {
		t.Errorf("empty label must fall back to id, got %q", opts[1].Label)
	}
}

func TestListExtractor_MissingKey(t *testing.T) {
	extract := ListExtractor("orders", []string{"id"}, func(map[string]any) string { return "x" })
	if opts := extract(map[string]any{}); len(opts) != 0 {
		t.Errorf("absent list key → no options, got %+v", opts)
	}
}

func okFetch(payload map[string]any) fetchFunc {
	return func(_ context.Context, _ string, _ map[string]any, out any) error {
		*out.(*map[string]any) = payload
		return nil
	}
}

func TestResolvePicker_ChoosesValue(t *testing.T) {
	p := &ResourcePicker{Path: "/orders", Extract: ListExtractor("orders", []string{"id"},
		func(o map[string]any) string { return FirstString(o, "name") })}
	fetch := okFetch(map[string]any{"orders": []any{map[string]any{"id": "o1", "name": "first"}}})
	var sawTitle string
	var sawOpts []interact.Option
	choose := func(title string, opts []interact.Option) (string, error) {
		sawTitle, sawOpts = title, opts
		return opts[0].Value, nil
	}
	v, ok, err := resolvePicker(context.Background(), p, "pick order", fetch, choose)
	if err != nil || !ok || v != "o1" {
		t.Fatalf("resolvePicker = (%q, %v, %v), want (o1, true, nil)", v, ok, err)
	}
	if sawTitle != "pick order" || len(sawOpts) != 1 || sawOpts[0].Label != "first" {
		t.Errorf("chooser got title=%q opts=%+v", sawTitle, sawOpts)
	}
}

func TestResolvePicker_EmptyPageFallsBack(t *testing.T) {
	p := &ResourcePicker{Extract: ListExtractor("orders", []string{"id"}, func(map[string]any) string { return "" })}
	chooseCalled := false
	choose := func(string, []interact.Option) (string, error) { chooseCalled = true; return "x", nil }
	v, ok, err := resolvePicker(context.Background(), p, "t", okFetch(map[string]any{"orders": []any{}}), choose)
	if err != nil || ok || v != "" {
		t.Errorf("empty page → (\"\", false, nil), got (%q, %v, %v)", v, ok, err)
	}
	if chooseCalled {
		t.Error("must not prompt when there is nothing to pick")
	}
}

func TestResolvePicker_FetchErrorFallsBack(t *testing.T) {
	p := &ResourcePicker{Extract: ListExtractor("orders", []string{"id"}, func(map[string]any) string { return "x" })}
	fetch := func(context.Context, string, map[string]any, any) error { return errors.New("network down") }
	choose := func(string, []interact.Option) (string, error) { return "x", nil }
	v, ok, err := resolvePicker(context.Background(), p, "t", fetch, choose)
	if err != nil || ok {
		t.Errorf("lookup failure must degrade gracefully to (\"\", false, nil), got (%q, %v, %v)", v, ok, err)
	}
}

func TestResolvePicker_ChooseErrorPropagates(t *testing.T) {
	p := &ResourcePicker{Extract: ListExtractor("orders", []string{"id"}, func(o map[string]any) string { return FirstString(o, "id") })}
	fetch := okFetch(map[string]any{"orders": []any{map[string]any{"id": "o1"}}})
	choose := func(string, []interact.Option) (string, error) { return "", output.ErrCanceled() }
	if _, _, err := resolvePicker(context.Background(), p, "t", fetch, choose); err == nil {
		t.Error("a canceled pick must propagate, not fall back")
	}
}

func TestPromptFlag_PickerFillsValue(t *testing.T) {
	f := Flag{Name: "order-id", Picker: &ResourcePicker{
		Extract: ListExtractor("orders", []string{"id"}, func(o map[string]any) string { return FirstString(o, "name") })}}
	fetch := okFetch(map[string]any{"orders": []any{map[string]any{"id": "o9", "name": "Order 9"}}})
	choose := func(_ string, opts []interact.Option) (string, error) { return opts[0].Value, nil }
	got, err := promptFlag(context.Background(), f, fetch, choose)
	if err != nil || got != "o9" {
		t.Errorf("promptFlag with a picker = (%q, %v), want (o9, nil)", got, err)
	}
}

// promptFlagWith dispatch (no picker, or picker yields nothing): a flag with
// Completions uses the enum Select; a plain flag uses text Input.
func promptLeaves() (
	func(string, []string) (string, error),
	func(string, func(string) error) (string, error),
	*string,
) {
	var hit string
	return func(_ string, o []string) (string, error) { hit = "enum"; return o[0], nil },
		func(string, func(string) error) (string, error) { hit = "input"; return "typed", nil },
		&hit
}

func TestPromptFlagWith_CompletionsUseEnumSelect(t *testing.T) {
	se, in, hit := promptLeaves()
	f := Flag{Name: "type", Completions: []string{"basic", "embed"}}
	got, err := promptFlagWith(context.Background(), f, nil, nil, se, in)
	if err != nil || got != "basic" || *hit != "enum" {
		t.Errorf("Completions flag = (%q,%v) via %q, want (basic,nil) via enum", got, err, *hit)
	}
}

func TestPromptFlagWith_PlainFlagUsesInput(t *testing.T) {
	se, in, hit := promptLeaves()
	got, err := promptFlagWith(context.Background(), Flag{Name: "note"}, nil, nil, se, in)
	if err != nil || got != "typed" || *hit != "input" {
		t.Errorf("plain flag = (%q,%v) via %q, want (typed,nil) via input", got, err, *hit)
	}
}

func TestPromptFlagWith_EmptyPickerFallsThrough(t *testing.T) {
	se, in, hit := promptLeaves()
	// picker returns no options → dispatch must fall through to Completions.
	f := Flag{Name: "type", Completions: []string{"a", "b"},
		Picker: &ResourcePicker{Extract: ListExtractor("x", []string{"id"}, func(map[string]any) string { return "" })}}
	fetch := okFetch(map[string]any{"x": []any{}})
	choose := func(string, []interact.Option) (string, error) { t.Fatal("empty picker must not open the fuzzy select"); return "", nil }
	if _, err := promptFlagWith(context.Background(), f, fetch, choose, se, in); err != nil {
		t.Fatal(err)
	}
	if *hit != "enum" {
		t.Errorf("empty picker must fall through to Completions, went to %q", *hit)
	}
}

func TestWithPicker_DoesNotMutateOriginal(t *testing.T) {
	base := IDFlag("Product ID")
	p := &ResourcePicker{Path: "/x"}
	withP := base.WithPicker(p)
	if base.Picker != nil {
		t.Error("WithPicker must return a copy, not mutate the original flag")
	}
	if withP.Picker != p {
		t.Error("returned copy must carry the picker")
	}
}

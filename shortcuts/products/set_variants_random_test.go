package products

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// Random edits on random matrices: every planned write must satisfy the
// server's rules, a repeated add/update must converge, and a repeated
// remove/rename must report what is gone.
func TestSetVariants_RandomizedInvariants(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	for i := range 2000 {
		product := randomProduct(rng)
		action, options, flags := randomEdit(rng)
		flags["id"], flags["action"] = "p-1", action
		name := fmt.Sprintf("#%d %s %q %v", i, action, options, flags)

		res, captured, err := runSetVariants(t, product, options, flags)
		if err != nil {
			requireValidation(t, name, err)
			continue
		}
		if captured == nil {
			if res.Body["no_change"] != true {
				t.Fatalf("%s: no write but no no_change: %+v", name, res.Body)
			}
			continue
		}
		checkPlannedWrite(t, name, product, captured, res.Body, action)

		// Apply the write the way the server does, then repeat the edit.
		next := applyWrite(product, captured)
		res2, captured2, err := runSetVariants(t, next, options, flags)
		switch {
		case err != nil && (action == "remove" || action == "rename"):
			requireValidation(t, name+" (repeat)", err) // what the first run renamed or removed is gone
		case err != nil:
			t.Fatalf("%s: repeat failed: %v", name, err)
		case action == "remove":
			t.Fatalf("%s: repeating a remove must report the missing value", name)
		case flags["sku-template"] != "" || flags["stock"] != "":
			// per-variant overrides always write
		case captured2 != nil || res2.Body["no_change"] != true:
			t.Fatalf("%s: repeat must converge, got write=%v body=%+v", name, captured2 != nil, res2.Body)
		}
	}
}

func requireValidation(t *testing.T, name string, err error) {
	t.Helper()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("%s: want a validation error, got %v", name, err)
	}
}

func randomProduct(rng *rand.Rand) map[string]any {
	names := []string{"Color", "Size", "Fit"}
	pools := [][]string{{"Red", "Blue", "red", "Green"}, {"S", "s", "M", "L"}, {"Slim", "Loose", "slim"}}
	var dims [][]string
	for d := range rng.IntN(4) { // 0 dims = a default-variant product
		vals := slices.Clone(pools[d])
		rng.Shuffle(len(vals), func(a, b int) { vals[a], vals[b] = vals[b], vals[a] })
		dims = append(dims, vals[:1+rng.IntN(len(vals)-1)])
	}
	combos := [][]string{{}}
	for _, vals := range dims {
		var next [][]string
		for _, c := range combos {
			for _, v := range vals {
				next = append(next, append(slices.Clone(c), v))
			}
		}
		combos = next
	}
	// Drop combinations at random: sparse matrices and idle values.
	var kept [][]string
	for _, c := range combos {
		if rng.IntN(4) > 0 {
			kept = append(kept, c)
		}
	}
	if len(kept) == 0 {
		kept = combos[:1]
	}
	needImage := rng.IntN(4) == 0
	variants := []any{}
	for i, c := range kept {
		v := map[string]any{"id": fmt.Sprintf("v-%d", i), "price": fmt.Sprintf("%d.00", 10+i),
			"sku": fmt.Sprintf("K-%d", i), "inventory_quantity": float64(i)}
		for j, val := range c {
			v[fmt.Sprintf("option%d", j+1)] = val
		}
		if needImage {
			v["image_id"] = "img-1"
		}
		variants = append(variants, v)
	}
	options := []any{}
	for j, vals := range dims {
		anyVals := make([]any, len(vals))
		for k, v := range vals {
			anyVals[k] = v
		}
		options = append(options, map[string]any{"name": names[j], "values": anyVals, "position": float64(j + 1)})
	}
	p := map[string]any{"id": "p-1", "options": options, "variants": variants}
	if needImage {
		p["need_variant_image"] = true
		p["images"] = []any{map[string]any{"id": "img-1", "src": "//cdn.example/a.png"}}
	}
	return p
}

func randomEdit(rng *rand.Rand) (string, []string, map[string]string) {
	action := []string{"add", "remove", "update", "rename"}[rng.IntN(4)]
	names := []string{"Color", "Size", "Fit", "color", "Material"}
	pool := []string{"Red", "red", "Blue", "S", "s", "M", "XL", "Slim", "Wine"}
	pick := func() []string {
		vals := slices.Clone(pool)
		rng.Shuffle(len(vals), func(a, b int) { vals[a], vals[b] = vals[b], vals[a] })
		return vals[:1+rng.IntN(3)]
	}
	var options []string
	seen := map[string]bool{}
	for range 1 + rng.IntN(2) {
		name := names[rng.IntN(len(names))]
		if seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		switch {
		case action == "remove" && rng.IntN(3) == 0:
			options = append(options, name)
		case action == "rename" && rng.IntN(3) == 0:
			options = append(options, name+"="+names[rng.IntN(len(names))]+"X")
		case action == "rename":
			vals, news := pick(), pick()
			var pairs []string
			for k, v := range vals {
				pairs = append(pairs, v+"="+news[k%len(news)])
			}
			options = append(options, name+":"+strings.Join(pairs, ","))
		default:
			options = append(options, name+":"+strings.Join(pick(), ","))
		}
	}
	flags := map[string]string{}
	if rng.IntN(2) == 0 {
		flags["price"] = "9"
	}
	if rng.IntN(4) == 0 {
		flags["image"] = "img-1"
	}
	if rng.IntN(10) == 0 {
		flags["sku-template"] = "T-{Color}"
	}
	return action, options, flags
}

// checkPlannedWrite verifies the captured PUT body and the summary.
func checkPlannedWrite(t *testing.T, name string, product, captured, summary map[string]any, action string) {
	t.Helper()
	body := captured["product"].(map[string]any)
	opts := body["options"].([]any)
	optValues := make([]map[string]bool, len(opts))
	for d, o := range opts {
		optValues[d] = map[string]bool{}
		for _, v := range o.(map[string]any)["values"].([]any) {
			optValues[d][v.(string)] = true
		}
	}
	oldTuple := map[string][3]string{}
	idleBefore := map[string]bool{}
	usedBefore := map[string]bool{}
	for _, raw := range product["variants"].([]any) {
		v := raw.(map[string]any)
		oldTuple[v["id"].(string)] = tupleOf(v)
		for _, x := range tupleOf(v) {
			usedBefore[x] = true
		}
	}
	for _, raw := range product["options"].([]any) {
		for _, x := range raw.(map[string]any)["values"].([]any) {
			if !usedBefore[x.(string)] {
				idleBefore[x.(string)] = true
			}
		}
	}
	needImage := product["need_variant_image"] == true

	ids := map[string]bool{}
	tuples := map[[3]string]bool{}
	used := map[string]bool{}
	variants := body["variants"].([]any)
	for _, raw := range variants {
		v := raw.(map[string]any)
		tp := tupleOf(v)
		for d := range 3 {
			if (d < len(opts)) != (tp[d] != "") {
				t.Fatalf("%s: variant %v must fill exactly option1..%d", name, v, len(opts))
			}
			if d < len(opts) && !optValues[d][tp[d]] {
				t.Fatalf("%s: variant %v uses %q, missing from options %v", name, v, tp[d], opts)
			}
			used[tp[d]] = true
		}
		if tuples[tp] {
			t.Fatalf("%s: two variants planned for %v", name, tp)
		}
		tuples[tp] = true
		if id, ok := v["id"].(string); ok {
			if _, known := oldTuple[id]; !known || ids[id] {
				t.Fatalf("%s: id %q unknown or sent twice", name, id)
			}
			ids[id] = true
			continue
		}
		if _, ok := v["price"]; !ok {
			t.Fatalf("%s: new variant %v has no price", name, v)
		}
		if _, ok := v["image_id"]; needImage && !ok {
			t.Fatalf("%s: new variant %v has no image on a product that requires one", name, v)
		}
	}
	// An id-less variant on the tuple of a variant being deleted would be
	// merged into it by the server instead.
	for _, raw := range variants {
		v := raw.(map[string]any)
		if _, ok := v["id"]; ok {
			continue
		}
		for id, tp := range oldTuple {
			if !ids[id] && tp == tupleOf(v) {
				t.Fatalf("%s: new variant %v collides with deleted %s", name, v, id)
			}
		}
	}
	for d := range opts {
		for v := range optValues[d] {
			if !used[v] && !idleBefore[v] && action != "rename" {
				t.Fatalf("%s: value %q has no variant but was not idle before", name, v)
			}
		}
	}
	// add never touches a value that already had no variant.
	if action == "add" {
		before := map[string][]any{}
		for _, raw := range product["options"].([]any) {
			o := raw.(map[string]any)
			before[o["name"].(string)] = o["values"].([]any)
		}
		for d, raw := range opts {
			for _, v := range before[raw.(map[string]any)["name"].(string)] {
				if idleBefore[v.(string)] && !optValues[d][v.(string)] {
					t.Fatalf("%s: idle value %q was dropped by an add", name, v)
				}
			}
		}
	}
	count := func(k string) int { n, _ := summary[k].(int); return n }
	if count("kept") != len(ids) || count("deleted") != len(oldTuple)-len(ids) ||
		count("kept")+count("cloned")+count("created") != len(variants) {
		t.Fatalf("%s: summary counts disagree with the body: %+v (ids=%d variants=%d)", name, summary, len(ids), len(variants))
	}
}

func tupleOf(v map[string]any) [3]string {
	var tp [3]string
	for d := range 3 {
		tp[d], _ = v[fmt.Sprintf("option%d", d+1)].(string)
	}
	return tp
}

// applyWrite returns the product as the server stores it after the PUT:
// id'd variants merge onto the old ones, an empty optionN clears the slot.
func applyWrite(product, captured map[string]any) map[string]any {
	body := captured["product"].(map[string]any)
	olds := map[string]map[string]any{}
	for _, raw := range product["variants"].([]any) {
		v := raw.(map[string]any)
		olds[v["id"].(string)] = v
	}
	var variants []any
	for i, raw := range body["variants"].([]any) {
		v := raw.(map[string]any)
		nv := map[string]any{}
		if id, ok := v["id"].(string); ok {
			for k, x := range olds[id] {
				nv[k] = x
			}
		} else {
			nv["id"] = fmt.Sprintf("n-%d", i)
		}
		for k, x := range v {
			if strings.HasPrefix(k, "option") && x == "" {
				delete(nv, k)
			} else {
				nv[k] = x
			}
		}
		variants = append(variants, nv)
	}
	var options []any
	for j, raw := range body["options"].([]any) {
		o := raw.(map[string]any)
		options = append(options, map[string]any{"name": o["name"], "values": o["values"], "position": float64(j + 1)})
	}
	next := map[string]any{"id": "p-1", "options": options, "variants": variants}
	for _, k := range []string{"need_variant_image", "images"} {
		if x, ok := product[k]; ok {
			next[k] = x
		}
	}
	return next
}

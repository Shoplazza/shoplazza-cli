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

// checkPlannedWrite verifies the captured PUT body and the summary.
func checkPlannedWrite(t *testing.T, name string, product, captured, summary map[string]any, kind string) {
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
			if !used[v] && !idleBefore[v] && kind != "rename" {
				t.Fatalf("%s: value %q has no variant but was not idle before", name, v)
			}
		}
	}
	// add never touches a value that already had no variant.
	if kind == "add" {
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

// Random combined edits: every planned write satisfies the server's rules, and
// an edit made only of --add/--set converges when repeated.
func TestSetVariants_RandomizedCombined(t *testing.T) {
	rng := rand.New(rand.NewPCG(13, 17))
	srv := newEditServer(t)
	for i := range 5000 {
		product := randomProduct(rng)
		args, kind, converges := randomCombinedEdit(rng, product)
		name := fmt.Sprintf("#%d %q", i, args)

		res, captured, err := srv.run(t, product, args...)
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
		checkPlannedWrite(t, name, product, captured, res.Body, kind)
		if _, ok := res.Body["changes"]; !ok {
			t.Fatalf("%s: a combined write must report changes", name)
		}
		if !converges {
			continue
		}
		res2, captured2, err := srv.run(t, applyWrite(product, captured), args...)
		if err != nil || captured2 != nil || res2.Body["no_change"] != true {
			t.Fatalf("%s: repeat must converge, got err=%v write=%v body=%+v", name, err, captured2 != nil, res2.Body)
		}
	}
}

// randomCombinedEdit returns combined-form flags, the checkPlannedWrite kind
// ("rename" when values may be renamed, "add" when only --add) and whether a
// repeat must converge (only --add/--set). Names and values mostly come from
// the product so most edits get past validation.
func randomCombinedEdit(rng *rand.Rand, product map[string]any) ([]string, string, bool) {
	type dim struct {
		name   string
		values []string
	}
	var dims []dim
	for _, raw := range product["options"].([]any) {
		o := raw.(map[string]any)
		d := dim{name: o["name"].(string)}
		for _, v := range o["values"].([]any) {
			d.values = append(d.values, v.(string))
		}
		dims = append(dims, d)
	}
	pool := []string{"Red", "red", "Blue", "S", "s", "M", "XL", "Slim", "Wine"}
	pickDim := func() dim {
		if len(dims) == 0 || rng.IntN(5) == 0 {
			return dim{name: []string{"Color", "Size", "Fit", "color", "Material"}[rng.IntN(5)], values: pool}
		}
		return dims[rng.IntN(len(dims))]
	}
	some := func(vals []string) string {
		vals = slices.Clone(vals)
		rng.Shuffle(len(vals), func(a, b int) { vals[a], vals[b] = vals[b], vals[a] })
		return strings.Join(vals[:1+rng.IntN(min(3, len(vals)))], ",")
	}
	one := func(vals []string) string { return vals[rng.IntN(len(vals))] }
	var args []string
	used := map[string]bool{}
	onlyGrow, onlyAdd, renames := true, true, false
	for range 1 + rng.IntN(3) {
		d := pickDim()
		kind := rng.IntN(6)
		if kind < 4 && used[strings.ToLower(d.name)] {
			continue
		}
		switch kind {
		case 0:
			used[strings.ToLower(d.name)] = true
			args = append(args, "--add="+d.name+":"+some(pool))
			if rng.IntN(2) == 0 {
				args = append(args, "--existing="+d.name+":"+one(pool))
			}
		case 1:
			used[strings.ToLower(d.name)] = true
			args, onlyAdd = append(args, "--set="+d.name+":"+some(append(slices.Clone(d.values), one(pool)))), false
		case 2:
			used[strings.ToLower(d.name)] = true
			args, onlyGrow, onlyAdd = append(args, "--remove="+d.name+":"+some(d.values)), false, false
		case 3:
			used[strings.ToLower(d.name)] = true
			drop := "--drop=" + d.name
			if rng.IntN(3) > 0 {
				drop += "=" + one(d.values)
			}
			args, onlyGrow, onlyAdd = append(args, drop), false, false
		case 4:
			args, onlyGrow, renames = append(args, "--rename="+d.name+":"+one(d.values)+"="+one(pool)+"X"), false, true
		default:
			args, onlyGrow = append(args, "--rename="+d.name+"="+d.name+"X"), false
		}
	}
	if rng.IntN(3) > 0 {
		args = append(args, "--new-price=9")
	}
	if len(dims) > 0 && rng.IntN(4) == 0 {
		d := dims[rng.IntN(len(dims))]
		args = append(args, "--new-price="+d.name+":"+one(d.values)+"=5")
	}
	if rng.IntN(3) == 0 {
		args = append(args, "--new-image=img-1")
	}
	if rng.IntN(6) == 0 {
		args, onlyGrow = append(args, "--set-price=3"), false
	}
	switch {
	case renames:
		return args, "rename", false
	case onlyGrow && onlyAdd:
		return args, "add", true
	}
	return args, "", onlyGrow
}

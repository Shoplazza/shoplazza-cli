package common

import (
	"context"
	"encoding/json"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
)

// ResourcePicker fetches one page of a resource and maps each row to a
// selectable option: the label is shown and fuzzy-filtered, the value is the id
// written back onto the flag. It exists only for interactive humans — the
// engine never invokes it non-interactively.
//
// Path/Query describe the list request; Extract turns the decoded JSON envelope
// into options and is a pure function, so it is unit-tested without a client.
type ResourcePicker struct {
	Path    string
	Query   map[string]any
	Extract func(resp map[string]any) []interact.Option
}

// fetchFunc is the list call the picker makes, injectable so the resolve path
// is testable without a live server. It mirrors client.GetJSONWithQuery.
type fetchFunc func(ctx context.Context, path string, query map[string]any, out any) error

// selectFunc renders the fuzzy picker, injectable for the same reason.
type selectFunc func(title string, options []interact.Option) (string, error)

// resolvePicker fetches the resource page and lets the user choose one. It
// returns ("", false, err) on a genuine error, or ("", false, nil) when there
// is nothing to pick (empty page / no picker) so the caller can fall back to a
// plain text prompt. On a successful choice it returns (value, true, nil).
func resolvePicker(ctx context.Context, p *ResourcePicker, title string, fetch fetchFunc, choose selectFunc) (string, bool, error) {
	if p == nil {
		return "", false, nil
	}
	var resp map[string]any
	if err := fetch(ctx, p.Path, p.Query, &resp); err != nil {
		// Degrade gracefully: the caller falls back to manual entry rather than
		// failing the command just because the lookup call did.
		return "", false, nil
	}
	options := p.Extract(resp)
	if len(options) == 0 {
		return "", false, nil
	}
	value, err := choose(title, options)
	if err != nil {
		return "", false, err // output.ErrCanceled on esc/ctrl+c
	}
	return value, true, nil
}

// clientFetch is the production fetchFunc backed by the factory's HTTP client.
func clientFetch(factory *cmdutil.Factory) fetchFunc {
	return func(ctx context.Context, path string, query map[string]any, out any) error {
		return factory.Client.GetJSONWithQuery(ctx, path, query, out)
	}
}

// FirstString returns the first non-empty value among keys, coercing the common
// JSON scalar shapes an id/label can arrive as (string, or json.Number when the
// client decoded with UseNumber). Missing/other types yield "".
func FirstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case json.Number:
			if s := v.String(); s != "" {
				return s
			}
		}
	}
	return ""
}

// ListExtractor builds a ResourcePicker.Extract for the common list shape
// {"<key>": [ {row}, … ]} (the client already unwrapped the {code,data}
// envelope). idKeys locate each row's value; label builds its display text.
// Rows with no id are skipped; an empty label falls back to the id so the row
// is never blank.
func ListExtractor(key string, idKeys []string, label func(row map[string]any) string) func(map[string]any) []interact.Option {
	return func(resp map[string]any) []interact.Option {
		rows, _ := resp[key].([]any)
		opts := make([]interact.Option, 0, len(rows))
		for _, r := range rows {
			m, ok := r.(map[string]any)
			if !ok {
				continue
			}
			id := FirstString(m, idKeys...)
			if id == "" {
				continue
			}
			lbl := label(m)
			if lbl == "" {
				lbl = id
			}
			opts = append(opts, interact.Option{Label: lbl, Value: id})
		}
		return opts
	}
}

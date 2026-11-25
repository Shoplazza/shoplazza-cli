package dynamic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/rawapi"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"

	"github.com/spf13/cobra"
)

func makeRunE(c registry.Command, spec *registry.Spec, factory *cmdutil.Factory) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		// Auth is gated by the module's PersistentPreRunE; RunE assumes login.
		if err := output.ValidateJQ(cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd)); err != nil {
			return err
		}
		params, err := parseParams(cmd)
		if err != nil {
			return err
		}
		body, err := parseBody(cmd, c, factory)
		if err != nil {
			return err
		}
		schemaHint := "run 'shoplazza schema " + strings.Join(strings.Fields(cmd.CommandPath())[1:], ".") + "'"
		if err := checkParamKeys(c, params, schemaHint); err != nil {
			return err
		}
		splitArrayParams(c, params)
		if err := checkBodyWrapper(c, spec, body, schemaHint); err != nil {
			return err
		}
		if err := checkRequiredBody(c, body, schemaHint); err != nil {
			return err
		}
		hint := dropHashPrefix(c, params)

		// A human is prompted for any {param} the --params JSON left unset (e.g.
		// theme_id); non-interactive callers are untouched, so a missing param
		// stays the structured error below (agents/CI keep fail-fast).
		if err := fillMissingPathParams(cmd, factory, c.HTTP.Path, params); err != nil {
			return err
		}
		if err := checkRequiredParams(c, params, schemaHint); err != nil {
			return err
		}

		resolvedPath, remainingQuery, err := rawapi.ResolveTemplatedPath(c.HTTP.Path, params)
		if err != nil {
			return output.ErrValidation("%v", err).WithHint(schemaHint + " to see required parameters")
		}

		req := client.RawRequest{
			Method: strings.ToUpper(c.HTTP.Method),
			Path:   resolvedPath,
			Params: remainingQuery,
			Data:   body,
		}

		format := cmdutil.GetFormat(cmd)
		jq := cmdutil.GetJQ(cmd)
		out := factory.IOStreams.Out

		if cmdutil.IsDryRun(cmd) {
			preview := map[string]any{
				"dry_run": true,
				"request": factory.Client.BuildRequestSummary(req.Method, req.Path, req.Params, req.Data),
			}
			if hint != "" {
				preview["hint"] = hint
			}
			return output.PrintBody(out, preview, format, jq)
		}

		// Human-only confirmation for irreversible writes (DELETE / cancel / …);
		// non-interactive callers (agents, pipes, CI) are gated out and proceed
		// unchanged. --dry-run already returned above, so it never reaches here.
		if isDestructive(c) {
			if err := cmdutil.ConfirmDestructive(factory,
				"Run '"+cmd.CommandPath()+"'? This is a destructive, irreversible write."); err != nil {
				return err
			}
		}

		ctx := context.Background()
		resp, err := factory.Client.DoRaw(ctx, req)
		if err != nil {
			var httpErr *client.HTTPError
			if errors.As(err, &httpErr) {
				return output.ErrAPI(httpErr.StatusCode, httpErr.Body, resp.RequestID()).WithEndpoint(httpErr.Method, httpErr.Path)
			}
			var netErr net.Error
			if errors.As(err, &netErr) {
				return output.ErrNetwork("%v", err)
			}
			return output.ErrInternal("%v", err)
		}
		return output.WithSentRequest(output.PrintAPISuccess(out, resp.Body, format, jq),
			req.Method, req.Path, resp.StatusCode, resp.RequestID())
	}
}

// checkParamKeys rejects --params keys that are neither a declared parameter
// nor a {var} of the path template. A key that is a body field gets its own
// subtype, so a caller knows to move it into --data rather than fix a typo.
func checkParamKeys(c registry.Command, params map[string]any, schemaHint string) error {
	declared := map[string]bool{}
	for _, p := range c.Parameters {
		declared[p.Name] = true
	}
	for _, name := range pathParamNames(c.HTTP.Path) {
		declared[name] = true
	}
	bodyField := map[string]bool{}
	if c.Body != nil {
		for _, f := range c.Body.Fields {
			bodyField[f.Name] = true
		}
	}
	var misplaced, unknown []string
	for k := range params {
		switch {
		case declared[k]:
		case bodyField[k]:
			misplaced = append(misplaced, k)
		default:
			unknown = append(unknown, k)
		}
	}
	if len(misplaced) > 0 {
		sort.Strings(misplaced)
		return output.ErrValidation("%s %s of this command, not %s", strings.Join(misplaced, ", "),
			plural(misplaced, "is a request-body field", "are request-body fields"),
			plural(misplaced, "a path/query parameter", "path/query parameters")).
			WithParam(misplaced[0]).
			WithSubtype(output.SubtypeMisplacedParam).
			WithHint("move " + plural(misplaced, "it", "them") + " from --params into --data")
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		accepted := make([]string, 0, len(declared))
		for k := range declared {
			accepted = append(accepted, k)
		}
		sort.Strings(accepted)
		hint := "this command takes no --params"
		if len(accepted) > 0 {
			hint = "accepted --params keys: " + strings.Join(accepted, ", ")
		}
		return output.ErrValidation("unknown --params %s: %s", plural(unknown, "key", "keys"), strings.Join(unknown, ", ")).
			WithParam(unknown[0]).
			WithSubtype(output.SubtypeUnknownParam).
			WithHint(hint + "; " + schemaHint + " for details")
	}
	return nil
}

func splitArrayParams(c registry.Command, params map[string]any) {
	for _, p := range c.Parameters {
		if p.In != "query" || p.Type != "array" {
			continue
		}
		s, ok := params[p.Name].(string)
		if !ok || !strings.Contains(s, ",") {
			continue
		}
		items := []any{}
		for _, part := range strings.Split(s, ",") {
			if part = strings.TrimSpace(part); part != "" {
				items = append(items, part)
			}
		}
		params[p.Name] = items
	}
}

// checkBodyWrapper rejects top-level --data keys that belong inside a body object field.
func checkBodyWrapper(c registry.Command, spec *registry.Spec, body any, schemaHint string) error {
	m, _ := body.(map[string]any)
	if c.Body == nil || spec == nil || len(m) == 0 {
		return nil
	}
	top := map[string]bool{}
	for _, f := range c.Body.Fields {
		top[f.Name] = true
	}
	owner := map[string]string{}
	for k := range m {
		if top[k] {
			continue
		}
		var hits []string
		for _, f := range c.Body.Fields {
			if f.Type == "object" && slices.ContainsFunc(spec.Schemas[f.Schema].Fields, func(sf registry.Field) bool { return sf.Name == k }) {
				hits = append(hits, f.Name)
			}
		}
		if len(hits) == 1 {
			owner[k] = hits[0]
		}
	}
	if len(owner) == 0 {
		return nil
	}
	fixed := maps.Clone(m)
	for k, w := range owner {
		inner := map[string]any{}
		if v, ok := fixed[w]; ok {
			given, isMap := v.(map[string]any)
			if !isMap {
				return nil
			}
			inner = maps.Clone(given)
		}
		inner[k] = fixed[k]
		fixed[w] = inner
		delete(fixed, k)
	}
	wrappers := slices.Sorted(maps.Values(owner))
	wrappers = slices.Compact(wrappers)
	quoted := make([]string, len(wrappers))
	for i, w := range wrappers {
		quoted[i] = strconv.Quote(w)
	}
	return output.ErrValidation("--data is missing the %s %s: %s", strings.Join(quoted, ", "),
		plural(wrappers, "wrapper", "wrappers"), renderBody(fixed)).
		WithParam(wrappers[0]).
		WithSubtype(output.SubtypeMissingRequiredField).
		WithHint(schemaHint + " to see the body fields")
}

func renderBody(m map[string]any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err == nil && buf.Len() <= 200 {
		return strings.TrimSpace(buf.String())
	}
	return bodyKeys(m)
}

func bodyKeys(m map[string]any) string {
	keys := slices.Sorted(maps.Keys(m))
	parts := make([]string, len(keys))
	for i, k := range keys {
		v := "…"
		if inner, ok := m[k].(map[string]any); ok {
			v = bodyKeys(inner)
		}
		parts[i] = strconv.Quote(k) + ":" + v
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// checkRequiredBody rejects a request whose --data lacks a required top-level
// body field. Nested fields are not checked: the metadata marks some of them
// required on partial-update endpoints (e.g. variant.price on variants update).
func checkRequiredBody(c registry.Command, body any, schemaHint string) error {
	if c.Body == nil || !commandHasBody(c) {
		return nil
	}
	m, _ := body.(map[string]any)
	var missing []string
	for _, f := range c.Body.Fields {
		if v, ok := m[f.Name]; f.Required && (!ok || v == nil) {
			missing = append(missing, f.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return output.ErrValidation("--data is missing required %s: %s", plural(missing, "field", "fields"), strings.Join(missing, ", ")).
		WithParam(missing[0]).
		WithSubtype(output.SubtypeMissingRequiredField).
		WithHint(schemaHint + " to see the body fields")
}

// checkRequiredParams reports, in one error, every path {var} and required query
// parameter that --params leaves absent, empty, or as an unfilled placeholder.
func checkRequiredParams(c registry.Command, params map[string]any, schemaHint string) error {
	required := pathParamNames(c.HTTP.Path)
	for _, p := range c.Parameters {
		if p.In == "query" && p.Required {
			required = append(required, p.Name)
		}
	}
	var names, listed []string
	for _, name := range required {
		v, ok := params[name]
		list, isList := v.([]any)
		s, isString := v.(string)
		switch {
		case !ok || v == nil || (isString && strings.TrimSpace(s) == "") || (isList && len(list) == 0):
			listed = append(listed, name)
		case isString && isPlaceholder(s):
			listed = append(listed, fmt.Sprintf("%s (placeholder %q)", name, s))
		default:
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}
	return output.ErrValidation("--params is missing required %s: %s", plural(names, "key", "keys"), strings.Join(listed, ", ")).
		WithParam(names[0]).
		WithSubtype(output.SubtypeMissingRequiredParam).
		WithHint(schemaHint + " to see required parameters")
}

func plural(items []string, one, many string) string {
	if len(items) == 1 {
		return one
	}
	return many
}

func parseParams(cmd *cobra.Command) (map[string]any, error) {
	raw, _ := cmd.Flags().GetString("params")
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := cmdutil.DecodeJSON([]byte(raw), &out); err != nil {
		return nil, output.ErrValidation("%v", cmdutil.JSONInputError("--params", []byte(raw), err, "object"))
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func parseBody(cmd *cobra.Command, c registry.Command, factory *cmdutil.Factory) (any, error) {
	if !commandHasBody(c) {
		return nil, nil
	}
	data, _ := cmd.Flags().GetString("data")
	if data == "" {
		return nil, nil
	}
	body, err := cmdutil.ParseJSONMap(data, "--data", factory.IOStreams.In)
	if err != nil {
		return nil, output.ErrValidation("%v", err)
	}
	return body, nil
}

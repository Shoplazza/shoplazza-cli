package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
	"github.com/Shoplazza/shoplazza-cli/internal/registry"

	"github.com/spf13/cobra"
)

func NewCmdSchema(spec *registry.Spec) *cobra.Command {
	var view string
	cmd := &cobra.Command{
		Use:   "schema [path]",
		Short: "View API command parameters, body, response",
		Long: `View API command parameters, body, response.

Examples:
  # List all available modules
  shoplazza schema

  # List all commands in the orders module
  shoplazza schema orders

  # Show full schema for a specific command
  shoplazza schema orders.count

  # Show only request parameters and body
  shoplazza schema orders.create --view request`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			if err := validateView(view); err != nil {
				return err
			}
			var path string
			if len(args) == 1 {
				path = args[0]
			}
			// --view only filters leaf command detail, not module overviews.
			if path != "" && !strings.Contains(path, ".") && view != registry.ViewAll {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"Note: --view is only effective for leaf commands (e.g. 'shoplazza schema %s.get --view %s').\n",
					path, view,
				)
			}

			payload, ok, err := registry.ResolveSpecSchema(spec, path, view)
			if err != nil {
				return err
			}
			if !ok {
				return output.ErrValidation("unknown schema: %s", path)
			}
			// JSON keeps the curated key ordering via orderedFields; pretty/table sort keys themselves, so pass the raw map.
			switch cmdutil.GetFormat(cmd) {
			case "pretty", "table":
				return output.PrintFormatted(w, payload, cmdutil.GetFormat(cmd))
			default:
				return output.PrintJSON(w, reorderSchemaPayload(payload))
			}
		},
	}
	cmd.Flags().StringVar(&view, "view", registry.ViewAll, "Schema view selector (all|request|response). Filters parameters/body/response on leaf commands; ignored on module list and overview.")
	return cmd
}

func validateView(view string) error {
	switch view {
	case "", registry.ViewAll, registry.ViewRequest, registry.ViewResponse:
		return nil
	default:
		return output.ErrValidation("unknown --view %q; expected one of: all, request, response", view)
	}
}

// schemaKeyOrder controls JSON key order in `shoplazza schema` output; unlisted keys sort alphabetically at the end.
var schemaKeyOrder = []string{
	"path", "summary",
	"module", "group", "target",
	"id", "description",
	"http",
	"parameters", "body", "response",
	"commands", "modules", "scopes",
}

type orderedFields []orderedField

type orderedField struct {
	Key   string
	Value any
}

func (o orderedFields) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(f.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(f.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func reorderSchemaPayload(payload any) any {
	switch v := payload.(type) {
	case map[string]any:
		return reorderMap(v)
	case []map[string]any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = reorderMap(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = reorderSchemaPayload(item)
		}
		return out
	default:
		return payload
	}
}

func reorderMap(m map[string]any) orderedFields {
	out := make(orderedFields, 0, len(m))
	seen := map[string]struct{}{}
	for _, k := range schemaKeyOrder {
		if v, exists := m[k]; exists {
			out = append(out, orderedField{Key: k, Value: reorderSchemaPayload(v)})
			seen[k] = struct{}{}
		}
	}
	var leftover []string
	for k := range m {
		if _, ok := seen[k]; !ok {
			leftover = append(leftover, k)
		}
	}
	sort.Strings(leftover)
	for _, k := range leftover {
		out = append(out, orderedField{Key: k, Value: reorderSchemaPayload(m[k])})
	}
	return out
}

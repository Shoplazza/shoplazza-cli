package dynamic

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	annotationParam     = "shoplazza.param"
	annotationFlagGroup = "shoplazza.flaggroup"

	groupRequired = "required"
	groupOptional = "optional"
	groupRaw      = "raw"
)

// idFlagByPath names the flag of an `id` parameter where the path segment before {id} does not.
var idFlagByPath = map[string]string{
	"/discounts/automatic/{id}":                    "discount-id",
	"/discounts/non-automatic/{id}":                "discount-id",
	"/collections/async-task/{id}":                 "task-id",
	"/metafields-shop/{id}":                        "metafield-id",
	"/metafields/{owner_resource}/{owner_id}/{id}": "metafield-id",
	"/metafield_definition":                        "metafield-definition-id",
}

var apiVersionPrefix = regexp.MustCompile(`^/openapi/[^/]+`)

type boundParam struct {
	param registry.Parameter
	flag  string
	value *paramValue
}

type paramsOnlyParam struct {
	param   registry.Parameter
	claimed *pflag.Flag
}

type paramBinder struct {
	bound      []boundParam
	paramsOnly []paramsOnlyParam
}

// commandParams returns the declared parameters plus any path {var} the metadata leaves out.
func commandParams(c registry.Command) []registry.Parameter {
	params := append([]registry.Parameter(nil), c.Parameters...)
	declared := map[string]bool{}
	for _, p := range params {
		declared[p.Name] = true
	}
	for _, name := range pathParamNames(c.HTTP.Path) {
		if !declared[name] {
			declared[name] = true
			params = append(params, registry.Parameter{Name: name, In: "path", Type: "string", Required: true})
		}
	}
	return params
}

func kebab(name string) string { return strings.ReplaceAll(name, "_", "-") }

func idFlagName(path string) string {
	path = apiVersionPrefix.ReplaceAllString(path, "")
	if name, ok := idFlagByPath[path]; ok {
		return name
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, seg := range segments {
		if seg != "{id}" || i == 0 || strings.HasPrefix(segments[i-1], "{") {
			continue
		}
		resource := kebab(segments[i-1])
		if strings.HasSuffix(resource, "s") && !strings.HasSuffix(resource, "ss") {
			resource = strings.TrimSuffix(resource, "s")
		}
		return resource + "-id"
	}
	return "id"
}

// bindParamFlags registers one flag per path/query parameter on cmd, after its
// own flags. A parameter whose flag name is taken stays reachable through --params.
func bindParamFlags(cmd *cobra.Command, c registry.Command, reserved *pflag.FlagSet) *paramBinder {
	cmd.InitDefaultHelpFlag()
	taken := func(name string) *pflag.Flag {
		if fl := cmd.Flags().Lookup(name); fl != nil {
			return fl
		}
		if reserved != nil {
			return reserved.Lookup(name)
		}
		return nil
	}
	params := commandParams(c)
	plain := map[string]bool{}
	for _, p := range params {
		if p.Name != "id" {
			plain[kebab(p.Name)] = true
		}
	}

	b := &paramBinder{}
	for _, p := range params {
		name := kebab(p.Name)
		if p.Name == "id" {
			if alt := idFlagName(c.HTTP.Path); !plain[alt] && taken(alt) == nil {
				name = alt
			}
		}
		if claimed := taken(name); claimed != nil {
			b.paramsOnly = append(b.paramsOnly, paramsOnlyParam{param: p, claimed: claimed})
			continue
		}
		value := newParamValue(p)
		fl := cmd.Flags().VarPF(value, name, "", paramUsage(p))
		group := groupOptional
		if p.Required {
			group = groupRequired
		}
		fl.Annotations = map[string][]string{annotationParam: {p.Name}, annotationFlagGroup: {group}}
		b.bound = append(b.bound, boundParam{param: p, flag: name, value: value})
		if p.Name == "id" && name != "id" && taken("id") == nil {
			cmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, given string) pflag.NormalizedName {
				if given == "id" {
					return pflag.NormalizedName(name)
				}
				return pflag.NormalizedName(given)
			})
		}
	}
	return b
}

// ParamFlags maps each parameter of a spec leaf to its flag ("product_id" -> "--product-id").
func ParamFlags(leaf *cobra.Command) map[string]string {
	flags := map[string]string{}
	leaf.Flags().VisitAll(func(fl *pflag.Flag) {
		if param := fl.Annotations[annotationParam]; len(param) == 1 {
			flags[param[0]] = "--" + fl.Name
		}
	})
	return flags
}

func (b *paramBinder) flagFor(param string) string {
	for _, bp := range b.bound {
		if bp.param.Name == param {
			return "--" + bp.flag
		}
	}
	return ""
}

// overlay writes each flag the caller set over the same key of params.
func (b *paramBinder) overlay(cmd *cobra.Command, params map[string]any) {
	for _, bp := range b.bound {
		if !cmd.Flags().Changed(bp.flag) {
			continue
		}
		if v, ok := bp.value.get(); ok {
			params[bp.param.Name] = v
		}
	}
}

func (b *paramBinder) paramsOnlyHelp() string {
	if len(b.paramsOnly) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\nParameters set via --params (the flag name is taken):\n")
	for _, p := range b.paramsOnly {
		fmt.Fprintf(&sb, "  %s: --params '{%q: \"<value>\"}'\n", p.param.Name, p.param.Name)
		if usage := paramUsage(p.param); usage != "" {
			fmt.Fprintf(&sb, "      %s\n", strings.ReplaceAll(usage, "\n", "\n      "))
		}
		fmt.Fprintf(&sb, "      do not use --%s (%s)\n", p.claimed.Name, p.claimed.Usage)
	}
	return sb.String()
}

func paramUsage(p registry.Parameter) string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(p.Description, "`", "'"), "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	desc := strings.Join(lines, "\n")
	parts := []string{}
	if desc = strings.TrimSuffix(desc, "."); desc != "" {
		parts = append(parts, desc)
	}
	if p.Type == "array" {
		parts = append(parts, "Repeat or comma-separate")
	}
	if p.Default != nil {
		parts = append(parts, "API default: "+scalarText(p.Default))
	}
	var bounds []string
	if p.Minimum != nil {
		bounds = append(bounds, "min "+scalarText(p.Minimum))
	}
	if p.Maximum != nil {
		bounds = append(bounds, "max "+scalarText(p.Maximum))
	}
	if len(bounds) > 0 {
		parts = append(parts, strings.Join(bounds, ", "))
	}
	if len(p.Enum) > 0 {
		options := make([]string, len(p.Enum))
		for i, e := range p.Enum {
			options[i] = scalarText(e)
		}
		parts = append(parts, "one of: "+strings.Join(options, "|"))
	}
	return strings.Join(parts, ". ")
}

func scalarText(v any) string {
	if f, ok := v.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

var (
	integerText = regexp.MustCompile(`^-?(0|[1-9]\d*)$`)
	numberText  = regexp.MustCompile(`^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$`)
)

// paramValue is the pflag.Value of one parameter flag. Numbers keep the digits
// the caller typed; a boolean needs its value written out.
type paramValue struct {
	kind  string
	items string
	text  string
	list  []any
}

func newParamValue(p registry.Parameter) *paramValue {
	v := &paramValue{kind: p.Type}
	if p.Items != nil {
		v.items = p.Items.Type
	}
	return v
}

func (v *paramValue) String() string { return "" }

func (v *paramValue) Type() string {
	switch v.kind {
	case "integer":
		return "int"
	case "number":
		return "number"
	case "boolean":
		return "true|false"
	case "array":
		if v.items == "integer" {
			return "ints"
		}
		return "strings"
	}
	return "string"
}

func (v *paramValue) Set(s string) error {
	switch v.kind {
	case "integer":
		if s = strings.TrimSpace(s); !integerText.MatchString(s) {
			return fmt.Errorf("must be an integer")
		}
	case "number":
		if s = strings.TrimSpace(s); !numberText.MatchString(s) {
			return fmt.Errorf("must be a number")
		}
	case "boolean":
		if s = strings.ToLower(strings.TrimSpace(s)); s != "true" && s != "false" {
			return fmt.Errorf("must be true or false")
		}
	case "array":
		for _, item := range strings.Split(s, ",") {
			if item = strings.TrimSpace(item); item == "" {
				continue
			}
			if v.items != "integer" {
				v.list = append(v.list, item)
				continue
			}
			if !integerText.MatchString(item) {
				return fmt.Errorf("%q must be an integer", item)
			}
			v.list = append(v.list, json.Number(item))
		}
		return nil
	}
	v.text = s
	return nil
}

// get returns the value to send, or false when the flag was given nothing to send.
func (v *paramValue) get() (any, bool) {
	switch v.kind {
	case "array":
		return v.list, len(v.list) > 0
	case "boolean":
		return v.text == "true", true
	case "integer", "number":
		return json.Number(v.text), true
	}
	return v.text, v.text != ""
}

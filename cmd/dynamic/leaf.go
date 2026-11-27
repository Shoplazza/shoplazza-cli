package dynamic

import (
	"fmt"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// buildLeafCommand creates the cobra leaf for a single Command. reserved holds
// the root's persistent flags, whose names a parameter flag must not take.
func buildLeafCommand(c registry.Command, spec *registry.Spec, factory *cmdutil.Factory, moduleName string, reserved *pflag.FlagSet) *cobra.Command {
	short := c.Summary
	if short == "" {
		short = c.ID
	}

	// Long = optional description plus a hint to the schema path for parameters.
	var long string
	if c.Description != "" {
		long = c.Description + "\n\n"
	}
	long += fmt.Sprintf(
		"Run 'shoplazza schema %s.%s' to view required parameters and response shape.",
		moduleName, strings.Join(c.Path, "."),
	)

	leaf := &cobra.Command{
		Use:    c.Path[len(c.Path)-1],
		Short:  short,
		Long:   long,
		Hidden: c.Hidden,
		Args:   cobra.NoArgs,
	}
	leaf.Flags().String("params", "", "JSON for path/query parameters (inline, @file, or - for stdin). A parameter flag overrides the same key")
	if commandHasBody(c) {
		leaf.Flags().String("data", "", "JSON for request body (inline, @file, or - for stdin)")
	}
	leaf.Flags().Bool("dry-run", false, "Print the request that would be sent without executing it")
	leaf.Flags().StringP("jq", "q", "", "jq expression to filter JSON output (e.g. '.data.products[].id')")
	for _, name := range []string{"params", "data"} {
		if fl := leaf.Flags().Lookup(name); fl != nil {
			fl.Annotations = map[string][]string{annotationFlagGroup: {groupRaw}}
		}
	}
	binder := bindParamFlags(leaf, c, reserved)
	leaf.RunE = makeRunE(c, spec, factory, binder)
	leaf.SetUsageFunc(func(cmd *cobra.Command) error {
		_, err := fmt.Fprint(cmd.OutOrStderr(), leafUsage(cmd, binder))
		return err
	})
	return leaf
}

// leafUsage renders a spec leaf's usage with its flags grouped: API parameters
// (required, then optional), the raw JSON inputs, then everything else.
func leafUsage(cmd *cobra.Command, binder *paramBinder) string {
	groups := map[string]*pflag.FlagSet{}
	cmd.LocalFlags().VisitAll(func(fl *pflag.Flag) {
		group := ""
		if g := fl.Annotations[annotationFlagGroup]; len(g) == 1 {
			group = g[0]
		}
		if groups[group] == nil {
			groups[group] = pflag.NewFlagSet(group, pflag.ContinueOnError)
		}
		groups[group].AddFlag(fl)
	})
	usages := func(group string) string {
		if groups[group] == nil {
			return ""
		}
		return strings.TrimRight(groups[group].FlagUsages(), " \n") + "\n"
	}

	var sb strings.Builder
	sb.WriteString("Usage:\n  " + cmd.UseLine() + "\n")
	required, optional, paramsOnly := usages(groupRequired), usages(groupOptional), binder.paramsOnlyHelp()
	if required+optional+paramsOnly != "" {
		sb.WriteString("\nAPI Parameters:\n")
		if required != "" {
			sb.WriteString("  Required:\n" + required)
		}
		if optional != "" {
			sb.WriteString("  Optional:\n" + optional)
		}
		sb.WriteString(paramsOnly)
	}
	sb.WriteString("\nRaw Input:\n" + usages(groupRaw))
	sb.WriteString("\nFlags:\n" + usages(""))
	if cmd.HasAvailableInheritedFlags() {
		sb.WriteString("\nGlobal Flags:\n" + strings.TrimRight(cmd.InheritedFlags().FlagUsages(), " \n") + "\n")
	}
	return sb.String()
}

func commandHasBody(c registry.Command) bool {
	if c.HTTP.Body != "*" {
		return false
	}
	switch strings.ToUpper(c.HTTP.Method) {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

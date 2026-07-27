package dynamic

import (
	"fmt"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"

	"github.com/spf13/cobra"
)

// buildLeafCommand creates the cobra leaf for a single Command.
func buildLeafCommand(c registry.Command, spec *registry.Spec, factory *cmdutil.Factory, moduleName string) *cobra.Command {
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
		RunE:   makeRunE(c, spec, factory),
	}
	leaf.Flags().String("params", "", "JSON for path/query parameters")
	if commandHasBody(c) {
		leaf.Flags().String("data", "", "JSON for request body (inline, @file, or - for stdin)")
	}
	leaf.Flags().Bool("dry-run", false, "Print the request that would be sent without executing it")
	leaf.Flags().StringP("jq", "q", "", "jq expression to filter JSON output (e.g. '.data.products[].id')")
	return leaf
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

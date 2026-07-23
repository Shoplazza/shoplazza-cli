package common

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/output"

	"github.com/spf13/cobra"
)

// noPositionalArgs rejects stray positional args with a comma-separation hint —
// the common case is "--variants a b" where only "a" binds and "b" is dropped.
func noPositionalArgs(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		return output.ErrValidation("unexpected argument %q; this command takes only flags. Quote values containing spaces (--name \"a b\") or comma-separate multiple values (--variants a,b,c)", args[0])
	}
	return nil
}

// Mount registers s as a cobra subcommand under parent.
//
// It panics at mount time if a Flag declaration's Default is incompatible with
// its Type, surfacing declaration bugs early.
func Mount(s Shortcut, parent *cobra.Command, factory *cmdutil.Factory) {
	// Shortcuts take only flags; reject stray positional args unless one opts in.
	args := s.Args
	if args == nil {
		args = noPositionalArgs
	}
	cmd := &cobra.Command{
		Use:   s.Use,
		Short: s.Short,
		Long:  s.Long,
		Args:  args,
	}
	annotations := map[string]string{}
	if s.AuthFree {
		// Purely local command: the auth gate honors this annotation.
		annotations[cmdutil.AnnotationAuthFree] = "true"
	}
	if s.NotScannable {
		// Interactive/long-running/local-write: blind CLI scans skip it.
		annotations[cmdutil.AnnotationNotScannable] = "true"
	}
	if len(annotations) > 0 {
		cmd.Annotations = annotations
	}

	for _, f := range s.Flags {
		bindFlag(cmd, f)
	}
	cmd.Flags().Bool("dry-run", false, "Print the request that would be sent without executing it")
	cmd.Flags().StringP("jq", "q", "", "jq expression to filter JSON output (e.g. '.data.products[].id')")
	for _, f := range s.Flags {
		if f.Required {
			_ = cmd.MarkFlagRequired(f.Name)
		}
		if len(f.Completions) > 0 {
			values := append([]string(nil), f.Completions...)
			_ = cmd.RegisterFlagCompletionFunc(f.Name, func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
				return values, cobra.ShellCompDirectiveNoFileComp
			})
		}
	}

	tool := strings.TrimPrefix(s.Command, "+")
	plan := s.Plan
	exec := s.Execute
	local := s.Local

	cmd.RunE = func(c *cobra.Command, args []string) error {
		dryRun := cmdutil.IsDryRun(c)
		format := cmdutil.GetFormat(c)
		jq := cmdutil.GetJQ(c)
		flags := NewCobraFlagSet(c)

		if exec != nil {
			in := ExecInput{
				Args:   args,
				Flags:  flags,
				Tool:   tool,
				Client: factory.Client,
				DryRun: dryRun,
			}
			result, err := exec(c.Context(), in)
			if err != nil {
				return classifyExecError(err)
			}
			if dryRun {
				summaries := make([]any, 0, len(result.Plans))
				for _, p := range result.Plans {
					summaries = append(summaries, factory.Client.BuildRequestSummary(p.Method, p.Path, p.Query, p.Body))
				}
				envelope := map[string]any{
					"dry_run":  true,
					"requests": summaries,
				}
				return output.PrintBody(c.OutOrStdout(), envelope, format, jq)
			}
			if local {
				// Local result (file paths, counts), not an API response, so no {ok,data} envelope.
				return output.PrintBody(c.OutOrStdout(), result.Body, format, jq)
			}
			return output.PrintAPISuccess(c.OutOrStdout(), result.Body, format, jq)
		}

		in := PlanInput{
			Args:  args,
			Flags: flags,
			Tool:  tool,
		}
		p, err := plan(in)
		if err != nil {
			return err
		}
		if dryRun {
			return output.PrintBody(c.OutOrStdout(), DryRun(factory.Client, p), format, jq)
		}
		resp, err := Send(c.Context(), factory.Client, p)
		if err != nil {
			return classifySendError(err)
		}
		return output.PrintAPISuccess(c.OutOrStdout(), resp, format, jq)
	}

	parent.AddCommand(cmd)
}

// classifySendError lifts a raw client error from Send into an output.ExitError
// so the root error handler emits a clean JSON envelope with the right exit code
// and 403→auth reclassification. The API envelope omits the request-id because
// Send discards the response wrapper on error.
func classifySendError(err error) error {
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		return output.ErrAPI(httpErr.StatusCode, httpErr.Body, "").WithEndpoint(httpErr.Method, httpErr.Path)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return output.ErrNetwork("%v", err)
	}
	return output.ErrInternal("%v", err)
}

// classifyExecError lifts an error from an Execute handler into an
// output.ExitError for the root error handler. An existing *output.ExitError is
// returned as-is so its original detail and exit code are preserved.
func classifyExecError(err error) error {
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		return exitErr
	}
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		return output.ErrAPI(httpErr.StatusCode, httpErr.Body, "").WithEndpoint(httpErr.Method, httpErr.Path)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return output.ErrNetwork("%v", err)
	}
	return output.ErrInternal("%v", err)
}

func bindFlag(cmd *cobra.Command, f Flag) {
	switch f.Type {
	case FlagString:
		def := defaultString(f)
		if f.Short != "" {
			cmd.Flags().StringP(f.Name, f.Short, def, f.Description)
		} else {
			cmd.Flags().String(f.Name, def, f.Description)
		}
	case FlagInt:
		def := defaultInt(f)
		if f.Short != "" {
			cmd.Flags().IntP(f.Name, f.Short, def, f.Description)
		} else {
			cmd.Flags().Int(f.Name, def, f.Description)
		}
	case FlagFloat:
		def := defaultFloat(f)
		if f.Short != "" {
			cmd.Flags().Float64P(f.Name, f.Short, def, f.Description)
		} else {
			cmd.Flags().Float64(f.Name, def, f.Description)
		}
	case FlagBool:
		def := defaultBool(f)
		if f.Short != "" {
			cmd.Flags().BoolP(f.Name, f.Short, def, f.Description)
		} else {
			cmd.Flags().Bool(f.Name, def, f.Description)
		}
	case FlagStringSlice:
		def := defaultStringSlice(f)
		if f.Short != "" {
			cmd.Flags().StringSliceP(f.Name, f.Short, def, f.Description)
		} else {
			cmd.Flags().StringSlice(f.Name, def, f.Description)
		}
	default:
		panic(fmt.Errorf("shortcuts: flag %q has unknown FlagType %v", f.Name, f.Type))
	}
}

func defaultString(f Flag) string {
	if f.Default == nil {
		return ""
	}
	v, ok := f.Default.(string)
	if !ok {
		panic(fmt.Errorf("shortcuts: flag %q has Type=FlagString but Default is %T", f.Name, f.Default))
	}
	return v
}

func defaultInt(f Flag) int {
	if f.Default == nil {
		return 0
	}
	v, ok := f.Default.(int)
	if !ok {
		panic(fmt.Errorf("shortcuts: flag %q has Type=FlagInt but Default is %T", f.Name, f.Default))
	}
	return v
}

func defaultFloat(f Flag) float64 {
	if f.Default == nil {
		return 0
	}
	v, ok := f.Default.(float64)
	if !ok {
		panic(fmt.Errorf("shortcuts: flag %q has Type=FlagFloat but Default is %T", f.Name, f.Default))
	}
	return v
}

func defaultBool(f Flag) bool {
	if f.Default == nil {
		return false
	}
	v, ok := f.Default.(bool)
	if !ok {
		panic(fmt.Errorf("shortcuts: flag %q has Type=FlagBool but Default is %T", f.Name, f.Default))
	}
	return v
}

func defaultStringSlice(f Flag) []string {
	if f.Default == nil {
		return nil
	}
	v, ok := f.Default.([]string)
	if !ok {
		panic(fmt.Errorf("shortcuts: flag %q has Type=FlagStringSlice but Default is %T", f.Name, f.Default))
	}
	return v
}

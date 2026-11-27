package dynamic

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/rawapi"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"

	"github.com/spf13/cobra"
)

func makeRunE(c registry.Command, spec *registry.Spec, factory *cmdutil.Factory, binder *paramBinder) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		// Auth is gated by the module's PersistentPreRunE; RunE assumes login.
		if err := output.ValidateJQ(cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd)); err != nil {
			return err
		}
		params, err := parseParams(cmd, factory)
		if err != nil {
			return err
		}
		body, err := parseBody(cmd, c, factory)
		if err != nil {
			return err
		}
		binder.overlay(cmd, params)
		schemaHint := "run 'shoplazza schema " + strings.Join(strings.Fields(cmd.CommandPath())[1:], ".") + "'"
		if err := checkParamKeys(c, params, schemaHint); err != nil {
			return err
		}
		splitArrayParams(c, params)
		if err := checkBodyWrapper(c, spec, body, schemaHint); err != nil {
			return err
		}
		if err := checkBodyFields(c, spec, body, binder, schemaHint); err != nil {
			return err
		}
		if err := checkRequiredBody(c, body, schemaHint); err != nil {
			return err
		}
		hint := dropHashPrefix(c, params)

		// A human is prompted for any {param} left unset (e.g. theme_id);
		// non-interactive callers are untouched, so a missing param stays the
		// structured error below (agents/CI keep fail-fast).
		if err := fillMissingPathParams(cmd, factory, c.HTTP.Path, params); err != nil {
			return err
		}
		if err := checkRequiredParams(c, params, binder, schemaHint); err != nil {
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

func parseParams(cmd *cobra.Command, factory *cmdutil.Factory) (map[string]any, error) {
	raw, _ := cmd.Flags().GetString("params")
	raw = strings.TrimSpace(raw)
	if data, _ := cmd.Flags().GetString("data"); raw == "-" && data == "-" {
		return nil, output.ErrValidation("--params and --data cannot both read stdin (-)").WithParam("--params")
	}
	params, err := cmdutil.ParseJSONMap(raw, "--params", factory.IOStreams.In)
	if err != nil {
		return nil, output.ErrValidation("%v", err)
	}
	if params == nil {
		params = map[string]any{}
	}
	return params, nil
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

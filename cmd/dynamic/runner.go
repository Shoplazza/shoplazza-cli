package dynamic

import (
	"context"
	"encoding/json"
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

func makeRunE(c registry.Command, spec *registry.Spec, factory *cmdutil.Factory) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		// Auth is gated by the module's PersistentPreRunE; RunE assumes login.
		params, err := parseParams(cmd)
		if err != nil {
			return err
		}
		body, err := parseBody(cmd, c, factory)
		if err != nil {
			return err
		}

		resolvedPath, remainingQuery, err := rawapi.ResolveTemplatedPath(c.HTTP.Path, params)
		if err != nil {
			return output.ErrValidation("%v", err)
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
			return output.PrintBody(out, map[string]any{
				"dry_run": true,
				"request": factory.Client.BuildRequestSummary(req.Method, req.Path, req.Params, req.Data),
			}, format, jq)
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
		return output.PrintAPISuccess(out, resp.Body, format, jq)
	}
}

func parseParams(cmd *cobra.Command) (map[string]any, error) {
	raw, _ := cmd.Flags().GetString("params")
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, output.ErrValidation("--params must be valid JSON object: %v", err)
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

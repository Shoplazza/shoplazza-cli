package api

import (
	"context"
	"errors"
	"net"
	"strings"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/rawapi"

	"github.com/spf13/cobra"
)

type apiOptions struct {
	Factory *cmdutil.Factory
	Ctx     context.Context
	Method  string
	Path    string
	Params  string
	Data    string
}

const apiLong = `Raw access to the Shoplazza Open Platform API.

Access tiers (high → low abstraction):
  shortcuts  shoplazza products +search / shoplazza discounts +flashsale
  resource   shoplazza products list / shoplazza orders get --params '{"id":"o1"}'
  api rest   shoplazza api rest <METHOD> <PATH>   (escape hatch — full coverage)

Requires an active session — run 'shoplazza auth login' first.

Run 'shoplazza api rest --help' for raw-request flag usage and examples.`

// NewCmdAPI creates the raw api command group.
func NewCmdAPI(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Raw Shoplazza API commands",
		Long:  apiLong,
	}

	cmd.AddCommand(newCmdRest(f))
	return cmd
}

const apiRestLong = `Send a raw HTTP request to the Shoplazza Open Platform API.

The path is sent literally — write the resolved URL yourself, e.g.
"/openapi/2026-01/products/gid_123" rather than ".../products/{product_id}".
This is by design so the URL preserves which endpoint is being invoked;
use a higher-level command (shoplazza products get --params '{"product_id":"..."}')
when you want automatic path-template substitution.

Examples:
  shoplazza api rest GET /openapi/2026-01/products
  shoplazza api rest GET /openapi/2026-01/products --params '{"page_size":10}'
  shoplazza api rest GET /openapi/2026-01/products/gid_123
  shoplazza api rest POST /openapi/2026-01/products --data @product.json
  shoplazza api rest POST /openapi/2026-01/discounts/automatic --data @discount.json

For high-level commands, see 'shoplazza --help'.`

func newCmdRest(f *cmdutil.Factory) *cobra.Command {
	opts := &apiOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "rest <method> <path>",
		Short: "Send a raw HTTP request",
		Long:  apiRestLong,
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			return cmdutil.RequireAuth(cmd.Context(), f, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			format := cmdutil.GetFormat(cmd)
			jq := cmdutil.GetJQ(cmd)
			opts.Method = strings.ToUpper(args[0])
			opts.Path = rawapi.NormalizePath(args[1])
			opts.Ctx = cmd.Context()

			request, err := buildRawRequest(opts)
			if err != nil {
				return err
			}
			if cmdutil.IsDryRun(cmd) {
				return output.PrintBody(cmd.OutOrStdout(), map[string]any{
					"dry_run": true,
					"request": f.Client.BuildRequestSummary(request.Method, request.Path, request.Params, request.Data),
				}, format, jq)
			}

			response, err := f.Client.DoRaw(opts.Ctx, request)
			if err != nil {
				var httpErr *client.HTTPError
				if errors.As(err, &httpErr) {
					return output.ErrAPI(httpErr.StatusCode, httpErr.Body, response.RequestID())
				}
				var netErr net.Error
				if errors.As(err, &netErr) {
					return output.ErrNetwork("%v", err)
				}
				return output.ErrInternal("%v", err)
			}
			if response.Body == nil && (response.StatusCode != 0 || response.ContentType != "" || len(response.Headers) > 0) {
				return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{
					"status_code":  response.StatusCode,
					"content_type": response.ContentType,
					"headers":      response.Headers,
				}, format, jq)
			}
			return output.PrintAPISuccess(cmd.OutOrStdout(), response.Body, format, jq)
		},
	}

	cmd.Flags().StringVar(&opts.Params, "params", "", "Query parameters JSON (supports - for stdin or @file).")
	cmd.Flags().StringVar(&opts.Data, "data", "", "Request body JSON (supports - for stdin or @file).")
	cmd.Flags().Bool("dry-run", false, "Print the request that would be sent without executing it")
	cmd.Flags().StringP("jq", "q", "", "jq expression to filter JSON output (e.g. '.data.products[].id')")
	return cmd
}

func buildRawRequest(opts *apiOptions) (client.RawRequest, error) {
	return rawapi.BuildRequest(opts.Method, opts.Path, opts.Params, opts.Data, opts.Factory.IOStreams.In)
}

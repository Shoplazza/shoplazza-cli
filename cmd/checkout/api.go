package checkout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/fsx"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// addDryRunFlag adds the --dry-run flag every API-backed checkout command
// shares. Checkout commands act on the CURRENT store (no --store-domain
// override), and carry no --jq (pipe to the `jq` tool; the raw `api` / `dynamic`
// commands keep built-in --jq).
func addDryRunFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("dry-run", false, "Print the request that would be sent without executing it")
}

// authPreRun is the auth gate for API-backed checkout commands (build/dev/create/
// extension are zero-auth). Commands act on the current store — there is no
// --store-domain override.
func authPreRun(f *cmdutil.Factory) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		return cmdutil.RequireAuth(cmd.Context(), f, cmd)
	}
}

// resolveStore returns the current store domain (normalized), or a validation
// error if none is selected. Checkout commands act on the current store; there
// is no per-command override. Credentials are already injected by the auth
// gate (RequireAuth) — this only resolves the domain for display/derivation.
func resolveStore(f *cmdutil.Factory) (string, *output.ExitError) {
	if s := cmdutil.NormalizeStoreDomain(f.Config.CurrentStoreDomain()); s != "" {
		return s, nil
	}
	return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
		"no current store selected",
		"run 'shoplazza auth store use --store-domain <domain>' to select one")
}

// doAPI fires a request and classifies transport errors.
func doAPI(ctx context.Context, f *cmdutil.Factory, req client.RawRequest) (client.RawResponse, *output.ExitError) {
	resp, err := f.Client.DoRaw(ctx, req)
	if err != nil {
		var httpErr *client.HTTPError
		if errors.As(err, &httpErr) {
			return resp, output.ErrAPI(httpErr.StatusCode, httpErr.Body, resp.RequestID()).
				WithEndpoint(httpErr.Method, httpErr.Path)
		}
		var netErr net.Error
		if errors.As(err, &netErr) {
			return resp, output.ErrNetwork("%v", err)
		}
		return resp, output.ErrInternal("%v", err)
	}
	return resp, nil
}

// fireAndPrint runs a single API request (dry-run aware) and prints the result.
// Used by the "fire and print" commands: list/versions/deploy/undeploy.
func fireAndPrint(cmd *cobra.Command, f *cmdutil.Factory, req client.RawRequest) error {
	format := cmdutil.GetFormat(cmd)
	jq := ""
	if cmdutil.IsDryRun(cmd) {
		return output.PrintBody(cmd.OutOrStdout(), map[string]any{
			"dry_run": true,
			"request": f.Client.BuildRequestSummary(req.Method, req.Path, req.Params, req.Data),
		}, format, jq)
	}
	resp, exitErr := doAPI(cmd.Context(), f, req)
	if exitErr != nil {
		return exitErr
	}
	// Checkout endpoints reject with HTTP 200 + {message, status != 0}; surface
	// that instead of printing {"ok":true,...} for a rejected request.
	if msg := checkoutFailureMessage(resp.Body); msg != "" {
		return output.Errorf(output.ExitAPI, output.TypeAPI, "server rejected the request: %s", msg)
	}
	return output.PrintAPISuccess(cmd.OutOrStdout(), resp.Body, format, jq)
}

// resolveCheckoutVersionID maps a human version string (e.g. "1.0") to the
// server-side version id, via GET /checkout_extensions/version/list. Shared by
// deploy and preview so callers take --version instead of an opaque --version-id.
// The endpoint nests the version array under a key literally named "extensions"
// (the backend's field name), each entry carrying {version, id}.
func resolveCheckoutVersionID(ctx context.Context, f *cmdutil.Factory, extID, version string) (string, *output.ExitError) {
	resp, exitErr := doAPI(ctx, f, client.RawRequest{
		Method: "GET",
		Path:   "/openapi/checkout_extensions/version/list",
		Params: map[string]any{"extension_id": extID},
	})
	if exitErr != nil {
		return "", exitErr
	}
	if msg := checkoutFailureMessage(resp.Body); msg != "" {
		return "", output.Errorf(output.ExitAPI, output.TypeAPI, "server rejected the request: %s", msg)
	}
	arr, ok := mapField(payload(resp.Body), "extensions").([]any)
	if !ok {
		return "", output.ErrInternal("version list for extension %q had no versions array", extID)
	}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if asString(m["version"]) == version {
			id := asString(m["id"])
			if id == "" {
				return "", output.ErrInternal("version %s found but the server returned no version id", version)
			}
			return id, nil
		}
	}
	return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
		"version "+version+" not found for extension "+extID,
		"run 'shoplazza checkout versions --extension-id "+extID+"' to list available versions")
}

// --- map/value navigation over a checkout API response body ---

// payload returns the inner result object of a checkout API response. Checkout
// endpoints signal success via status/message (not the {ok:true}/{code:"Success"}
// envelopes the client auto-unwraps), so the "data" wrapper survives on resp.Body;
// payload digs to .data when present and otherwise returns body unchanged.
func payload(v any) any {
	if m, ok := v.(map[string]any); ok {
		if d, ok := m["data"].(map[string]any); ok {
			return d
		}
	}
	return v
}

func mapField(v any, key string) any {
	if m, ok := v.(map[string]any); ok {
		return m[key]
	}
	return nil
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

// checkoutFailureMessage returns the server's failure message when the checkout
// envelope signals a business failure, or "" on success / a statusless body.
// Checkout endpoints reply 200 OK with {data, errors, message, status} and use
// status:0 / message:"success" for success; a non-zero status is a business
// failure (e.g. {"message":"INVALID_VERSION","status":3}).
func checkoutFailureMessage(body any) string {
	m, ok := body.(map[string]any)
	if !ok {
		return ""
	}
	status, present := m["status"]
	if !present || statusIsZero(status) {
		return ""
	}
	if msg := asString(m["message"]); msg != "" {
		return msg
	}
	return "request rejected by the server"
}

// statusIsZero reports whether the checkout envelope's status field means
// success (numeric 0 across the json.Number / float64 / int forms; empty/"0"
// string; or absent/nil).
func statusIsZero(v any) bool {
	switch t := v.(type) {
	case json.Number:
		n, err := t.Int64()
		return err == nil && n == 0
	case float64:
		return t == 0
	case int:
		return t == 0
	case int64:
		return t == 0
	case string:
		return t == "" || t == "0"
	case nil:
		return true
	default:
		return false
	}
}

func firstNonEmpty(vals ...any) any {
	for _, v := range vals {
		if asString(v) != "" {
			return v
		}
	}
	return ""
}

func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return fsx.WriteFileAtomic(path, append(b, '\n'), 0o644)
}

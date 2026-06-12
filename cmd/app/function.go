package appcmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/app"
	"shoplazza-cli-v2/internal/app/project"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/extbuild/javy"
	"shoplazza-cli-v2/internal/output"
)

// newCmdFunction is the `app function` subgroup: the v1 `function` module lands
// here (NOT as a top-level command) under the app-extension model. Operates on
// function extensions in the current app project's extensions/<name>/.
func newCmdFunction(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "function",
		Short: "Create, compile, release and list app function extensions",
		Long: `Manage function (cart_transform) extensions of the current app.

These commands operate on a function extension under the app project's
extensions/<name>/ directory and use the current app's token.

  Whole-app publish:   shoplazza app deploy
  Single function:     shoplazza app function release --extension <name>`,
	}
	cmd.AddCommand(
		newCmdFunctionCompile(f),
		newCmdFunctionRelease(f),
		newCmdFunctionList(f),
	)
	return cmd
}

// requireExtensionName gates the --name flag (compile and release): required, and
// a bare directory name — it is joined under extensions/, so a path like "../x"
// would escape the project tree.
func requireExtensionName(name string) error {
	if name == "" {
		return output.ErrValidation("--name is required")
	}
	if filepath.Base(name) != name {
		return output.ErrValidation("invalid --name %q: must be a bare directory name under extensions/, without path separators", name)
	}
	return nil
}

// nextPatchVersion bumps the last dotted numeric component of v ("1.0.0" →
// "1.0.1"). The 2025-06 /functions/commit requires a version strictly greater
// than the function's current one; the recorded version (written back from the
// previous create/commit) is the current one, so the next release publishes
// current+1. An empty or non-numeric version falls back to "1.0.0".
func nextPatchVersion(v string) string {
	parts := strings.Split(v, ".")
	if v == "" || len(parts) == 0 {
		return "1.0.0"
	}
	last := parts[len(parts)-1]
	n, err := strconv.Atoi(last)
	if err != nil {
		return "1.0.0"
	}
	parts[len(parts)-1] = strconv.Itoa(n + 1)
	return strings.Join(parts, ".")
}

func newCmdFunctionCompile(f *cmdutil.Factory) *cobra.Command {
	var name, path string
	cmd := &cobra.Command{
		Use:   "compile",
		Short: "Compile a single function extension's src/index.js to WASM (javy)",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireExtensionName(name); err != nil {
				return err
			}
			return nil // local-only: no auth gate
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			p, err := openProject(path)
			if err != nil {
				return err
			}
			entry := filepath.Join(p.Root, project.ExtensionsDir, name, "src", "index.js")
			if _, statErr := os.Stat(entry); statErr != nil {
				return output.ErrValidation("function entry not found: %s", entry)
			}
			// Per-step progress on stderr (javy toolchain fetch + WASM compile
			// both block) so the result JSON on stdout stays pipe-clean.
			prog := output.NewProgress(cmd.ErrOrStderr())
			ensureStep := prog.Begin("[compile] preparing javy toolchain")
			javyPath, jErr := javy.Ensure(ctx)
			if jErr != nil {
				ensureStep.Fail()
				return jErr
			}
			ensureStep.Done()
			outDir := filepath.Join(p.Root, "app-deploy")
			buildStep := prog.Begin("[compile] compiling " + name + " to WASM")
			outPath, bErr := javy.Build(ctx, javyPath, entry, outDir, name)
			if bErr != nil {
				buildStep.Fail()
				return bErr
			}
			buildStep.Done()
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"extension": name,
				"wasm":      outPath,
			}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Function extension name under extensions/ (required)")
	cmd.Flags().StringVar(&path, "path", ".", "Project root")
	return cmd
}
func newCmdFunctionRelease(f *cmdutil.Factory) *cobra.Command {
	var name, clientID, path string
	var debug bool
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Compile + create/commit a single function extension (does not touch theme/checkout)",
		Long:  "Publishes ONE function extension. For a whole-app publish use `shoplazza app deploy`.",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireExtensionName(name); err != nil {
				return err
			}
			return requireLogin(cmd.Context(), f)
		},
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			ctx := cmd.Context()
			// Live elapsed timer per phase on a TTY (output.Progress) — release does
			// several blocking network calls plus a WASM compile. The deferred Fail
			// marks whichever phase is in flight if we return early, so each error path
			// doesn't need its own Fail(). Progress → stderr; result JSON → stdout.
			prog := output.NewProgress(cmd.ErrOrStderr())
			var step *output.Step
			defer func() {
				if err != nil && step != nil {
					step.Fail()
				}
			}()

			p, err := openProject(path)
			if err != nil {
				return err
			}
			cfg, ex := activeAppConfig(p)
			if ex != nil {
				return ex
			}
			// Build the Dashboard client up front: ensurePartnerID resolves the
			// partner live from /info for v1-created projects that don't persist it.
			d, err := dashboardClient(ctx, f)
			if err != nil {
				return err
			}
			// partner_id comes from the active config; --client-id may override the
			// app this release targets (partner still read from config).
			cid := cfg.ClientID
			if clientID != "" {
				cid = clientID
			}
			pid, ex := ensurePartnerID(ctx, d, cfg)
			if ex != nil {
				return ex
			}

			// Locate the target function locally. Its toml `id` decides create vs
			// commit: present → commit THAT function_id (reuses the id, publishes a
			// new version); absent → create. Functions live in the partner-openapi
			// /functions store, NOT the Dashboard extension_versions system — so
			// there is deliberately NO GetExtensionVersions / GenerateVersion /
			// store-id step here (that machinery is for `app deploy` and never
			// tracks these functions).
			locals, scanErr := app.ScanLocalExtensions(p.Root)
			if scanErr != nil {
				return scanErr
			}
			var target *app.LocalExt
			for i := range locals {
				if locals[i].Dir == name {
					target = &locals[i]
					break
				}
			}
			if target == nil || target.Type != "function" {
				return output.ErrValidation("no function extension %q under extensions/", name)
			}

			step = prog.Begin("[release] resolving app config")
			appCfg, err := d.GetAppConfig(ctx, pid, cid)
			if err != nil {
				return apiError(err)
			}
			partnerClient, err := partnerOpenapiClient(ctx, f, cid, appCfg.ClientSecret, appCfg.PartnerID, f.AuthClient.BaseURL)
			if err != nil {
				return err
			}
			step.Done()

			step = prog.Begin("[release] compiling " + name + " to WASM")
			javyPath, jErr := javy.Ensure(ctx)
			if jErr != nil {
				return jErr
			}
			entry := filepath.Join(p.Root, project.ExtensionsDir, name, "src", "index.js")
			wasm, bErr := javy.Build(ctx, javyPath, entry, filepath.Join(p.Root, "app-deploy"), name)
			if bErr != nil {
				return bErr
			}
			step.Done()

			// Commit when the function_id is already recorded locally; else create.
			// The 2025-06 /functions/commit requires the NEXT version (greater than the
			// function's current one — it 400s "Version not match" otherwise), so bump
			// the patch of the recorded version. The backend returns the authoritative
			// version, which we write back for the next bump.
			ext := app.Extension{ExtensionName: target.Name, ExtensionType: "function"}
			if target.ExtensionID != "" {
				ext.ExtensionID = target.ExtensionID
				ext.ExtensionVersion = nextPatchVersion(target.Version)
			}

			step = prog.Begin("[release] publishing " + name + " to app")
			res, uErr := app.UpsertFunction(ctx, ext, partnerClient, entry, wasm)
			if uErr != nil {
				return uErr
			}
			// Write back id/version to the extension toml (cache, NOT truth source).
			if wErr := app.WriteBackExtensionVersion(p.Root, name, res.ExtensionID, res.ExtensionVersion); wErr != nil {
				return output.ErrInternal("release succeeded but failed to update local toml: %v", wErr)
			}
			step.Done()
			step = nil // success past the last phase — don't let a print error fail it

			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{
				"extension":  target.Name,
				"id":         res.ExtensionID,
				"version":    res.ExtensionVersion,
				"version_id": res.ExtensionVersionID,
			}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Function extension name under extensions/ (required)")
	cmd.Flags().StringVar(&clientID, "client-id", "", "App client_id (defaults to active config; partner_id is always read from the active config, so overriding to an app under a different partner will 404)")
	cmd.Flags().StringVar(&path, "path", ".", "Project root")
	cmd.Flags().BoolVar(&debug, "debug", false, "(reserved; javy build is not debug-aware)")
	return cmd
}

// extractFunctions drills the GET /openapi/2024-07/functions envelope. The
// primary shape is {code,message,data:{functions:[...],total:N}}; it also
// tolerates {data:[...]}, {data:{data:[...]}} or a bare [...] defensively.
// source_code is stripped (it can be large/sensitive).
func extractFunctions(body any) []map[string]any {
	arr := digToArray(body)
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		delete(m, "source_code")
		out = append(out, m)
	}
	return out
}

// digToArray returns the first []any found at body, body.functions (the shape
// left after unmarshalUnwrapped strips a code:"Success" envelope), body.data,
// body.data.functions (the real 2024-07 shape) or body.data.data (legacy
// double-data fallback).
func digToArray(v any) []any {
	if a, ok := v.([]any); ok {
		return a
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if a, ok := m["functions"].([]any); ok {
		return a
	}
	switch d := m["data"].(type) {
	case []any:
		return d
	case map[string]any:
		if a, ok := d["functions"].([]any); ok {
			return a
		}
		if a, ok := d["data"].([]any); ok {
			return a
		}
	}
	return nil
}

func newCmdFunctionList(f *cmdutil.Factory) *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List the current app's function extensions",
		Args:    cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error { return requireLogin(cmd.Context(), f) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			p, err := openProject(path)
			if err != nil {
				return err
			}
			cfg, ex := activeAppConfig(p)
			if ex != nil {
				return ex
			}
			d, err := dashboardClient(ctx, f)
			if err != nil {
				return err
			}
			cid := cfg.ClientID
			pid, ex := ensurePartnerID(ctx, d, cfg)
			if ex != nil {
				return ex
			}
			appCfg, err := d.GetAppConfig(ctx, pid, cid)
			if err != nil {
				return apiError(err)
			}
			partnerClient, err := partnerOpenapiClient(ctx, f, cid, appCfg.ClientSecret, appCfg.PartnerID, f.AuthClient.BaseURL)
			if err != nil {
				return err
			}
			var body any
			// Single page capped at 1000 functions — no pagination loop; apps with
			// more functions than that would be truncated here.
			if gErr := partnerClient.GetJSONWithQuery(ctx, "/openapi/2024-07/functions",
				map[string]any{"page": 1, "limit": 1000}, &body); gErr != nil {
				return apiError(gErr)
			}
			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{"functions": extractFunctions(body)}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&path, "path", ".", "Project root")
	return cmd
}

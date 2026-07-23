package appcmd

import (
	"context"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/app/project"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// runInfo prints app/partner/user from the Dashboard /info endpoint. By default
// the app is the active config's client_id and the local project is scanned for
// extensions. A non-empty clientID overrides the lookup to an arbitrary app (any
// the partner token can see) — then p may be nil and extensions are omitted,
// since the local extensions/ dirs belong to the project's app, not the queried
// one.
func runInfo(ctx context.Context, d *app.Dashboard, p *project.Project, clientID string, w, errW io.Writer, format, jq string) (err error) {
	var localScopes string
	if clientID == "" {
		cfg, cfgErr := p.ActiveConfig()
		if cfgErr != nil {
			return output.ErrValidation("cannot read active config: %v", cfgErr)
		}
		if cfg.ClientID == "" {
			return output.ErrValidation("active config has no client_id; run 'shoplazza app config use'")
		}
		clientID = cfg.ClientID
		localScopes = cfg.Scopes
	}

	// Live elapsed timer on a TTY (output.Progress) — the /info round-trip blocks.
	// The deferred Fail marks the in-flight phase on early return; progress → errW
	// (stderr), result → w.
	prog := output.NewProgress(errW)
	var step *output.Step
	defer func() {
		if err != nil && step != nil {
			step.Fail()
		}
	}()

	step = prog.Begin("[info] fetching app info")
	info, err := d.GetCompleteInfo(ctx, clientID)
	if err != nil {
		return apiError(err)
	}
	step.Done()
	step = nil
	// The Dashboard /info endpoint doesn't return scopes, so mirror v1 and show the
	// scopes recorded in the local config (the space-joined string written at
	// link/init time from the app-config endpoint). Only available when inspecting
	// the local app — a --client-id lookup has no local config to read.
	if localScopes != "" {
		info.App.Scopes = strings.Fields(localScopes)
	}
	out := map[string]any{
		"app":     info.App,
		"partner": info.Partner,
		"user":    info.User,
	}
	// Extensions are local to the current project's app, so only list them when
	// inspecting that app (no --client-id override). Shared scanner (internal/app)
	// — a malformed extension toml is a validation error, not a silently skipped dir.
	if p != nil {
		locals, scanErr := app.ScanLocalExtensions(p.Root)
		if scanErr != nil {
			return scanErr
		}
		exts := make([]map[string]string, 0, len(locals))
		for _, l := range locals {
			exts = append(exts, map[string]string{"dir": l.Dir, "name": l.Name, "type": l.Type})
		}
		out["extensions"] = exts
	}
	return output.PrintAPISuccess(w, out, format, jq)
}

func newCmdInfo(f *cmdutil.Factory) *cobra.Command {
	var path, clientID string
	cmd := &cobra.Command{
		Use:     "info",
		Short:   "Print app and extension info",
		Args:    cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error { return requireLogin(cmd.Context(), f) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			// With --client-id we look up that app directly and need no local
			// project; otherwise the active config supplies the client_id and the
			// project root is scanned for extensions.
			var p *project.Project
			if clientID == "" {
				var err error
				if p, err = openProject(path); err != nil {
					return err
				}
			}
			d, err := dashboardClient(cmd.Context(), f)
			if err != nil {
				return err
			}
			return runInfo(cmd.Context(), d, p, clientID, cmd.OutOrStdout(), cmd.ErrOrStderr(), cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&path, "path", ".", "Project root (ignored with --client-id)")
	cmd.Flags().StringVar(&clientID, "client-id", "", "Look up a specific app by client_id (skips the local project; extensions are not listed)")
	return cmd
}

package appcmd

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/internal/app"
	"github.com/Shoplazza/shoplazza-cli/internal/app/project"
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

func newCmdConfig(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage app config files and the active config"}
	cmd.AddCommand(newCmdConfigUse(f))
	cmd.AddCommand(newCmdConfigLink(f))
	return cmd
}

func newCmdConfigUse(f *cmdutil.Factory) *cobra.Command {
	var configName, path string
	cmd := &cobra.Command{
		Use:     "use",
		Short:   "Switch the active app config (validated online)",
		Args:    cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error { return requireLogin(cmd.Context(), f) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			// --config is a name segment → shoplazza.app.<name>.toml; empty = base config.
			fileName := "shoplazza.app.toml"
			if configName != "" {
				fileName = configFileForName(configName)
			}
			p, err := openProject(path)
			if err != nil {
				return err
			}
			d, err := dashboardClient(cmd.Context(), f)
			if err != nil {
				return err
			}
			return runConfigUse(cmd.Context(), d, p, fileName, cmd.OutOrStdout(), cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&configName, "config", "", "The name of the app configuration to switch to (default: the base shoplazza.app.toml)")
	cmd.Flags().StringVar(&path, "path", ".", "Project root")
	return cmd
}

func runConfigUse(ctx context.Context, d *app.Dashboard, p *project.Project, configName string, w io.Writer, format, jq string) error {
	cfg, err := p.ReadConfig(configName)
	if err != nil {
		return output.ErrValidation("cannot read %s: %v", configName, err)
	}
	if cfg.ClientID == "" {
		return output.ErrValidation("%s has no client_id", configName)
	}
	if _, err := d.GetCompleteInfo(ctx, cfg.ClientID); err != nil {
		return apiError(err).WithHint("check the client_id in " + configName + " and ensure you have access")
	}
	if err := p.SetActiveConfig(configName, cfg.ClientID); err != nil {
		return output.ErrInternal("failed to write app-state: %v", err)
	}
	return output.PrintBody(w, map[string]any{"active_config": configName, "client_id": cfg.ClientID}, format, jq)
}

// linkOpts holds the resolved flag values for `app config link`.
type linkOpts struct {
	ClientID   string
	Create     bool
	Name       string
	Partner    string
	ConfigName string
}

func runConfigLink(ctx context.Context, d *app.Dashboard, p *project.Project, o linkOpts, w io.Writer, format, jq string) error {
	ref, err := resolveAppRef(ctx, d, o.ClientID, o.Create, o.Name, o.Partner)
	if err != nil {
		return err
	}

	// --config is a name segment; default to the app's name (else client_id).
	nameSeg := o.ConfigName
	if nameSeg == "" {
		nameSeg = ref.Name
		if nameSeg == "" {
			nameSeg = ref.ClientID
		}
	}
	configName := configFileForName(nameSeg)
	// Merge semantics (same as init): relinking must not blank out hand-filled
	// scopes when the dashboard has none configured.
	set := map[string]any{"client_id": ref.ClientID, "partner_id": ref.PartnerID}
	if len(ref.Scopes) > 0 {
		set["scopes"] = strings.Join(ref.Scopes, " ")
	} else if existing, _ := p.ReadConfig(configName); existing.Scopes == "" {
		// No scopes from the dashboard or the target config: write the template default.
		set["scopes"] = project.DefaultScopes
	}
	if err := p.UpdateConfig(configName, set); err != nil {
		// An invalid --config name surfaces as the validation error it is;
		// anything else (encode/write) is an internal fault.
		var ee *output.ExitError
		if errors.As(err, &ee) {
			return ee
		}
		return output.ErrInternal("failed to write %s: %v", configName, err)
	}
	// Linking implies "use this app now" — activate it (client_id already validated).
	if err := p.SetActiveConfig(configName, ref.ClientID); err != nil {
		return output.ErrInternal("failed to write app-state: %v", err)
	}
	return output.PrintBody(w, map[string]any{"config": configName, "active_config": configName, "client_id": ref.ClientID, "partner_id": ref.PartnerID}, format, jq)
}

// configFileForName maps a config name segment to its toml filename
// ("prod" -> shoplazza.app.prod.toml), slugified.
func configFileForName(name string) string {
	return "shoplazza.app." + sanitizeConfigName(name) + ".toml"
}

// sanitizeConfigName lowercases and replaces non [a-z0-9-_] runs with '-'.
func sanitizeConfigName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "app"
	}
	return out
}

func newCmdConfigLink(f *cmdutil.Factory) *cobra.Command {
	var o linkOpts
	var path string
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Write an app config into the current project — link an existing app or create a new one (no scaffold)",
		Long: `Write an app configuration file into the current project. Unlike 'app init' it
does NOT scaffold a template — it only adds a shoplazza.app.<name>.toml.

Two mutually-exclusive modes:

  Link an EXISTING app:  shoplazza app config link --client-id <client_id>
  Create a NEW app:      shoplazza app config link --create --name "My App" [--partner <id>]

Link mode pulls the app's client_id / partner / scopes from the Dashboard. Create
mode first creates a new app in the backend, then writes its config. Afterwards run
'shoplazza app config use' to make this config the active one.`,
		Args:    cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error { return requireLogin(cmd.Context(), f) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := openProject(path)
			if err != nil {
				return err
			}
			d, err := dashboardClient(cmd.Context(), f)
			if err != nil {
				return err
			}
			return runConfigLink(cmd.Context(), d, p, o, cmd.OutOrStdout(), cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&o.ClientID, "client-id", "", "Link mode: pull an EXISTING app's config by client_id. Mutually exclusive with --create")
	cmd.Flags().BoolVar(&o.Create, "create", false, "Create mode: create a NEW app in the backend, then write its config (pair with --name). Mutually exclusive with --client-id")
	cmd.Flags().StringVar(&o.Name, "name", "", "Create mode: name for the NEW app (required with --create)")
	cmd.Flags().StringVar(&o.Partner, "partner", "", "Create mode: partner (org) to create the app under; auto-selected when you belong to only one")
	cmd.Flags().StringVar(&o.ConfigName, "config", "", "The name of the app configuration (default: the app's name) — written to shoplazza.app.<name>.toml, merged if it exists")
	cmd.Flags().StringVar(&path, "path", ".", "Project root")
	// The two modes can't be combined, and exactly one entry point is required.
	cmd.MarkFlagsMutuallyExclusive("client-id", "create")
	cmd.MarkFlagsOneRequired("client-id", "create")
	return cmd
}

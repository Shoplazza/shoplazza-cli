package appcmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/app"
	"shoplazza-cli-v2/internal/app/project"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
)

type initOpts struct {
	ClientID string
	Create   bool
	Name     string
	Partner  string
}

// cloneTemplate shallow-clones gitURL into dest then removes .git.
func cloneTemplate(ctx context.Context, gitURL, dest string) error {
	c := exec.CommandContext(ctx, "git", "clone", "--depth", "1", gitURL, dest)
	if out, err := c.CombinedOutput(); err != nil {
		// Keep both the exec error (exit status / not-found) and git's own
		// output — either alone can be empty or uninformative.
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return output.ErrInternal("git clone failed: %v: %s", err, msg)
		}
		return output.ErrInternal("git clone failed: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(dest, ".git")); err != nil {
		return output.ErrInternal("failed to remove .git from cloned template: %v", err)
	}
	return nil
}

// ensureGitignore appends entry to <root>/.gitignore if not already present.
func ensureGitignore(root, entry string) error {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}
	body := string(data)
	if len(body) > 0 && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += entry + "\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

func runInit(ctx context.Context, d *app.Dashboard, p *project.Project, o initOpts, w, errW io.Writer, format, jq string) (err error) {
	// Link mode derives the owning partner FROM the app via /info (see
	// resolveAppRef) — a --partner flag is never consulted there. Warn instead
	// of silently ignoring it (mirrors `app config link`, where --partner is
	// likewise only used by create mode).
	if !o.Create && o.Partner != "" {
		fmt.Fprintln(errW, "warning: --partner is ignored when linking an existing app (the partner is derived from the app info)")
	}

	// Live elapsed timer per phase on a TTY (output.Progress) — resolving/creating
	// the app, fetching the template, and the git clone all block. The deferred Fail
	// marks the in-flight phase on early return; progress → errW (stderr), result → w.
	prog := output.NewProgress(errW)
	var step *output.Step
	defer func() {
		if err != nil && step != nil {
			step.Fail()
		}
	}()

	// Resolve the app first — its name decides the project sub-directory name.
	resolveLabel := "[init] linking app"
	if o.Create {
		resolveLabel = "[init] creating app"
	}
	step = prog.Begin(resolveLabel)
	ref, err := resolveAppRef(ctx, d, o.ClientID, o.Create, o.Name, o.Partner)
	if err != nil {
		return err
	}
	step.Done()
	step = nil
	clientID, scopes := ref.ClientID, ref.Scopes
	partnerID := ref.PartnerID

	base := ref.Name
	if base == "" {
		base = o.Name
	}
	if base == "" {
		base = clientID
	}
	dirName := sanitizeConfigName(base) // slugify (lowercase, hyphenate) — mirrors v1 slugify
	targetDir := filepath.Join(p.Root, dirName)
	if _, statErr := os.Stat(targetDir); statErr == nil {
		return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"a directory named '"+dirName+"' already exists at "+targetDir,
			"choose a different --name, or remove the existing directory")
	} else if !os.IsNotExist(statErr) {
		return output.ErrInternal("failed to check target dir: %v", statErr)
	}

	step = prog.Begin("[init] fetching app template")
	tmpl, err := d.GetTemplate(ctx, "app")
	if err != nil {
		return apiError(err)
	}
	if tmpl.HTTPS == "" {
		return output.ErrInternal("template URL is empty")
	}
	step.Done()
	step = nil

	step = prog.Begin("[init] cloning template into " + dirName)
	if cErr := cloneTemplate(ctx, tmpl.HTTPS, targetDir); cErr != nil {
		return cErr
	}
	step.Done()
	step = nil

	sub, err := project.Open(targetDir)
	if err != nil {
		return output.ErrInternal("open new project: %v", err)
	}
	// Merge into the template's toml instead of overwriting it: the template
	// ships default scopes (v1 string format) the first authorization depends
	// on. Real configured scopes from the API still take precedence.
	set := map[string]any{"client_id": clientID, "partner_id": partnerID}
	if len(scopes) > 0 {
		set["scopes"] = strings.Join(scopes, " ")
	}
	if err := sub.UpdateConfig("shoplazza.app.toml", set); err != nil {
		return output.ErrInternal("failed to write shoplazza.app.toml: %v", err)
	}
	if err := sub.SetActiveConfig("shoplazza.app.toml", clientID); err != nil {
		return output.ErrInternal("failed to set active config: %v", err)
	}
	if err := ensureGitignore(targetDir, ".shoplazza/"); err != nil {
		return output.ErrInternal("failed to update .gitignore: %v", err)
	}
	if err := output.PrintBody(w, map[string]any{"project": targetDir, "client_id": clientID, "partner_id": partnerID, "active_config": "shoplazza.app.toml"}, format, jq); err != nil {
		return err
	}
	// Human next-steps go to STDERR so stdout stays clean machine JSON (mirrors v1's
	// successTips). dirName is the freshly-created sub-directory.
	fmt.Fprintf(errW, "\nNext steps:\n  • Run `cd %s`\n  • For extensions, run `shoplazza app extension create`\n  • To see your app, run `shoplazza app deploy`\n", dirName)
	return nil
}

func newCmdInit(f *cmdutil.Factory) *cobra.Command {
	var clientID, name, partner string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create or link an app project (creates a sub-dir named after the app)",
		Long: `Scaffold an app project into a new sub-directory of the current directory.

Two mutually-exclusive modes:

  Create a NEW app:    shoplazza app init --name "My App" [--partner <id>]
  Link an EXISTING:    shoplazza app init --client-id <client_id>

--name and --partner belong to create mode (--partner is only needed when your
account has more than one partner). --client-id is link mode, used on its own —
the owning partner is derived from the app.`,
		Args:    cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error { return requireLogin(cmd.Context(), f) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			// create-vs-link is enforced by the flag groups below (mutually exclusive,
			// one required), so reaching here means exactly one mode is selected. The
			// project is created as a sub-dir under the current working directory.
			p, err := openProject(".")
			if err != nil {
				return err
			}
			d, err := dashboardClient(cmd.Context(), f)
			if err != nil {
				return err
			}
			// --name's VALUE is the new app's name; its presence flips to create-mode.
			o := initOpts{ClientID: clientID, Create: name != "", Name: name, Partner: partner}
			return runInit(cmd.Context(), d, p, o, cmd.OutOrStdout(), cmd.ErrOrStderr(), cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Create mode: name for the NEW app (pair with --partner). Mutually exclusive with --client-id")
	cmd.Flags().StringVar(&partner, "partner", "", "Create mode: partner (org) id to create the app under; needed only when your account has multiple partners (ignored in link mode)")
	cmd.Flags().StringVar(&clientID, "client-id", "", "Link mode: link an EXISTING app by client_id (the project dir is named after it). Mutually exclusive with --name")
	// Idiomatic cobra flag-group validation: the two modes cannot be combined, and
	// exactly one entry point (--name or --client-id) must be present.
	cmd.MarkFlagsMutuallyExclusive("client-id", "name")
	cmd.MarkFlagsOneRequired("client-id", "name")
	return cmd
}

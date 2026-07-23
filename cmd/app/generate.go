package appcmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/internal/app"
	"github.com/Shoplazza/shoplazza-cli/internal/app/project"
	"github.com/Shoplazza/shoplazza-cli/internal/app/scaffold"
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// templateTypeFor maps the user-facing extension type to the Dashboard
// template_type identifier downloaded via GetTemplate.
func templateTypeFor(extType string) (string, bool) {
	switch extType {
	case "theme":
		return "ext_thm", true
	case "checkout":
		return "ext_co", true
	case "function":
		return "ext_func", true
	default:
		return "", false
	}
}

// extensionNamePattern is the conservative shape an extension name must match
// (same alphabet as sanitizeConfigName's slugs). The name becomes both a
// directory under extensions/ and a remote identifier, so rather than silently
// sanitizing we REJECT anything else — in particular path-traversal values
// like "../../x".
var extensionNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// runGenerateExtension scaffolds a new extension into <projectRoot>/extensions/<name>.
// It validates inputs, ensures the target dir is absent, fetches + clones the
// template, then applies the deterministic rewrite (theme: basic/embed; others:
// name/type only). Pure logic lives in internal/app/scaffold so this stays thin
// and testable with a local template repo.
func runGenerateExtension(ctx context.Context, d *app.Dashboard, projectRoot, extType, name, themeType string, w, errW io.Writer, format, jq string) (err error) {
	if name == "" {
		return output.ErrValidation("--name is required")
	}
	if !extensionNamePattern.MatchString(name) {
		return output.ErrValidation("invalid --name %q: use lowercase letters, digits, '-' or '_', starting with a letter or digit", name)
	}
	templateType, ok := templateTypeFor(extType)
	if !ok {
		return output.ErrValidation("invalid --type %q: must be theme, checkout, or function", extType)
	}
	if extType == "theme" && themeType != "basic" && themeType != "embed" {
		return output.ErrValidation("--theme-type is required for theme extensions and must be basic or embed")
	}

	extDir := filepath.Join(projectRoot, project.ExtensionsDir, name)
	if _, statErr := os.Stat(extDir); statErr == nil {
		return output.ErrValidation("extension %q already exists at %s", name, extDir)
	} else if !os.IsNotExist(statErr) {
		return output.ErrInternal("failed to check target dir: %v", statErr)
	}

	// Live elapsed timer per phase on a TTY (output.Progress) — the template fetch
	// and git clone block. The deferred Fail marks the in-flight phase on early
	// return; progress → errW (stderr), result → w.
	prog := output.NewProgress(errW)
	var step *output.Step
	defer func() {
		if err != nil && step != nil {
			step.Fail()
		}
	}()

	step = prog.Begin("[create] fetching template")
	tmpl, err := d.GetTemplate(ctx, templateType)
	if err != nil {
		return apiError(err)
	}
	if tmpl.HTTPS == "" {
		return output.ErrInternal("template URL is empty")
	}
	step.Done()
	step = nil

	step = prog.Begin("[create] cloning template into extensions/" + name)
	tmpDir, err := os.MkdirTemp("", "shoplazza-ext-*")
	if err != nil {
		return output.ErrInternal("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cloneDir := filepath.Join(tmpDir, "tmpl")
	if cErr := cloneTemplate(ctx, tmpl.HTTPS, cloneDir); cErr != nil {
		return cErr
	}
	if extType == "theme" {
		if sErr := scaffold.ScaffoldTheme(cloneDir, extDir, name, themeType); sErr != nil {
			return output.ErrInternal("failed to scaffold theme extension: %v", sErr)
		}
	} else {
		if sErr := scaffold.ScaffoldSimple(cloneDir, extDir, name, extType); sErr != nil {
			return output.ErrInternal("failed to scaffold extension: %v", sErr)
		}
	}
	step.Done()
	step = nil

	return output.PrintBody(w, map[string]any{
		"extension": name,
		"type":      extType,
		"path":      extDir,
	}, format, jq)
}

func newCmdExtensionCreate(f *cmdutil.Factory) *cobra.Command {
	var extType, name, themeType, path string
	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Scaffold a new extension (theme / checkout / function)",
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
			return runGenerateExtension(cmd.Context(), d, p.Root, extType, name, themeType, cmd.OutOrStdout(), cmd.ErrOrStderr(), cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&extType, "type", "", "Extension type: theme, checkout, or function (required)")
	cmd.Flags().StringVar(&name, "name", "", "Extension name / target directory (required)")
	cmd.Flags().StringVar(&themeType, "theme-type", "", "Theme subtype: basic or embed (required when --type theme)")
	cmd.Flags().StringVar(&path, "path", ".", "Project root")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newCmdExtension(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extension",
		Short: "Create and manage app extensions",
	}
	cmd.AddCommand(newCmdExtensionCreate(f))
	return cmd
}

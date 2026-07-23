package theme_extension

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	te "github.com/Shoplazza/shoplazza-cli/v2/internal/theme_extension"
)

// projectNameRe constrains --name to a single safe path segment: it starts
// alphanumeric and contains only [a-zA-Z0-9_-], max 64 chars. This rejects
// path separators, "..", absolute paths and shell-hostile characters BEFORE
// any filesystem op — `--name ../escapee` used to scaffold outside the cwd
// (and the failure-path RemoveAll then pointed outside it too).
var projectNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func newCmdCreate(f *cmdutil.Factory) *cobra.Command {
	var name, teType string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Scaffold a standalone theme-extension project (basic|embed)",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return output.ErrValidation("--name is required")
			}
			if !projectNameRe.MatchString(name) {
				return output.ErrValidation(
					"invalid --name %q: must start with a letter or digit and contain only letters, digits, '-' or '_' (max 64 chars, no path separators)", name)
			}
			if teType != "basic" && teType != "embed" {
				return output.ErrValidation("--type is required and must be basic or embed")
			}
			return nil // local-only, non-interactive
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			// name passed projectNameRe, so dest is always a direct child of the
			// cwd — the failure-path RemoveAll below can only target the freshly
			// created scaffold dir.
			dest := filepath.Join(".", name)
			if _, err := os.Stat(dest); err == nil {
				return output.ErrValidation("directory %q already exists", dest)
			} else if !os.IsNotExist(err) {
				// EACCES etc.: "does not exist" is unproven — scaffolding anyway
				// could clobber something we couldn't see.
				return output.ErrInternal("stat %q: %v", dest, err)
			}
			if err := te.Scaffold(dest, name, teType); err != nil {
				_ = os.RemoveAll(dest) // don't leave a half-materialized dir blocking a retry
				return output.ErrInternal("scaffold te project: %v", err)
			}
			if err := te.WriteConfig(dest, te.Config{Name: name, Type: "theme", Subtype: teType}); err != nil {
				_ = os.RemoveAll(dest)
				return output.ErrInternal("write te config: %v", err)
			}
			if err := output.PrintBody(cmd.OutOrStdout(), map[string]any{"project": name, "type": teType, "path": dest}, cmdutil.GetFormat(cmd), ""); err != nil {
				return err
			}
			// Human next-steps go to STDERR so stdout stays clean machine JSON
			// (mirrors `app init`). dest is the freshly-created project dir.
			fmt.Fprintf(cmd.ErrOrStderr(),
				"\nNext steps:\n  • Run `cd %s`\n  • Link it to an app: `shoplazza te connect`\n  • Start developing: `shoplazza te serve`\n",
				dest)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Project name / target directory (required)")
	cmd.Flags().StringVar(&teType, "type", "", "Template type: basic or embed (required)")
	return cmd
}

package checkout

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/internal/checkout/scaffold"
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

func newCmdInit(f *cmdutil.Factory) *cobra.Command {
	var name, ext string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new checkout extension project (local, no network)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" || ext == "" {
				return output.ErrValidation("--name <project> and --extension <first-extension> are required")
			}
			if vErr := validPlainName("--name", name); vErr != nil {
				return vErr
			}
			if vErr := validPlainName("--extension", ext); vErr != nil {
				return vErr
			}
			cwd, err := os.Getwd()
			if err != nil {
				return output.ErrInternal("cannot determine working directory: %s", err.Error())
			}
			dest := filepath.Join(cwd, name)
			if _, statErr := os.Stat(dest); statErr == nil {
				return output.ErrValidation("target directory '%s' already exists", name)
			}
			if scErr := scaffold.Project(dest, name, ext); scErr != nil {
				return output.ErrInternal("scaffold failed: %s", scErr.Error())
			}
			if err := output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"ok": true, "project": name, "extension": ext, "path": dest,
			}, cmdutil.GetFormat(cmd), ""); err != nil {
				return err
			}
			// Human next-steps go to STDERR so stdout stays clean machine JSON.
			fmt.Fprintf(cmd.ErrOrStderr(),
				"\nNext steps:\n  • Run `cd %s`\n  • Start the dev server: `shoplazza checkout dev`\n  • Publish a version: `shoplazza checkout push --name %s`\n",
				name, ext)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Project name (directory created in cwd)")
	cmd.Flags().StringVar(&ext, "extension", "", "First extension name")
	return cmd
}

// validPlainName rejects a flag value that isn't a single path segment, so a
// value like "../../x" cannot escape the project via filepath.Join+Clean.
func validPlainName(flag, val string) *output.ExitError {
	if val == "" || val == "." || val == ".." || val != filepath.Base(val) {
		return output.ErrValidation("%s must be a plain name without path separators or '..' (got %q)", flag, val)
	}
	return nil
}

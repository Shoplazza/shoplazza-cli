package checkout

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/checkout/scaffold"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
)

func newCmdExtensionCreate(f *cmdutil.Factory) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new extension in an existing project (local, no network)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return output.ErrValidation("--name <extension> is required")
			}
			if vErr := validPlainName("--name", name); vErr != nil {
				return vErr
			}
			cwd, err := os.Getwd()
			if err != nil {
				return output.ErrInternal("cannot determine working directory: %s", err.Error())
			}
			if info, statErr := os.Stat(filepath.Join(cwd, "extensions")); statErr != nil || !info.IsDir() {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"current directory is not a checkout extension project (no ./extensions)",
					"run this inside a project created by 'shoplazza checkout init'")
			}
			if _, statErr := os.Stat(filepath.Join(cwd, "extensions", name)); statErr == nil {
				return output.ErrValidation("extension '%s' already exists", name)
			}
			if scErr := scaffold.Extension(cwd, name); scErr != nil {
				return output.ErrInternal("scaffold failed: %s", scErr.Error())
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"ok": true, "extension": name, "path": filepath.Join(cwd, "extensions", name),
			}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Extension name")
	return cmd
}

package themecmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
)

// newCmdPackage builds `themes package`: zip the current theme directory into a
// "<name>-<version>.zip" artifact (name/version read from
// config/settings_schema.json), honoring .themeignore unless --no-ignore.
// Purely local — no store client, so AuthFree.
func newCmdPackage(f *cmdutil.Factory) *cobra.Command {
	var noIgnore bool
	cmd := &cobra.Command{
		Use:         "package",
		Short:       "Package the current theme directory into a zip",
		Long:        "Package the current theme directory into a '<name>-<version>.zip' artifact locally (name/version read from config/settings_schema.json); honors .themeignore unless --no-ignore is set.",
		Example:     "  shoplazza themes package",
		Args:        cobra.NoArgs,
		Annotations: authFreeWrite, // writes the zip artifact to the local filesystem
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return theme.ErrLocalIO("getwd", err)
			}
			name, version, err := theme.ReadInfo(cwd)
			if err != nil {
				return err
			}
			zipName := theme.ZipName(name, version)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "[package] packaging into %s\n", zipName)

			opts := pack.PackOptions{}
			if noIgnore {
				opts.IgnoreFile = "/dev/null" // pack sentinel: force-disable .themeignore
			}
			zipPath, err := pack.Pack(cwd, zipName, opts)
			if err != nil {
				return theme.ErrLocalIO("pack zip", err)
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"zip_path": zipPath,
				"name":     name,
				"version":  version,
			}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().BoolVar(&noIgnore, "no-ignore", false, "Ignore .themeignore")
	return cmd
}

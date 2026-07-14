package checkout

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/jsbuild"
	"shoplazza-cli-v2/internal/output"
)

// ResolveBuildTarget resolves the build target from cwd and an optional --name.
// Resolution order:
//  1. cwd is itself an extension dir (contains extension.json) → use its base name
//  2. --name given → require <cwd-or-extensions>/<name> exists
//  3. neither → type=validation (hint: pass --name)
//
// projectRoot is the directory the Node build must treat as cwd: extPath is
// always <projectRoot>/extensions/<id>, and the build entry is derived as
// <cwd>/extensions/<id>/src/index.js — so when the user runs inside the
// extension (or extensions/) dir, projectRoot differs from their cwd.
func ResolveBuildTarget(cwd, idFlag string) (id, extPath, projectRoot string, err *output.ExitError) {
	// Branch 1: cwd is an extension dir.
	if _, statErr := os.Stat(filepath.Join(cwd, "extension.json")); statErr == nil {
		return filepath.Base(cwd), cwd, filepath.Dir(filepath.Dir(cwd)), nil
	}
	// Branch 2: --id provided.
	if idFlag != "" {
		if vErr := validPlainName("--name", idFlag); vErr != nil {
			return "", "", "", vErr
		}
		base := cwd
		if filepath.Base(cwd) != "extensions" {
			base = filepath.Join(cwd, "extensions")
		}
		p := filepath.Join(base, idFlag)
		if info, statErr := os.Stat(p); statErr != nil || !info.IsDir() {
			return "", "", "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				"the extension '"+idFlag+"' does not exist in your local project",
				"run from your project root and check ./extensions/"+idFlag)
		}
		return idFlag, p, filepath.Dir(base), nil
	}
	// Branch 3: no id, not in an extension dir.
	return "", "", "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
		"no extension specified and the current directory is not an extension directory",
		"pass --name <extension name> or run inside extensions/<name>/")
}

func newCmdBuild(f *cmdutil.Factory) *cobra.Command {
	var idFlag string
	var debug bool
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a checkout extension with the bundled Vite toolchain",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return output.ErrInternal("cannot determine working directory: %s", err.Error())
			}
			id, _, projectRoot, exitErr := ResolveBuildTarget(cwd, idFlag)
			if exitErr != nil {
				return exitErr
			}
			res, runErr := jsbuild.RunBuild(cmd.Context(), jsbuild.BuildRequest{
				Action: "build", Name: id, Debug: debug,
			}, projectRoot)
			if runErr != nil {
				return runErr
			}
			format := cmdutil.GetFormat(cmd)
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"ok":           true,
				"extension_id": id,
				"artifacts":    res.Artifacts,
				"duration_ms":  res.DurationMs,
			}, format, "")
		},
	}
	cmd.Flags().StringVar(&idFlag, "name", "", "Extension name (the directory under ./extensions); optional inside an extension dir")
	cmd.Flags().BoolVar(&debug, "debug", false, "Verbose build logs to stderr")
	return cmd
}

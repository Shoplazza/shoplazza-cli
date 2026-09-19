package themecmd

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
)

// newCmdInit builds `themes init`: clone the Nova-2023 template into a new
// directory and print a cd hint. Purely local (GitHub fetch + extraction) — no
// store client, so AuthFree.
func newCmdInit(f *cmdutil.Factory) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:         "init --name <dir>",
		Short:       "Initialize a new theme by cloning the Nova-2023 template",
		Long:        "Scaffold a new theme directory by cloning the Shoplazza/Nova-2023 template, then print a cd hint. Local — no login required.",
		Example:     "  shoplazza themes init --name my-theme",
		Args:        cobra.NoArgs,
		Annotations: authFreeWrite, // clones into the local filesystem
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Interactive fill for --name; non-interactively an unset flag is the
			// structured missing-flag error.
			if err := cmdutil.ResolveFlags(cmd, f, cmdutil.PromptField{Flag: "name", Title: "New theme directory name"}); err != nil {
				return err
			}
			if err := validateInitName(name); err != nil {
				return err
			}
			errW := cmd.ErrOrStderr()
			_, _ = fmt.Fprintf(errW, "[init] cloning template Shoplazza/Nova-2023...\n")
			if err := pack.CloneTemplate(cmd.Context(), name); err != nil {
				return classifyCloneErr(err)
			}
			_, _ = fmt.Fprintf(errW, "[init] theme initialized at ./%s\n", name)
			printCdHint(errW, name)
			return output.PrintBody(cmd.OutOrStdout(),
				map[string]any{"theme_dir": "./" + name, "status": "initialized"}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Target directory name for the new theme (required)")
	return cmd
}

// validateInitName rejects --name values that are not a plain directory name:
// path separators, "..", "." and absolute paths would let the clone extract
// outside the cwd.
func validateInitName(name string) error {
	if filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) || name == ".." || name == "." {
		return theme.ErrValidation(
			"invalid --name %q: must be a plain directory name (no path separators, '..' or absolute paths)", name)
	}
	return nil
}

// classifyCloneErr maps a pack.CloneTemplate failure to the right envelope.
func classifyCloneErr(err error) error {
	if errors.Is(err, pack.ErrTargetDirNotEmpty) {
		return theme.ErrValidation("%v; choose another --name or empty the directory first", err)
	}
	if errors.Is(err, pack.ErrTemplateDownload) {
		return theme.ErrCloneNetwork(err)
	}
	var netErr net.Error
	var urlErr *url.Error
	if errors.As(err, &netErr) || errors.As(err, &urlErr) {
		return theme.ErrCloneNetwork(err)
	}
	return theme.ErrLocalIO("clone template", err)
}

// printCdHint writes the multi-line "next steps" cd hint to w.
func printCdHint(w io.Writer, name string) {
	_, _ = fmt.Fprintf(w, "\nnext steps:\n   cd %s\n   shoplazza themes serve\n", name)
}

package checkout

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/jsbuild"
	"shoplazza-cli-v2/internal/output"
)

// ParseDevIDs flattens repeated/comma-separated --extension-name values (local
// extension dir names under ./extensions — NOT server-side ids).
// --all (allFlag) → empty slice (Node reads every extensions/ dir).
// Neither → type=validation. Exported for testing.
func ParseDevIDs(raw []string, allFlag bool) ([]string, *output.ExitError) {
	if allFlag {
		return []string{}, nil
	}
	var ids []string
	for _, r := range raw {
		for _, part := range strings.Split(r, ",") {
			if p := strings.TrimSpace(part); p != "" {
				ids = append(ids, p)
			}
		}
	}
	if len(ids) == 0 {
		return nil, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"no extension selected for dev",
			"pass --extension-name <name[,name]> (repeatable) or --all")
	}
	return ids, nil
}

func newCmdDev(f *cmdutil.Factory) *cobra.Command {
	var ids []string
	var all bool
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Run the checkout extension dev server (rebuild + HMR on :8888)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			selected, exitErr := ParseDevIDs(ids, all)
			if exitErr != nil {
				return exitErr
			}
			pkgRoot, err := jsbuild.PkgRoot()
			if err != nil {
				return output.ErrInternal("cannot resolve package root: %s", err.Error())
			}
			nodePath, nErr := jsbuild.EnsureNode(cmd.Context()) // existence + version gate
			if nErr != nil {
				return nErr
			}
			cwd, cwdErr := os.Getwd()
			if cwdErr != nil {
				return output.ErrInternal("cannot determine working directory: %s", cwdErr.Error())
			}
			if dErr := jsbuild.EnsureProjectDeps(cmd.Context(), cwd); dErr != nil { // first-run auto `npm install` (same as build)
				return dErr
			}
			args := append([]string{jsbuild.DevEntryPath(pkgRoot)}, selected...) // --all → no ids appended
			c := exec.CommandContext(cmd.Context(), nodePath, args...)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr // inherit stdio
			// The root signal context cancels cmd.Context() on Ctrl-C; the default
			// Cancel would SIGKILL the dev server before the forwarded SIGINT below
			// lets it shut down cleanly. The forwarding goroutine owns termination.
			c.Cancel = func() error { return nil }

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			if err := c.Start(); err != nil {
				return output.ErrInternal("failed to start dev server: %s", err.Error())
			}
			go func() {
				for sig := range sigCh {
					if c.Process != nil {
						_ = c.Process.Signal(sig) // forward Ctrl+C / SIGTERM
					}
				}
			}()
			if err := c.Wait(); err != nil {
				// Ctrl-C (forwarded signal / canceled root ctx) is a clean stop;
				// a real non-zero exit must surface as a CLI failure.
				if exitErr := ClassifyDevExit(err, cmd.Context().Err()); exitErr != nil {
					return exitErr
				}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&ids, "extension-name", nil, "Extension name(s) to develop (repeatable, comma-separated)")
	cmd.Flags().BoolVar(&all, "all", false, "Develop all extensions under ./extensions")
	return cmd
}

// ClassifyDevExit maps the dev server's Wait error onto the CLI error
// contract. Clean stops return nil: the root signal ctx was canceled (Ctrl-C —
// with the no-op c.Cancel above, Wait can absorb it as ctx.Err() even after a
// clean child exit) or the child was killed by the forwarded SIGINT/SIGTERM.
// A real non-zero exit is internal-class: the failure happened inside the
// spawned Node toolchain, not in user input, auth, or the network.
// Exported for testing.
func ClassifyDevExit(waitErr, ctxErr error) *output.ExitError {
	if waitErr == nil || ctxErr != nil || errors.Is(waitErr, context.Canceled) {
		return nil
	}
	var ee *exec.ExitError
	if errors.As(waitErr, &ee) {
		// Guard the Sys() assertion for portability (non-POSIX wait status).
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return nil // signal-driven exit — the forwarding goroutine's doing
		}
		return output.Errorf(output.ExitInternal, output.TypeInternal,
			"dev server exited with code %d", ee.ExitCode())
	}
	return output.ErrInternal("dev server: %v", waitErr)
}

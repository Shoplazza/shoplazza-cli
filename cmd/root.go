package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"shoplazza-cli-v2/cmd/api"
	appcmd "shoplazza-cli-v2/cmd/app"
	"shoplazza-cli-v2/cmd/auth"
	"shoplazza-cli-v2/cmd/checkout"
	"shoplazza-cli-v2/cmd/completion"
	"shoplazza-cli-v2/cmd/doctor"
	"shoplazza-cli-v2/cmd/dynamic"
	"shoplazza-cli-v2/cmd/schema"
	"shoplazza-cli-v2/cmd/theme_extension"
	"shoplazza-cli-v2/cmd/update"
	"shoplazza-cli-v2/internal/build"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/registry"
	"shoplazza-cli-v2/shortcuts"

	"github.com/spf13/cobra"
)

// Execute runs the root command and returns the process exit code.
func Execute() int {
	factory := cmdutil.NewDefaultFactory()

	spec := registry.LoadSpec()

	rootCmd := &cobra.Command{
		Use:   "shoplazza",
		Short: "Shoplazza Open Platform command-line interface",
		Long: fmt.Sprintf(`Shoplazza CLI — official command-line interface to the Shoplazza Open Platform (OpenAPI %s).

Common workflows:
  shoplazza auth login                    authenticate to your account
  shoplazza <module> --help                explore a resource's commands
  shoplazza <module> <command> [--params <json>] [--data <json>]
                                           invoke an API endpoint
  shoplazza schema <module>.<command>      inspect parameters / body / response
  shoplazza api rest <METHOD> <PATH>       raw HTTP call (escape hatch)

Run any command with --dry-run to print the request without sending it.`, spec.Version),
		Version:       build.DisplayVersion(),
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	rootCmd.SetVersionTemplate(fmt.Sprintf("shoplazza version %s (%s)\n", build.DisplayVersion(), build.Date))
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	RegisterGlobalFlags(rootCmd.PersistentFlags())
	rootCmd.AddCommand(auth.NewCmdAuth(factory))
	rootCmd.AddCommand(appcmd.NewCmdApp(factory))
	rootCmd.AddCommand(checkout.NewCmdCheckout(factory))
	rootCmd.AddCommand(theme_extension.NewCmdThemeExtension(factory))
	rootCmd.AddCommand(api.NewCmdAPI(factory))
	rootCmd.AddCommand(schema.NewCmdSchema(spec))
	rootCmd.AddCommand(doctor.NewCmdDoctor())
	rootCmd.AddCommand(completion.NewCmdCompletion(factory))
	rootCmd.AddCommand(update.NewCmdUpdate(factory))
	dynamic.RegisterCommands(rootCmd, spec, factory)
	shortcuts.RegisterShortcuts(rootCmd, factory)

	// Ctrl-C / SIGTERM cancel the command context so in-flight work can unwind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Restore default signal disposition after the first signal so a second
	// Ctrl-C force-kills even if the command ignores ctx.
	go func() { <-ctx.Done(); stop() }()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			output.WriteErrorEnvelope(os.Stderr, exitErr)
			return exitErr.Code
		}

		if failing, _, ferr := rootCmd.Find(os.Args[1:]); ferr == nil && failing != nil {
			_ = failing.Usage()
		}
		fmt.Fprintln(os.Stderr, "Error:", err.Error())

		return output.ExitValidation
	}

	return output.ExitOK
}

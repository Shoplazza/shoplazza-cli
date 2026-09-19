package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Shoplazza/shoplazza-cli/v2/cmd/api"
	appcmd "github.com/Shoplazza/shoplazza-cli/v2/cmd/app"
	"github.com/Shoplazza/shoplazza-cli/v2/cmd/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/cmd/checkoutext"
	"github.com/Shoplazza/shoplazza-cli/v2/cmd/completion"
	"github.com/Shoplazza/shoplazza-cli/v2/cmd/doctor"
	"github.com/Shoplazza/shoplazza-cli/v2/cmd/dynamic"
	"github.com/Shoplazza/shoplazza-cli/v2/cmd/profile"
	"github.com/Shoplazza/shoplazza-cli/v2/cmd/schema"
	"github.com/Shoplazza/shoplazza-cli/v2/cmd/skill"
	themecmd "github.com/Shoplazza/shoplazza-cli/v2/cmd/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/cmd/themeext"
	"github.com/Shoplazza/shoplazza-cli/v2/cmd/update"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/build"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/metasync"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/updatecheck"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts"

	"github.com/spf13/cobra"
)

// NewRootCmd assembles and returns the full root command tree without
// executing it. Used by Execute and by tests that enumerate commands.
func NewRootCmd() *cobra.Command {
	factory := cmdutil.NewDefaultFactory()

	spec := registry.LoadSpec()

	rootCmd := &cobra.Command{
		Use:   "shoplazza",
		Short: "Shoplazza Open Platform command-line interface",
		Long: fmt.Sprintf(`Shoplazza CLI — official command-line interface to the Shoplazza Open Platform (OpenAPI %s).

New here? Run 'shoplazza auth login' to authenticate first.

Tips: 'shoplazza schema <module>.<command>' inspects an endpoint's params/body/response;
add --dry-run to preview any request without sending it.`, spec.Version),
		Version:       build.DisplayVersion(),
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	rootCmd.SetVersionTemplate(fmt.Sprintf("shoplazza version %s (%s)\n", build.DisplayVersion(), build.DisplayDate()))
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	RegisterGlobalFlags(rootCmd.PersistentFlags(), defaultOutputFormat())
	// --profile completes from configured profile names (best-effort: a
	// registration failure here would only affect shell completion, never
	// command execution).
	_ = rootCmd.RegisterFlagCompletionFunc("profile", cmdutil.ProfileNameCompletionFunc(factory))
	rootCmd.AddCommand(auth.NewCmdAuth(factory))
	rootCmd.AddCommand(appcmd.NewCmdApp(factory))
	rootCmd.AddCommand(checkoutext.NewCmdCheckout(factory))
	rootCmd.AddCommand(themeext.NewCmdThemeExtension(factory))
	rootCmd.AddCommand(api.NewCmdAPI(factory))
	rootCmd.AddCommand(profile.NewCmdProfile(factory))
	rootCmd.AddCommand(schema.NewCmdSchema(spec))
	rootCmd.AddCommand(skill.NewCmdSkill())
	rootCmd.AddCommand(doctor.NewCmdDoctor(factory))
	rootCmd.AddCommand(completion.NewCmdCompletion(factory))
	rootCmd.AddCommand(update.NewCmdUpdate(factory))
	dynamic.RegisterCommands(rootCmd, spec, factory)
	shortcuts.RegisterShortcuts(rootCmd, factory)
	// Plain-cobra theme workflow commands (push/…) mount under `themes` after it
	// and its help groups exist. They own their store client, which is what lets
	// -e select the store/profile locally (see cmd/theme).
	themecmd.RegisterCommands(rootCmd, factory)

	applyRootGroups(rootCmd)

	return rootCmd
}

// defaultOutputFormat resolves the --format flag's default from
// SHOPLAZZA_CLI_FORMAT (a human/CI convenience so pretty needn't be typed each
// time), falling back to json. An invalid value is ignored, and an explicit
// --format on any command still overrides it.
func defaultOutputFormat() string {
	if v := os.Getenv("SHOPLAZZA_CLI_FORMAT"); output.ValidFormat(v) {
		return v
	}
	return output.FormatJSON
}

// Execute runs the root command and returns the process exit code.
func Execute() (exitCode int) {
	// Last-resort catchall (error_types.md): a panic must still honor the
	// JSON error envelope contract instead of leaking a raw stack trace.
	defer func() {
		if r := recover(); r != nil {
			exitErr := output.Errorf(output.ExitInternal, output.TypeInternal,
				"unexpected internal error: %v", r)
			output.WriteErrorEnvelope(os.Stderr, exitErr)
			exitCode = output.ExitInternal
		}
	}()

	rootCmd := NewRootCmd()

	// Ctrl-C / SIGTERM cancel the command context so in-flight work can unwind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Restore default signal disposition after the first signal so a second
	// Ctrl-C force-kills even if the command ignores ctx.
	go func() { <-ctx.Done(); stop() }()

	// Auto update check (interactive use only). Read the cache synchronously (zero latency),
	// refresh in a background goroutine for the next run.
	// Skip update/completion commands to avoid nagging mid-update and avoid corrupting completion output.
	var pendingUpdate *updatecheck.Info
	if !isUpdateCheckSkippedCommand(rootCmd, os.Args[1:]) {
		pendingUpdate = updatecheck.CheckCached(build.Version)
		// Fire-and-forget, and genuinely so: a command that exits in tens of
		// milliseconds outruns these entirely, and neither persists partial
		// progress, so nothing carries over to the next run either. Commands
		// that do real I/O leave enough time to finish them, and `shoplazza
		// update` refreshes both in the foreground regardless. The trade is
		// deliberate — no command pays latency for a cache it isn't using.
		go updatecheck.RefreshCache(build.Version)
		go metasync.Refresh(ctx, build.Version)
		// Surface staleness to agents inside the json success envelope's
		// "_notice" block (opt out with SHOPLAZZA_CLI_NO_NOTICE=1).
		output.SetNotice(buildNotice(pendingUpdate))
	}

	// The template is a plain string built up front, so the skills line is
	// appended only here — otherwise every command pays a read it never prints.
	if wantsVersion(os.Args[1:]) {
		rootCmd.SetVersionTemplate(fmt.Sprintf("shoplazza version %s (%s)\n%s\n",
			build.DisplayVersion(), build.DisplayDate(), skillLine()))
	}

	execErr := rootCmd.ExecuteContext(ctx)

	var exitErr *output.ExitError
	isExitErr := errors.As(execErr, &exitErr)

	// After the command output, print a one-line notice to stderr for interactive use
	// (printed on both success and failure paths — never touches stdout).
	// Skipped on cancel (ExitCanceled): Ctrl-C must leave both streams empty.
	canceled := isExitErr && exitErr.Code == output.ExitCanceled
	if pendingUpdate != nil && !canceled && output.IsTerminal(os.Stderr) {
		fmt.Fprintln(os.Stderr, "\n"+pendingUpdate.Message())
	}

	if execErr != nil {
		if isExitErr {
			output.WriteErrorEnvelope(os.Stderr, exitErr)
			return exitErr.Code
		}

		// A non-ExitError here is a cobra/pflag usage error (unknown command,
		// unknown flag, missing required flag, bad argument). Emit it through the
		// JSON error envelope with a stable subtype so agents parse stderr as JSON
		// even for the commonest usage mistakes — never leak plain "Error: ..." text.
		usageErr := output.ClassifyUsageError(execErr)
		output.WriteErrorEnvelope(os.Stderr, usageErr)
		return usageErr.Code
	}

	return output.ExitOK
}

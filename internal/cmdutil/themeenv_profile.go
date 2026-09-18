package cmdutil

import (
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/themeenv"
)

// EnvironmentFlag is the flag theme commands register to opt into
// shoplazza.theme.toml environment selection (-e/--environment). ResolveProfile
// consults the environment file ONLY when the executing command defines this
// flag, so a stray SHOPLAZZA_CLI_ENVIRONMENT never perturbs a non-theme command.
const EnvironmentFlag = "environment"

// EnvironmentVar is the non-interactive equivalent of --environment (CI).
const EnvironmentVar = "SHOPLAZZA_CLI_ENVIRONMENT"

// selectedEnvironment returns the environment name chosen for this command: the
// --environment flag, else SHOPLAZZA_CLI_ENVIRONMENT. It returns "" when the
// command does not define the flag (i.e. is not a theme command) or nothing
// selected one, so the environment machinery stays inert everywhere else.
func selectedEnvironment(cmd *cobra.Command) string {
	if cmd == nil || cmd.Flags().Lookup(EnvironmentFlag) == nil {
		return ""
	}
	if v, _ := cmd.Flags().GetString(EnvironmentFlag); v != "" {
		return v
	}
	return os.Getenv(EnvironmentVar)
}

// profileFromThemeEnv resolves the profile a theme environment selects, keyed on
// the environment's profile= (a profile name) or, failing that, store= (matched
// against the authenticated profiles). It returns (nil,false,nil) when no
// environment is in play or the environment names neither — the signal to fall
// through to today's resolution. A missing file, unknown environment, unknown
// profile, or a store with no matching profile is a structured error, never a
// silent wrong-store.
func profileFromThemeEnv(f *Factory, cmd *cobra.Command) (*core.ProfileConfig, bool, error) {
	name := selectedEnvironment(cmd)
	if name == "" {
		return nil, false, nil
	}

	start := "."
	if cmd.Flags().Lookup("path") != nil {
		if v, _ := cmd.Flags().GetString("path"); v != "" {
			start = v
		}
	}
	path, err := themeenv.Find(start)
	if err != nil {
		if errors.Is(err, themeenv.ErrNotFound) {
			return nil, false, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				"-e "+name+" was given but no "+themeenv.FileName+" was found",
				"create "+themeenv.FileName+" with an [environments."+name+"] block, or drop -e")
		}
		return nil, false, output.ErrInternal("find %s: %v", themeenv.FileName, err)
	}
	file, err := themeenv.Load(path)
	if err != nil {
		return nil, false, output.ErrValidation("%v", err)
	}
	env, ok := file.Environment(name)
	if !ok {
		hint := "no environments are defined in " + path
		if names := file.Names(); len(names) > 0 {
			hint = "defined environments: " + strings.Join(names, ", ")
		}
		return nil, false, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"environment "+name+" not found in "+themeenv.FileName, hint)
	}

	if env.Profile != "" {
		if p := f.Config.FindProfile(env.Profile); p != nil {
			return p, true, nil
		}
		return nil, false, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"environment "+name+" names profile "+env.Profile+", which is not configured",
			"run 'shoplazza profile list', or 'shoplazza auth login -s <store>' to create it")
	}
	if env.Store != "" {
		domain := NormalizeStoreDomain(env.Store)
		if p := f.Config.FindProfileByStore(domain); p != nil {
			return p, true, nil
		}
		return nil, false, output.ErrWithHint(output.ExitAuth, output.TypeAuth,
			"environment "+name+" targets store "+domain+", but no profile is authenticated for it",
			"run 'shoplazza auth login -s "+domain+"', or set profile= in the environment")
	}
	return nil, false, nil // environment sets neither profile nor store → today's resolution
}

// ApplyThemeEnvironment injects a selected theme environment's values into any
// matching flags the command left UNSET: theme→--theme-id, path→--path,
// ignore→--ignore (only the flags a given command actually defines). Explicit
// flags always win — a set flag is never overwritten.
//
// It is gated on --environment (a no-op for non-theme commands / no selection)
// and best-effort: the environment file and name were already validated when the
// profile resolved (RequireAuth runs first), so a re-read hiccup here must not
// fail an otherwise-valid command. Call it before the required-flag fill so an
// injected value counts as "provided".
func ApplyThemeEnvironment(cmd *cobra.Command) {
	name := selectedEnvironment(cmd)
	if name == "" {
		return
	}
	start := "."
	if cmd.Flags().Lookup("path") != nil {
		if v, _ := cmd.Flags().GetString("path"); v != "" {
			start = v
		}
	}
	path, err := themeenv.Find(start)
	if err != nil {
		return
	}
	file, err := themeenv.Load(path)
	if err != nil {
		return
	}
	env, ok := file.Environment(name)
	if !ok {
		return
	}
	setFlagIfUnset(cmd, "theme-id", env.Theme)
	setFlagIfUnset(cmd, "path", env.Path)
	setSliceIfUnset(cmd, "ignore", env.Ignore)
}

// setFlagIfUnset sets flag to val when the command defines it, the user did not
// set it, and val is non-empty.
func setFlagIfUnset(cmd *cobra.Command, flag, val string) {
	if val == "" {
		return
	}
	f := cmd.Flags().Lookup(flag)
	if f == nil || f.Changed {
		return
	}
	_ = cmd.Flags().Set(flag, val)
}

// setSliceIfUnset fills a StringSlice/StringArray flag from vals only when the
// user did not set it (an explicit --ignore replaces the environment's list, it
// does not merge). The user-set check is read once before the first Set, since
// Set itself flips the flag's Changed state.
func setSliceIfUnset(cmd *cobra.Command, flag string, vals []string) {
	if len(vals) == 0 {
		return
	}
	f := cmd.Flags().Lookup(flag)
	if f == nil || f.Changed {
		return
	}
	for _, v := range vals {
		_ = cmd.Flags().Set(flag, v)
	}
}

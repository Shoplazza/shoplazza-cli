package themes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/themeenv"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// envPathFlag is shared by the read-only env commands: where to start the
// upward search for shoplazza.theme.toml (defaults to the working directory).
var envPathFlag = common.Flag{
	Name:        "path",
	Type:        common.FlagString,
	Default:     ".",
	Description: "Directory to search upward for " + themeenv.FileName,
}

// envListShortcut lists the environments defined in the project's
// shoplazza.theme.toml. Local + read-only: no auth, no network, so agents can
// `themes env list --format json` to discover the environments before choosing
// one. A missing file is a structured error pointing at how to create one.
var envListShortcut = common.Shortcut{
	Service:  "themes env",
	Command:  "list",
	Use:      "list [--path <dir>]",
	Short:    "List the theme environments defined in " + themeenv.FileName,
	Long:     "List the environments defined in the project's " + themeenv.FileName + " (found by searching up from --path). Read-only and offline.",
	Example:  "  shoplazza themes env list",
	AuthFree: true,
	Local:    true,
	Flags:    []common.Flag{envPathFlag},
	Execute: func(_ context.Context, in common.ExecInput) (common.ExecResult, error) {
		f, path, err := loadThemeEnvFile(in.Flags.GetString("path"))
		if err != nil {
			return common.ExecResult{}, err
		}
		envs := make([]map[string]any, 0, len(f.Environments))
		for _, name := range f.Names() {
			e, _ := f.Environment(name)
			envs = append(envs, envToMap(name, e))
		}
		return common.ExecResult{Body: map[string]any{"file": path, "environments": envs}}, nil
	},
}

// envShowShortcut shows one environment's settings. An empty name resolves to
// the "default" environment; a missing name is a structured error listing the
// ones that exist.
var envShowShortcut = common.Shortcut{
	Service:  "themes env",
	Command:  "show",
	Use:      "show [name] [--path <dir>]",
	Short:    "Show one theme environment's settings",
	Long:     "Show a single environment from " + themeenv.FileName + "; omit the name for the 'default' environment. Read-only and offline.",
	Example:  "  shoplazza themes env show staging",
	Args:     cobra.MaximumNArgs(1),
	AuthFree: true,
	Local:    true,
	Flags:    []common.Flag{envPathFlag},
	Execute: func(_ context.Context, in common.ExecInput) (common.ExecResult, error) {
		name := ""
		if len(in.Args) > 0 {
			name = in.Args[0]
		}
		f, path, err := loadThemeEnvFile(in.Flags.GetString("path"))
		if err != nil {
			return common.ExecResult{}, err
		}
		e, ok := f.Environment(name)
		if !ok {
			shown := name
			if shown == "" {
				shown = themeenv.DefaultEnvironment
			}
			hint := "no environments are defined in " + path
			if names := f.Names(); len(names) > 0 {
				hint = "defined environments: " + strings.Join(names, ", ")
			}
			return common.ExecResult{}, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				"environment "+shown+" not found in "+themeenv.FileName, hint)
		}
		resolved := name
		if resolved == "" {
			resolved = themeenv.DefaultEnvironment
		}
		return common.ExecResult{Body: envToMap(resolved, e)}, nil
	},
}

// envCheckShortcut validates the environments offline: every referenced profile
// must be configured, and a store-only environment must have an authenticated
// profile — the two mistakes that would only surface as an auth failure when you
// actually run push -e. Online theme-existence probing is deliberately out of
// scope here (it needs per-environment auth). Exits non-zero with the failing
// environments named when any fail, so CI can gate on it.
var envCheckShortcut = common.Shortcut{
	Service:  "themes env",
	Command:  "check",
	Use:      "check [name] [--path <dir>]",
	Short:    "Validate theme environments offline (each resolves to a configured profile/store)",
	Long:     "Offline-validate the environments in " + themeenv.FileName + ": referenced profiles exist and store-only environments have an authenticated profile. Omit the name to check them all. Exits non-zero if any fail.",
	Example:  "  shoplazza themes env check",
	Args:     cobra.MaximumNArgs(1),
	AuthFree: true,
	Local:    true,
	Flags:    []common.Flag{envPathFlag},
	Execute: func(_ context.Context, in common.ExecInput) (common.ExecResult, error) {
		f, path, err := loadThemeEnvFile(in.Flags.GetString("path"))
		if err != nil {
			return common.ExecResult{}, err
		}
		names := f.Names()
		if len(in.Args) > 0 {
			if _, ok := f.Environment(in.Args[0]); !ok {
				return common.ExecResult{}, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"environment "+in.Args[0]+" not found in "+themeenv.FileName,
					"defined environments: "+strings.Join(f.Names(), ", "))
			}
			names = []string{in.Args[0]}
		}

		cfg := loadProfileConfig()
		results := make([]map[string]any, 0, len(names))
		var failures []string
		for _, name := range names {
			e, _ := f.Environment(name)
			issues := checkEnvironment(&cfg, e)
			r := map[string]any{"name": name, "ok": len(issues) == 0}
			if len(issues) > 0 {
				r["issues"] = issues
				for _, msg := range issues {
					failures = append(failures, name+": "+msg)
				}
			}
			results = append(results, r)
		}
		if len(failures) > 0 {
			return common.ExecResult{}, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				fmt.Sprintf("%d environment issue(s) in %s", len(failures), themeenv.FileName),
				strings.Join(failures, "; "))
		}
		return common.ExecResult{Body: map[string]any{"file": path, "ok": true, "environments": results}}, nil
	},
}

// checkEnvironment returns the offline problems with one environment: a named
// profile that is not configured, or a store-only environment with no
// authenticated profile, or a wholly empty block. An environment that only sets
// theme/path/ignore (no store/profile) is valid — it rides the current store.
func checkEnvironment(cfg *core.CliConfig, e themeenv.Environment) []string {
	var issues []string
	if e.Profile != "" && cfg.FindProfile(e.Profile) == nil {
		issues = append(issues, "profile "+e.Profile+" is not configured (run 'shoplazza profile list')")
	}
	if e.Profile == "" && e.Store != "" {
		domain := cmdutil.NormalizeStoreDomain(e.Store)
		if cfg.FindProfileByStore(domain) == nil {
			issues = append(issues, "no profile is authenticated for store "+domain+" (run 'shoplazza auth login -s "+domain+"')")
		}
	}
	if e.Store == "" && e.Profile == "" && e.Theme == "" && e.Path == "" && len(e.Ignore) == 0 {
		issues = append(issues, "environment is empty (sets nothing)")
	}
	return issues
}

// loadProfileConfig reads the CLI config (profile library) straight from disk —
// a Local shortcut's Execute gets no factory, but the profile list it validates
// against lives in the config file.
func loadProfileConfig() core.CliConfig {
	path, _ := core.DefaultConfigPath()
	cfg, _ := core.LoadConfig(path)
	return cfg
}

// loadThemeEnvFile finds shoplazza.theme.toml upward from startPath and parses
// it. A missing file is a structured, hinted error (not a crash); a parse error
// is a validation error naming the file.
func loadThemeEnvFile(startPath string) (themeenv.File, string, error) {
	if startPath == "" {
		startPath = "."
	}
	path, err := themeenv.Find(startPath)
	if err != nil {
		if errors.Is(err, themeenv.ErrNotFound) {
			return themeenv.File{}, "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				"no "+themeenv.FileName+" found in this directory or any parent",
				"create one at the project root with an [environments.<name>] block (store / theme / path / ignore / profile)")
		}
		return themeenv.File{}, "", theme.ErrLocalIO("find "+themeenv.FileName, err)
	}
	f, err := themeenv.Load(path)
	if err != nil {
		return themeenv.File{}, "", theme.ErrValidation("%v", err)
	}
	return f, path, nil
}

// envToMap renders one environment for output, omitting zero-value fields so the
// JSON shows only what the block actually set.
func envToMap(name string, e themeenv.Environment) map[string]any {
	m := map[string]any{"name": name}
	if e.Store != "" {
		m["store"] = e.Store
	}
	if e.Theme != "" {
		m["theme"] = e.Theme
	}
	if e.Path != "" {
		m["path"] = e.Path
	}
	if len(e.Ignore) > 0 {
		m["ignore"] = e.Ignore
	}
	if e.Profile != "" {
		m["profile"] = e.Profile
	}
	if e.Live {
		m["live"] = true
	}
	if e.Config != "" {
		m["config"] = e.Config
	}
	return m
}

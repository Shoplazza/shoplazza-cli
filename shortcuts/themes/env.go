package themes

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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

// env-field flags shared by add/set. --store/--theme/--profile/--live map to the
// environment's toml keys; path= and ignore= are advanced and hand-edited.
var (
	envStoreFlag   = common.Flag{Name: "store", Type: common.FlagString, Description: "Store domain (e.g. staging.myshoplaza.com)"}
	envThemeFlag   = common.Flag{Name: "theme", Type: common.FlagString, Description: "Theme id the environment targets"}
	envProfileFlag = common.Flag{Name: "profile", Type: common.FlagString, Description: "Keychain profile to authenticate with (else matched by store)"}
	envLiveFlag    = common.Flag{Name: "live", Type: common.FlagBool, Description: "Target the store's published (live) theme"}
)

// envAddShortcut adds a new environment to shoplazza.theme.toml (creating the
// file when none exists up-tree). Errors if the environment already exists.
var envAddShortcut = common.Shortcut{
	Service:      "themes env",
	Command:      "add",
	Use:          "add <name> --store <domain> [--theme <id>] [--profile <name>] [--live]",
	Short:        "Add a new environment to " + themeenv.FileName,
	Long:         "Add a new [environments.<name>] block to " + themeenv.FileName + " (created if absent). Errors if the environment exists — use 'themes env set' to change one. Advanced keys (path/ignore) are hand-edited; this rewrites the file without comments.",
	Example:      "  shoplazza themes env add staging --store staging.myshoplaza.com --theme 123456 --profile staging",
	Args:         cobra.ExactArgs(1),
	AuthFree:     true,
	Local:        true,
	NotScannable: true, // writes the local filesystem
	Flags:        []common.Flag{envPathFlag, envStoreFlag, envThemeFlag, envProfileFlag, envLiveFlag},
	Execute: func(_ context.Context, in common.ExecInput) (common.ExecResult, error) {
		name := in.Args[0]
		f, path, existed, err := loadOrNewThemeEnvFile(in.Flags.GetString("path"))
		if err != nil {
			return common.ExecResult{}, err
		}
		if _, ok := f.Environment(name); ok {
			return common.ExecResult{}, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				"environment "+name+" already exists in "+themeenv.FileName,
				"use 'shoplazza themes env set "+name+"' to change it")
		}
		env := themeenv.Environment{
			Store:   in.Flags.GetString("store"),
			Theme:   in.Flags.GetString("theme"),
			Profile: in.Flags.GetString("profile"),
			Live:    in.Flags.GetBool("live"),
		}
		if f.Environments == nil {
			f.Environments = map[string]themeenv.Environment{}
		}
		f.Environments[name] = env
		if serr := themeenv.Save(path, f); serr != nil {
			return common.ExecResult{}, theme.ErrLocalIO("write "+themeenv.FileName, serr)
		}
		body := envToMap(name, env)
		body["file"] = path
		body["created_file"] = !existed
		return common.ExecResult{Body: body}, nil
	},
}

// envSetShortcut updates fields of an existing environment (only flags the user
// passed are changed). Errors if the environment does not exist.
var envSetShortcut = common.Shortcut{
	Service:      "themes env",
	Command:      "set",
	Use:          "set <name> [--store <domain>] [--theme <id>] [--profile <name>] [--live]",
	Short:        "Change fields of an existing environment in " + themeenv.FileName,
	Long:         "Update an existing [environments.<name>] block; only the flags you pass are changed. Errors if the environment does not exist — use 'themes env add'. Rewrites the file without comments.",
	Example:      "  shoplazza themes env set staging --theme 654321",
	Args:         cobra.ExactArgs(1),
	AuthFree:     true,
	Local:        true,
	NotScannable: true,
	Flags:        []common.Flag{envPathFlag, envStoreFlag, envThemeFlag, envProfileFlag, envLiveFlag},
	Execute: func(_ context.Context, in common.ExecInput) (common.ExecResult, error) {
		name := in.Args[0]
		f, path, err := loadThemeEnvFile(in.Flags.GetString("path"))
		if err != nil {
			return common.ExecResult{}, err
		}
		env, ok := f.Environment(name)
		if !ok {
			return common.ExecResult{}, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				"environment "+name+" not found in "+themeenv.FileName,
				"use 'shoplazza themes env add "+name+"' to create it")
		}
		if in.Flags.Changed("store") {
			env.Store = in.Flags.GetString("store")
		}
		if in.Flags.Changed("theme") {
			env.Theme = in.Flags.GetString("theme")
		}
		if in.Flags.Changed("profile") {
			env.Profile = in.Flags.GetString("profile")
		}
		if in.Flags.Changed("live") {
			env.Live = in.Flags.GetBool("live")
		}
		f.Environments[name] = env
		if serr := themeenv.Save(path, f); serr != nil {
			return common.ExecResult{}, theme.ErrLocalIO("write "+themeenv.FileName, serr)
		}
		body := envToMap(name, env)
		body["file"] = path
		return common.ExecResult{Body: body}, nil
	},
}

// envRemoveShortcut deletes an environment. Errors if it does not exist.
var envRemoveShortcut = common.Shortcut{
	Service:      "themes env",
	Command:      "remove",
	Use:          "remove <name>",
	Short:        "Remove an environment from " + themeenv.FileName,
	Long:         "Delete the [environments.<name>] block from " + themeenv.FileName + ". Rewrites the file without comments.",
	Example:      "  shoplazza themes env remove staging",
	Args:         cobra.ExactArgs(1),
	AuthFree:     true,
	Local:        true,
	NotScannable: true,
	Flags:        []common.Flag{envPathFlag},
	Execute: func(_ context.Context, in common.ExecInput) (common.ExecResult, error) {
		name := in.Args[0]
		f, path, err := loadThemeEnvFile(in.Flags.GetString("path"))
		if err != nil {
			return common.ExecResult{}, err
		}
		if _, ok := f.Environment(name); !ok {
			return common.ExecResult{}, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				"environment "+name+" not found in "+themeenv.FileName,
				"run 'shoplazza themes env list' to see the defined environments")
		}
		delete(f.Environments, name)
		if serr := themeenv.Save(path, f); serr != nil {
			return common.ExecResult{}, theme.ErrLocalIO("write "+themeenv.FileName, serr)
		}
		return common.ExecResult{Body: map[string]any{"file": path, "removed": name}}, nil
	},
}

// loadOrNewThemeEnvFile finds shoplazza.theme.toml upward from startPath and
// parses it; when none exists it returns an empty File and the path where a new
// one should be written (startPath/FileName), with existed=false.
func loadOrNewThemeEnvFile(startPath string) (themeenv.File, string, bool, error) {
	if startPath == "" {
		startPath = "."
	}
	path, err := themeenv.Find(startPath)
	if err == nil {
		f, lerr := themeenv.Load(path)
		if lerr != nil {
			return themeenv.File{}, "", false, theme.ErrValidation("%v", lerr)
		}
		return f, path, true, nil
	}
	if !errors.Is(err, themeenv.ErrNotFound) {
		return themeenv.File{}, "", false, theme.ErrLocalIO("find "+themeenv.FileName, err)
	}
	return themeenv.File{}, filepath.Join(startPath, themeenv.FileName), false, nil
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

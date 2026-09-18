package themecmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/env"
)

// newCmdEnv is the `themes env` subtree: read + edit the project's
// shoplazza.theme.toml environments. Every subcommand is local (no store client,
// no auth) — the AuthFree annotation makes the shared `themes` module auth gate
// skip them.
func newCmdEnv(f *cmdutil.Factory) *cobra.Command {
	root := &cobra.Command{
		Use:   "env",
		Short: "Inspect and edit theme environments (" + env.FileName + ")",
		Long:  "Read and edit the project's " + env.FileName + " environments. Hand-editing the file stays fully supported; add/set/remove are helpers that rewrite it without comments.",
	}
	root.AddCommand(
		newCmdEnvList(f),
		newCmdEnvShow(f),
		newCmdEnvCheck(f),
		newCmdEnvAdd(f),
		newCmdEnvSet(f),
		newCmdEnvRemove(f),
	)
	return root
}

// authFree marks a local command so the `themes` module auth gate skips it.
// authFreeWrite additionally marks it NotScannable — it writes the filesystem,
// so blind CLI scans must skip it.
var (
	authFree      = map[string]string{cmdutil.AnnotationAuthFree: "true"}
	authFreeWrite = map[string]string{cmdutil.AnnotationAuthFree: "true", cmdutil.AnnotationNotScannable: "true"}
)

func newCmdEnvList(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List the theme environments defined in " + env.FileName,
		Long:        "List the environments in the project's " + env.FileName + " (found by searching up from the current directory). Read-only and offline.",
		Example:     "  shoplazza themes env list",
		Args:        cobra.NoArgs,
		Annotations: authFree,
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, p, err := loadThemeEnvFile(".")
			if err != nil {
				return err
			}
			envs := make([]map[string]any, 0, len(file.Environments))
			for _, name := range file.Names() {
				e, _ := file.Environment(name)
				envs = append(envs, envToMap(name, e))
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{"file": p, "environments": envs}, cmdutil.GetFormat(cmd), "")
		},
	}
}

func newCmdEnvShow(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:         "show [name]",
		Short:       "Show one theme environment's settings",
		Long:        "Show a single environment from " + env.FileName + "; omit the name for the 'default' environment. Read-only and offline.",
		Example:     "  shoplazza themes env show staging",
		Args:        cobra.MaximumNArgs(1),
		Annotations: authFree,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			file, _, err := loadThemeEnvFile(".")
			if err != nil {
				return err
			}
			e, ok := file.Environment(name)
			if !ok {
				return envNotFound(file, name)
			}
			resolved := name
			if resolved == "" {
				resolved = env.DefaultEnvironment
			}
			return output.PrintBody(cmd.OutOrStdout(), envToMap(resolved, e), cmdutil.GetFormat(cmd), "")
		},
	}
}

func newCmdEnvCheck(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:         "check [name]",
		Short:       "Validate theme environments offline (each resolves to a configured profile/store)",
		Long:        "Offline-validate the environments in " + env.FileName + ": referenced profiles exist and store-only environments have an authenticated profile. Omit the name to check them all. Exits non-zero if any fail.",
		Example:     "  shoplazza themes env check",
		Args:        cobra.MaximumNArgs(1),
		Annotations: authFree,
		RunE: func(cmd *cobra.Command, args []string) error {
			file, p, err := loadThemeEnvFile(".")
			if err != nil {
				return err
			}
			names := file.Names()
			if len(args) > 0 {
				if _, ok := file.Environment(args[0]); !ok {
					return envNotFound(file, args[0])
				}
				names = []string{args[0]}
			}
			cfg := loadProfileConfig()
			results := make([]map[string]any, 0, len(names))
			var failures []string
			for _, name := range names {
				e, _ := file.Environment(name)
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
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					strconv.Itoa(len(failures))+" environment issue(s) in "+env.FileName, strings.Join(failures, "; "))
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{"file": p, "ok": true, "environments": results}, cmdutil.GetFormat(cmd), "")
		},
	}
}

func newCmdEnvAdd(f *cmdutil.Factory) *cobra.Command {
	var store, themeID, profile string
	var live bool
	cmd := &cobra.Command{
		Use:         "add [name] --store <domain> [--theme <id>] [--profile <name>] [--live]",
		Short:       "Add a new environment to " + env.FileName,
		Long:        "Add a new [environments.<name>] block to " + env.FileName + " (created if absent). Omit the name to be prompted for it. Errors if the environment exists — use 'themes env set'. Advanced keys (path/ignore) are hand-edited; this rewrites the file without comments.",
		Example:     "  shoplazza themes env add staging --store staging.myshoplaza.com --theme 123456 --profile staging",
		Args:        cobra.MaximumNArgs(1),
		Annotations: authFreeWrite,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Interactive fill (same contract as themeext/app): a human is prompted
			// for the name and store (required) and offered theme/profile; an agent
			// that omits the name or --store gets one structured error instead of a
			// silently-empty environment.
			name, err := resolveNewEnvName(cmd, f, args)
			if err != nil {
				return err
			}
			if err := cmdutil.ResolveFlags(cmd, f,
				cmdutil.PromptField{Flag: "store", Title: "Store domain (e.g. my-dev.myshoplaza.com)"},
				cmdutil.PromptField{Flag: "theme", Title: "Theme id (optional)", Optional: true},
				cmdutil.PromptField{Flag: "profile", Title: "Profile to authenticate with (optional; else matched by store)", Optional: true, Picker: profilePickerChoices},
			); err != nil {
				return err
			}
			file, p, existed, err := loadOrNewThemeEnvFile(".")
			if err != nil {
				return err
			}
			if _, ok := file.Environment(name); ok {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"environment "+name+" already exists in "+env.FileName,
					"use 'shoplazza themes env set "+name+"' to change it")
			}
			e := env.Environment{Store: store, Theme: themeID, Profile: profile, Live: live}
			if file.Environments == nil {
				file.Environments = map[string]env.Environment{}
			}
			file.Environments[name] = e
			if serr := env.Save(p, file); serr != nil {
				return theme.ErrLocalIO("write "+env.FileName, serr)
			}
			body := envToMap(name, e)
			body["file"] = p
			body["created_file"] = !existed
			return output.PrintBody(cmd.OutOrStdout(), body, cmdutil.GetFormat(cmd), "")
		},
	}
	bindEnvWriteFlags(cmd, &store, &themeID, &profile, &live)
	return cmd
}

func newCmdEnvSet(f *cmdutil.Factory) *cobra.Command {
	var store, themeID, profile string
	var live bool
	cmd := &cobra.Command{
		Use:         "set [name] [--store <domain>] [--theme <id>] [--profile <name>] [--live]",
		Short:       "Change fields of an existing environment in " + env.FileName,
		Long:        "Update an existing [environments.<name>] block; only the flags you pass are changed. Omit the name to pick one interactively. Errors if the environment does not exist — use 'themes env add'. Rewrites the file without comments.",
		Example:     "  shoplazza themes env set staging --theme 654321",
		Args:        cobra.MaximumNArgs(1),
		Annotations: authFreeWrite,
		RunE: func(cmd *cobra.Command, args []string) error {
			file, p, err := loadThemeEnvFile(".")
			if err != nil {
				return err
			}
			name, err := resolveEnvName(f, file, args)
			if err != nil {
				return err
			}
			e, ok := file.Environment(name)
			if !ok {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"environment "+name+" not found in "+env.FileName,
					"use 'shoplazza themes env add "+name+"' to create it")
			}
			if cmd.Flags().Changed("store") {
				e.Store = store
			}
			if cmd.Flags().Changed("theme") {
				e.Theme = themeID
			}
			if cmd.Flags().Changed("profile") {
				e.Profile = profile
			}
			if cmd.Flags().Changed("live") {
				e.Live = live
			}
			file.Environments[name] = e
			if serr := env.Save(p, file); serr != nil {
				return theme.ErrLocalIO("write "+env.FileName, serr)
			}
			body := envToMap(name, e)
			body["file"] = p
			return output.PrintBody(cmd.OutOrStdout(), body, cmdutil.GetFormat(cmd), "")
		},
	}
	bindEnvWriteFlags(cmd, &store, &themeID, &profile, &live)
	return cmd
}

func newCmdEnvRemove(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:         "remove [name]",
		Short:       "Remove an environment from " + env.FileName,
		Long:        "Delete the [environments.<name>] block from " + env.FileName + ". Omit the name to pick one interactively. Rewrites the file without comments.",
		Example:     "  shoplazza themes env remove staging",
		Args:        cobra.MaximumNArgs(1),
		Annotations: authFreeWrite,
		RunE: func(cmd *cobra.Command, args []string) error {
			file, p, err := loadThemeEnvFile(".")
			if err != nil {
				return err
			}
			name, err := resolveEnvName(f, file, args)
			if err != nil {
				return err
			}
			if _, ok := file.Environment(name); !ok {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"environment "+name+" not found in "+env.FileName,
					"run 'shoplazza themes env list' to see the defined environments")
			}
			delete(file.Environments, name)
			if serr := env.Save(p, file); serr != nil {
				return theme.ErrLocalIO("write "+env.FileName, serr)
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{"file": p, "removed": name}, cmdutil.GetFormat(cmd), "")
		},
	}
}

// bindEnvWriteFlags binds the shared add/set env-field flags.
func bindEnvWriteFlags(cmd *cobra.Command, store, themeID, profile *string, live *bool) {
	cmd.Flags().StringVar(store, "store", "", "Store domain (e.g. staging.myshoplaza.com)")
	cmd.Flags().StringVar(themeID, "theme", "", "Theme id the environment targets")
	cmd.Flags().StringVar(profile, "profile", "", "Keychain profile to authenticate with (else matched by store)")
	cmd.Flags().BoolVar(live, "live", false, "Target the store's published (live) theme")
}

// profilePickerChoices lists the configured keychain profiles for env add's
// --profile prompt, so a human picks a real profile instead of typing a name
// they'd have to look up. A leading "(skip)" option (value "") leaves --profile
// unset (the store matches a profile at run time). No profiles configured ->
// nil, letting ResolveFlags fall through to a (skippable) text input.
func profilePickerChoices(_ context.Context, _ *cobra.Command, f *cmdutil.Factory) ([]interact.Option, error) {
	if f == nil || len(f.Config.Profiles) == 0 {
		return nil, nil
	}
	opts := make([]interact.Option, 0, len(f.Config.Profiles)+1)
	opts = append(opts, interact.Option{Label: "(skip — match by store at run time)", Value: ""})
	for i := range f.Config.Profiles {
		p := f.Config.Profiles[i]
		label := p.Name
		if p.StoreDomain != "" {
			label += " — " + p.StoreDomain
		}
		opts = append(opts, interact.Option{Label: label, Value: p.Name})
	}
	return opts, nil
}

// resolveNewEnvName resolves the name for a new environment (env add): the
// positional arg when given, else a text prompt for a human. Non-interactively
// an omitted name is a structured error — the name is required for agents. Unlike
// set/remove this must NOT pick from existing names: the name is new.
func resolveNewEnvName(cmd *cobra.Command, f *cmdutil.Factory, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if !cmdutil.Interactive(f) {
		return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"environment name is required",
			"pass a name, e.g. 'shoplazza themes env add staging --store <domain>'")
	}
	name, err := interact.Input("Environment name (e.g. staging)", func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("environment name is required")
		}
		return nil
	})
	return strings.TrimSpace(name), err
}

// resolveEnvName resolves the target environment for set/remove: the positional
// arg when given, else a fuzzy picker over the file's environments for a human.
// Non-interactively an omitted name is a structured error (agents must name the
// environment); an empty file is likewise an error either way.
func resolveEnvName(f *cmdutil.Factory, file env.File, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	names := file.Names()
	if len(names) == 0 {
		return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"no environments are defined in "+env.FileName,
			"add one with 'shoplazza themes env add <name> --store <domain>'")
	}
	if !cmdutil.Interactive(f) {
		return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"environment name is required",
			"pass a name (one of: "+strings.Join(names, ", ")+")")
	}
	opts := make([]interact.Option, 0, len(names))
	for _, n := range names {
		opts = append(opts, interact.Option{Label: n, Value: n})
	}
	return interact.SelectFiltered("Which environment? (type to filter)", opts)
}

// envNotFound builds the structured "environment not found" error listing the
// defined names.
func envNotFound(file env.File, name string) error {
	shown := name
	if shown == "" {
		shown = env.DefaultEnvironment
	}
	hint := "no environments are defined"
	if names := file.Names(); len(names) > 0 {
		hint = "defined environments: " + strings.Join(names, ", ")
	}
	return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
		"environment "+shown+" not found in "+env.FileName, hint)
}

// loadThemeEnvFile finds shoplazza.theme.toml upward from startPath and parses
// it. A missing file is a structured, hinted error; a parse error is a
// validation error naming the file.
func loadThemeEnvFile(startPath string) (env.File, string, error) {
	if startPath == "" {
		startPath = "."
	}
	p, err := env.Find(startPath)
	if err != nil {
		if errors.Is(err, env.ErrNotFound) {
			return env.File{}, "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				"no "+env.FileName+" found in this directory or any parent",
				"create one with an [environments.<name>] block, e.g. 'shoplazza themes env add <name> --store <domain>'")
		}
		return env.File{}, "", theme.ErrLocalIO("find "+env.FileName, err)
	}
	file, err := env.Load(p)
	if err != nil {
		return env.File{}, "", theme.ErrValidation("%v", err)
	}
	return file, p, nil
}

// loadOrNewThemeEnvFile is loadThemeEnvFile, but a missing file is not an error:
// it returns an empty File and the path where a new one should be written
// (startPath/FileName), with existed=false.
func loadOrNewThemeEnvFile(startPath string) (env.File, string, bool, error) {
	if startPath == "" {
		startPath = "."
	}
	p, err := env.Find(startPath)
	if err == nil {
		file, lerr := env.Load(p)
		if lerr != nil {
			return env.File{}, "", false, theme.ErrValidation("%v", lerr)
		}
		return file, p, true, nil
	}
	if !errors.Is(err, env.ErrNotFound) {
		return env.File{}, "", false, theme.ErrLocalIO("find "+env.FileName, err)
	}
	return env.File{}, filepath.Join(startPath, env.FileName), false, nil
}

// checkEnvironment returns the offline problems with one environment.
func checkEnvironment(cfg *core.CliConfig, e env.Environment) []string {
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

// loadProfileConfig reads the CLI config (profile library) from disk — env check
// validates against it without needing a factory.
func loadProfileConfig() core.CliConfig {
	p, _ := core.DefaultConfigPath()
	cfg, _ := core.LoadConfig(p)
	return cfg
}

// envToMap renders one environment for output, omitting zero-value fields.
func envToMap(name string, e env.Environment) map[string]any {
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

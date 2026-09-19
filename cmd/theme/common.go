// Package themecmd hosts the theme development workflow commands (push / pull /
// serve) as plain cobra commands. They are complex, filesystem-heavy, sometimes
// long-running dev loops that need per-command control over which store and
// profile they act on — the same shape as cmd/themeext, NOT the
// single-request shortcut model. Because they build their own store client
// here, project-level multi-environment (-e) is resolved entirely in this
// package: nothing in the shared shortcut engine or cmdutil.ResolveProfile needs
// to know about theme environments.
package themecmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/spf13/cobra"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/env"
)

// envAccessToken is the CI/test bypass honored across the CLI: a non-empty
// SHOPLAZZA_ACCESS_TOKEN is used verbatim as the store bearer token.
const envAccessToken = "SHOPLAZZA_ACCESS_TOKEN"

// resolvedStore is the outcome of resolving where a theme command acts: a store
// client bound to a token, the store domain (for banners/results), and the
// environment block that was selected (empty when no -e), whose theme/path/
// ignore a command applies to its own unset inputs.
type resolvedStore struct {
	Client *client.Client
	Domain string
	Env    env.Environment
	// Profile is the keychain profile name the client authenticated with, or
	// "" when a token came from the CI env var (no profile). Used by pull to
	// record a default environment in shoplazza.theme.toml.
	Profile string
}

// resolveStore decides the target store+profile for a theme command and builds a
// client for it. With -e/SHOPLAZZA_CLI_ENVIRONMENT it uses the environment's
// profile= (by name) or store= (matched to an authenticated profile). With
// neither, it auto-applies the project's committed [environments.default] when a
// shoplazza.theme.toml declares one (a visible, version-controlled default — not
// hidden mutable state); absent that, it falls back to the ordinary profile chain
// (cmdutil.ResolveProfile: --profile > SHOPLAZZA_CLI_PROFILE > current). Either
// way the resolved store is echoed to stderr so the target is never silent. All
// multi-environment logic lives here — the generic resolver is only consulted for
// the no-env case.
func resolveStore(ctx context.Context, f *cmdutil.Factory, cmd *cobra.Command) (resolvedStore, error) {
	var selEnv env.Environment
	envName := environmentName(cmd)
	if envName != "" {
		e, err := loadSelectedEnvironment(envName)
		if err != nil {
			return resolvedStore{}, err
		}
		selEnv = e
	} else if e, ok, err := defaultEnvironmentAt("."); err != nil {
		// No -e was given, so a malformed shoplazza.theme.toml must NOT break the
		// command — warn and fall back to the ordinary profile chain. (With -e the
		// parse error is surfaced hard, via loadSelectedEnvironment above.)
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: ignoring %s: %v\n", env.FileName, err)
	} else if ok {
		envName = env.DefaultEnvironment
		selEnv = e
	}

	// CI/env-token bypass FIRST — it needs no profile. The store target comes
	// from -e (store=/profile=), else SHOPLAZZA_CLI_API_BASE_URL, else the
	// current profile's domain. Base URL is that env var when set, else
	// https://<domain>.
	if tok := os.Getenv(envAccessToken); tok != "" {
		domain := envDomain(f, selEnv)
		base := os.Getenv("SHOPLAZZA_CLI_API_BASE_URL")
		if base == "" {
			if domain == "" {
				return resolvedStore{}, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					envAccessToken+" is set but no store target is available",
					"set SHOPLAZZA_CLI_API_BASE_URL, or select a store via -e or a profile")
			}
			base = "https://" + domain
		}
		c := client.New(base)
		c.SetBearerToken(tok)
		echoTarget(cmd, domain, envName)
		return resolvedStore{Client: c, Domain: domain, Env: selEnv}, nil
	}

	// Normal path: resolve the profile (env-aware) and mint its token.
	var profile *core.ProfileConfig
	if envName != "" {
		p, err := profileForEnvironment(f, envName, selEnv)
		if err != nil {
			return resolvedStore{}, err
		}
		profile = p
	} else {
		p, err := cmdutil.ResolveProfile(f, cmd)
		if err != nil {
			return resolvedStore{}, err
		}
		profile = p
	}

	mgr := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
	tok, err := mgr.AccessTokenReadyForProfile(ctx, f.ConfigPath, *profile)
	if err != nil {
		return resolvedStore{}, storeTokenError(err)
	}
	c := client.New("https://" + profile.StoreDomain)
	c.SetBearerToken(tok)
	echoTarget(cmd, profile.StoreDomain, envName)
	return resolvedStore{Client: c, Domain: profile.StoreDomain, Env: selEnv, Profile: profile.Name}, nil
}

// defaultEnvironmentAt returns the committed [environments.default] block when a
// shoplazza.theme.toml exists at/above startPath and declares one. ok=false means
// "no toml, or no default block" → the caller keeps today's no-environment
// behavior. A malformed toml is surfaced, not swallowed.
func defaultEnvironmentAt(startPath string) (env.Environment, bool, error) {
	if startPath == "" {
		startPath = "."
	}
	path, err := env.Find(startPath)
	if err != nil {
		if errors.Is(err, env.ErrNotFound) {
			return env.Environment{}, false, nil
		}
		return env.Environment{}, false, output.ErrInternal("find %s: %v", env.FileName, err)
	}
	file, err := env.Load(path)
	if err != nil {
		return env.Environment{}, false, output.ErrValidation("%v", err)
	}
	e, ok := file.Environment(env.DefaultEnvironment)
	return e, ok, nil
}

// echoTarget prints the resolved store (and environment, when one applies) to
// stderr, so no theme command ever acts on a store the user can't see. Empty
// domain (rare env-token-with-no-target path) prints nothing.
func echoTarget(cmd *cobra.Command, domain, envName string) {
	if domain == "" {
		return
	}
	msg := "→ store: " + domain
	if envName != "" {
		msg += " (environment: " + envName + ")"
	}
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), msg)
}

// envDomain derives the store domain for the env-token path: a selected
// environment's store=/profile=, else the current profile's domain, else "".
func envDomain(f *cmdutil.Factory, e env.Environment) string {
	if e.Store != "" {
		return cmdutil.NormalizeStoreDomain(e.Store)
	}
	if e.Profile != "" {
		if p := f.Config.FindProfile(e.Profile); p != nil {
			return p.StoreDomain
		}
	}
	if p := f.Config.Current(); p != nil {
		return p.StoreDomain
	}
	return ""
}

// environmentName is the selected environment: --environment else
// SHOPLAZZA_CLI_ENVIRONMENT. Empty when the command defines no such flag.
func environmentName(cmd *cobra.Command) string {
	if cmd.Flags().Lookup(env.EnvironmentFlag) == nil {
		return ""
	}
	if v, _ := cmd.Flags().GetString(env.EnvironmentFlag); v != "" {
		return v
	}
	return os.Getenv(env.EnvironmentVar)
}

// loadSelectedEnvironment finds shoplazza.theme.toml upward from the current
// directory, parses it, and returns the named environment. A missing file or
// unknown environment is a structured, hinted error.
func loadSelectedEnvironment(name string) (env.Environment, error) {
	path, err := env.Find(".")
	if err != nil {
		if errors.Is(err, env.ErrNotFound) {
			return env.Environment{}, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				"-e "+name+" was given but no "+env.FileName+" was found",
				"create "+env.FileName+" with an [environments."+name+"] block, or drop -e")
		}
		return env.Environment{}, output.ErrInternal("find %s: %v", env.FileName, err)
	}
	file, err := env.Load(path)
	if err != nil {
		return env.Environment{}, output.ErrValidation("%v", err)
	}
	e, ok := file.Environment(name)
	if !ok {
		hint := "no environments are defined in " + path
		if names := file.Names(); len(names) > 0 {
			hint = "defined environments: " + joinNames(names)
		}
		return env.Environment{}, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"environment "+name+" not found in "+env.FileName, hint)
	}
	return e, nil
}

// profileForEnvironment maps a selected environment to an authenticated profile:
// profile= by name, else store= matched to a profile. Any miss is a structured
// error — never a silent wrong-store.
func profileForEnvironment(f *cmdutil.Factory, name string, e env.Environment) (*core.ProfileConfig, error) {
	if e.Profile != "" {
		if p := f.Config.FindProfile(e.Profile); p != nil {
			return p, nil
		}
		return nil, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"environment "+name+" names profile "+e.Profile+", which is not configured",
			"run 'shoplazza profile list', or 'shoplazza auth login -s <store>' to create it")
	}
	if e.Store != "" {
		domain := cmdutil.NormalizeStoreDomain(e.Store)
		if p := f.Config.FindProfileByStore(domain); p != nil {
			return p, nil
		}
		return nil, output.ErrWithHint(output.ExitAuth, output.TypeAuth,
			"environment "+name+" targets store "+domain+", but no profile is authenticated for it",
			"run 'shoplazza auth login -s "+domain+"', or set profile= in the environment")
	}
	// The environment sets neither store nor profile (only theme/path/ignore):
	// ride the ordinary current profile.
	return cmdutil.ResolveProfile(f, nil)
}

// storeTokenError classifies a failed store-token mint (mirrors cmd/themeext):
// an already-structured error passes through; an HTTP error becomes auth-class
// with a re-login hint; a wire failure is network-class; else a plain auth error.
func storeTokenError(err error) *output.ExitError {
	var already *output.ExitError
	if errors.As(err, &already) {
		return already
	}
	const hint = "run 'shoplazza auth login' to re-authenticate"
	var he *client.HTTPError
	if errors.As(err, &he) {
		return output.ErrAPIAuthHint(he.StatusCode, he.Body, he.RequestID, hint)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return output.ErrNetwork("store access token unavailable: %v", err)
	}
	return output.ErrAuth("store access token unavailable: %v", err)
}

// joinNames is a tiny comma-join kept local so common.go pulls in no extra deps.
func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

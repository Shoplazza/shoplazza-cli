package cmdutil

import (
	"fmt"
	"io"
	"os"

	"shoplazza-cli-v2/internal/build"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/migrate"
)

// IOStreams groups command IO handles.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

// Factory bundles common dependencies for commands.
type Factory struct {
	IOStreams  IOStreams
	ConfigPath string
	Config     core.CliConfig
	Client     *client.Client
	AuthClient *client.Client
}

// NewDefaultFactory creates a minimal default command factory.
//
// Two-phase wiring: the store Client starts with an empty base URL and no
// token — RequireAuth (the auth gate) resolves the target profile and
// injects both once a command that needs them actually runs. Auth-free
// commands never touch either.
func NewDefaultFactory() *Factory {
	configPath, _ := core.DefaultConfigPath()
	// One-time v1->v2 migration. Errors surface as a stderr warning rather
	// than blocking: auth-free commands must keep working even on a broken
	// migration, and login-requiring commands will fail loudly at the gate.
	if err := migrate.Run(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: config migration failed: %v\n", err)
	}
	cfg, _ := core.LoadConfig(configPath)

	authBaseURL := os.Getenv("SHOPLAZZA_CLI_AUTH_BASE_URL")
	if authBaseURL == "" {
		authBaseURL = build.DefaultAuthBaseURL
	}

	return &Factory{
		IOStreams: IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
		ConfigPath: configPath,
		Config:     cfg,
		Client:     client.New(""),
		AuthClient: client.New(authBaseURL),
	}
}

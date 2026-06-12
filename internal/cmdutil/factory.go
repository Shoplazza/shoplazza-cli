package cmdutil

import (
	"io"
	"os"

	"shoplazza-cli-v2/internal/build"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/core"
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
func NewDefaultFactory() *Factory {
	configPath, _ := core.DefaultConfigPath()
	cfg, _ := core.LoadConfig(configPath)

	storeBaseURL := ""
	if cfg.StoreDomain != "" {
		storeBaseURL = "https://" + cfg.StoreDomain
	}
	authBaseURL := os.Getenv("SHOPLAZZA_CLI_AUTH_BASE_URL")
	if authBaseURL == "" {
		authBaseURL = build.DefaultAuthBaseURL
	}
	apiBaseURL := os.Getenv("SHOPLAZZA_CLI_API_BASE_URL")
	if apiBaseURL == "" {
		apiBaseURL = storeBaseURL
	}

	apiClient := client.New(apiBaseURL)
	if token := os.Getenv("SHOPLAZZA_ACCESS_TOKEN"); token != "" {
		apiClient.SetBearerToken(token)
	}

	return &Factory{
		IOStreams: IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
		ConfigPath: configPath,
		Config:     cfg,
		Client:     apiClient,
		AuthClient: client.New(authBaseURL),
	}
}

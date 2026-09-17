package cmdutil

// Sidecar (credential-isolation) environment variables, shared by both build
// variants so the fail-closed guard and the interceptor installer agree on the
// names. See internal/sidecar and docs/AGENT_CLI_TECH_SOLUTION.md (M4).
const (
	// EnvAuthProxy points the CLI at a local sidecar (e.g. http://127.0.0.1:16384).
	// Its presence is what triggers sidecar mode.
	EnvAuthProxy = "SHOPLAZZA_CLI_AUTH_PROXY"
	// EnvProxyKey is the hex-encoded HMAC key shared with the sidecar.
	EnvProxyKey = "SHOPLAZZA_CLI_PROXY_KEY"
)

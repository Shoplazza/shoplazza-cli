//go:build authsidecar

package cmdutil

import (
	"fmt"
	"os"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/sidecar"
)

// applySidecar installs the credential-isolation interceptor on the factory's
// HTTP clients when the sidecar proxy env is set. Real tokens never enter this
// process: the sandbox presents sentinel tokens (via SHOPLAZZA_ACCESS_TOKEN),
// which the interceptor signs and reroutes to the trusted sidecar for injection.
func applySidecar(f *Factory) {
	proxy := os.Getenv(EnvAuthProxy)
	if proxy == "" {
		return // not in sidecar mode
	}
	key, err := sidecar.DecodeKey(os.Getenv(EnvProxyKey))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s: %v\n", EnvProxyKey, err)
		os.Exit(2)
	}
	host := sidecarHostFromURL(proxy)
	install := func(c *client.Client) {
		if c == nil || c.HTTPClient == nil {
			return
		}
		c.HTTPClient.Transport = sidecar.NewInterceptor(key, host, c.HTTPClient.Transport)
	}
	install(f.Client)
	install(f.AuthClient)
}

// sidecarHostFromURL reduces a proxy URL to its host:port, tolerating a bare
// host:port with no scheme.
func sidecarHostFromURL(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	return strings.TrimRight(u, "/")
}

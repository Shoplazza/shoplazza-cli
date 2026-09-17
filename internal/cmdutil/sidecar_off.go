//go:build !authsidecar

package cmdutil

import (
	"fmt"
	"os"
)

// applySidecar is a no-op in the standard build — except that it MUST fail
// closed. If the sidecar proxy env is set here, this binary was compiled
// WITHOUT the `authsidecar` tag, so the credential-isolation interceptor is not
// present. Running would send whatever real token the environment provides
// straight to the API, defeating the whole point. Refuse to run.
func applySidecar(_ *Factory) {
	if os.Getenv(EnvAuthProxy) != "" {
		fmt.Fprintf(os.Stderr,
			"ERROR: %s is set, but this shoplazza binary was built WITHOUT the "+
				"'authsidecar' build tag.\nCredential isolation is compiled out — running "+
				"would bypass the sidecar and send real credentials directly to the API.\n"+
				"Use a binary built with `-tags authsidecar`, or unset %s.\n",
			EnvAuthProxy, EnvAuthProxy)
		os.Exit(2)
	}
}

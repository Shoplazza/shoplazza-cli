//go:build !authsidecar

package main

import (
	"fmt"
	"os"

	"github.com/Shoplazza/shoplazza-cli/v2/sidecar"
)

// In the standard build the interceptor is compiled out. If the sidecar proxy
// env is set here, this binary was built WITHOUT `-tags authsidecar`, so running
// would bypass credential isolation — refuse to run (fail closed), matching the
// Lark CLI. The check is a single getenv; it adds no meaningful weight.
func init() {
	if sidecar.ProxyConfigured() {
		fmt.Fprintf(os.Stderr,
			"ERROR: %s is set, but this shoplazza binary was built WITHOUT the "+
				"'authsidecar' build tag.\nCredential isolation is compiled out — running "+
				"would bypass the sidecar. Refusing to run.\nUse a binary built with "+
				"`-tags authsidecar`, or unset %s.\n",
			sidecar.EnvAuthProxy, sidecar.EnvAuthProxy)
		os.Exit(2)
	}
}

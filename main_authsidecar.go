//go:build authsidecar

package main

import (
	"fmt"
	"os"

	"github.com/Shoplazza/shoplazza-cli/v2/sidecar"
)

// In an authsidecar build, install the credential-isolation interceptor when the
// sidecar proxy env is set (a no-op otherwise). The sandbox holds only sentinel
// tokens; this reroutes its API requests through the trusted sidecar, which
// injects the real token. A bad key fails closed. Build with `-tags authsidecar`
// to produce a sandbox-ready CLI. See sidecar/server-demo/README.md.
func init() {
	if err := sidecar.InstallFromEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "shoplazza: sidecar setup failed: %v\n", err)
		os.Exit(2)
	}
}

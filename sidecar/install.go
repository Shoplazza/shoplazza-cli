package sidecar

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Sidecar mode is driven by two environment variables in the sandbox.
const (
	// EnvAuthProxy is the sidecar URL; its presence enables sidecar mode.
	EnvAuthProxy = "SHOPLAZZA_CLI_AUTH_PROXY"
	// EnvProxyKey is the hex-encoded HMAC key shared with the sidecar.
	EnvProxyKey = "SHOPLAZZA_CLI_PROXY_KEY"
)

// ProxyConfigured reports whether the sidecar proxy env is set. A CLI built
// WITHOUT sidecar support should call this at startup and fail closed if it is
// true, so a misconfigured binary can't bypass isolation.
func ProxyConfigured() bool { return os.Getenv(EnvAuthProxy) != "" }

// InstallFromEnv wires the credential-isolation interceptor onto
// http.DefaultTransport when the sidecar proxy env is set (a no-op otherwise).
//
// This is NOT wired into the shoplazza CLI itself — a hosting platform opts in
// by adding a one-line, build-tagged file to its own CLI build, e.g.:
//
//	//go:build authsidecar
//	package main
//	func init() { _ = sidecar.InstallFromEnv() }
//
// Requests made through clients with a nil Transport (the CLI's default) then
// get signed and rerouted to the sidecar. Keeping this opt-in means the shipped
// CLI carries none of it.
func InstallFromEnv() error {
	proxy := os.Getenv(EnvAuthProxy)
	if proxy == "" {
		return nil
	}
	key, err := DecodeKey(os.Getenv(EnvProxyKey))
	if err != nil {
		return fmt.Errorf("%s: %w", EnvProxyKey, err)
	}
	http.DefaultTransport = NewInterceptor(key, hostFromURL(proxy), http.DefaultTransport)
	return nil
}

// hostFromURL reduces a proxy URL to host:port, tolerating a bare host:port.
func hostFromURL(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	return strings.TrimRight(u, "/")
}

package app

import (
	"errors"
	"net"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// apiOrInternal maps a client error to an ExitError: *client.HTTPError becomes
// an API-class error (naming the failing endpoint), a transport-level net.Error
// becomes a network-class error, and anything else becomes an internal error.
func apiOrInternal(err error) *output.ExitError {
	var he *client.HTTPError
	if errors.As(err, &he) {
		return output.ErrAPI(he.StatusCode, he.Body, "").WithEndpoint(he.Method, he.Path)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return output.ErrNetwork("%v", err)
	}
	return output.ErrInternal("%v", err)
}

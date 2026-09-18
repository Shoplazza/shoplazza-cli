package dynamic

import (
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
)

// destructiveVerbs are command leaf names that perform an irreversible write
// even when the HTTP method is not DELETE (e.g. cancelling an order is a POST).
var destructiveVerbs = map[string]bool{
	"delete":       true,
	"batch-delete": true,
	"cancel":       true,
	"undeploy":     true,
	"unpublish":    true,
	"remove":       true,
	"destroy":      true,
	"publish":      true, // sets the live storefront default (customer-facing)
	"upgrade":      true, // irreversibly bumps the theme framework version
}

// isDestructive reports whether a dynamic command performs an irreversible
// write: an HTTP DELETE, or a leaf verb (the last command segment) that removes
// or cancels a resource. It drives a human-only confirmation before execution;
// non-interactive callers are gated out earlier and proceed unchanged.
func isDestructive(c registry.Command) bool {
	if strings.EqualFold(c.HTTP.Method, "DELETE") {
		return true
	}
	if len(c.Path) == 0 {
		return false
	}
	return destructiveVerbs[strings.ToLower(c.Path[len(c.Path)-1])]
}

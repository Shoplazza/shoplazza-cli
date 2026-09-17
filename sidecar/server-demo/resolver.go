package main

import (
	"context"
	"fmt"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/sidecar"
)

// AuthResolver is the single-tenant TokenResolver: it resolves the real token
// for one configured profile out of the trusted host's own config + keychain.
// Store identity → the profile's (auto-refreshed) store access token; partner
// identity → the account-level partner token.
type AuthResolver struct {
	cfg        core.CliConfig
	configPath string
	authClient *client.Client
	profile    core.ProfileConfig
}

// NewAuthResolver builds a single-tenant resolver bound to one profile.
func NewAuthResolver(cfg core.CliConfig, configPath string, authClient *client.Client, profile core.ProfileConfig) *AuthResolver {
	return &AuthResolver{cfg: cfg, configPath: configPath, authClient: authClient, profile: profile}
}

// Resolve returns the real token for a verified identity.
func (r *AuthResolver) Resolve(ctx context.Context, identity string) (string, error) {
	mgr := auth.NewManager(r.cfg, r.configPath, r.authClient)
	switch identity {
	case sidecar.IdentityStore:
		return mgr.AccessTokenReadyForProfile(ctx, r.configPath, r.profile)
	case sidecar.IdentityPartner:
		return mgr.PartnerToken()
	default:
		return "", fmt.Errorf("no token resolver for identity %q", identity)
	}
}

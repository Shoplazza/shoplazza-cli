package app

import (
	"context"

	internalauth "shoplazza-cli-v2/internal/auth"
)

// EnsureAppToken runs the acquisition chain: Dashboard app-config (client_secret
// + partner_id) -> saiga ExchangeAppAT -> keychain app:<client_id>. The secret
// is never persisted. clientID comes from current-app (caller resolves).
func EnsureAppToken(ctx context.Context, d *Dashboard, mgr *internalauth.Manager, partnerID, clientID string) (string, error) {
	cfg, err := d.GetAppConfig(ctx, partnerID, clientID)
	if err != nil {
		return "", err
	}
	return mgr.AppTokenReady(ctx, clientID, cfg.ClientSecret, cfg.PartnerID)
}

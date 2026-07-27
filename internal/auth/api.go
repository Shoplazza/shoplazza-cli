package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
)

func (m *Manager) createSession(ctx context.Context, storeDomain string, scopes []string) (createSessionResponse, error) {
	req := createSessionRequest{StoreDomain: storeDomain, Scopes: scopes}
	var resp createSessionResponse
	err := m.Client.PostJSON(ctx, "/api/saiga/cli/auth/sessions", req, &resp)
	return resp, err
}

func (m *Manager) pollSessionToken(ctx context.Context, sessionID string) (pollSessionTokenResponse, error) {
	var resp pollSessionTokenResponse
	err := m.Client.GetJSON(ctx, "/api/saiga/cli/auth/sessions/"+sessionID+"/token", &resp)
	return resp, err
}

func (m *Manager) me(ctx context.Context, uat string) (meResponse, error) {
	var resp meResponse
	err := m.Client.PostJSON(ctx, "/api/saiga/cli/auth/me", meRequest{UAT: uat}, &resp)
	return resp, err
}

// exchangeStoreAT mints a store token for storeDomain with a full scope grant.
// The CLI only supplies store_domain; the server resolves (store_id, slug).
func (m *Manager) exchangeStoreAT(ctx context.Context, uat, storeDomain string) (storeATBlock, error) {
	return m.exchangeStoreATScoped(ctx, uat, storeDomain, nil)
}

// exchangeStoreATScoped mints a store token for storeDomain, optionally
// requesting a scope subset. A nil/empty scopes omits the field so the server
// grants the account's full scope set (unchanged behavior for exchangeStoreAT).
func (m *Manager) exchangeStoreATScoped(ctx context.Context, uat, storeDomain string, scopes []string) (storeATBlock, error) {
	var resp storeATBlock
	err := m.Client.PostJSON(ctx, "/api/saiga/cli/auth/exchange/store-at",
		exchangeStoreATRequest{UAT: uat, StoreDomain: storeDomain, Scopes: scopes}, &resp)
	return resp, err
}

// exchangeAppAT mints an app token for clientID. The CLI supplies
// client_secret + partner_id (fetched from the Dashboard app-config endpoint by
// the caller) and never persists them.
func (m *Manager) exchangeAppAT(ctx context.Context, uat, clientID, clientSecret, partnerID string) (appATBlock, error) {
	var resp appATBlock
	err := m.Client.PostJSON(ctx, "/api/saiga/cli/auth/exchange/app-at",
		exchangeAppATRequest{UAT: uat, ClientID: clientID, ClientSecret: clientSecret, PartnerID: partnerID}, &resp)
	return resp, err
}

// parseSaigaAuthError maps a gRPC-gateway error body ({"code":...}) to a clean
// account-level error. Denied/expired sessions arrive as HTTP 403/504.
func parseSaigaAuthError(he *client.HTTPError) error {
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(he.Body), &body)
	switch body.Code {
	case "user_denied":
		return errors.New("authorization was denied in the browser")
	case "session_expired":
		return errors.New("login session expired before authorization completed")
	default:
		return errors.New("authentication failed (status " + strconv.Itoa(he.StatusCode) + ")")
	}
}

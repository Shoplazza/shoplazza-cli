package auth

import (
	"time"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/core"
)

// atRefreshMargin is how long before AT expiry we proactively refresh.
const atRefreshMargin = 5 * time.Minute

type Manager struct {
	Config     core.CliConfig
	ConfigPath string
	AuthPath   string
	Client     *client.Client
}

// StoreState is the in-memory state for one store. Token comes from keychain
// ("store:<domain>") and is never serialized to the metadata JSON file.
type StoreState struct {
	Token         string // keychain "store:<domain>" — never serialized
	StoreID       string // numeric store id from the store-AT exchange (sent as ?store_id on app deploy/dev/generate)
	ExpiresAt     string
	GrantedScopes []string
}

// AppState is the in-memory state for one app. Token comes from keychain
// ("app:<client_id>") and is never serialized to the metadata JSON file.
type AppState struct {
	Token     string // keychain "app:<client_id>" — never serialized
	ExpiresAt string
}

// AuthState is the in-memory auth state. UAT, Partner, and per-store / per-app
// tokens come from keychain and are never written to the metadata JSON file.
type AuthState struct {
	Account          string
	UserID           string // login user id (poll/me user_id) — sent as login-user-id header
	UAT              string // keychain "uat" — never serialized
	Partner          string // keychain "partner" — never serialized
	UATExpiresAt     string
	PartnerExpiresAt string
	GrantedScopes    []string // account-level; mirror of store-AT passthrough
	Stores           map[string]StoreState
	Apps             map[string]AppState
	CurrentStore     string // from config.json.store_domain
}

// StoreTokenMeta is the on-disk metadata for one store (no token).
type StoreTokenMeta struct {
	StoreID       string   `json:"store_id,omitempty"` // not sensitive; needed by app deploy/dev/generate
	ExpiresAt     string   `json:"expires_at,omitempty"`
	GrantedScopes []string `json:"granted_scopes,omitempty"`
}

// AppTokenMeta is the on-disk metadata for one app (no token).
type AppTokenMeta struct {
	ExpiresAt string `json:"expires_at,omitempty"`
}

// authMeta is the on-disk metadata. Sensitive tokens live in keychain and are
// intentionally absent from this struct.
type authMeta struct {
	Account          string                    `json:"account,omitempty"`
	UserID           string                    `json:"user_id,omitempty"` // not sensitive; sent as login-user-id header
	UATExpiresAt     string                    `json:"uat_expires_at,omitempty"`
	PartnerExpiresAt string                    `json:"partner_expires_at,omitempty"`
	GrantedScopes    []string                  `json:"granted_scopes,omitempty"`
	Stores           map[string]StoreTokenMeta `json:"stores,omitempty"`
	Apps             map[string]AppTokenMeta   `json:"apps,omitempty"`
}

// StoreStatus is the safe-to-print per-store view.
type StoreStatus struct {
	TokenAvailable bool   `json:"token_available"`
	ExpiresAt      string `json:"expires_at,omitempty"`
}

// Status is the safe-to-print view of current authentication state.
type Status struct {
	LoggedIn      bool                   `json:"logged_in"`
	Account       string                 `json:"account,omitempty"`
	UserID        string                 `json:"user_id,omitempty"`
	CurrentStore  string                 `json:"current_store,omitempty"`
	GrantedScopes []string               `json:"granted_scopes,omitempty"`
	UATAvailable  bool                   `json:"uat_available"`
	UATExpiresAt  string                 `json:"uat_expires_at,omitempty"`
	Stores        map[string]StoreStatus `json:"stores,omitempty"`
}

type LoginResult struct {
	Flow         string
	UAT          string
	AuthorizeURL string // non-empty for web flow
	Status       Status
}

// ── HTTP request / response types ────────────────────────────────────────────

type createSessionRequest struct {
	StoreDomain string   `json:"store_domain,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

type createSessionResponse struct {
	SessionID    string `json:"session_id"`
	AuthorizeURL string `json:"authorize_url"`
}

type pollSessionTokenResponse struct {
	Status       string             `json:"status"`
	UAT          string             `json:"uat"`
	UATExpiresAt string             `json:"uat_expires_at"`
	UserID       string             `json:"user_id"`
	Account      string             `json:"account"`
	StoreToken   *storeATBlock      `json:"store_token,omitempty"`
	PartnerToken *partnerTokenBlock `json:"partner_token,omitempty"`
}

type storeATBlock struct {
	AccessToken   string   `json:"access_token"`
	StoreID       string   `json:"store_id"` // string: protojson serializes uint64 as "123"
	StoreDomain   string   `json:"store_domain"`
	GrantedScopes []string `json:"granted_scopes"`
	ATExpiresAt   string   `json:"at_expires_at"`
}

type partnerTokenBlock struct {
	AccessToken string `json:"access_token"`
	PartnerID   string `json:"partner_id"` // string: protojson serializes uint64 as "123"
	ATExpiresAt string `json:"at_expires_at"`
}

type meRequest struct {
	UAT string `json:"uat"`
}

type meResponse struct {
	UserID  string `json:"user_id"`
	Account string `json:"account"`
}

type exchangeStoreATRequest struct {
	UAT         string `json:"uat"`
	StoreDomain string `json:"store_domain"`
}

// exchangeAppATRequest is the ExchangeAppAT payload. client_secret + partner_id
// come from the Dashboard app-config endpoint and are NEVER persisted locally.
// partner_id is a uint64 in the proto; over the grpc-gateway JSON it is carried
// as a string (protojson's canonical uint64 encoding).
type exchangeAppATRequest struct {
	UAT          string `json:"uat"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	PartnerID    string `json:"partner_id"`
}

// appATBlock is the ExchangeAppAT response. access_token/client_id are strings;
// partner_id is a uint64 carried as a string by protojson; at_expires_at is a
// google.protobuf.Timestamp, serialized by protojson as an RFC3339 string.
type appATBlock struct {
	AccessToken string `json:"access_token"`
	PartnerID   string `json:"partner_id"`
	ClientID    string `json:"client_id"`
	ATExpiresAt string `json:"at_expires_at"`
}

package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
)

func NewManager(cfg core.CliConfig, configPath string, cl *client.Client) *Manager {
	authPath, _ := defaultAuthMetaPath()
	return &Manager{
		Config:     cfg,
		ConfigPath: configPath,
		AuthPath:   authPath,
		Client:     cl,
	}
}

func (m *Manager) CurrentStatus() (Status, error) {
	state, err := m.LoadState()
	if err != nil {
		return Status{}, err
	}
	return statusFromState(state), nil
}

func (m *Manager) Login(ctx context.Context, storeDomain string, scopes []string, uat string, timeout, pollInterval time.Duration, onAuthorize func(string)) (LoginResult, error) {
	if uat == "" {
		uat = os.Getenv("SHOPLAZZA_UAT")
	}
	if uat != "" {
		return m.loginWithUAT(ctx, uat, storeDomain)
	}

	session, err := m.createSession(ctx, storeDomain, scopes)
	if err != nil {
		return LoginResult{}, err
	}
	if onAuthorize != nil {
		onAuthorize(session.AuthorizeURL)
	}

	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-deadlineCtx.Done():
			return LoginResult{Flow: "web", AuthorizeURL: session.AuthorizeURL}, deadlineCtx.Err()
		default:
		}

		pollRes, err := m.pollSessionToken(deadlineCtx, session.SessionID)
		if err != nil {
			var he *client.HTTPError
			if errors.As(err, &he) {
				return LoginResult{Flow: "web", AuthorizeURL: session.AuthorizeURL}, parseSaigaAuthError(he)
			}
			return LoginResult{Flow: "web", AuthorizeURL: session.AuthorizeURL}, err
		}
		switch strings.ToLower(pollRes.Status) {
		case "pending":
			time.Sleep(pollInterval)
			continue
		case "ok":
			state := stateFromPoll(pollRes, storeDomain)
			warning := ""
			storeBlock := pollRes.StoreToken
			// Validate the requested store now (post-consent) unless the session
			// pre-warmed its token; a bad store is reported, not set as current.
			// The minted token rides out on LoginResult for the command layer to
			// persist under the profile key — not into the legacy store slot.
			if storeDomain != "" && storeBlock == nil {
				if block, sErr := m.exchangeStoreAT(ctx, pollRes.UAT, storeDomain); sErr != nil {
					state.CurrentStore = ""
					warning = storeValidationWarning(storeDomain, sErr)
				} else {
					storeBlock = &block
					state.GrantedScopes = block.GrantedScopes
				}
			}
			if err := m.persistState(state); err != nil {
				return LoginResult{Flow: "web", AuthorizeURL: session.AuthorizeURL}, err
			}
			return LoginResult{Flow: "web", UAT: pollRes.UAT, AuthorizeURL: session.AuthorizeURL, Status: statusFromState(state), StoreWarning: warning, StoreToken: storeBlock}, nil
		default:
			return LoginResult{Flow: "web", AuthorizeURL: session.AuthorizeURL}, errors.New("unexpected session status: " + pollRes.Status)
		}
	}
}

// storeValidationWarning renders the login-time message for a store that failed
// validation. A 404 means the domain doesn't exist or isn't accessible.
func storeValidationWarning(domain string, err error) string {
	var he *client.HTTPError
	if errors.As(err, &he) && he.StatusCode == 404 {
		return fmt.Sprintf("store %q not found or not accessible — not set as current store", domain)
	}
	return fmt.Sprintf("could not validate store %q (%v) — not set as current store", domain, err)
}

// applyStoreToken records a freshly minted store token in state, keyed by the
// domain the caller requested when known (the key AccessTokenReady later looks
// up via the current store), falling back to the domain the server returned. It
// mirrors the store-AT granted scopes to the account level and returns the key.
func applyStoreToken(state *AuthState, block storeATBlock, requested string) string {
	key := requested
	if key == "" {
		key = block.StoreDomain
	}
	if state.Stores == nil {
		state.Stores = map[string]StoreState{}
	}
	state.Stores[key] = StoreState{
		Token:         block.AccessToken,
		StoreID:       block.StoreID,
		ExpiresAt:     block.ATExpiresAt,
		GrantedScopes: block.GrantedScopes,
	}
	state.GrantedScopes = block.GrantedScopes
	return key
}

// stateFromPoll builds AuthState from a successful poll response. partner_token
// and store_token are best-effort: absence is not an error.
func stateFromPoll(poll pollSessionTokenResponse, storeDomain string) AuthState {
	state := AuthState{
		Account:      poll.Account,
		UserID:       poll.UserID,
		UAT:          poll.UAT,
		UATExpiresAt: poll.UATExpiresAt,
		Stores:       map[string]StoreState{},
		Apps:         map[string]AppState{},
	}
	if poll.PartnerToken != nil {
		state.Partner = poll.PartnerToken.AccessToken
		state.PartnerExpiresAt = poll.PartnerToken.ATExpiresAt
	}
	if storeDomain != "" {
		state.CurrentStore = storeDomain
	}
	if poll.StoreToken != nil {
		// Mirror the store-AT scopes to the account level; the token itself is
		// persisted under the profile key by the command layer, not here.
		state.GrantedScopes = poll.StoreToken.GrantedScopes
		if state.CurrentStore == "" {
			state.CurrentStore = poll.StoreToken.StoreDomain
		}
	}
	return state
}

// loginWithUAT performs non-interactive login: write the supplied UAT, call Me
// for account info, and (when storeDomain is set) exchange a store token. No
// partner token — those are only minted at interactive consent time.
func (m *Manager) loginWithUAT(ctx context.Context, uat, storeDomain string) (LoginResult, error) {
	meRes, err := m.me(ctx, uat)
	if err != nil {
		return LoginResult{}, err
	}
	state := AuthState{
		Account: meRes.Account,
		UserID:  meRes.UserID,
		UAT:     uat,
		Stores:  map[string]StoreState{},
		Apps:    map[string]AppState{},
	}
	var storeBlock *storeATBlock
	if storeDomain != "" {
		block, err := m.exchangeStoreAT(ctx, uat, storeDomain)
		if err != nil {
			return LoginResult{}, err
		}
		storeBlock = &block
		state.CurrentStore = storeDomain
		state.GrantedScopes = block.GrantedScopes
	}
	if err := m.persistState(state); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Flow: "uat", UAT: uat, Status: statusFromState(state), StoreToken: storeBlock}, nil
}

// Logout clears local state only — no server-side revocation.
func (m *Manager) Logout() (Status, error) {
	state, err := m.LoadState()
	if err != nil {
		return Status{}, err
	}
	_ = keychain.Remove(keychain.ShoplazzaCliService, AccountUATKey(state.Account))
	_ = keychain.Remove(keychain.ShoplazzaCliService, AccountPartnerKey(state.Account))
	for dom := range state.Stores { // auth.json map is the authoritative removal list
		_ = keychain.Remove(keychain.ShoplazzaCliService, storeKcKey(dom))
	}
	for id := range state.Apps {
		_ = keychain.Remove(keychain.ShoplazzaCliService, appKcKey(id))
	}
	if err := removeAuthMeta(m.AuthPath); err != nil {
		return Status{}, err
	}
	return Status{}, nil
}

// AvailableScopes returns the granted scopes recorded in state, or nil when
// state is nil. There is no implicit default — callers that need a fallback
// should fail with a clear validation error instead.
func (m *Manager) AvailableScopes(state *AuthState) []string {
	if state != nil && len(state.GrantedScopes) > 0 {
		return append([]string(nil), state.GrantedScopes...)
	}
	return nil
}

func (m *Manager) LoadState() (AuthState, error) {
	meta, err := loadAuthMeta(m.AuthPath)
	if err != nil {
		return AuthState{}, err
	}
	// The account keys the v2 credential lookups. auth.json is the canonical
	// source (persistState writes it alongside the UAT); fall back to the v2
	// config's account when auth.json is absent (e.g. before it is written).
	account := meta.Account
	if account == "" {
		if a := m.Config.Account(); a != nil {
			account = a.Name
		}
	}
	state := AuthState{
		Account:          account,
		UserID:           meta.UserID,
		UATExpiresAt:     meta.UATExpiresAt,
		PartnerExpiresAt: meta.PartnerExpiresAt,
		GrantedScopes:    meta.GrantedScopes,
		Stores:           map[string]StoreState{},
		Apps:             map[string]AppState{},
		CurrentStore:     m.Config.CurrentStoreDomain(),
	}
	// Propagate genuine read/decrypt failures for UAT/partner: swallowing them
	// makes a corrupted keychain look like "not logged in". The per-store/app
	// loops below stay tolerant — a missing/corrupt token self-heals via re-mint.
	uat, err := keychain.Get(keychain.ShoplazzaCliService, AccountUATKey(account))
	if err != nil {
		return AuthState{}, fmt.Errorf("reading UAT from keychain (it may be corrupted): %w", err)
	}
	state.UAT = uat
	partner, err := keychain.Get(keychain.ShoplazzaCliService, AccountPartnerKey(account))
	if err != nil {
		return AuthState{}, fmt.Errorf("reading partner token from keychain (it may be corrupted): %w", err)
	}
	state.Partner = partner
	for dom, sm := range meta.Stores {
		entry := StoreState{StoreID: sm.StoreID, ExpiresAt: sm.ExpiresAt, GrantedScopes: sm.GrantedScopes}
		if tok, err := keychain.Get(keychain.ShoplazzaCliService, storeKcKey(dom)); err == nil {
			entry.Token = tok
		}
		state.Stores[dom] = entry
	}
	for id, am := range meta.Apps {
		entry := AppState{ExpiresAt: am.ExpiresAt}
		if tok, err := keychain.Get(keychain.ShoplazzaCliService, appKcKey(id)); err == nil {
			entry.Token = tok
		}
		state.Apps[id] = entry
	}
	return state, nil
}

// RefreshAccessToken mints a fresh store token for storeDomain via the account
// UAT and caches it. Does not change the current store.
func (m *Manager) RefreshAccessToken(ctx context.Context, storeDomain string) (string, error) {
	if storeDomain == "" {
		return "", errors.New("no current store selected")
	}
	state, err := m.LoadState()
	if err != nil {
		return "", err
	}
	if state.UAT == "" {
		return "", errors.New("no UAT available — please run 'auth login' again")
	}
	block, err := m.exchangeStoreAT(ctx, state.UAT, storeDomain)
	if err != nil {
		return "", err
	}
	applyStoreToken(&state, block, storeDomain)
	if err := m.persistState(state); err != nil {
		return "", err
	}
	return block.AccessToken, nil
}

// AccessTokenReady returns the store token for storeDomain, minting/refreshing
// it when absent or within atRefreshMargin of expiry.
func (m *Manager) AccessTokenReady(ctx context.Context, storeDomain string) (string, error) {
	if storeDomain == "" {
		return "", errors.New("no current store selected")
	}
	state, err := m.LoadState()
	if err != nil {
		return "", err
	}
	if s, ok := state.Stores[storeDomain]; ok && s.Token != "" && !isNearExpiry(s.ExpiresAt, atRefreshMargin) {
		return s.Token, nil
	}
	return m.RefreshAccessToken(ctx, storeDomain)
}

// applyAppToken records a freshly minted app token in state, keyed by clientID.
// Mirrors applyStoreToken.
func applyAppToken(state *AuthState, block appATBlock, clientID string) string {
	key := clientID
	if key == "" {
		key = block.ClientID
	}
	if state.Apps == nil {
		state.Apps = map[string]AppState{}
	}
	state.Apps[key] = AppState{Token: block.AccessToken, ExpiresAt: block.ATExpiresAt}
	return key
}

// AppTokenReady returns the app token for clientID, minting/caching it when
// absent or near expiry. clientSecret/partnerID come from the Dashboard
// app-config endpoint (caller-supplied) and are never persisted.
func (m *Manager) AppTokenReady(ctx context.Context, clientID, clientSecret, partnerID string) (string, error) {
	state, err := m.LoadState()
	if err != nil {
		return "", err
	}
	if state.UAT == "" {
		return "", errors.New("no UAT available — please run 'auth login' again")
	}
	if a, ok := state.Apps[clientID]; ok && a.Token != "" && !isNearExpiry(a.ExpiresAt, atRefreshMargin) {
		return a.Token, nil
	}
	block, err := m.exchangeAppAT(ctx, state.UAT, clientID, clientSecret, partnerID)
	if err != nil {
		return "", err
	}
	applyAppToken(&state, block, clientID)
	if err := m.persistState(state); err != nil {
		return "", err
	}
	return block.AccessToken, nil
}

// PartnerToken returns the account-level partner token (keychain
// AccountPartnerKey, minted at login). Empty string means "not available" —
// caller maps that to a
// re-login auth error.
func (m *Manager) PartnerToken() (string, error) {
	state, err := m.LoadState()
	if err != nil {
		return "", err
	}
	return state.Partner, nil
}

// UserIDReady returns the login user id, sent as the login-user-id header on
// /api/cli/v2 Dashboard calls. Sessions that predate user-id capture have it
// empty in meta; backfill once via the Me endpoint (and persist), best-effort.
func (m *Manager) UserIDReady(ctx context.Context) (string, error) {
	state, err := m.LoadState()
	if err != nil {
		return "", err
	}
	if state.UserID != "" {
		return state.UserID, nil
	}
	if state.UAT == "" {
		return "", nil // not logged in; caller surfaces the auth error
	}
	meRes, err := m.me(ctx, state.UAT)
	if err != nil {
		return "", err
	}
	state.UserID = meRes.UserID
	_ = m.persistState(state) // best-effort backfill; loaded keychain tokens are re-written idempotently
	return state.UserID, nil
}

// StoreIDFor returns the numeric store id for domain (sent as ?store_id on app
// deploy/dev/generate — the backend resolves the target store from it). Older
// sessions persisted the store token without its id; backfill by re-minting via
// the UAT (RefreshAccessToken now captures store_id). Empty when not resolvable.
func (m *Manager) StoreIDFor(ctx context.Context, domain string) (string, error) {
	if domain == "" {
		return "", nil
	}
	state, err := m.LoadState()
	if err != nil {
		return "", err
	}
	if s, ok := state.Stores[domain]; ok && s.StoreID != "" {
		return s.StoreID, nil
	}
	if state.UAT == "" {
		return "", nil
	}
	if _, err := m.RefreshAccessToken(ctx, domain); err != nil {
		return "", err
	}
	state, err = m.LoadState()
	if err != nil {
		return "", err
	}
	return state.Stores[domain].StoreID, nil
}

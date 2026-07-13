package auth

import (
	"strings"

	"shoplazza-cli-v2/internal/core"
)

// EffectiveScopes resolves the scope set a profile's store token carries (or
// will request on its next mint): the minted grant if present, else the
// explicit per-profile narrowing, else the account's full granted set (the
// nil-means-inherit default).
func EffectiveScopes(p core.ProfileConfig, meta ProfileMeta, acct *core.AccountConfig) []string {
	if len(meta.GrantedScopes) > 0 {
		return meta.GrantedScopes
	}
	if len(p.Scopes) > 0 {
		return p.Scopes
	}
	if acct != nil && strings.EqualFold(acct.Name, p.Account) {
		return acct.GrantedScopes
	}
	return nil
}

// ProfileRows renders every profile as the shared display row used by both
// 'profile list' and 'auth status'. Always returns a non-nil slice.
func ProfileRows(cfg core.CliConfig, authDir string) []map[string]any {
	acct := cfg.Account()
	rows := make([]map[string]any, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		meta, _ := LoadProfileMeta(authDir, strings.ToLower(p.Name))
		storeID := p.StoreID
		if storeID == "" {
			storeID = meta.StoreID
		}
		rows = append(rows, map[string]any{
			"name":         p.Name,
			"store_domain": p.StoreDomain,
			"store_id":     storeID,
			"scopes":       EffectiveScopes(p, meta, acct),
			"token_status": TokenStatus(meta.ExpiresAt),
			"current":      strings.EqualFold(p.Name, cfg.CurrentProfile),
		})
	}
	return rows
}

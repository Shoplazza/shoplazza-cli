package auth

import "time"

// statusFromState copies the granted-scopes slice so callers can't mutate
// internal state.
func statusFromState(state AuthState) Status {
	st := Status{
		LoggedIn:      state.UAT != "",
		Account:       state.Account,
		UserID:        state.UserID,
		CurrentStore:  state.CurrentStore,
		GrantedScopes: append([]string(nil), state.GrantedScopes...),
		UATAvailable:  state.UAT != "",
		UATExpiresAt:  state.UATExpiresAt,
	}
	if len(state.Stores) > 0 {
		st.Stores = map[string]StoreStatus{}
		for dom, s := range state.Stores {
			st.Stores[dom] = StoreStatus{TokenAvailable: s.Token != "", ExpiresAt: s.ExpiresAt}
		}
	}
	return st
}

// isNearExpiry treats empty / unparseable RFC3339 timestamps as already expired.
func isNearExpiry(expiresAt string, margin time.Duration) bool {
	if expiresAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return true
	}
	return time.Until(t) <= margin
}

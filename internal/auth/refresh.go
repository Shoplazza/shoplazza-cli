package auth

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/lockfile"
)

// profileLockTimeout is the per-profile lock wait budget. A var (not const)
// so tests can lower it to exercise the lock-timeout degrade path.
var profileLockTimeout = 5 * time.Second

// profileLockPath returns the per-profile lock file path under configPath's
// locks dir. Accepted race: profile-admin ops (update/remove/rename,
// profile_sync) clear/move a profile's token+meta under config.lock only, not
// this lock, so a concurrent mint can leave a stale token cached until expiry.
// Self-heals on the next mint; nesting the two locks isn't worth the
// complexity for that window.
func profileLockPath(configPath, name string) string {
	return filepath.Join(core.LocksDir(configPath), "profile_"+strings.ToLower(name)+".lock")
}

// cachedProfileToken returns p's store AT if its metadata is fresh (not near
// expiry) and the keychain still has the token. Read-only; never mints.
func (m *Manager) cachedProfileToken(authDir string, p core.ProfileConfig) (string, bool) {
	meta, err := LoadProfileMeta(authDir, strings.ToLower(p.Name))
	if err != nil || meta.ExpiresAt == "" || isNearExpiry(meta.ExpiresAt, atRefreshMargin) {
		return "", false
	}
	tok, err := keychain.Get(keychain.ShoplazzaCliService, ProfileStoreKey(p.Name))
	if err != nil || tok == "" {
		return "", false
	}
	return tok, true
}

// AccessTokenReadyForProfile returns a valid store AT for p, minting under a
// per-profile flock when absent/near expiry. Lock timeout degrades to a
// direct exchange (correctness-safe: redundant minting is harmless, hanging
// forever is not).
func (m *Manager) AccessTokenReadyForProfile(ctx context.Context, configPath string, p core.ProfileConfig) (string, error) {
	authDir := AuthDir(configPath)
	if tok, ok := m.cachedProfileToken(authDir, p); ok {
		return tok, nil // pure-read fast path, no lock
	}
	release, err := lockfile.Acquire(profileLockPath(configPath, p.Name), profileLockTimeout)
	switch {
	case errors.Is(err, lockfile.ErrTimeout):
		// degrade: exchange directly rather than hang
		return m.ExchangeForProfile(ctx, authDir, p)
	case err != nil:
		return "", fmt.Errorf("profile lock: %w", err)
	}
	defer release()
	if tok, ok := m.cachedProfileToken(authDir, p); ok {
		return tok, nil // double-check: another holder already minted
	}
	return m.ExchangeForProfile(ctx, authDir, p)
}

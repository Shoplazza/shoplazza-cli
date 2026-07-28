package updatecheck

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/fsx"
)

const (
	cacheTTL  = 24 * time.Hour
	stateFile = "update-check.json"
)

// EnvDisable disables the background update check when set.
const EnvDisable = "SHOPLAZZA_CLI_NO_UPDATE_CHECK"

// osUserConfigDir is overridable in tests.
var osUserConfigDir = os.UserConfigDir

// Info describes an available update.
type Info struct {
	Current string
	Latest  string
}

// Message returns a single-line stderr notice string.
func (i *Info) Message() string {
	return fmt.Sprintf("⚡ shoplazza-cli %s is available (current %s) — run 'shoplazza update'", i.Latest, i.Current)
}

type state struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
}

func statePath() (string, error) {
	dir, err := osUserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "shoplazza-cli", stateFile), nil
}

func loadState() (*state, error) {
	path, err := statePath()
	if err != nil {
		return nil, err
	}
	return fsx.ReadJSON[state](path)
}

func saveState(s *state) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	return fsx.WriteJSON(path, s, 0o600)
}

// CheckCached reads the local cache only (no network). Returns Info when not skipped, the cache has a latest version, and it is newer than current.
func CheckCached(currentVersion string) *Info {
	if ShouldSkip(EnvDisable, currentVersion) {
		return nil
	}
	s, err := loadState()
	if err != nil || s == nil || s.LatestVersion == "" {
		return nil
	}
	if !IsNewer(s.LatestVersion, currentVersion) {
		return nil
	}
	return &Info{Current: currentVersion, Latest: s.LatestVersion}
}

// RefreshCache fetches the latest version over the network and writes it back when the cache is stale (>24h); no-op when fresh or skipped.
// All errors are silenced. Safe to call from a goroutine.
func RefreshCache(currentVersion string) {
	if ShouldSkip(EnvDisable, currentVersion) {
		return
	}
	if s, err := loadState(); err == nil && s != nil && Fresh(time.Unix(s.CheckedAt, 0), cacheTTL) {
		return
	}
	latest, err := fetchLatest()
	if err != nil {
		return
	}
	_ = saveState(&state{LatestVersion: latest, CheckedAt: time.Now().Unix()})
}

// Fresh reports whether a check stamped ts is still within ttl. A future
// timestamp (corrected clock) counts as stale, so the gate self-heals instead
// of latching shut.
func Fresh(ts time.Time, ttl time.Duration) bool {
	d := time.Since(ts)
	return d >= 0 && d < ttl
}

// ShouldSkip reports whether a background self-maintenance check should stay
// quiet: opted out, in CI, or not a released build. envDisable is the caller's
// own opt-out var, so the two callers stay separately disablable.
func ShouldSkip(envDisable, version string) bool {
	if os.Getenv(envDisable) != "" {
		return true
	}
	return isCIEnv() || !isReleaseVersion(version)
}

func isCIEnv() bool {
	for _, k := range []string{"CI", "BUILD_NUMBER", "RUN_ID"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

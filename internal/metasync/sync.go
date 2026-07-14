package metasync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"shoplazza-cli-v2/internal/fsx"
	"shoplazza-cli-v2/internal/registry"
	"shoplazza-cli-v2/internal/updatecheck"
)

const (
	cacheTTL       = 24 * time.Hour
	failureBackoff = time.Hour
)

// EnvDisable disables all metadata refreshes when set.
const EnvDisable = "SHOPLAZZA_CLI_NO_META_UPDATE"

// Result describes the outcome of a refresh.
type Result struct {
	OldRevision string
	NewRevision string
	Updated     bool
}

// Status is the observability snapshot surfaced by `doctor`.
type Status struct {
	Source        string    // registry.SourceEmbedded or registry.SourceCached
	Revision      string    // generated_at of the active spec
	LastCheckedAt time.Time // zero when no check has completed yet
}

// Refresh is the silent background path: TTL-gated, failures backed off,
// errors swallowed. Safe fire-and-forget.
func Refresh(ctx context.Context, currentVersion string) {
	if shouldSkip(currentVersion) {
		return
	}
	if s := loadState(); s != nil {
		// A negative Since means a future timestamp (clock rollback); treat
		// as stale so the next successful check self-heals it.
		if d := time.Since(time.Unix(s.LastCheckedAt, 0)); d >= 0 && d < cacheTTL {
			return
		}
		if d := time.Since(time.Unix(s.LastFailureAt, 0)); s.LastFailureAt > 0 && d >= 0 && d < failureBackoff {
			return
		}
	}
	if _, err := doRefresh(ctx, currentVersion); err != nil {
		markFailed()
	}
}

// ForceRefresh skips the TTL and skip-guards (explicit user action, e.g.
// `shoplazza update`) and reports what happened.
func ForceRefresh(ctx context.Context, currentVersion string) (Result, error) {
	res, err := doRefresh(ctx, currentVersion)
	if err != nil {
		markFailed()
	}
	return res, err
}

// CurrentStatus reports the active spec provenance and last check time.
func CurrentStatus() Status {
	st := Status{
		Source:   registry.SpecSource(),
		Revision: registry.LoadSpec().GeneratedAt,
	}
	if s := loadState(); s != nil && s.LastCheckedAt > 0 {
		st.LastCheckedAt = time.Unix(s.LastCheckedAt, 0)
	}
	return st
}

func doRefresh(ctx context.Context, currentVersion string) (Result, error) {
	origin := originURL()
	// A cache downloaded from a different origin never gates this one, so
	// switching origins (e.g. a staging override) repairs itself.
	local := registry.EmbeddedRevision()
	if s := loadState(); s != nil && s.Origin == origin {
		local = registry.NewestLocalRevision()
	}
	res := Result{OldRevision: local}
	m, err := fetchManifest(ctx)
	if err != nil {
		return res, err
	}
	// Fully processed gates advance the TTL clock.
	if m.FormatVersion != formatVersion || tooOld(m.MinCLIVersion, currentVersion) || m.Revision <= local {
		markChecked(origin)
		return res, nil
	}
	raw, err := fetchSpec(ctx, m)
	if err != nil {
		return res, err
	}
	spec, err := registry.ParseSpec(raw)
	if err != nil {
		return res, err
	}
	if spec.GeneratedAt != m.Revision {
		return res, errors.New("metasync: spec generated_at does not match manifest revision")
	}
	path, err := registry.CachedSpecPath()
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return res, err
	}
	if err := fsx.WriteFileAtomic(path, raw, 0o600); err != nil {
		return res, err
	}
	_ = saveState(&state{LastCheckedAt: time.Now().Unix(), Origin: origin})
	res.NewRevision, res.Updated = m.Revision, true
	return res, nil
}

// markChecked advances the TTL clock and clears any failure backoff.
func markChecked(origin string) {
	_ = saveState(&state{LastCheckedAt: time.Now().Unix(), Origin: origin})
}

// markFailed records a completed-but-failed attempt for the backoff guard.
func markFailed() {
	s := loadState()
	if s == nil {
		s = &state{}
	}
	s.LastFailureAt = time.Now().Unix()
	_ = saveState(s)
}

// tooOld reports whether the manifest requires a newer CLI; non-release
// (dev) builds always pass.
func tooOld(minVersion, current string) bool {
	return minVersion != "" && updatecheck.IsReleaseVersion(current) && updatecheck.IsNewer(minVersion, current)
}

// shouldSkip mirrors updatecheck.shouldSkip with metasync's own disable knob.
func shouldSkip(version string) bool {
	if os.Getenv(EnvDisable) != "" {
		return true
	}
	if updatecheck.IsCIEnv() {
		return true
	}
	return !updatecheck.IsReleaseVersion(version)
}

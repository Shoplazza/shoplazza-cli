package metasync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/fsx"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/updatecheck"
)

const (
	cacheTTL       = 24 * time.Hour
	failureBackoff = time.Hour
)

const (
	// EnvDisable disables all metadata refreshes when set.
	EnvDisable = "SHOPLAZZA_CLI_NO_META_UPDATE"
	// EnvOrigin overrides the metadata origin (tests, non-default environments).
	EnvOrigin = "SHOPLAZZA_CLI_META_ORIGIN"
)

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

// Refresh is the background path: TTL-gated, backed off on failure, silent.
func Refresh(ctx context.Context, currentVersion string) {
	if updatecheck.ShouldSkip(EnvDisable, currentVersion) {
		return
	}
	if s := loadState(); s != nil {
		if updatecheck.Fresh(time.Unix(s.LastCheckedAt, 0), cacheTTL) {
			return
		}
		if s.LastFailureAt > 0 && updatecheck.Fresh(time.Unix(s.LastFailureAt, 0), failureBackoff) {
			return
		}
	}
	if _, err := doRefresh(ctx, currentVersion); err != nil {
		recordFailure(err)
	}
}

// ForceRefresh skips the TTL and skip-guards (explicit user action, e.g.
// `shoplazza update`) and reports what happened.
func ForceRefresh(ctx context.Context, currentVersion string) (Result, error) {
	res, err := doRefresh(ctx, currentVersion)
	if err != nil {
		recordFailure(err)
	}
	return res, err
}

// recordFailure arms the backoff, except when we cancelled the attempt
// ourselves: the backoff is for leaving an unhealthy origin alone, and Ctrl-C
// says nothing about the origin. A timeout still counts — that one is evidence.
func recordFailure(err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	markFailed()
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
	// A different-origin cache never gates this one, so origin switches self-heal.
	local := registry.EmbeddedRevision()
	if s := loadState(); s != nil && s.Origin == origin {
		local = registry.NewestLocalRevision()
	}
	res := Result{OldRevision: local}
	m, err := fetchManifest(ctx, currentVersion, local)
	if err != nil {
		return res, err
	}
	// The server compared for us. Fully processed gates advance the TTL clock.
	if m.UpToDate {
		markChecked(origin)
		return res, nil
	}
	// LoadSpec adopts by this same rule, so overwriting the cache with anything
	// it would refuse just drops us to the embedded spec and re-downloads every
	// TTL.
	if m.Revision <= local {
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
	markChecked(origin)
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

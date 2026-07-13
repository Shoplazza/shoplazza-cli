package metasync

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"shoplazza-cli-v2/internal/fsx"
	"shoplazza-cli-v2/internal/registry"
	"shoplazza-cli-v2/internal/updatecheck"
)

const cacheTTL = 24 * time.Hour

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

// Refresh is the silent TTL-gated background path. Safe fire-and-forget: all
// errors are swallowed and the process exiting mid-download just retries on
// a later run (state is only written after a fully processed check).
func Refresh(ctx context.Context, currentVersion string) {
	if shouldSkip(currentVersion) {
		return
	}
	if s := loadState(); s != nil && time.Since(time.Unix(s.LastCheckedAt, 0)) < cacheTTL {
		return
	}
	_, _ = doRefresh(ctx, currentVersion)
}

// ForceRefresh skips the TTL and skip-guards (explicit user action, e.g.
// `shoplazza update`) and reports what happened.
func ForceRefresh(ctx context.Context, currentVersion string) (Result, error) {
	return doRefresh(ctx, currentVersion)
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
	res := Result{OldRevision: registry.NewestLocalRevision()}
	m, err := fetchManifest(ctx)
	if err != nil {
		return res, err
	}
	// Gates: unknown manifest format, binary too old, or nothing newer.
	// All three are fully processed checks — advance the TTL clock.
	if m.FormatVersion != formatVersion || tooOld(m.MinCLIVersion, currentVersion) || m.Revision <= res.OldRevision {
		markChecked()
		return res, nil
	}
	raw, err := fetchSpec(ctx, m)
	if err != nil {
		return res, err
	}
	var spec registry.Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return res, err
	}
	if len(spec.Modules) == 0 || spec.GeneratedAt != m.Revision {
		return res, errors.New("metasync: downloaded spec failed validation")
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
	_ = saveState(&state{LastCheckedAt: time.Now().Unix(), Revision: m.Revision})
	res.NewRevision, res.Updated = m.Revision, true
	return res, nil
}

// markChecked advances the TTL clock without changing the recorded revision.
func markChecked() {
	rev := ""
	if s := loadState(); s != nil {
		rev = s.Revision
	}
	_ = saveState(&state{LastCheckedAt: time.Now().Unix(), Revision: rev})
}

// tooOld reports whether the manifest requires a newer CLI. Non-release
// builds (dev) always pass the gate.
func tooOld(minVersion, current string) bool {
	return minVersion != "" && updatecheck.IsReleaseVersion(current) && updatecheck.IsNewer(minVersion, current)
}

// shouldSkip mirrors updatecheck.shouldSkip semantics with metasync's own
// disable knob.
func shouldSkip(version string) bool {
	if os.Getenv("SHOPLAZZA_CLI_NO_META_UPDATE") != "" {
		return true
	}
	if isCIEnv() {
		return true
	}
	return !updatecheck.IsReleaseVersion(version)
}

func isCIEnv() bool {
	for _, k := range []string{"CI", "BUILD_NUMBER", "RUN_ID"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

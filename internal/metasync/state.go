package metasync

import (
	"path/filepath"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/fsx"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
)

const stateFile = "state.json"

// state tracks the last fully processed check (TTL), the last failed
// attempt (backoff), and the origin it was checked against.
type state struct {
	LastCheckedAt int64  `json:"last_checked_at"`
	LastFailureAt int64  `json:"last_failure_at,omitempty"`
	Origin        string `json:"origin,omitempty"`
}

func statePath() (string, error) {
	dir, err := registry.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFile), nil
}

// loadState reports nil for every failure: a missing or corrupt state file
// only costs an extra check.
func loadState() *state {
	path, err := statePath()
	if err != nil {
		return nil
	}
	s, err := fsx.ReadJSON[state](path)
	if err != nil {
		return nil
	}
	return s
}

func saveState(s *state) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	return fsx.WriteJSON(path, s, 0o600)
}

package metasync

import (
	"encoding/json"
	"os"
	"path/filepath"

	"shoplazza-cli-v2/internal/fsx"
	"shoplazza-cli-v2/internal/registry"
)

const stateFile = "state.json"

// state tracks the last refresh attempt. LastCheckedAt is only written after
// a fully processed check, so an interrupted download retries on the next run.
type state struct {
	LastCheckedAt int64  `json:"last_checked_at"`
	Revision      string `json:"revision,omitempty"`
}

func statePath() (string, error) {
	dir, err := registry.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFile), nil
}

func loadState() *state {
	path, err := statePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	return &s
}

func saveState(s *state) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return fsx.WriteFileAtomic(path, data, 0o600)
}

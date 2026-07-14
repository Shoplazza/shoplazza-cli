package schema

import (
	"os"
	"testing"

	"shoplazza-cli-v2/internal/testenv"
)

// TestMain isolates the config dir so tests calling registry.LoadSpec compare
// against the embedded spec, never a real user's downloaded cache.
func TestMain(m *testing.M) {
	cleanup, err := testenv.IsolateConfigDirGlobal()
	if err != nil {
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

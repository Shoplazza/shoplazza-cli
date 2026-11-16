package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

func TestClientID(t *testing.T) {
	testenv.IsolateConfigDir(t)
	cfg, _ := os.UserConfigDir()
	path := filepath.Join(cfg, "shoplazza-cli", clientIDFile)

	first := ClientID()
	data, err := os.ReadFile(path)
	if !uuidRe.MatchString(first) || err != nil || strings.TrimSpace(string(data)) != first {
		t.Fatalf("ClientID() = %q, file = %q (%v)", first, data, err)
	}
	if again := ClientID(); again != first {
		t.Errorf("second call = %q, want the stored %q", again, first)
	}

	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if replaced := ClientID(); replaced == first || !uuidRe.MatchString(replaced) {
		t.Errorf("a non-UUID file must be replaced, got %q", replaced)
	}
}

func TestClientID_Unwritable(t *testing.T) {
	testenv.IsolateConfigDir(t)
	cfg, _ := os.UserConfigDir()
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfg, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfg, 0o755) })
	testenv.SkipIfDirWritable(t, cfg)

	if a, b := ClientID(), ClientID(); !uuidRe.MatchString(a) || a == b {
		t.Errorf("ids %q / %q: want a fresh UUID per call when nothing can be stored", a, b)
	}
}

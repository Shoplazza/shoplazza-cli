package docs

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/fsx"
)

const clientIDFile = "client-id"

var uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ClientID returns this install's anonymous id, a UUID kept in
// <UserConfigDir>/shoplazza-cli/client-id; unsaved, it lasts one process.
// It ignores SHOPLAZZA_CLI_CONFIG_DIR so one machine keeps one id.
func ClientID() string {
	path := clientIDPath()
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if id := strings.TrimSpace(string(data)); uuidRe.MatchString(id) {
				return id
			}
		}
	}
	id := newUUID()
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			_ = fsx.WriteFileAtomic(path, []byte(id+"\n"), 0o600)
		}
	}
	return id
}

func clientIDPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "shoplazza-cli", clientIDFile)
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

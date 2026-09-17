package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/sidecar"
)

// ClientKeys holds per-client HMAC keys loaded from a directory of <client>.key
// files (each a hex-encoded 32-byte key). The filename stem is the client name,
// which is also the CLI profile name the client acts as. Thread-safe; reloadable
// so a newly-provisioned client is picked up without a restart.
type ClientKeys struct {
	dir  string
	mu   sync.RWMutex
	keys map[string][]byte
}

// LoadClientKeys reads every <name>.key in dir.
func LoadClientKeys(dir string) (*ClientKeys, error) {
	ck := &ClientKeys{dir: dir}
	if err := ck.reload(); err != nil {
		return nil, err
	}
	return ck, nil
}

func (ck *ClientKeys) reload() error {
	entries, err := os.ReadDir(ck.dir)
	if err != nil {
		return err
	}
	m := make(map[string][]byte)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".key") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(ck.dir, e.Name()))
		if err != nil {
			continue
		}
		key, err := sidecar.DecodeKey(strings.TrimSpace(string(b)))
		if err != nil {
			continue // skip malformed key files rather than fail the whole set
		}
		m[strings.TrimSuffix(e.Name(), ".key")] = key
	}
	ck.mu.Lock()
	ck.keys = m
	ck.mu.Unlock()
	return nil
}

func (ck *ClientKeys) snapshot() map[string][]byte {
	ck.mu.RLock()
	defer ck.mu.RUnlock()
	out := make(map[string][]byte, len(ck.keys))
	for k, v := range ck.keys {
		out[k] = v
	}
	return out
}

// multiAuth identifies the client whose per-client key validates the signature.
// No match means rejected — there is deliberately no shared/fallback key.
type multiAuth struct{ keys *ClientKeys }

// NewMultiAuth builds an Authenticator over a per-client key directory.
func NewMultiAuth(keys *ClientKeys) Authenticator { return multiAuth{keys: keys} }

func (m multiAuth) Identify(cr sidecar.CanonicalRequest, signature string, now time.Time) (string, error) {
	if name, ok := m.match(cr, signature, now); ok {
		return name, nil
	}
	// A miss may be a client provisioned since the last load; reload once.
	_ = m.keys.reload()
	if name, ok := m.match(cr, signature, now); ok {
		return name, nil
	}
	return "", errors.New("no client key matched the signature")
}

func (m multiAuth) match(cr sidecar.CanonicalRequest, signature string, now time.Time) (string, bool) {
	for name, key := range m.keys.snapshot() {
		if sidecar.Verify(key, cr, signature, now) == nil {
			return name, true
		}
	}
	return "", false
}

// MultiResolver resolves each client's token from its own CLI profile (client
// name == profile name). A client may only reach its profile's store domain, and
// only its own token is ever resolved — a wrong/unknown client is an error, never
// another tenant's credentials.
type MultiResolver struct {
	cfg        core.CliConfig
	configPath string
	authClient *client.Client
}

// NewMultiResolver builds the multi-tenant TenantResolver.
func NewMultiResolver(cfg core.CliConfig, configPath string, authClient *client.Client) *MultiResolver {
	return &MultiResolver{cfg: cfg, configPath: configPath, authClient: authClient}
}

func (r *MultiResolver) AllowHost(client, host string) bool {
	p := r.cfg.FindProfile(client)
	return p != nil && p.StoreDomain == host
}

func (r *MultiResolver) Token(ctx context.Context, clientName, identity string) (string, error) {
	p := r.cfg.FindProfile(clientName)
	if p == nil {
		return "", fmt.Errorf("no profile for client %q", clientName)
	}
	mgr := auth.NewManager(r.cfg, r.configPath, r.authClient)
	switch identity {
	case sidecar.IdentityStore:
		return mgr.AccessTokenReadyForProfile(ctx, r.configPath, *p)
	case sidecar.IdentityPartner:
		return mgr.PartnerToken()
	default:
		return "", fmt.Errorf("no token resolver for identity %q", identity)
	}
}

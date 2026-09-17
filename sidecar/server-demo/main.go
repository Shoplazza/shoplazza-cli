// Command shoplazza-sidecar is the trusted-host half of the credential-isolation
// sidecar: it verifies HMAC-signed proxy requests from a sandboxed shoplazza CLI
// (built with -tags authsidecar), injects the real token, and forwards to the
// Shoplazza API. Run it on a trusted host that holds the real credentials; the
// sandbox never sees them.
//
// Two modes:
//   - single-tenant (default): one shared HMAC key, one profile.
//   - multi-tenant (--keys-dir): a directory of <profile>.key files; the key
//     that verifies a request identifies the client, and only that client's
//     profile token is ever resolved (no cross-tenant fallback).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/build"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/sidecar"
)

func main() {
	var listen, keyFile, keysDir, logFile, profileName string
	flag.StringVar(&listen, "listen", "127.0.0.1:16384", "address to bind")
	flag.StringVar(&keyFile, "key-file", defaultKeyFile(), "single-tenant HMAC key file (hex); generated on first run")
	flag.StringVar(&keysDir, "keys-dir", "", "multi-tenant: directory of <profile>.key files (enables multi-tenant mode)")
	flag.StringVar(&logFile, "log-file", "", "audit log file (default: stderr)")
	flag.StringVar(&profileName, "profile", "", "single-tenant: CLI profile to serve (default: the active profile)")
	flag.Parse()

	// The proxy env is a sandbox-only variable; on the trusted host it must be unset.
	if os.Getenv(sidecar.EnvAuthProxy) != "" {
		fatal("%s must not be set on the sidecar host (it is a sandbox-only variable)", sidecar.EnvAuthProxy)
	}

	configPath, err := core.DefaultConfigPath()
	if err != nil {
		fatal("config path: %v", err)
	}
	cfg, err := core.LoadConfig(configPath)
	if err != nil {
		fatal("load config: %v", err)
	}
	logger := newLogger(logFile)
	authBaseURL := os.Getenv("SHOPLAZZA_CLI_AUTH_BASE_URL")
	if authBaseURL == "" {
		authBaseURL = build.DefaultAuthBaseURL
	}

	var handler http.Handler
	if keysDir != "" {
		handler = buildMultiTenant(cfg, configPath, authBaseURL, keysDir, logger, listen)
	} else {
		handler = buildSingleTenant(cfg, configPath, authBaseURL, keyFile, profileName, logger, listen)
	}

	run(&http.Server{
		Addr:              listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	})
}

func buildSingleTenant(cfg core.CliConfig, configPath, authBaseURL, keyFile, profileName string, logger *log.Logger, listen string) http.Handler {
	key, hexKey, err := loadOrCreateKey(keyFile)
	if err != nil {
		fatal("key: %v", err)
	}
	profile := cfg.Current()
	if profileName != "" {
		profile = cfg.FindProfile(profileName)
	}
	if profile == nil || profile.StoreDomain == "" {
		fatal("no usable profile; run 'shoplazza auth login' on this host first (or pass --profile)")
	}
	resolver := NewAuthResolver(cfg, configPath, client.New(authBaseURL), *profile)

	fmt.Printf("shoplazza credential-isolation sidecar (single-tenant) on http://%s\n", listen)
	fmt.Printf("serving store: %s\n", profile.StoreDomain)
	fmt.Printf("HMAC key prefix: %s (full key in %s, mode 0600)\n\n", hexKey[:8], keyFile)
	fmt.Println("Set in the SANDBOX (CLI must be built with -tags authsidecar):")
	fmt.Printf("  export SHOPLAZZA_CLI_AUTH_PROXY=%q\n", "http://"+listen)
	fmt.Printf("  export SHOPLAZZA_CLI_PROXY_KEY=%q\n", hexKey)
	fmt.Printf("  export SHOPLAZZA_ACCESS_TOKEN=%q\n", sidecar.SentinelStore)
	fmt.Printf("  export SHOPLAZZA_CLI_API_BASE_URL=%q\n\n", "https://"+profile.StoreDomain)

	return New(key, []string{profile.StoreDomain}, resolver, logger, nil)
}

func buildMultiTenant(cfg core.CliConfig, configPath, authBaseURL, keysDir string, logger *log.Logger, listen string) http.Handler {
	keys, err := LoadClientKeys(keysDir)
	if err != nil {
		fatal("keys-dir: %v", err)
	}
	resolver := NewMultiResolver(cfg, configPath, client.New(authBaseURL))

	fmt.Printf("shoplazza credential-isolation sidecar (multi-tenant) on http://%s\n", listen)
	fmt.Printf("client keys: %s/<profile>.key  (each client acts as its own CLI profile)\n\n", keysDir)
	fmt.Println("Provision a client:")
	fmt.Println("  1. on this host: shoplazza auth login   (creates the profile/store)")
	fmt.Println("  2. write a 32-byte hex key to <keys-dir>/<profile>.key (mode 0600)")
	fmt.Println("  3. give that key + this sidecar URL to the sandbox for that client")
	fmt.Println("Sandbox env (per client, CLI built with -tags authsidecar):")
	fmt.Printf("  export SHOPLAZZA_CLI_AUTH_PROXY=%q\n", "http://"+listen)
	fmt.Println("  export SHOPLAZZA_CLI_PROXY_KEY=\"<that client's key>\"")
	fmt.Printf("  export SHOPLAZZA_ACCESS_TOKEN=%q\n", sidecar.SentinelStore)
	fmt.Println("  export SHOPLAZZA_CLI_API_BASE_URL=\"https://<that client's store>\"")

	return NewMultiTenant(NewMultiAuth(keys), resolver, logger, nil)
}

func newLogger(logFile string) *log.Logger {
	if logFile == "" {
		return log.New(os.Stderr, "[sidecar] ", log.LstdFlags)
	}
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fatal("open log file: %v", err)
	}
	return log.New(f, "", log.LstdFlags)
}

// run serves until SIGINT/SIGTERM, then drains in-flight requests.
func run(server *http.Server) {
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			fatal("serve: %v", err)
		}
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}
}

func defaultKeyFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "proxy.key"
	}
	return filepath.Join(home, ".shoplazza-sidecar", "proxy.key")
}

// loadOrCreateKey reads a hex key from path, or generates + persists one (0600).
func loadOrCreateKey(path string) (key []byte, hexKey string, err error) {
	if b, readErr := os.ReadFile(path); readErr == nil {
		hexKey = string(b)
		key, err = sidecar.DecodeKey(hexKey)
		return key, hexKey, err
	}
	raw := make([]byte, sidecar.KeySize)
	if _, err = rand.Read(raw); err != nil {
		return nil, "", err
	}
	hexKey = hex.EncodeToString(raw)
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, "", err
	}
	if err = os.WriteFile(path, []byte(hexKey), 0o600); err != nil {
		return nil, "", err
	}
	return raw, hexKey, nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "shoplazza-sidecar: "+format+"\n", args...)
	os.Exit(1)
}

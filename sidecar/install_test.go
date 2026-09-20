package sidecar

import (
	"encoding/hex"
	"net/http"
	"testing"
)

func TestHostFromURL(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:16384":   "127.0.0.1:16384",
		"https://sidecar.internal": "sidecar.internal",
		"127.0.0.1:16384":          "127.0.0.1:16384",
		"http://host:9/":           "host:9",
	}
	for in, want := range cases {
		if got := hostFromURL(in); got != want {
			t.Errorf("hostFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProxyConfigured(t *testing.T) {
	t.Setenv(EnvAuthProxy, "")
	if ProxyConfigured() {
		t.Error("unset proxy env should report not configured")
	}
	t.Setenv(EnvAuthProxy, "http://127.0.0.1:16384")
	if !ProxyConfigured() {
		t.Error("set proxy env should report configured")
	}
}

func TestInstallFromEnv(t *testing.T) {
	orig := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = orig })

	// No env → no-op, transport untouched.
	t.Setenv(EnvAuthProxy, "")
	if err := InstallFromEnv(); err != nil {
		t.Fatalf("no-op install: %v", err)
	}
	if http.DefaultTransport != orig {
		t.Error("with no proxy env, DefaultTransport must be untouched")
	}

	// Bad key → error, transport still untouched.
	t.Setenv(EnvAuthProxy, "http://127.0.0.1:16384")
	t.Setenv(EnvProxyKey, "not-hex")
	if err := InstallFromEnv(); err == nil {
		t.Error("a bad key must error")
	}
	if http.DefaultTransport != orig {
		t.Error("a failed install must not swap DefaultTransport")
	}

	// Valid key → interceptor installed.
	t.Setenv(EnvProxyKey, hex.EncodeToString(testKey()))
	if err := InstallFromEnv(); err != nil {
		t.Fatalf("valid install: %v", err)
	}
	if _, ok := http.DefaultTransport.(*Interceptor); !ok {
		t.Errorf("DefaultTransport should be the sidecar interceptor, got %T", http.DefaultTransport)
	}
}

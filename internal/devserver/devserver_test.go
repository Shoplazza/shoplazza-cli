package devserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestListenAndShutdown(t *testing.T) {
	s := New()
	p, err := s.Listen(3457)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if p < 3457 {
		t.Fatalf("port = %d, want >= 3457", p)
	}
	if s.Port() != p {
		t.Fatalf("Port() = %d, want %d", s.Port(), p)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("pong")) })
	s.Serve(mux)
	defer s.Shutdown(context.Background())

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", p))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "pong" {
		t.Fatalf("body = %q", b)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	// second shutdown must not panic or error
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}

func TestTwoServersGetDifferentPorts(t *testing.T) {
	s1 := New()
	p1, err := s1.Listen(3457)
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Shutdown(context.Background())
	s2 := New()
	p2, err := s2.Listen(3457)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Shutdown(context.Background())
	if p1 == p2 {
		t.Fatalf("both servers got the same port %d", p1)
	}
}

// TestShutdownReleasesListenerWithoutServe checks that Shutdown after Listen but
// before Serve closes the bound listener and releases the port.
func TestShutdownReleasesListenerWithoutServe(t *testing.T) {
	s := New()
	p, err := s.Listen(3557)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	// Shutdown WITHOUT ever calling Serve — should close the listener.
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown (no serve): %v", err)
	}
	// The port should now be free — a new server can bind the same port.
	s2 := New()
	p2, err := s2.Listen(p)
	if err != nil {
		t.Fatalf("expected port %d to be free after Shutdown, got error: %v", p, err)
	}
	defer s2.Shutdown(context.Background())
	if p2 != p {
		t.Fatalf("expected port %d, got %d", p, p2)
	}
	// Second shutdown on the already-shut server must be a no-op.
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}

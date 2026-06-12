// Package devserver is a thin, stdlib-level dev server core: port allocation
// from a base port, http.Server lifecycle, and graceful shutdown. Listen and
// Serve are split so the caller can learn the bound port before building the
// handler.
package devserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
)

type Server struct {
	srv  *http.Server
	ln   net.Listener
	port int
}

// New returns an empty Server; call Listen then Serve.
func New() *Server { return &Server{} }

// Listen binds the first free port >= startPort (127.0.0.1), trying ~100 ports,
// and returns it. It does not begin serving; call Serve once the handler is ready.
func (s *Server) Listen(startPort int) (int, error) {
	for p := startPort; p < startPort+100; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue
		}
		s.ln, s.port = ln, p
		return p, nil
	}
	return 0, fmt.Errorf("no free port in [%d,%d)", startPort, startPort+100)
}

// Serve starts serving h in a goroutine on the listener bound by Listen.
func (s *Server) Serve(h http.Handler) {
	srv := &http.Server{Handler: h}
	s.srv = srv
	go func() { _ = srv.Serve(s.ln) }()
}

// Port returns the bound port.
func (s *Server) Port() int { return s.port }

// Shutdown gracefully stops the server. Safe to call more than once.
// If Listen ran but Serve did not, the bound listener is closed to release the port.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		if s.ln != nil {
			err := s.ln.Close()
			s.ln = nil
			return err
		}
		return nil
	}
	err := s.srv.Shutdown(ctx)
	s.srv = nil
	s.ln = nil // Shutdown already closed the listener; nil it to prevent double-close.
	return err
}

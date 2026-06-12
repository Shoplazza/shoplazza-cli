package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// LiveReloadServer implements the livereload v6 protocol over WebSocket.
// Endpoints:
//
//	GET  /livereload.js  → embedded livereload v6 client JS
//	GET  /livereload     → WebSocket upgrade
type LiveReloadServer struct {
	requestedPort int
	srv           *http.Server
	ln            net.Listener
	mu            sync.Mutex
	conns         map[*websocket.Conn]struct{}
	closed        bool
}

// NewLiveReloadServer returns a server; pass 0 for an ephemeral port.
func NewLiveReloadServer(port int) *LiveReloadServer {
	return &LiveReloadServer{requestedPort: port, conns: map[*websocket.Conn]struct{}{}}
}

// Port returns the actual bound port (after Start).
func (s *LiveReloadServer) Port() int {
	if s.ln == nil {
		return s.requestedPort
	}
	if addr, ok := s.ln.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return s.requestedPort
}

// Addr returns host:port.
func (s *LiveReloadServer) Addr() string {
	if s.ln == nil {
		return fmt.Sprintf("127.0.0.1:%d", s.requestedPort)
	}
	return s.ln.Addr().String()
}

// Start binds the listener and serves in a goroutine. Returns immediately.
// Port-bind failure is returned synchronously.
func (s *LiveReloadServer) Start(ctx context.Context) error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.requestedPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("livereload bind: %w", err)
	}
	s.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/livereload.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write(liveReloadClientJS)
	})
	mux.HandleFunc("/livereload", s.handleWS)
	s.srv = &http.Server{Handler: mux}
	go func() {
		_ = s.srv.Serve(ln)
	}()
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	return nil
}

func (s *LiveReloadServer) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Origin is deliberately unchecked: the client JS is served from the
		// store domain, so cross-origin connections are the normal case. The
		// listener is bound to 127.0.0.1 and only broadcasts "reload" commands.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// 1. Read client hello with a short deadline.
	helloCtx, helloCancel := context.WithTimeout(r.Context(), 10*time.Second)
	_, _, err = c.Read(helloCtx)
	helloCancel()
	if err != nil {
		return
	}

	// 2. Send server hello before registering for broadcasts.
	helloPayload := map[string]any{
		"command":    "hello",
		"protocols":  []string{"http://livereload.com/protocols/official-7"},
		"serverName": "shoplazza-cli/themes-serve",
	}
	b, _ := json.Marshal(helloPayload)
	writeCtx, writeCancel := context.WithTimeout(r.Context(), 2*time.Second)
	err = c.Write(writeCtx, websocket.MessageText, b)
	writeCancel()
	if err != nil {
		return
	}

	// 3. Only now is the client eligible for broadcasts.
	s.mu.Lock()
	s.conns[c] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
	}()

	// 4. Block until the client disconnects.
	for {
		if _, _, err := c.Read(r.Context()); err != nil {
			return
		}
	}
}

// Refresh broadcasts a reload command to all connected clients. Writes fan out
// concurrently (one goroutine per conn, 2s write timeout each) so a stalled
// browser can't block the broadcast; conns whose write fails are dropped and closed.
func (s *LiveReloadServer) Refresh(relPath string) error {
	payload := map[string]any{
		"command": "reload",
		"path":    relPath,
		"liveCSS": false,
		"liveImg": false,
	}
	b, _ := json.Marshal(payload)
	s.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, c := range conns {
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := c.Write(ctx, websocket.MessageText, b); err != nil {
				// Dead/stuck client: deregister and close so future broadcasts
				// don't wait on it (handleWS's deferred delete is idempotent).
				s.mu.Lock()
				delete(s.conns, c)
				s.mu.Unlock()
				_ = c.Close(websocket.StatusGoingAway, "write failed")
			}
		}(c)
	}
	wg.Wait()
	return nil
}

// Close stops the server and disconnects all clients. Idempotent.
func (s *LiveReloadServer) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	conns := make([]*websocket.Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	for _, c := range conns {
		_ = c.Close(websocket.StatusGoingAway, "server shutdown")
	}
	if s.srv != nil {
		return s.srv.Close()
	}
	return nil
}

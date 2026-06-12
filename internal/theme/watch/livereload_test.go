package watch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestLiveReloadServer_ServesLiveReloadJS(t *testing.T) {
	srv := NewLiveReloadServer(0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start err: %v", err)
	}
	defer srv.Close()

	url := "http://" + srv.Addr() + "/livereload.js"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET /livereload.js: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want javascript", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) < 10*1024 {
		t.Errorf("body too small to be livereload-js: %d bytes", len(body))
	}
	lower := strings.ToLower(string(body))
	if !strings.Contains(lower, "livereload") {
		t.Errorf("body does not look like livereload-js (no 'livereload' marker found)")
	}
}

func TestLiveReloadServer_WSHandshakeAndReloadBroadcast(t *testing.T) {
	srv := NewLiveReloadServer(0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start err: %v", err)
	}
	defer srv.Close()

	url := "ws://" + srv.Addr() + "/livereload"
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("WS dial err: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	hello := `{"command":"hello","protocols":["http://livereload.com/protocols/official-7"]}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(hello)); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	_, srvHello, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read server hello: %v", err)
	}
	var helloFrame map[string]any
	_ = json.Unmarshal(srvHello, &helloFrame)
	if helloFrame["command"] != "hello" {
		t.Fatalf("expected hello frame, got %v", helloFrame)
	}

	// Give the server a beat to register the conn in its broadcast set.
	time.Sleep(50 * time.Millisecond)

	if err := srv.Refresh("layout/theme.liquid"); err != nil {
		t.Fatalf("Refresh err: %v", err)
	}
	_, reloadMsg, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read reload: %v", err)
	}
	var reloadFrame map[string]any
	_ = json.Unmarshal(reloadMsg, &reloadFrame)
	if reloadFrame["command"] != "reload" {
		t.Errorf("expected reload command, got %v", reloadFrame)
	}
	if reloadFrame["path"] != "layout/theme.liquid" {
		t.Errorf("expected path layout/theme.liquid, got %v", reloadFrame["path"])
	}
	if reloadFrame["liveCSS"] != false {
		t.Errorf("liveCSS must be false (Liquid render state mismatch risk)")
	}
	if reloadFrame["liveImg"] != false {
		t.Errorf("liveImg must be false")
	}
}

func TestLiveReloadServer_PortConflictHardFails(t *testing.T) {
	srv1 := NewLiveReloadServer(0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv1.Start(ctx); err != nil {
		t.Fatalf("srv1 start: %v", err)
	}
	defer srv1.Close()
	port := srv1.Port()

	srv2 := NewLiveReloadServer(port)
	err := srv2.Start(ctx)
	if err == nil {
		srv2.Close()
		t.Fatalf("expected bind error on port conflict")
	}
}

// TestLiveReloadServer_RefreshSurvivesDeadConn: a client that vanished without
// a clean close must not stall or fail the broadcast for healthy clients.
func TestLiveReloadServer_RefreshSurvivesDeadConn(t *testing.T) {
	srv := NewLiveReloadServer(0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start err: %v", err)
	}
	defer srv.Close()

	dial := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.Dial(ctx, "ws://"+srv.Addr()+"/livereload", nil)
		if err != nil {
			t.Fatalf("WS dial err: %v", err)
		}
		hello := `{"command":"hello","protocols":["http://livereload.com/protocols/official-7"]}`
		if err := conn.Write(ctx, websocket.MessageText, []byte(hello)); err != nil {
			t.Fatalf("write hello: %v", err)
		}
		if _, _, err := conn.Read(ctx); err != nil {
			t.Fatalf("read server hello: %v", err)
		}
		return conn
	}

	dead := dial()
	healthy := dial()
	defer healthy.Close(websocket.StatusNormalClosure, "")
	time.Sleep(50 * time.Millisecond) // let both register in the broadcast set

	// Abrupt client death (no close frame).
	dead.CloseNow()

	start := time.Now()
	if err := srv.Refresh("assets/app.css"); err != nil {
		t.Fatalf("Refresh err: %v", err)
	}
	// Concurrent fan-out: the broadcast completes within roughly one
	// write-timeout, never N-serial timeouts.
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Refresh stalled %v on a dead conn (must be bounded by one 2s timeout)", elapsed)
	}

	readCtx, readCancel := context.WithTimeout(ctx, 3*time.Second)
	defer readCancel()
	_, msg, err := healthy.Read(readCtx)
	if err != nil {
		t.Fatalf("healthy client did not receive the reload: %v", err)
	}
	var frame map[string]any
	_ = json.Unmarshal(msg, &frame)
	if frame["command"] != "reload" {
		t.Errorf("expected reload frame, got %v", frame)
	}
}

func TestLiveReloadServer_NoSubsidiaryFilesInCwd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	beforeSet := make(map[string]struct{}, len(before))
	for _, e := range before {
		beforeSet[e.Name()] = struct{}{}
	}

	srv := NewLiveReloadServer(0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	_ = srv.Refresh("any.css")
	time.Sleep(50 * time.Millisecond)

	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range after {
		if _, existed := beforeSet[e.Name()]; !existed {
			t.Errorf("LiveReloadServer created a file/dir in CWD: %s", e.Name())
		}
	}
}

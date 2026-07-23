package tunnel

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// Ngrok is the fallback tunnel strategy. It is non-interactive: the authtoken
// (required) and reserved domain (optional) come from the struct fields, each
// falling back to the corresponding environment variable when empty. `app dev`
// populates Token from its --ngrok-authtoken flag (or the project .env). If no
// authtoken resolves, or the ngrok CLI isn't on PATH, the strategy is
// unavailable and Start returns an error.
type Ngrok struct {
	// Token is the ngrok authtoken; empty → fall back to $NGROK_AUTHTOKEN.
	Token string
	// Domain is an optional ngrok reserved domain; empty → fall back to $NGROK_DOMAIN.
	Domain string
	// Progress, when non-nil, reports the tunnel-startup step (a live elapsed
	// timer) to the user. nil disables reporting.
	Progress *output.Progress
}

// Name implements Strategy.
func (*Ngrok) Name() string { return "ngrok" }

// authToken resolves the authtoken: the Token field wins, else $NGROK_AUTHTOKEN.
func (n *Ngrok) authToken() string {
	if n.Token != "" {
		return n.Token
	}
	return os.Getenv("NGROK_AUTHTOKEN")
}

// reservedDomain resolves the optional domain: the Domain field wins, else $NGROK_DOMAIN.
func (n *Ngrok) reservedDomain() string {
	if n.Domain != "" {
		return n.Domain
	}
	return os.Getenv("NGROK_DOMAIN")
}

// Start implements Strategy. It runs the ngrok agent as a subprocess and parses
// its JSON log (`--log=stdout --log-format=json`) for the public URL, keying off
// the "started tunnel" record. Verified against ngrok v3.39.6.
func (n *Ngrok) Start(ctx context.Context, port int) (*Tunnel, error) {
	token := n.authToken()
	if token == "" {
		return nil, fmt.Errorf("NGROK_AUTHTOKEN is not set; ngrok fallback unavailable")
	}
	bin, err := exec.LookPath("ngrok")
	if err != nil {
		return nil, fmt.Errorf("ngrok CLI not found on PATH; ngrok fallback unavailable")
	}

	args := []string{"http", strconv.Itoa(port), "--log=stdout", "--log-format=json"}
	if domain := n.reservedDomain(); domain != "" {
		args = append(args, "--domain="+domain)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "NGROK_AUTHTOKEN="+token)

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	// Time the startup: from launch until ngrok reports its public URL.
	step := n.Progress.Begin("Starting ngrok tunnel")

	if err := cmd.Start(); err != nil {
		step.Fail()
		_ = pw.Close()
		return nil, fmt.Errorf("starting ngrok: %w", err)
	}

	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
		_ = pw.Close()
	}()

	urlCh := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			if u := parseNgrokURL(sc.Text()); u != "" {
				urlCh <- u
				_, _ = io.Copy(io.Discard, pr)
				return
			}
		}
	}()

	kill := func() error { return gracefulKill(cmd.Process) }

	select {
	case u := <-urlCh:
		step.Done()
		return &Tunnel{URL: u, Close: kill}, nil
	case err := <-exited:
		step.Fail()
		return nil, fmt.Errorf("ngrok exited before reporting a tunnel URL: %v", err)
	case <-time.After(startupTimeout):
		step.Fail()
		_ = kill()
		return nil, fmt.Errorf("ngrok did not report a tunnel URL within %s", startupTimeout)
	}
}

// parseNgrokURL extracts the public https URL from a single ngrok JSON log line,
// keying off the "url" field of the "started tunnel" record. Returns "" for
// unrelated lines. Keying off the JSON field is domain-agnostic (handles .app
// and .dev).
func parseNgrokURL(line string) string {
	var rec struct {
		Msg string `json:"msg"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		return ""
	}
	if rec.Msg != "started tunnel" {
		return ""
	}
	return rec.URL
}

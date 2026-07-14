package appcmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/app"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/devserver"
	"shoplazza-cli-v2/internal/fsx"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/tunnel"
)

const (
	devBasePort     = 3457
	devAppPath      = "/auth"
	devCallbackPath = "/auth/callback"
)

func newCmdDev(f *cmdutil.Factory) *cobra.Command {
	var (
		path           string
		debug          bool
		ngrokAuthToken string
	)
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Run the app in development mode",
		Args:  cobra.NoArgs,
		// Long-running local dev server.
		Annotations: map[string]string{cmdutil.AnnotationNotScannable: "true"},
		PreRunE:     func(cmd *cobra.Command, _ []string) error { return requireLogin(cmd.Context(), f) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			p, err := openProject(path)
			if err != nil {
				return err
			}

			cfg, ex := activeAppConfig(p)
			if ex != nil {
				return ex
			}

			d, err := dashboardClient(ctx, f)
			if err != nil {
				return err
			}
			cid := cfg.ClientID
			pid, ex := ensurePartnerID(ctx, d, cfg)
			if ex != nil {
				return ex
			}

			targetStore, err := resolveTargetStore(f.Config.CurrentStoreDomain())
			if err != nil {
				return err
			}
			store, err := storeClient(ctx, f, targetStore)
			if err != nil {
				return err
			}
			// version/generate needs the numeric store_id; surface a resolution
			// failure instead of swallowing it (empty store_id → backend 500). See deploy.go.
			storeID, sErr := resolveStoreID(ctx, f, targetStore)
			if sErr != nil {
				return sErr
			}

			// GetAppConfig yields client_secret + partner_id. The secret feeds
			// BOTH the app-token chain (partnerOpenapiClient) and the OAuth
			// handler's token exchange. It is never persisted.
			appCfg, err := d.GetAppConfig(ctx, pid, cid)
			if err != nil {
				return apiError(err)
			}

			// App-info header (stderr, so the stdout JSON envelope stays clean):
			// name/client-id/partner-id are all in hand before the tunnel comes up.
			fmt.Fprintf(cmd.ErrOrStderr(), "App:        %s\nClient ID:  %s\nPartner ID: %s\n\n",
				appCfg.Name, cid, pid)

			// Progress reporter for the tunnel/binary-download steps. Writes to
			// stderr; on a TTY it shows a live elapsed timer per step, otherwise
			// one static line per step.
			prog := output.NewProgress(cmd.ErrOrStderr())
			partnerClient, err := partnerOpenapiClient(ctx, f, cid, appCfg.ClientSecret, appCfg.PartnerID, f.AuthClient.BaseURL)
			if err != nil {
				return err
			}

			locals, scanErr := app.ScanLocalExtensions(p.Root)
			if scanErr != nil {
				return scanErr
			}

			// Load <root>/.env so NGROK_AUTHTOKEN / NGROK_DOMAIN are available to
			// the ngrok tunnel fallback. Best-effort: never overrides real env, and
			// a missing file is not an error.
			loadDotEnv(filepath.Join(p.Root, ".env"))

			// Resolve the ngrok authtoken: --ngrok-authtoken wins, else the
			// env/.env value. When the flag is given, persist it to <root>/.env
			// (key NGROK_AUTHTOKEN) so later runs reuse it without re-passing the
			// flag (v1 parity: v1 prompts then writes .env). It is only consumed if
			// the primary cloudflared tunnel fails and the ngrok fallback runs.
			ngrokToken := ngrokAuthToken
			if ngrokToken == "" {
				ngrokToken = os.Getenv("NGROK_AUTHTOKEN")
			}
			if ngrokAuthToken != "" {
				envPath := filepath.Join(p.Root, ".env")
				if werr := upsertDotEnv(envPath, "NGROK_AUTHTOKEN", ngrokAuthToken); werr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not save ngrok authtoken to %s: %v\n", envPath, werr)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "saved ngrok authtoken to %s — keep this file out of version control\n", envPath)
				}
			}

			// Ordering: the OAuth redirect_uri needs the tunnel URL, which needs the
			// port. So: bind the port FIRST, open the tunnel, build the handler with
			// the tunnel-derived redirect_uri, THEN serve.
			srv := devserver.New()
			port, lErr := srv.Listen(devBasePort)
			if lErr != nil {
				return output.ErrInternal("failed to bind dev server port: %v", lErr)
			}
			// From here on the listener is live: shut it down on EVERY exit path.
			// On the success path this defer runs only after the signal wait below
			// ends, so it never kills a running dev server early.
			defer func() {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Shutdown(shutdownCtx); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "dev server shutdown: %v\n", err)
				}
			}()

			// cloudflared primary, ngrok fallback (mirrors tunnel.Default()), but
			// with the resolved authtoken injected so ngrok can run without an
			// ambient env var.
			tun, tErr := tunnel.Open(ctx, port, &tunnel.Cloudflared{Progress: prog}, &tunnel.Ngrok{Token: ngrokToken, Progress: prog})
			if tErr != nil {
				return tErr
			}
			// Same discipline for the tunnel process.
			defer func() {
				if tun.Close != nil {
					_ = tun.Close()
				}
			}()

			// Config.Scopes is already the v1-format space-separated string the
			// OAuth handler forwards verbatim (template default:
			// "read_customer write_cart_transform").
			scopes := cfg.Scopes
			handler := app.NewOAuthHandler(app.OAuthConfig{
				ClientID:     cid,
				ClientSecret: appCfg.ClientSecret,
				RedirectURI:  tun.URL + devCallbackPath,
				Scopes:       scopes,
				InstallPath:  devAppPath,
				CallbackPath: devCallbackPath,
			})
			srv.Serve(handler)

			res, dErr := app.DevReport(ctx, app.DeployDeps{
				Dashboard:   d,
				Store:       store,
				Partner:     partnerClient,
				HTTPClient:  &http.Client{Timeout: 60 * time.Second},
				PartnerID:   pid,
				ClientID:    cid,
				StoreID:     storeID,
				ProjectRoot: p.Root,
				Locals:      locals,
				IsDev:       true,
				Progress:    prog,
				BuildArtifact: func(ctx context.Context, l app.LocalExt) (string, *output.ExitError) {
					return app.BuildArtifactFor(ctx, p.Root, l, debug)
				},
			}, tun.URL, devAppPath, devCallbackPath)
			if dErr != nil {
				return dErr
			}

			if err := output.PrintAPISuccess(cmd.OutOrStdout(), res, cmdutil.GetFormat(cmd), ""); err != nil {
				return err
			}
			// Tell the developer how to actually exercise the tunnel: the App URL and
			// Redirect URL must be registered on the Partner dashboard before the
			// install link can complete the OAuth handshake.
			fmt.Fprintf(cmd.ErrOrStderr(),
				"\nNext steps:\n"+
					"  1. In the Partner dashboard, configure this app's App URL and Redirect URL:\n"+
					"       App URL:      %s\n"+
					"       Redirect URL: %s\n"+
					"  2. Then open the install URL in your browser to install the app on your store:\n"+
					"       %s\n",
				res.AppURL, res.RedirectURL, res.InstallURL)
			fmt.Fprintf(cmd.ErrOrStderr(), "\nDev server running on port %d. Press Ctrl+C to stop.\n", port)

			// Block until SIGINT/SIGTERM; the deferred cleanup above then closes
			// the tunnel and gracefully shuts the dev server down.
			waitCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			<-waitCtx.Done()
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", ".", "Project root")
	cmd.Flags().BoolVar(&debug, "debug", false, "Build extensions in debug mode")
	cmd.Flags().StringVar(&ngrokAuthToken, "ngrok-authtoken", "",
		"Your personal ngrok authtoken (get it at https://dashboard.ngrok.com/get-started/your-authtoken). "+
			"Saved to <project>/.env as NGROK_AUTHTOKEN and reused on later runs, so you only pass it once. "+
			"Used ONLY as a fallback when the primary cloudflared tunnel can't start.")
	return cmd
}

// upsertDotEnv sets key=value in the .env file at path, preserving every other
// line (other keys, comments, blanks). An existing key is replaced in place;
// otherwise the pair is appended. A missing file is created with 0600 perms
// (the file holds a secret). The value is written verbatim to match
// loadDotEnv's parser, atomically — a crash mid-write must not truncate the
// user's .env. Mirrors v1's writeEnvWithDotenv.
func upsertDotEnv(path, key, value string) error {
	var lines []string
	replaced := false
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if k, _, ok := strings.Cut(trimmed, "="); ok && !strings.HasPrefix(trimmed, "#") && strings.TrimSpace(k) == key {
				lines = append(lines, key+"="+value)
				replaced = true
			} else {
				lines = append(lines, line)
			}
		}
	case errors.Is(err, os.ErrNotExist):
		// create below
	default:
		return err
	}
	if !replaced {
		// Trim trailing blank lines so appends don't accumulate gaps.
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, key+"="+value)
	}
	return fsx.WriteFileAtomic(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

// loadDotEnv reads simple KEY=VALUE lines from path into the process env,
// setting only keys not already present (never overriding the real env). A
// missing file is not an error. Blank lines and '#' comments are skipped, and
// surrounding single/double quotes on the value are stripped.
func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return // best-effort: absent .env is fine
	}
	defer file.Close()

	sc := bufio.NewScanner(file)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		if k == "" {
			continue
		}
		if _, present := os.LookupEnv(k); !present {
			_ = os.Setenv(k, v)
		}
	}
}

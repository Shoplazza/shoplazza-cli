package checkout_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	checkout "github.com/Shoplazza/shoplazza-cli/cmd/checkout"
	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/core"
)

// writeCheckoutVersionList writes a checkout /version/list response mapping one
// version string to its server id, matching the backend shape
// {data:{extensions:[{version,id}]}} that resolveCheckoutVersionID reads. deploy
// and preview call /version/list to resolve --version → id before acting.
func writeCheckoutVersionList(w http.ResponseWriter, version, id string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{"extensions": []map[string]any{{"version": version, "id": id}}},
	})
}

func tempCheckoutFactory(t *testing.T, srvURL string) (*cmdutil.Factory, *bytes.Buffer) {
	t.Helper()
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "test-token") // RequireAuth fast-path (CI bypass)
	t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", srvURL)   // gate still needs an explicit store target
	out := &bytes.Buffer{}
	cl := client.New(srvURL)
	cl.SetBearerToken("test-token")
	return &cmdutil.Factory{
		IOStreams:  cmdutil.IOStreams{In: strings.NewReader(""), Out: out, ErrOut: io.Discard},
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
		Config: core.CliConfig{
			CurrentProfile: "test",
			Profiles:       []core.ProfileConfig{{Name: "test", StoreDomain: "test-store.myshoplaza.com"}},
		},
		Client:     cl,
		AuthClient: client.New(srvURL),
	}, out
}

func execCheckout(t *testing.T, f *cmdutil.Factory, out *bytes.Buffer, args ...string) error {
	t.Helper()
	cmd := checkout.NewCmdCheckout(f)
	cmd.PersistentFlags().String("format", "json", "") // root provides this in real use
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	return cmd.Execute()
}

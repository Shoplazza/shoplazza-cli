package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/metasync"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

	"github.com/spf13/cobra"
)

// NewCmdDoctor creates the doctor command group.
func NewCmdDoctor(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "doctor",
		Short:       "Run diagnostic checks",
		Hidden:      true,
		Annotations: map[string]string{cmdutil.AnnotationAuthFree: "true"},
	}

	cmd.AddCommand(
		newCmdCheck(f),
	)

	return cmd
}

// checkResult is one diagnostic check's outcome: status is ok | warn | fail.
type checkResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func newCmdCheck(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check current CLI health",
		RunE: func(cmd *cobra.Command, _ []string) error {
			checks := runChecks(f)
			ok := true
			for _, c := range checks {
				if c.Status != "ok" {
					ok = false
				}
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"ok":     ok,
				"checks": checks,
			}, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
}

// runChecks runs the local-disk health checks — no network, no keychain. Any
// non-ok result fails the whole verdict, so a check must report real breakage.
func runChecks(f *cmdutil.Factory) []checkResult {
	return []checkResult{
		checkConfigVersion(f),
		checkAuthLocksDirs(f),
		checkMetadata(),
	}
}

// checkMetadata reports the active metadata source and last refresh check.
func checkMetadata() checkResult {
	st := metasync.CurrentStatus()
	last := "never"
	if !st.LastCheckedAt.IsZero() {
		last = st.LastCheckedAt.UTC().Format(time.RFC3339)
	}
	msg := fmt.Sprintf("source=%s revision=%s last_check=%s", st.Source, st.Revision, last)
	if st.Revision == "" {
		return checkResult{"metadata", "warn", "active spec has no generated_at — " + msg}
	}
	return checkResult{"metadata", "ok", msg}
}

func configExists(f *cmdutil.Factory) bool {
	_, err := os.Stat(f.ConfigPath)
	return err == nil
}

// checkConfigVersion verifies config.json completed the v1->v2 migration. A
// fresh install has nothing to migrate, so it passes.
func checkConfigVersion(f *cmdutil.Factory) checkResult {
	if !configExists(f) {
		return checkResult{"config_version", "ok", "no config file yet — run 'shoplazza auth login' to get started"}
	}
	if f.Config.ConfigVersion == 2 {
		return checkResult{"config_version", "ok", "config is v2"}
	}
	return checkResult{"config_version", "warn",
		fmt.Sprintf("config is not on v2 (configVersion=%d) — run any command to trigger migration", f.Config.ConfigVersion)}
}

// checkAuthLocksDirs verifies the CLI can write auth/ (credentials) and locks/
// (taken on every config.json update). Both are created on first use, so a
// missing one only matters when the config dir itself refuses writes.
func checkAuthLocksDirs(f *cmdutil.Factory) checkResult {
	if !configExists(f) {
		return checkResult{"auth_locks_dirs", "ok", "no config yet — directories are created on first use"}
	}
	configDir := filepath.Dir(f.ConfigPath)

	var blocked []string
	for _, d := range []struct{ name, path, breaks string }{
		{"auth/", internalauth.AuthDir(f.ConfigPath), "logins cannot store credentials"},
		{"locks/", core.LocksDir(f.ConfigPath), "every config.json update fails"},
	} {
		probe := d.path
		if !isDir(probe) {
			probe = configDir // not created yet — can it still be?
		}
		if !isWritable(probe) {
			blocked = append(blocked, d.name+" — "+d.breaks)
		}
	}
	if len(blocked) > 0 {
		return checkResult{"auth_locks_dirs", "fail",
			fmt.Sprintf("not writable under %s: %s", configDir, strings.Join(blocked, "; "))}
	}
	return checkResult{"auth_locks_dirs", "ok", "auth/ and locks/ are writable"}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isWritable probes dir by creating and removing a throwaway file.
func isWritable(dir string) bool {
	probe := filepath.Join(dir, ".doctor-write-probe")
	f, err := os.Create(probe)
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return true
}

package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
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

// runChecks runs the v2 config-health checks: configVersion, the auth/+locks
// directory layout, and leftover v1 migration residue. All are local-disk
// reads only — no network, no keychain.
func runChecks(f *cmdutil.Factory) []checkResult {
	return []checkResult{
		checkConfigVersion(f),
		checkAuthLocksDirs(f),
		checkMigrationResidue(f),
	}
}

func configExists(f *cmdutil.Factory) bool {
	_, err := os.Stat(f.ConfigPath)
	return err == nil
}

// checkConfigVersion verifies config.json has completed the v1->v2 migration.
// A fresh install (no config file yet) is a pass, not a warning — there's
// nothing to migrate.
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

// checkAuthLocksDirs verifies the auth/ and locks/ directories next to
// config.json exist and that locks/ is writable (config updates lock there).
// Both directories are created lazily on first use, so their absence on a
// fresh install is expected, not an error.
func checkAuthLocksDirs(f *cmdutil.Factory) checkResult {
	if !configExists(f) {
		return checkResult{"auth_locks_dirs", "ok", "no config yet — directories are created on first use"}
	}
	authDir := internalauth.AuthDir(f.ConfigPath)
	locksDir := core.LocksDir(f.ConfigPath)

	var missing []string
	if !isDir(authDir) {
		missing = append(missing, "auth/")
	}
	if !isDir(locksDir) {
		missing = append(missing, "locks/")
	}
	if len(missing) > 0 {
		return checkResult{"auth_locks_dirs", "warn",
			fmt.Sprintf("missing directories: %s (created lazily on first use)", strings.Join(missing, ", "))}
	}
	if !isWritable(locksDir) {
		return checkResult{"auth_locks_dirs", "fail", "locks/ directory is not writable — commands that update config.json will fail"}
	}
	return checkResult{"auth_locks_dirs", "ok", "auth/ and locks/ directories present; locks/ is writable"}
}

// checkMigrationResidue flags an incomplete migration or v1 leftovers. A
// config.json.v1.bak backup is expected and purely informational (not
// flagged); a leftover v1 auth.json next to a v2 config is residue worth
// cleaning up.
func checkMigrationResidue(f *cmdutil.Factory) checkResult {
	if !configExists(f) {
		return checkResult{"migration_residue", "ok", "no config yet"}
	}
	if f.Config.ConfigVersion < 2 {
		return checkResult{"migration_residue", "warn", "config is still pre-v2 — migration has not completed"}
	}
	legacyAuth := filepath.Join(filepath.Dir(f.ConfigPath), "auth.json")
	if _, err := os.Stat(legacyAuth); err == nil {
		return checkResult{"migration_residue", "warn",
			"leftover v1 auth.json found next to a v2 config — safe to remove once v2 is confirmed working"}
	}
	return checkResult{"migration_residue", "ok", "no migration residue found"}
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

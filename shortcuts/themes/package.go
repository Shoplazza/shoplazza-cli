package themes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// packageShortcut is the `themes package` workflow: reads
// config/settings_schema.json from the cwd to derive <theme_name,
// theme_version>, then zips the cwd into "<name>-<version>.zip" via
// internal/theme/pack.Pack. The operation is entirely local.
//
// .themeignore handling is delegated to pack.PackOptions: the default
// auto-detects .themeignore at srcDir, while --no-ignore force-disables it.
// Filename fallbacks (mirroring v1) are filepath.Base(cwd) for a missing
// theme_name and "unknown" for a missing theme_version. A missing or
// malformed config/settings_schema.json is a validation error.
var packageShortcut = common.Shortcut{
	Service: "themes",
	Command: "package",
	Use:     "package",
	Short:   "Package the current theme directory into a zip",
	Long:    "Package the current theme directory into a '<name>-<version>.zip' artifact locally (name/version read from config/settings_schema.json); honors .themeignore unless --no-ignore is set.",
	Example: `  # Preview which files would be packaged, without writing the zip
  shoplazza themes package --dry-run

  # Package the current theme directory into a zip
  shoplazza themes package`,
	// Purely local (reads cwd, writes a zip): runs without login and reports a
	// local artifact rather than an API response (no {ok,data} envelope).
	// Writes the local filesystem, so blind scans skip it.
	AuthFree:     true,
	Local:        true,
	NotScannable: true,
	Flags: []common.Flag{
		{
			Name:        "no-ignore",
			Type:        common.FlagBool,
			Default:     false,
			Description: "Ignore .themeignore",
		},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		noIgnore := in.Flags.GetBool("no-ignore")
		cwd, err := os.Getwd()
		if err != nil {
			return common.ExecResult{}, theme.ErrLocalIO("getwd", err)
		}
		name, version, err := readThemeInfo(cwd)
		if err != nil {
			return common.ExecResult{}, err
		}
		zipName := themeZipName(name, version)
		out := filepath.Join(cwd, zipName)
		// ExecInput has no Stderr field yet, so write directly to os.Stderr.
		fmt.Fprintf(os.Stderr, "[package] packaging into %s\n", zipName)

		opts := pack.PackOptions{}
		if noIgnore {
			// "/dev/null" is pack.PackOptions' sentinel for force-disable.
			opts.IgnoreFile = "/dev/null"
		}

		if in.DryRun {
			// Dry-run: skip pack.Pack (no disk side-effects) but still
			// enumerate for a file_count. A silent file_count of 0 previously
			// masked permission errors, so surface enumeration failures.
			files, eerr := pack.EnumerateThemeFiles(cwd)
			if eerr != nil {
				return common.ExecResult{}, theme.ErrLocalIO("enumerate theme files", eerr)
			}
			return common.ExecResult{Body: map[string]any{
				"zip_path":   out,
				"file_count": len(files),
				"dry_run":    true,
				"name":       name,
				"version":    version,
				"no_ignore":  noIgnore,
			}}, nil
		}

		zipPath, err := pack.Pack(cwd, out, opts)
		if err != nil {
			return common.ExecResult{}, theme.ErrLocalIO("pack zip", err)
		}
		return common.ExecResult{Body: map[string]any{
			"zip_path": zipPath,
			"name":     name,
			"version":  version,
		}}, nil
	},
}

// readThemeInfo / themeZipName / sanitizeFileComponent moved to internal/theme
// (theme.ReadInfo / theme.ZipName / theme.SanitizeFileComponent) so cmd/theme
// can share them. Thin aliases keep this package's call sites unchanged.
func readThemeInfo(cwd string) (name, version string, err error) { return theme.ReadInfo(cwd) }
func themeZipName(name, version string) string                   { return theme.ZipName(name, version) }
func sanitizeFileComponent(s string) string                      { return theme.SanitizeFileComponent(s) }

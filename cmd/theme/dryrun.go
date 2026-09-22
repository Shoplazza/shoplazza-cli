package themecmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/env"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
)

// Dry-run previews for push / share / pull. They send nothing: whatever a real
// run would resolve from the server stays a placeholder, and the package
// figures come from the same selection Pack archives.
const (
	phStore   = "<store>"
	phThemeID = "<theme_id>"
)

// dryRunEnvironment reads the -e (or default) environment from the local file.
// A parse error is ignored: a preview must not fail on config it can do without.
func dryRunEnvironment(cmd *cobra.Command) env.Environment {
	if name := environmentName(cmd); name != "" {
		if e, err := loadSelectedEnvironment(name); err == nil {
			return e
		}
		return env.Environment{}
	}
	if e, ok, err := defaultEnvironmentAt("."); err == nil && ok {
		return e
	}
	return env.Environment{}
}

// dryRunTarget names the store and theme a real run would use, from local
// config only — resolving either for real would mint a token or list themes.
func dryRunTarget(f *cmdutil.Factory, cmd *cobra.Command, themeIDFlag string) (string, string) {
	e := dryRunEnvironment(cmd)
	store := envDomain(f, e)
	if store == "" {
		store = phStore
	}
	themeID := themeIDFlag
	if themeID == "" {
		themeID = e.Theme
	}
	if themeID == "" {
		themeID = phThemeID
	}
	return store, themeID
}

// packagePreview reports what the upload would carry. It goes through Pack's
// own selection, so the figures cannot disagree with the archive that is sent.
func packagePreview(cwd string) (map[string]any, error) {
	plan, err := pack.PlanPack(cwd, pack.PackOptions{})
	if err != nil {
		return nil, theme.ErrLocalIO("plan package", err)
	}
	out := map[string]any{"files": len(plan.Files), "bytes": plan.TotalBytes}
	if plan.Ignored > 0 {
		out["ignored"] = plan.Ignored
	}
	if plan.Hidden > 0 {
		out["hidden"] = plan.Hidden
	}
	return out, nil
}

// printDryRun writes the preview: the requests a real run would send, plus the
// local figures that decide whether the upload can succeed at all.
func printDryRun(cmd *cobra.Command, requests []map[string]any, extra map[string]any) error {
	body := map[string]any{"dry_run": true, "requests": requests}
	for k, v := range extra {
		body[k] = v
	}
	return output.PrintBody(cmd.OutOrStdout(), body, cmdutil.GetFormat(cmd), "")
}

// pushDryRun previews `themes push`: the existence check, then the multipart
// upload of what the package would hold.
func pushDryRun(cmd *cobra.Command, f *cmdutil.Factory, themeIDFlag string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return theme.ErrLocalIO("getwd", err)
	}
	name, version, err := theme.ReadInfo(cwd)
	if err != nil {
		return err
	}
	pkg, err := packagePreview(cwd)
	if err != nil {
		return err
	}
	store, themeID := dryRunTarget(f, cmd, themeIDFlag)
	return printDryRun(cmd, []map[string]any{
		{"method": "GET", "path": themeBaseV202601 + "/" + themeID},
		{"method": "POST", "path": themeBaseV1 + "/upload", "params": map[string]any{
			"name": name, "version": version, "merchant_theme_id": "", "theme_id": themeID,
		}, "body": "multipart: " + theme.ZipName(name, version)},
	}, map[string]any{"store": store, "package": pkg, "overwrites_theme": themeID})
}

// shareDryRun previews `themes share`: it uploads with an empty theme_id, so it
// always creates a theme and overwrites nothing.
func shareDryRun(cmd *cobra.Command, f *cmdutil.Factory) error {
	cwd, err := os.Getwd()
	if err != nil {
		return theme.ErrLocalIO("getwd", err)
	}
	name, version, err := theme.ReadInfo(cwd)
	if err != nil {
		return err
	}
	pkg, err := packagePreview(cwd)
	if err != nil {
		return err
	}
	store, _ := dryRunTarget(f, cmd, "")
	return printDryRun(cmd, []map[string]any{
		{"method": "GET", "path": shopV1},
		{"method": "POST", "path": themeBaseV1 + "/upload", "params": map[string]any{
			"name": name, "version": version, "merchant_theme_id": "", "theme_id": "",
		}, "body": "multipart: " + theme.ZipName(name, version)},
	}, map[string]any{"store": store, "package": pkg, "creates_theme": true})
}

// pullDryRun previews `themes pull`. The archive's contents are unknown until
// it is downloaded, so the preview reports the theme files already in the
// directory instead: those are the ones a real run can overwrite.
func pullDryRun(cmd *cobra.Command, f *cmdutil.Factory, themeIDFlag string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return theme.ErrLocalIO("getwd", err)
	}
	scanned, err := pack.EnumerateThemeFiles(cwd)
	if err != nil {
		return theme.ErrLocalIO("scan theme files", err)
	}
	local := 0
	for _, rel := range scanned {
		if rel != ".themeignore" { // the scan reports it; a pull does not write it
			local++
		}
	}
	store, themeID := dryRunTarget(f, cmd, themeIDFlag)
	return printDryRun(cmd, []map[string]any{
		{"method": "GET", "path": themeBaseV202601 + "/" + themeID},
		{"method": "GET", "path": themeBaseV1 + "/" + themeID + "/download"},
	}, map[string]any{
		"store":  store,
		"target": "./",
		"local":  map[string]any{"theme_files": local, "overwritten_in_place": true, "extras_kept": true},
	})
}

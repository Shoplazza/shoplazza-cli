package app

import (
	"context"

	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// DevResult is the printable outcome of a dev session: the install URL plus the
// tunnel-derived OAuth URIs (mirrors v1's "Your App/Redirect/Install URL").
type DevResult struct {
	InstallURL  string        `json:"install_url"`
	AppURL      string        `json:"app_url"`
	RedirectURL string        `json:"redirect_url"`
	Version     string        `json:"version"`
	Extensions  []DeployedExt `json:"extensions"`
}

// DevReport runs the dev half: build/upload/upsert the locals (is_dev), then
// report the dev session to the Dashboard /dev endpoint with the tunnel-derived
// URIs, and return the install URL. publicURL is the tunnel base; appPath and
// callbackPath are the OAuth routes (e.g. "/auth", "/auth/callback").
func DevReport(ctx context.Context, deps DeployDeps, publicURL, appPath, callbackPath string) (DevResult, *output.ExitError) {
	deps.IsDev = true // dev always lists/builds against is_dev extension_versions.

	// buildUploadUpsert runs the generateVersion-first flow (is_dev) and returns
	// the generated app_version used in the /dev payload (v1 parity: dev.js uses
	// generateNewVersion's app_version).
	deployed, appVersion, ex := buildUploadUpsert(ctx, deps)
	if ex != nil {
		return DevResult{}, ex
	}

	appURL := publicURL + appPath
	redirectURL := publicURL + callbackPath

	// extension_version and extension_version_id now populated from upsert results
	// (v1 parity: completeExtensions includes these fields in the /dev body).
	exts := make([]Extension, 0, len(deployed))
	for _, d := range deployed {
		exts = append(exts, Extension{
			ExtensionID: d.ExtensionID, ExtensionName: d.Name,
			ExtensionType: d.Type, ExtensionVersion: d.Version,
			ExtensionVersionID: flexStr(d.VersionID), ResourceURL: d.ResourceURL,
		})
	}
	appPayload := map[string]any{
		"version":          appVersion,
		"extensions":       exts,
		"dev_app_uri":      appURL,
		"dev_redirect_uri": redirectURL,
	}
	devStep := deps.Progress.Begin("Reporting dev session")
	m, err := deps.Dashboard.ExtensionDev(ctx, deps.PartnerID, deps.ClientID, deps.StoreID, appPayload)
	if err != nil {
		devStep.Fail()
		return DevResult{}, apiOrInternal(err)
	}
	devStep.Done()
	installURL, _ := m["install_url"].(string)

	return DevResult{
		InstallURL:  installURL,
		AppURL:      appURL,
		RedirectURL: redirectURL,
		Version:     appVersion,
		Extensions:  deployed,
	}, nil
}

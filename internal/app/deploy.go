package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/ossupload"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// wrapExtErr prefixes the failing extension's identity onto ex's message,
// preserving its error class (Code/Type) and detail context. Satisfies the
// app-commands requirement that deploy failures name the failing extension.
func wrapExtErr(l LocalExt, ex *output.ExitError) *output.ExitError {
	if ex != nil && ex.Detail != nil {
		ex.Detail.Message = fmt.Sprintf("extension %q (%s): %s", l.Name, l.Dir, ex.Detail.Message)
	}
	return ex
}

// DeployDeps are the inputs to Deploy. BuildArtifact is injected so tests don't
// need Node/Vite; the command wires the real buildCheckout/zipExtension.
type DeployDeps struct {
	Dashboard     *Dashboard
	Store         *client.Client
	Partner       *client.Client // partner-openapi client (app token + app-client-id header): theme connection + function create/commit
	HTTPClient    *http.Client   // OSS POST client (nil -> default)
	PartnerID     string
	ClientID      string
	StoreID       string // numeric store id → ?store_id on GenerateVersion (backend resolves the target store)
	ProjectRoot   string // locates a function's src/index.js (source_code)
	Locals        []LocalExt
	BuildArtifact func(ctx context.Context, l LocalExt) (string, *output.ExitError)
	IsDev         bool
	// ThemePollInterval/MaxRetry drive upsertTheme's task polling. Zero values
	// default to v1's 1s / 10 (tests inject fast values).
	ThemePollInterval time.Duration
	ThemePollMaxRetry int
	// Progress, when non-nil, reports the build/upload/upsert and report steps as
	// live timed lines. nil disables reporting (the default for tests).
	Progress *output.Progress
}

type DeployedExt struct {
	ExtensionID string `json:"extension_id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Version     string `json:"version"`
	VersionID   string `json:"version_id"`
	ResourceURL string `json:"resource_url"`
}

type DeployResult struct {
	Version    string        `json:"version"`
	Extensions []DeployedExt `json:"extensions"`
}

// Deploy runs the chain: remote extension_versions -> diff -> per-type build ->
// OSS upload (checkout/theme only) -> upsert -> Dashboard deploy. checkout,
// theme and function legs are all implemented.
func Deploy(ctx context.Context, deps DeployDeps) (DeployResult, *output.ExitError) {
	deployed, appVersion, ex := buildUploadUpsert(ctx, deps)
	if ex != nil {
		return DeployResult{}, ex
	}

	// Build the deploy payload. The app-level "version" carries the generated
	// app_version (v1 uses generateNewVersion's app_version, not the deploy
	// response). Each extension carries extension_version (the per-extension
	// generated version) + extension_version_id from the upsert results.
	exts := make([]Extension, 0, len(deployed))
	for _, d := range deployed {
		exts = append(exts, Extension{
			ExtensionID: d.ExtensionID, ExtensionName: d.Name,
			ExtensionType: d.Type, ExtensionVersion: d.Version,
			ExtensionVersionID: flexStr(d.VersionID), ResourceURL: d.ResourceURL,
		})
	}
	appPayload := map[string]any{"version": appVersion, "extensions": exts}
	depStep := deps.Progress.Begin("Deploying app")
	if _, err := deps.Dashboard.ExtensionDeploy(ctx, deps.PartnerID, deps.ClientID, deps.StoreID, appPayload); err != nil {
		depStep.Fail()
		return DeployResult{}, apiOrInternal(err)
	}
	depStep.Done()
	return DeployResult{Version: appVersion, Extensions: deployed}, nil
}

// versionOrDefault returns "1.0.0" when v is empty. On a first deploy
// GenerateVersion returns no per-extension versions, so update-path extensions
// (theme/function, which already carry an extension_id) end up with an empty
// version. The /deploy backend rejects an empty extension_version with
// InvalidParameter, so default it — checkout already does the same in its upsert
// leg. Later deploys carry the real generated version from newVers.
func versionOrDefault(v string) string {
	if v == "" {
		return "1.0.0"
	}
	return v
}

// buildUploadUpsert runs the shared first half of deploy/dev: validate locals,
// fetch remote extension_versions, diff, generate the new app+extension versions,
// then per-type build -> OSS upload (checkout/theme) -> upsert. It returns the
// deployed extensions and the generated app_version. Both Deploy (then
// ExtensionDeploy) and DevReport (then ExtensionDev) build on this.
//
// Version flow (v1 parity, deploy.js/dev.js generateNewVersion):
//   - generateVersion-first: after Diff, call GenerateVersion and build a
//     newVers map (extension_id -> extension_version) from its extensions.
//   - update extensions (extension_id known, from remote or local toml): take
//     their version from newVers[extID] BEFORE build/upsert.
//   - add extensions (no extension_id): use "1.0.0" (the upsert create path).
func buildUploadUpsert(ctx context.Context, deps DeployDeps) ([]DeployedExt, string, *output.ExitError) {
	// No local extensions is valid for both deploy and dev (v1 parity).
	pollInterval := deps.ThemePollInterval
	if pollInterval == 0 {
		pollInterval = 1 * time.Second
	}
	maxRetry := deps.ThemePollMaxRetry
	if maxRetry == 0 {
		maxRetry = 10
	}
	q := map[string]any{}
	if deps.IsDev {
		q["is_dev"] = "1"
	}
	verStep := deps.Progress.Begin("Fetching extension versions")
	remote, err := deps.Dashboard.GetExtensionVersions(ctx, deps.PartnerID, deps.ClientID, q)
	if err != nil {
		verStep.Fail()
		return nil, "", apiOrInternal(err)
	}
	verStep.Done()
	pairs, dx := Diff(deps.Locals, remote.Extensions)
	if dx != nil {
		return nil, "", dx
	}

	// generateVersion-first: build newVers[extension_id] = extension_version from
	// the freshly generated versions; keep gen.AppVersion for the app payload.
	isDevStr := ""
	if deps.IsDev {
		isDevStr = "1"
	}
	genStep := deps.Progress.Begin("Generating version")
	gen, err := deps.Dashboard.GenerateVersion(ctx, deps.PartnerID, deps.ClientID, isDevStr, deps.StoreID)
	if err != nil {
		genStep.Fail()
		return nil, "", apiOrInternal(err)
	}
	genStep.Done()
	newVers := make(map[string]string, len(gen.Extensions))
	for _, e := range gen.Extensions {
		newVers[e.ExtensionID] = e.ExtensionVersion
	}

	httpc := deps.HTTPClient
	if httpc == nil {
		httpc = &http.Client{}
	}
	up := &ossupload.Uploader{Client: deps.Store, HTTPClient: httpc}

	deployed := make([]DeployedExt, 0, len(pairs)) // non-nil so a no-extension deploy reports "[]", not null
	for _, p := range pairs {
		// extID is the existing extension id (remote match, else the local toml
		// id). For an update (extID set) the version comes from the generated
		// newVers map; for an add (no extID) it is "1.0.0" (create path).
		extID := p.Local.ExtensionID
		if p.Remote != nil {
			extID = p.Remote.ExtensionID
		}
		version := "1.0.0"
		if extID != "" {
			version = newVers[extID]
		}

		// Build the artifact, then OSS-upload it (resource_url). This pair is
		// identical for all three types — checkout/theme upload their bundle, a
		// function uploads its javy wasm (which ALSO rides the multipart
		// functions/create|commit path; v1 parity: deploy.js builds every
		// extension then OSS-uploads its distPath). Build is often the slow leg
		// (vite/esbuild/javy), so build and upload are reported as separate steps.
		bld := deps.Progress.Begin("Building " + p.Local.Name)
		artifact, bErr := deps.BuildArtifact(ctx, p.Local)
		if bErr != nil {
			bld.Fail()
			return nil, "", wrapExtErr(p.Local, bErr)
		}
		bld.Done()

		upl := deps.Progress.Begin("Uploading " + p.Local.Name)
		resourceURL, uErr := up.Upload(ctx, artifact)
		if uErr != nil {
			upl.Fail()
			return nil, "", wrapExtErr(p.Local, uErr)
		}
		upl.Done()

		ups := deps.Progress.Begin("Upserting " + p.Local.Name)
		switch p.Local.Type {
		case "checkout":
			// v1's app-module upsertCheckout sends only {name, version, resource_url}
			// on create (version "1.0.0"); commit adds extension_id. The standalone
			// `checkout push` payload (template_name/theme_name/extends_fields) does
			// NOT apply to the app module.
			//
			// Commit-gate: mirror theme/function — require BOTH a known id AND a
			// non-empty generated version. A stale local toml id with no remote match
			// yields extID!="" but version=="" (not in newVers); treat it as a create.
			commitID := ""
			coVersion := "1.0.0"
			if extID != "" && version != "" {
				commitID = extID
				coVersion = version
			}
			inner := map[string]any{
				"resource_url": resourceURL,
				"version":      coVersion,
				"name":         p.Local.Name,
			}
			gotID, verID, cErr := upsertCheckout(ctx, deps.Store, inner, commitID)
			if cErr != nil {
				ups.Fail()
				return nil, "", wrapExtErr(p.Local, cErr)
			}
			deployed = append(deployed, DeployedExt{
				ExtensionID: gotID, Name: p.Local.Name, Type: "checkout",
				Version: coVersion, VersionID: verID, ResourceURL: resourceURL,
			})
		case "theme":
			// extID + the generated version drive upsertTheme's update path; an add
			// (empty extID, version "1.0.0") takes the create path.
			ext := Extension{
				ExtensionID:      extID,
				ExtensionName:    p.Local.Name,
				ExtensionType:    "theme",
				ExtensionVersion: version,
				ResourceURL:      resourceURL,
			}
			r, tErr := upsertTheme(ctx, ext, deps.Store, deps.Partner, pollInterval, maxRetry)
			if tErr != nil {
				ups.Fail()
				return nil, "", wrapExtErr(p.Local, tErr)
			}
			deployed = append(deployed, DeployedExt{
				ExtensionID: r.ExtensionID, Name: p.Local.Name, Type: "theme",
				Version: versionOrDefault(version), VersionID: r.ExtensionVersionID, ResourceURL: resourceURL,
			})
		case "function":
			// Upsert the function code via the partner-openapi multipart endpoint.
			// The wasm (artifact) is the multipart `file` on functions/create|commit.
			// NOTE: whether the backend strictly REQUIRES the OSS resource_url for
			// the function leg is unconfirmed — on a dev store, empty resource_url →
			// 404 but a populated one still fails (404/500, non-deterministic), so a
			// dev /deploy never completed a function deploy. We match v1 regardless.
			entry := functionEntryPath(deps.ProjectRoot, p.Local.Dir)
			// extID + the generated version drive upsertFunction's commit path; an
			// add (empty extID, version "1.0.0") takes the create path.
			ext := Extension{
				ExtensionID:      extID,
				ExtensionName:    p.Local.Name,
				ExtensionType:    "function",
				ExtensionVersion: version,
			}
			r, fErr := UpsertFunction(ctx, ext, deps.Partner, entry, artifact)
			if fErr != nil {
				ups.Fail()
				return nil, "", wrapExtErr(p.Local, fErr)
			}
			deployed = append(deployed, DeployedExt{
				ExtensionID: r.ExtensionID, Name: p.Local.Name, Type: "function",
				Version: versionOrDefault(version), VersionID: r.ExtensionVersionID, ResourceURL: resourceURL,
			})
		default:
			ups.Fail()
			return nil, "", output.ErrValidation("unknown extension type %q in %s", p.Local.Type, p.Local.Dir)
		}
		ups.Done()

		// Persist the server-issued id into the extension toml so the NEXT
		// dev/deploy id-matches instead of re-creating (v1 deploy.js:156 /
		// dev.js:154). Only on change — the common update path stays write-free.
		if got := deployed[len(deployed)-1].ExtensionID; got != "" && deps.ProjectRoot != "" {
			if got != p.Local.ExtensionID {
				if wErr := WriteBackExtensionVersion(deps.ProjectRoot, p.Local.Dir, got, ""); wErr != nil {
					return nil, "", wrapExtErr(p.Local, output.ErrInternal("upsert succeeded but writing back extension id failed: %v", wErr))
				}
			}
			// v1-compat: migrate a legacy extension.config.json to a v2 toml. No-op otherwise.
			ext := deployed[len(deployed)-1]
			if mErr := MigrateV1Extension(deps.ProjectRoot, p.Local.Dir, got, ext.Name, ext.Type, ext.Version); mErr != nil {
				return nil, "", wrapExtErr(p.Local, output.ErrInternal("upsert succeeded but migrating v1 config failed: %v", mErr))
			}
		}
	}
	return deployed, gen.AppVersion, nil
}

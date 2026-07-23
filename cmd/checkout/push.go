package checkout

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/jsbuild"
	"github.com/Shoplazza/shoplazza-cli/internal/ossupload"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

func newCmdPush(f *cmdutil.Factory) *cobra.Command {
	var localID string
	var debug bool
	var version string
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Build and upload a new extension version (does NOT activate it)",
		Long: `Build the extension locally, upload the artifact, and create or commit
a new version on the server. The version is NOT activated — use
'shoplazza checkout deploy' to activate it afterward.`,
		PreRunE: authPreRun(f),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if localID == "" {
				return output.ErrValidation("--name <extension name> is required (the directory under ./extensions)")
			}
			if vErr := validPlainName("--name", localID); vErr != nil {
				return vErr
			}
			if cmdutil.IsDryRun(cmd) {
				return output.ErrValidation("--dry-run is not supported for 'checkout push'; use 'checkout build' to inspect the build step")
			}
			cwd, err := os.Getwd()
			if err != nil {
				return output.ErrInternal("cannot determine working directory: %s", err.Error())
			}
			extDir := filepath.Join(cwd, "extensions", localID)
			if info, statErr := os.Stat(extDir); statErr != nil || !info.IsDir() {
				return output.ErrValidation("local extension '%s' not found under ./extensions", localID)
			}
			res, buildErr := jsbuild.RunBuild(cmd.Context(), jsbuild.BuildRequest{
				Action: "build", Name: localID, Debug: debug,
			}, cwd)
			if buildErr != nil {
				return buildErr
			}
			if len(res.Artifacts) == 0 {
				return output.ErrInternal("build produced no artifacts for '%s'", localID)
			}
			artifactRel, aErr := selectJSArtifact(localID, res.Artifacts)
			if aErr != nil {
				return aErr
			}
			artifact := filepath.Join(cwd, artifactRel)
			// push always targets the current store (no --store-domain override).
			store, sErr := resolveStore(f)
			if sErr != nil {
				return sErr
			}
			return runPush(cmd.Context(), cmd, f, extDir, artifact, store)
		},
	}
	cmd.Flags().StringVar(&localID, "name", "", "Extension name (the directory under ./extensions)")
	cmd.Flags().StringVar(&version, "version", "", "Optional. Write this version into extension.json before pushing (must be greater than the last pushed version).")
	cmd.Flags().BoolVar(&debug, "debug", false, "Verbose build logs to stderr")
	addDryRunFlag(cmd) // no --store-domain: push always acts on the current store
	// push has no dry-run (the runtime rejection above is the backstop);
	// hide the shared flag so the help text stops advertising it.
	_ = cmd.Flags().MarkHidden("dry-run")
	return cmd
}

// selectJSArtifact picks the JS bundle from the build output. The upload
// target is the .js entry — not source maps or extracted CSS, which the build
// can emit alongside it.
func selectJSArtifact(name string, artifacts []string) (string, *output.ExitError) {
	for _, a := range artifacts {
		if strings.HasSuffix(a, ".js") {
			return a, nil
		}
	}
	return "", output.ErrValidation("build for '%s' produced no .js artifact (got: %s)",
		name, strings.Join(artifacts, ", "))
}

// runPush uploads the built artifact, creates/commits the extension, writes back
// the server id on first push, and (non-fatally) prints a preview URL.
func runPush(ctx context.Context, cmd *cobra.Command, f *cmdutil.Factory, extDir, artifact, store string) error {
	cfgPath := filepath.Join(extDir, "extension.json")
	raw, readErr := os.ReadFile(cfgPath)
	if readErr != nil {
		return output.ErrValidation("cannot read %s: %s", cfgPath, readErr.Error())
	}
	var cfg map[string]any
	if jErr := json.Unmarshal(raw, &cfg); jErr != nil {
		return output.ErrValidation("invalid extension.json: %s", jErr.Error())
	}
	existingID := asString(cfg["extensionId"])
	isFirst := existingID == ""
	// currentVersion is what extension.json declares now. With --version's write
	// deferred to a successful push (below), the file tracks the last good push,
	// so this is a meaningful "current version" for the INVALID_VERSION message.
	currentVersion := asString(cfg["version"])

	// --version (optional): override the version for THIS push. Applied in memory
	// (payload + extends_fields); persisted to the file only AFTER a successful
	// push, so a rejected push never corrupts the declared version.
	versionOverridden := false
	if v, _ := cmd.Flags().GetString("version"); v != "" {
		cfg["version"] = v
		versionOverridden = true
		rb, mErr := json.Marshal(cfg)
		if mErr != nil {
			return output.ErrInternal("cannot apply --version to extension.json: %s", mErr.Error())
		}
		raw = rb // extends_fields below reflects the override (keys re-sorted; the server parses JSON)
	}

	up := &ossupload.Uploader{Client: f.Client, HTTPClient: &http.Client{Timeout: 60 * time.Second}}
	resourceURL, upErr := up.Upload(ctx, artifact)
	if upErr != nil {
		return upErr
	}

	// extends_fields is the compact extension.json (original key order), computed
	// BEFORE writeback. Compacting the raw bytes preserves key order, unlike
	// json.Marshal(cfg) which would re-sort them.
	var compactBuf bytes.Buffer
	if cErr := json.Compact(&compactBuf, raw); cErr != nil {
		return output.ErrValidation("invalid extension.json: %s", cErr.Error())
	}
	extendsFields := compactBuf.String()

	inner := map[string]any{
		"resource_url":   resourceURL,
		"version":        cfg["version"],
		"scope":          "", // always present
		"template_name":  firstNonEmpty(cfg["templateName"], cfg["template_name"]),
		"theme_name":     firstNonEmpty(cfg["themeName"], cfg["theme_name"]),
		"name":           firstNonEmpty(cfg["extensionName"], cfg["extensionId"]),
		"description":    firstNonEmpty(cfg["extensionDescription"], ""),
		"extends_fields": extendsFields,
	}
	path := "/openapi/checkout_extensions/create"
	if !isFirst {
		inner["extension_id"] = existingID
		path = "/openapi/checkout_extensions/commit"
	}
	resp, apiErr := doAPI(ctx, f, client.RawRequest{
		Method: "POST", Path: path, Data: map[string]any{"extension": inner},
	})
	if apiErr != nil {
		return apiErr
	}
	if dbg, _ := cmd.Flags().GetBool("debug"); dbg {
		if b, mErr := json.Marshal(resp.Body); mErr == nil {
			_, _ = cmd.ErrOrStderr().Write([]byte("[checkout push] response: " + string(b) + "\n"))
		}
	}

	// The push endpoint can reject with a 200 + failure envelope (e.g.
	// {"message":"INVALID_VERSION","status":3}); surface it instead of
	// reporting ok:true with empty ids.
	if fail := checkoutFailureMessage(resp.Body); fail != "" {
		if fail == "INVALID_VERSION" {
			return output.ErrValidation("INVALID_VERSION: the new version must be greater than the current; pushed version %s, current version %s", asString(cfg["version"]), currentVersion).
				WithHint("pass --version <greater-semver> or bump the version field in " + cfgPath + ", then push again")
		}
		return output.ErrValidation("server rejected the request: %s", fail)
	}

	// Real create/commit response: {data:{extension:{...}}, errors, message,
	// status}. payload() digs to .data (DoRaw doesn't unwrap this envelope);
	// fall back to the payload root if the fields aren't nested under
	// "extension".
	respData := payload(resp.Body)
	ext := mapField(respData, "extension")
	if ext == nil {
		ext = respData
	}
	newExtID := asString(mapField(ext, "extension_id"))
	versionID := asString(mapField(ext, "id"))
	newName := asString(mapField(ext, "name"))

	if isFirst && newExtID != "" {
		cfg["extensionId"] = newExtID
		if newName != "" {
			cfg["extensionName"] = newName
		}
	}
	// Persist on success: the first-push id writeback and/or the --version bump.
	// Deferred to here so a rejected push leaves extension.json untouched.
	if (isFirst && newExtID != "") || versionOverridden {
		if wErr := writeJSONFile(cfgPath, cfg); wErr != nil {
			// The server-side create/commit already happened — the error must
			// carry the new ids, or (on a first push) the next push would
			// re-create a duplicate extension.
			hint := "update " + cfgPath + " manually before pushing again"
			if isFirst {
				hint = `add "extensionId": "` + newExtID + `" to ` + cfgPath +
					" manually before pushing again, or the next push will create a duplicate extension"
			}
			return output.ErrInternal("push succeeded (extension_id %s, version_id %s) but writing extension.json failed: %s",
				newExtID, versionID, wErr.Error()).WithHint(hint)
		}
	}

	if newExtID == "" || versionID == "" {
		_, _ = cmd.ErrOrStderr().Write([]byte("warning: server response did not include extension_id/version_id; preview URL unavailable (re-run with --debug to see the raw response)\n"))
	}

	// Auto-preview — non-fatal.
	previewURL := ""
	if newExtID != "" && versionID != "" {
		if u, pErr := buildPreviewURL(ctx, f, store, newExtID, versionID); pErr == nil {
			previewURL = u
		} else if cmd.ErrOrStderr() != nil {
			_, _ = cmd.ErrOrStderr().Write([]byte("warning: push succeeded but preview failed: " + pErr.Error() + "\n"))
		}
	}

	// Human-readable preview line. Goes to stderr so the stdout JSON envelope
	// stays pipe-clean.
	if previewURL != "" {
		_, _ = cmd.ErrOrStderr().Write([]byte("\nYour extension's preview URL:\n   " + previewURL + "\n"))
	}

	return output.PrintBody(cmd.OutOrStdout(), map[string]any{
		"ok":           true,
		"extension_id": newExtID,
		"version_id":   versionID,
		"resource_url": resourceURL,
		"preview_url":  previewURL,
		"committed":    !isFirst,
	}, cmdutil.GetFormat(cmd), "")
}

// RunPushForTest exposes runPush for tests (skips the build spawn).
func RunPushForTest(ctx context.Context, cmd *cobra.Command, f *cmdutil.Factory, extDir, artifact, store string) error {
	return runPush(ctx, cmd, f, extDir, artifact, store)
}

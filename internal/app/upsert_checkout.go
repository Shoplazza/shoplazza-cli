package app

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app/project"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

type checkoutUpsertResp struct {
	Extension extInfo `json:"extension"`
	Data      struct {
		Extension extInfo `json:"extension"`
	} `json:"data"`
}

type extInfo struct {
	ExtensionID string `json:"extension_id"`
	ID          string `json:"id"`
	Name        string `json:"name"`
}

// upsertCheckout creates (existingID == "") or commits (existingID set) a
// checkout extension via store-openapi, returning the extension_id + version id.
// Mirrors cmd/checkout/push.go's create/commit. inner is the pre-built
// {resource_url, version, name, ...} payload (built by the deploy orchestrator).
//
// PostJSON is used rather than DoRaw: doJSON calls unmarshalUnwrapped, which
// only strips the Shoplazza envelope when code=="Success" or ok==true. The
// checkout_extensions response {data:{extension:{...}}, status:"ok"} has no
// such field, so the full body lands in resp — meaning resp.Data.Extension is
// populated. The dual-shape struct covers both that case and any future
// envelope-unwrapped variant where resp.Extension would be populated instead.
func upsertCheckout(ctx context.Context, c *client.Client, inner map[string]any, existingID string) (string, string, *output.ExitError) {
	path := "/openapi/checkout_extensions/create"
	if existingID != "" {
		inner["extension_id"] = existingID
		path = "/openapi/checkout_extensions/commit"
	}
	var resp checkoutUpsertResp
	if err := c.PostJSON(ctx, path, map[string]any{"extension": inner}, &resp); err != nil {
		return "", "", apiOrInternal(err)
	}
	ext := resp.Extension
	if ext.ExtensionID == "" && ext.ID == "" {
		ext = resp.Data.Extension
	}
	if ext.ExtensionID == "" && ext.ID == "" {
		return "", "", output.ErrInternal("checkout upsert (%s) returned no extension_id — unexpected response body", path)
	}
	return ext.ExtensionID, ext.ID, nil
}

// loadCheckoutExtJSON reads extensions/<dir>/extension.json. nil when absent
// (app-flow projects may not have one); validation error when it isn't a JSON
// object. Numbers decode as json.Number so re-marshaling keeps precision.
func loadCheckoutExtJSON(projectRoot, dir string) (map[string]any, *output.ExitError) {
	if projectRoot == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(extensionJSONPath(projectRoot, dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, output.ErrInternal("extensions/%s/extension.json: %v", dir, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var cfg map[string]any
	if jErr := dec.Decode(&cfg); jErr != nil {
		return nil, output.ErrValidation("extensions/%s/extension.json is malformed: %v", dir, jErr)
	}
	if cfg == nil {
		return nil, output.ErrValidation("extensions/%s/extension.json is malformed: not a JSON object", dir)
	}
	return cfg, nil
}

// mergeCheckoutExtJSON adds cfg to a checkout upsert payload the way
// `checkout push` does: the whole file as extends_fields plus the promoted
// name fields (skipped when empty so a partial file can't blank server-side
// values). version/extensionId inside extends_fields carry the app-flow
// values; cfg itself is not mutated.
func mergeCheckoutExtJSON(inner map[string]any, cfg map[string]any, version, extID string) *output.ExitError {
	if cfg == nil {
		return nil
	}
	m := make(map[string]any, len(cfg)+2)
	maps.Copy(m, cfg)
	m["version"] = version
	m["extensionId"] = extID
	b, err := json.Marshal(m)
	if err != nil {
		return output.ErrInternal("extension.json: %v", err)
	}
	inner["extends_fields"] = string(b)
	setNonEmpty(inner, "template_name", strField(cfg, "templateName", "template_name"))
	setNonEmpty(inner, "theme_name", strField(cfg, "themeName", "theme_name"))
	setNonEmpty(inner, "description", strField(cfg, "extensionDescription"))
	return nil
}

// writeBackExtensionJSONID persists the server-issued id into
// extensions/<dir>/extension.json so a later `checkout push` (which reads only
// that file) commits instead of re-creating a duplicate.
func writeBackExtensionJSONID(projectRoot, dir, id string, cfg map[string]any) error {
	cfg["extensionId"] = id
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(extensionJSONPath(projectRoot, dir), append(b, '\n'), 0o644)
}

func extensionJSONPath(projectRoot, dir string) string {
	return filepath.Join(projectRoot, project.ExtensionsDir, dir, "extension.json")
}

// strField returns the first key in cfg whose value is a non-empty string.
func strField(cfg map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := cfg[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func setNonEmpty(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

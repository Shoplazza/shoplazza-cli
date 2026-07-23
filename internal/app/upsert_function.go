package app

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/multipartx"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// cartTransformNamespace is the function-extension namespace. v1 hardcodes it
// (lib/app/services/extension-upsert/upsertFunction.js NAMESPACE).
const cartTransformNamespace = "cart_transform"

// functionUpsertResp is the enveloped partner-openapi (2025-06) functions
// create/commit response: {code, data:{function_id, version, version_id}}.
//
// This relies on the partner returning code "SUCCESS" (uppercase), NOT the exact
// string "Success". DoRaw's envelope-unwrap only strips {code:"Success",data:{...}}
// on an exact "Success" match, so for "SUCCESS"/"FAILED" the full top-level object
// survives in RawResponse.Body and we read code + data ourselves here. If the API
// ever returned exact "Success", DoRaw would peel the envelope and Code would
// decode empty (false negative).
type functionUpsertResp struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		FunctionID string `json:"function_id"`
		Version    string `json:"version"`
		VersionID  string `json:"version_id"`
	} `json:"data"`
}

// UpsertFunction ports v1 lib/app/services/extension-upsert/upsertFunction.js.
//
// Create path (ext.ExtensionID == "" || ext.ExtensionVersion == ""): multipart
// POST functions/create with name, namespace, source_code (the CONTENT of
// entryJSPath), and the compiled wasm (wasmPath) as the file part.
//
// Commit path (both set): multipart POST functions/commit, additionally sending
// function_id + version.
//
// The request rides the partner client's headers (app-client-id + Access-Token)
// via DoRaw, which applies c.Headers last. On code "SUCCESS" (case-insensitive,
// matching v1 intent) we map data → UpsertResult; otherwise we error naming the
// extension.
func UpsertFunction(ctx context.Context, ext Extension, partner *client.Client, entryJSPath, wasmPath string) (UpsertResult, *output.ExitError) {
	commit := ext.ExtensionID != "" && ext.ExtensionVersion != ""

	sourceCode, err := os.ReadFile(entryJSPath)
	if err != nil {
		return UpsertResult{}, output.ErrInternal("cannot read function source %s: %s", entryJSPath, err.Error())
	}

	// Build the multipart body: string fields, then the wasm file LAST (v1 order).
	b := multipartx.New()
	if ferr := b.AddField("name", ext.ExtensionName); ferr != nil {
		return UpsertResult{}, output.ErrInternal("build function form: %s", ferr.Error())
	}
	if ferr := b.AddField("namespace", cartTransformNamespace); ferr != nil {
		return UpsertResult{}, output.ErrInternal("build function form: %s", ferr.Error())
	}
	if ferr := b.AddField("source_code", string(sourceCode)); ferr != nil {
		return UpsertResult{}, output.ErrInternal("build function form: %s", ferr.Error())
	}
	if commit {
		if ferr := b.AddField("function_id", ext.ExtensionID); ferr != nil {
			return UpsertResult{}, output.ErrInternal("build function form: %s", ferr.Error())
		}
		if ferr := b.AddField("version", ext.ExtensionVersion); ferr != nil {
			return UpsertResult{}, output.ErrInternal("build function form: %s", ferr.Error())
		}
	}
	wasm, werr := os.Open(wasmPath)
	if werr != nil {
		return UpsertResult{}, output.ErrInternal("cannot open function wasm %s: %s", wasmPath, werr.Error())
	}
	defer wasm.Close()
	if ferr := b.AddFile("file", filepath.Base(wasmPath), wasm, "application/octet-stream"); ferr != nil {
		return UpsertResult{}, output.ErrInternal("build function form: %s", ferr.Error())
	}
	body, contentType, berr := b.Build()
	if berr != nil {
		return UpsertResult{}, output.ErrInternal("build function form: %s", berr.Error())
	}

	path := "/openapi/2025-06/functions/create"
	if commit {
		path = "/openapi/2025-06/functions/commit"
	}

	resp, derr := partner.DoRaw(ctx, client.RawRequest{
		Method:  http.MethodPost,
		Path:    path,
		Data:    body,
		Headers: map[string]string{"Content-Type": contentType},
	})
	if derr != nil {
		return UpsertResult{}, apiOrInternal(derr)
	}

	// Re-marshal the parsed body and decode into the typed envelope. The
	// function endpoint returns "SUCCESS"/"FAILED" (never exact "Success"), so
	// DoRaw leaves the full {code,data} object intact.
	raw, merr := json.Marshal(resp.Body)
	if merr != nil {
		return UpsertResult{}, output.ErrInternal("function %q upsert: decode response: %s", ext.ExtensionName, merr.Error())
	}
	var res functionUpsertResp
	if uerr := json.Unmarshal(raw, &res); uerr != nil {
		return UpsertResult{}, output.ErrInternal("function %q upsert: decode response: %s", ext.ExtensionName, uerr.Error())
	}

	if !strings.EqualFold(res.Code, "success") {
		msg := res.Message
		if msg == "" {
			if commit {
				msg = "commit function extension failed"
			} else {
				msg = "create function extension failed"
			}
		}
		return UpsertResult{}, output.ErrInternal("function extension %q upsert failed: %s", ext.ExtensionName, msg)
	}

	return UpsertResult{
		ExtensionID:        res.Data.FunctionID,
		ExtensionVersion:   res.Data.Version,
		ExtensionVersionID: res.Data.VersionID,
	}, nil
}

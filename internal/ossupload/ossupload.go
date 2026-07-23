// Package ossupload uploads a file to a store's Aliyun OSS bucket via a
// presigned POST and returns its public URL. Sign goes through the store client
// (store base + Access-Token); the upload POST goes to the external OSS host
// with a bare HTTP client (no store base, no store token).
package ossupload

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/multipartx"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// signResp is the flat (non-enveloped) body returned by the sign endpoint.
type signResp struct {
	WriteHost string `json:"write_host"`
	ReadHost  string `json:"read_host"`
	Policy    string `json:"policy"`
	AccessID  string `json:"access_id"`
	Sign      string `json:"sign"`
}

// Uploader holds the store client (sign) and a bare HTTP client (OSS POST).
type Uploader struct {
	Client     *client.Client
	HTTPClient *http.Client
}

// ossPostURL builds the OSS POST target. A protocol-relative write_host
// ("//host/...") gets an https: prefix; an already-absolute URL passes through.
func ossPostURL(writeHost string) string {
	if strings.HasPrefix(writeHost, "http://") || strings.HasPrefix(writeHost, "https://") {
		return writeHost
	}
	return "https:" + writeHost
}

// buildOSSForm assembles the multipart body: six string fields in order, then
// the file last. Field order is significant to OSS.
func buildOSSForm(sr signResp, key, fileName string, file io.Reader) (io.Reader, string, error) {
	b := multipartx.New()
	for _, f := range []struct{ k, v string }{
		{"policy", sr.Policy},
		{"OSSAccessKeyId", sr.AccessID},
		{"success_action_status", "200"},
		{"signature", sr.Sign},
		{"x-oss-forbid-overwrite", "true"}, // triggers 409 instead of silent overwrite — MUST keep
		{"key", key},
	} {
		if err := b.AddField(f.k, f.v); err != nil {
			return nil, "", err
		}
	}
	if err := b.AddFile("file", fileName, file, contentTypeFor(fileName)); err != nil { // file LAST
		return nil, "", err
	}
	return b.Build()
}

// contentTypeFor derives the OSS object Content-Type from the artifact's
// extension. Checkout-extension JS must declare a JavaScript MIME type, or the
// browser refuses to execute it as <script type="module"> under nosniff. Other
// artifacts (function .wasm, theme .zip) keep the generic binary type.
func contentTypeFor(fileName string) string {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".js", ".mjs":
		return "text/javascript"
	default:
		return "application/octet-stream"
	}
}

// Upload signs, uploads, and returns read_host + key as the resource URL.
func (u *Uploader) Upload(ctx context.Context, filePath string) (string, *output.ExitError) {
	fileName := filepath.Base(filePath)
	key := "chick-extension/" + fileName // sign is computed for THIS key; do not change

	var sr signResp
	if err := u.Client.GetJSONWithQuery(ctx, "/openapi/checkout_extensions/file/sign", map[string]any{"key": key}, &sr); err != nil {
		var httpErr *client.HTTPError
		if errors.As(err, &httpErr) {
			return "", output.ErrAPI(httpErr.StatusCode, httpErr.Body, "").
				WithEndpoint(httpErr.Method, httpErr.Path)
		}
		return "", output.ErrInternal("OSS sign failed: %s", err.Error())
	}
	if sr.WriteHost == "" {
		return "", output.ErrInternal("OSS sign returned no write_host")
	}

	f, openErr := os.Open(filePath)
	if openErr != nil {
		return "", output.ErrInternal("cannot open artifact %s: %s", filePath, openErr.Error())
	}
	defer f.Close()
	body, ct, formErr := buildOSSForm(sr, key, fileName, f)
	if formErr != nil {
		return "", output.ErrInternal("build OSS form: %s", formErr.Error())
	}

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, ossPostURL(sr.WriteHost), body)
	if reqErr != nil {
		return "", output.ErrInternal("OSS upload: invalid write_host %q: %v", sr.WriteHost, reqErr)
	}
	req.Header.Set("Content-Type", ct) // NOTE: no Access-Token — presigned params authorize this
	httpClient := u.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, doErr := httpClient.Do(req)
	if doErr != nil {
		var netErr net.Error
		if errors.As(doErr, &netErr) {
			return "", output.ErrNetwork("OSS upload: %v", doErr)
		}
		return "", output.ErrInternal("OSS upload: %s", doErr.Error())
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		// Keep going on the status code alone; this placeholder surfaces in the error below.
		respBody = []byte("(body unreadable: " + readErr.Error() + ")")
	}
	// 409 / FileAlreadyExists → graceful skip: the file is already on OSS, so
	// the resource URL is still valid.
	if resp.StatusCode == http.StatusConflict || strings.Contains(string(respBody), "FileAlreadyExists") {
		return resourceURL(sr.ReadHost, key), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", output.ErrInternal("OSS upload failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return resourceURL(sr.ReadHost, key), nil
}

func resourceURL(readHost, key string) string {
	if !strings.HasSuffix(readHost, "/") {
		readHost += "/"
	}
	return readHost + key
}

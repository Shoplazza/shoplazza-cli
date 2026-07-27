package rawapi

import (
	"fmt"
	"io"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// NormalizePath normalizes a raw path into an API path.
func NormalizePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return raw
}

// BuildRequest builds a generic raw api request from CLI inputs.
func BuildRequest(method, path, paramsInput, dataInput string, stdin io.Reader) (client.RawRequest, error) {
	if paramsInput == "-" && dataInput == "-" {
		return client.RawRequest{}, fmt.Errorf("--params and --data cannot both read from stdin (-)")
	}
	params, err := cmdutil.ParseJSONMap(paramsInput, "--params", stdin)
	if err != nil {
		return client.RawRequest{}, err
	}
	data, err := cmdutil.ParseOptionalBody(method, dataInput, stdin)
	if err != nil {
		return client.RawRequest{}, err
	}
	return client.RawRequest{
		Method: strings.ToUpper(method),
		Path:   NormalizePath(path),
		Params: params,
		Data:   data,
	}, nil
}

// BuildTemplatedRequest builds a request and resolves path template variables
// from the parsed params object. Path parameters are removed from query params.
func BuildTemplatedRequest(method, pathTemplate, paramsInput, dataInput string, stdin io.Reader) (client.RawRequest, error) {
	req, err := BuildRequest(method, pathTemplate, paramsInput, dataInput, stdin)
	if err != nil {
		return client.RawRequest{}, err
	}
	resolvedPath, resolvedParams, err := expandPath(req.Path, req.Params)
	if err != nil {
		return client.RawRequest{}, err
	}
	req.Path = resolvedPath
	req.Params = resolvedParams
	return req, nil
}

// ResolveTemplatedPath resolves a templated path using the provided params map.
// Consumed path parameters are removed from the returned query params map.
func ResolveTemplatedPath(pathTemplate string, params map[string]any) (string, map[string]any, error) {
	return expandPath(pathTemplate, params)
}

func expandPath(path string, params map[string]any) (string, map[string]any, error) {
	if params == nil {
		params = map[string]any{}
	}
	out := map[string]any{}
	for k, v := range params {
		out[k] = v
	}

	for {
		start := strings.Index(path, "{")
		if start < 0 {
			break
		}
		end := strings.Index(path[start:], "}")
		if end < 0 {
			break
		}
		end += start
		key := path[start+1 : end]
		raw, ok := out[key]
		if !ok || fmt.Sprint(raw) == "" {
			return "", nil, fmt.Errorf("missing required path parameter: %s", key)
		}
		path = strings.Replace(path, "{"+key+"}", fmt.Sprint(raw), 1)
		delete(out, key)
	}
	return NormalizePath(path), out, nil
}

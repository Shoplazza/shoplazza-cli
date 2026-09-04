package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (c *Client) GetJSON(ctx context.Context, rawPath string, out any) error {
	return c.doJSON(ctx, http.MethodGet, rawPath, nil, nil, out)
}

func (c *Client) GetJSONWithQuery(ctx context.Context, rawPath string, query map[string]any, out any) error {
	return c.doJSON(ctx, http.MethodGet, rawPath, query, nil, out)
}

func (c *Client) PostJSON(ctx context.Context, rawPath string, payload any, out any) error {
	return c.doJSON(ctx, http.MethodPost, rawPath, nil, payload, out)
}

func (c *Client) PutJSON(ctx context.Context, rawPath string, payload any, out any) error {
	return c.doJSON(ctx, http.MethodPut, rawPath, nil, payload, out)
}

func (c *Client) PatchJSON(ctx context.Context, rawPath string, payload any, out any) error {
	return c.doJSON(ctx, http.MethodPatch, rawPath, nil, payload, out)
}

func (c *Client) DeleteJSON(ctx context.Context, rawPath string, out any) error {
	return c.doJSON(ctx, http.MethodDelete, rawPath, nil, nil, out)
}

func (c *Client) DeleteJSONWithQuery(ctx context.Context, rawPath string, query map[string]any, out any) error {
	return c.doJSON(ctx, http.MethodDelete, rawPath, query, nil, out)
}

// DeleteJSONWithBody issues DELETE with a JSON payload. The themes
// edit-session endpoints (remove-block / remove-section) take their target
// coordinates in the request body.
func (c *Client) DeleteJSONWithBody(ctx context.Context, rawPath string, payload any, out any) error {
	return c.doJSON(ctx, http.MethodDelete, rawPath, nil, payload, out)
}

// SendStream executes an HTTP request and returns the response body as an
// io.ReadCloser without buffering it in memory. Use for large/streamed
// downloads (zip / wasm / streamed exports). Callers MUST defer Close.
//
// Uses an internal http.Client with NO Timeout — long downloads must rely on
// ctx for cancellation. c.HTTPClient.Timeout would otherwise cut large transfers.
func (c *Client) SendStream(ctx context.Context, request RawRequest) (io.ReadCloser, error) {
	var body io.Reader
	if request.Data != nil {
		data, err := json.Marshal(request.Data)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(request.Method), c.ResolveURL(request.Path), body)
	if err != nil {
		return nil, err
	}
	if len(request.Params) > 0 {
		req.URL.RawQuery = encodeQuery(request.Params).Encode()
	}
	if request.Data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "*/*")
	for k, v := range c.Headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	httpClient := &http.Client{} // no Timeout; ctx controls cancellation
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(errBody), Method: strings.ToUpper(request.Method), Path: request.Path}
	}
	return resp.Body, nil
}

// DoRaw executes a generic HTTP request and returns a parsed response payload.
//
// Header write order (later writes overwrite earlier ones):
//  1. Content-Type (from Headers["Content-Type"] or "application/json" default)
//  2. Accept default
//  3. request.Headers (Content-Type excluded; empty values skipped)
//  4. c.Headers — always last, so caller cannot forge Access-Token via request.Headers.
func (c *Client) DoRaw(ctx context.Context, request RawRequest) (RawResponse, error) {
	var body io.Reader
	contentType := request.Headers["Content-Type"]

	if contentType != "" {
		switch v := request.Data.(type) {
		case nil:
			body = nil
		case io.Reader:
			body = v
		case []byte:
			body = bytes.NewReader(v)
		default:
			return RawResponse{}, fmt.Errorf(
				"RawRequest.Headers[\"Content-Type\"] set but Data is not io.Reader/[]byte/nil (got %T)", v)
		}
	} else if request.Data != nil {
		data, err := json.Marshal(request.Data)
		if err != nil {
			return RawResponse{}, err
		}
		body = bytes.NewReader(data)
		contentType = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(request.Method), c.ResolveURL(request.Path), body)
	if err != nil {
		return RawResponse{}, err
	}
	if len(request.Params) > 0 {
		req.URL.RawQuery = encodeQuery(request.Params).Encode()
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.8")
	for k, v := range request.Headers {
		if k == "Content-Type" || v == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	for key, value := range c.Headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}

	httpClient := c.HTTPClient
	if request.NoTimeout {
		// Per-call copy with the global timeout stripped; cancellation falls to ctx.
		nc := *c.HTTPClient
		nc.Timeout = 0
		httpClient = &nc
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return RawResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return RawResponse{}, err
	}
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	parsedBody, parseErr := parseResponseBody(resp.Header.Get("Content-Type"), respBody)
	if parseErr != nil {
		if success {
			return RawResponse{}, parseErr
		}
		// Non-2xx with an unparseable body (HTML error page behind a JSON
		// content-type, truncated proxy response): the HTTP failure must win,
		// so the status code survives for later error classification.
		parsedBody = string(respBody)
	}

	unwrapped := unwrapDataEnvelope(parsedBody)
	rawResponse := RawResponse{
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     map[string][]string(resp.Header),
		Body:        unwrapped,
	}
	if !success {
		return rawResponse, &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody), Method: strings.ToUpper(request.Method), Path: request.Path}
	}
	return rawResponse, nil
}

func (c *Client) doJSON(ctx context.Context, method, rawPath string, query map[string]any, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.ResolveURL(rawPath), body)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		req.URL.RawQuery = encodeQuery(query).Encode()
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for key, value := range c.Headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}

	if c.Debug != nil {
		fmt.Fprintf(c.Debug, "[debug] > %s %s\n", method, req.URL.String())
		for k, v := range req.Header {
			val := strings.Join(v, ",")
			if k == "Access-Token" || k == "Authorization" || k == "Cli-Partner-Token" {
				val = redactToken(val)
			}
			fmt.Fprintf(c.Debug, "[debug] >   %s: %s\n", k, val)
		}
		if payload != nil {
			if b, mErr := json.Marshal(payload); mErr == nil {
				fmt.Fprintf(c.Debug, "[debug] >   body: %s\n", string(b))
			}
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if c.Debug != nil {
		fmt.Fprintf(c.Debug, "[debug] < %d  body: %s\n", resp.StatusCode, string(respBody))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody), Method: method, Path: rawPath}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return unmarshalUnwrapped(respBody, out)
}

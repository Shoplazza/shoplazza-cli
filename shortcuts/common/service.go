package common

import (
	"context"
	"fmt"
	"io"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
)

// APIPrefix is the single declaration of the Shoplazza Open Platform API
// version used by every shortcut service. Bumping the version is a one-line edit.
const APIPrefix = "/openapi/2026-01"

// PlannedRequest is the resolved shape of an HTTP request — everything needed
// to either send it or summarise it for --dry-run. Both paths read from the
// same value, so method-and-path can't drift between live and dry-run output.
type PlannedRequest struct {
	Method string
	Path   string
	Query  map[string]any
	Body   any
}

// DryRun renders a planned request as a dry-run envelope without sending.
func DryRun(c *client.Client, p PlannedRequest) map[string]any {
	return map[string]any{
		"dry_run": true,
		"request": c.BuildRequestSummary(p.Method, p.Path, p.Query, p.Body),
	}
}

// Send executes a planned request and returns the decoded JSON body.
func Send(ctx context.Context, c *client.Client, p PlannedRequest) (map[string]any, error) {
	var out map[string]any
	var err error
	switch p.Method {
	case "GET":
		if len(p.Query) == 0 {
			err = c.GetJSON(ctx, p.Path, &out)
		} else {
			err = c.GetJSONWithQuery(ctx, p.Path, p.Query, &out)
		}
	case "POST":
		err = c.PostJSON(ctx, p.Path, p.Body, &out)
	case "PUT":
		err = c.PutJSON(ctx, p.Path, p.Body, &out)
	case "PATCH":
		err = c.PatchJSON(ctx, p.Path, p.Body, &out)
	case "DELETE":
		if len(p.Query) == 0 {
			err = c.DeleteJSON(ctx, p.Path, &out)
		} else {
			err = c.DeleteJSONWithQuery(ctx, p.Path, p.Query, &out)
		}
	default:
		return nil, fmt.Errorf("unsupported HTTP method %q", p.Method)
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SendStream executes a PlannedRequest and returns the response body as a
// stream for non-JSON or large binary responses. Caller MUST defer reader.Close().
func SendStream(ctx context.Context, c *client.Client, p PlannedRequest) (io.ReadCloser, error) {
	return c.SendStream(ctx, client.RawRequest{
		Method: p.Method,
		Path:   p.Path,
		Params: p.Query,
		Data:   p.Body,
	})
}

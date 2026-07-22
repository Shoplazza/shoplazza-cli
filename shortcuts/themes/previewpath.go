package themes

import (
	"context"
	"strings"
	"time"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/shortcuts/common"
)

// Preview-path resolution for themes +edit: map the edited template to a
// representative storefront path so preview_url lands on the page being
// edited. Fail-open by contract: any unknown page, lookup failure, or
// timeout falls back to "" (homepage) and never blocks the write.

// previewStaticPaths maps static template names straight to a path
// ("" renders as / in buildPreviewURL).
var previewStaticPaths = map[string]string{
	"index":  "",
	"cart":   "cart",
	"search": "search",
	"404":    "404",
}

// previewResourcePages maps resource templates to their storefront prefix
// and the list endpoint (with its page-size param) that yields a
// representative handle.
var previewResourcePages = map[string]struct{ prefix, queryPath, sizeParam string }{
	"product":    {"products", common.APIPrefix + "/products", "per_page"},
	"collection": {"collections", common.APIPrefix + "/collections", "page_size"},
	"page":       {"pages", common.APIPrefix + "/pages", "page_size"},
	"blog":       {"blogs", common.APIPrefix + "/blogs", "page_size"},
}

const previewHandleTimeout = 5 * time.Second

// resolvePreviewPath maps --template/--file to a storefront path: static
// pages resolve locally, resource pages fetch one representative handle.
// article and non-template files have no storefront page → homepage.
func resolvePreviewPath(ctx context.Context, c *client.Client, template, file string) string {
	page := previewPageName(template, file)
	if page == "" {
		return ""
	}
	if path, ok := previewStaticPaths[page]; ok {
		return path
	}
	res, ok := previewResourcePages[page]
	if !ok {
		return ""
	}
	handle := representativeHandle(ctx, c, res.queryPath, res.sizeParam)
	if handle == "" {
		return ""
	}
	return res.prefix + "/" + handle
}

// previewPageName extracts the page name: the first dot segment of the
// template name (product.custom → product). A --file counts only when it is
// a templates-group file; sections/snippets/... have no storefront page.
func previewPageName(template, file string) string {
	name := template
	if name == "" {
		group, location, err := templateLocation("", file)
		if err != nil || group != "templates" {
			return ""
		}
		name = strings.TrimSuffix(location, ".liquid")
	}
	if i := strings.IndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	return name
}

// representativeHandle fetches one item from a list endpoint and returns its
// handle; "" on any failure, bounded by previewHandleTimeout.
func representativeHandle(ctx context.Context, c *client.Client, path, sizeParam string) string {
	ctx, cancel := context.WithTimeout(ctx, previewHandleTimeout)
	defer cancel()
	resp, err := common.Send(ctx, c, common.PlannedRequest{
		Method: "GET", Path: path, Query: map[string]any{sizeParam: "1"},
	})
	if err != nil {
		return ""
	}
	return firstHandleIn(resp)
}

// firstHandleIn scans a list response for the first object slice whose head
// carries a non-empty handle, tolerating data wrappers and the per-resource
// list key (products / collections / pages / blogs / list / items).
func firstHandleIn(resp map[string]any) string {
	root := resp
	for i := 0; i < 2; i++ {
		if d := mapField(root, "data"); d != nil {
			root = d
		}
	}
	for _, key := range []string{"products", "collections", "pages", "blogs", "list", "items"} {
		if h := headHandle(root[key]); h != "" {
			return h
		}
	}
	for _, v := range root {
		if h := headHandle(v); h != "" {
			return h
		}
	}
	return ""
}

func headHandle(v any) string {
	items := mapSlice(v)
	if len(items) == 0 {
		return ""
	}
	return getString(items[0], "handle")
}

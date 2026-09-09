package themes

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// Preview-path resolution for themes +edit: map the edited template to a
// representative storefront path, with a custom template's suffix carried as
// the storefront's template=<suffix> parameter. Fail-open: any miss falls
// back to the homepage and never blocks the write.

// previewStaticPaths maps static template names straight to a path
// ("" renders as / in buildPreviewURL).
var previewStaticPaths = map[string]string{
	"index":  "",
	"cart":   "cart",
	"search": "search",
	"404":    "404",
}

// previewResourcePages maps resource templates to their storefront prefix and
// the list endpoint (with its page-size param) yielding a representative item.
var previewResourcePages = map[string]struct{ prefix, queryPath, sizeParam string }{
	"product":    {"products", common.APIPrefix + "/products", "per_page"},
	"collection": {"collections", common.APIPrefix + "/collections", "page_size"},
	"page":       {"pages", common.APIPrefix + "/pages", "page_size"},
	"blog":       {"blogs", common.APIPrefix + "/blogs", "page_size"},
}

const previewPathTimeout = 5 * time.Second

// resolvePreviewPath maps --template/--file to a storefront path: static pages
// resolve locally, resource pages fetch one representative item. A custom
// template's suffix is appended as ?template=<suffix>.
func resolvePreviewPath(ctx context.Context, c *client.Client, template, file string) string {
	page, suffix := previewPageName(template, file)
	if page == "" {
		return ""
	}
	path, ok := previewStaticPaths[page]
	if !ok {
		res, known := previewResourcePages[page]
		if !known {
			return ""
		}
		if path = representativePath(ctx, c, res.queryPath, res.sizeParam, res.prefix); path == "" {
			return ""
		}
	}
	if suffix != "" {
		path += "?template=" + url.QueryEscape(suffix)
	}
	return path
}

// previewPageName splits --template/--file into the page name and the custom
// template suffix (product.custom → "product", "custom"); a --file counts only
// when it is a templates-group file.
func previewPageName(template, file string) (page, suffix string) {
	name := template
	if name == "" {
		group, location, err := templateLocation("", file)
		if err != nil || group != "templates" {
			return "", ""
		}
		name = strings.TrimSuffix(location, ".liquid")
	}
	if i := strings.IndexByte(name, '.'); i > 0 {
		return name[:i], name[i+1:]
	}
	return name, ""
}

// representativePath fetches one item from a list endpoint and returns its
// storefront path; "" on any failure, bounded by previewPathTimeout.
func representativePath(ctx context.Context, c *client.Client, path, sizeParam, prefix string) string {
	ctx, cancel := context.WithTimeout(ctx, previewPathTimeout)
	defer cancel()
	resp, err := common.Send(ctx, c, common.PlannedRequest{
		Method: "GET", Path: path, Query: map[string]any{sizeParam: "1"},
	})
	if err != nil {
		return ""
	}
	return firstPathIn(resp, prefix)
}

// firstPathIn scans a list response for the first object slice whose head
// yields a storefront path, tolerating data wrappers and per-resource list keys.
func firstPathIn(resp map[string]any, prefix string) string {
	root := resp
	for i := 0; i < 2; i++ {
		if d := mapField(root, "data"); d != nil {
			root = d
		}
	}
	for _, key := range []string{"products", "collections", "pages", "blogs", "list", "items"} {
		if p := headPath(root[key], prefix); p != "" {
			return p
		}
	}
	for _, v := range root {
		if p := headPath(v, prefix); p != "" {
			return p
		}
	}
	return ""
}

func headPath(v any, prefix string) string {
	items := mapSlice(v)
	if len(items) == 0 {
		return ""
	}
	if h := getString(items[0], "handle"); h != "" {
		return prefix + "/" + h
	}
	if u := getString(items[0], "url"); u != "" {
		return strings.TrimPrefix(u, "/")
	}
	return ""
}

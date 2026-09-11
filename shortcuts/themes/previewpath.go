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
// resolve locally, resource pages resolve the custom template's bound object
// and otherwise fetch one representative item. A custom template's suffix is
// appended as ?template=<suffix>.
func resolvePreviewPath(ctx context.Context, c *client.Client, themeID, template, file string) string {
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
		if suffix != "" {
			path = boundPath(ctx, c, themeID, page, suffix, res.queryPath, res.prefix)
		}
		if path == "" {
			if path = representativePath(ctx, c, res.queryPath, res.sizeParam, res.prefix); path == "" {
				return ""
			}
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

// boundPath resolves a custom template to the storefront path of the object it
// is bound to: the template list carries obj_id once bound, and that object's
// detail carries the handle. "" when the theme is unknown, the template is
// unbound, or either call fails; bounded by previewPathTimeout.
func boundPath(ctx context.Context, c *client.Client, themeID, page, suffix, queryPath, prefix string) string {
	if themeID == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, previewPathTimeout)
	defer cancel()
	resp, err := common.Send(ctx, c, PlanListTemplates(themeID, map[string]any{"type": page, "per_page": "100"}))
	if err != nil {
		return ""
	}
	var objID string
	for _, item := range mapSlice(unwrapData(resp)["theme_templates"]) {
		if getString(item, "suffix") != suffix {
			continue
		}
		if objID = getString(item, "obj_id"); objID != "" {
			break
		}
	}
	if objID == "" {
		return ""
	}
	detail, err := common.Send(ctx, c, common.PlannedRequest{Method: "GET", Path: queryPath + "/" + objID})
	if err != nil {
		return ""
	}
	return detailPath(detail, prefix)
}

// detailPath reads the storefront path out of a single-object detail response,
// which wraps the object under its resource name ({"collection":{…}}),
// optionally inside a data envelope.
func detailPath(resp map[string]any, prefix string) string {
	root := unwrapData(resp)
	if p := objPath(root, prefix); p != "" {
		return p
	}
	for _, v := range root {
		if m, ok := v.(map[string]any); ok {
			if p := objPath(m, prefix); p != "" {
				return p
			}
		}
	}
	return ""
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
	return objPath(items[0], prefix)
}

// objPath maps one resource object to its storefront path: a handle under the
// resource prefix, else the url the object carries itself (pages have no
// handle field, only url).
func objPath(item map[string]any, prefix string) string {
	if h := getString(item, "handle"); h != "" {
		return prefix + "/" + h
	}
	if u := getString(item, "url"); u != "" {
		return strings.TrimPrefix(u, "/")
	}
	return ""
}

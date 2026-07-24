# shop blogs & articles — storefront blog content

A **blog** is the container/channel (e.g. "News"); an **article** is a post published
within one or more blogs. "发一篇博客文章 / write a blog post" = `articles create`
(attach to a blog via `blog_ids`), not `blogs create`.

## Articles

```bash
shop articles list   --params '{"page_size":10,"author":"…"}'      # cursor pagination
shop articles get    --params '{"id":"<id>"}'
shop articles count
shop articles author                                               # list author names
shop articles create --data '{"article":{"title":"…","content":"…","blog_ids":["<blog-id>"],"published":true}}'
shop articles update --params '{"id":"<id>"}' --data '{"article":{…}}'
shop articles delete --params '{"id":"<id>"}'                      # --dry-run first
```

`articles create` body (`article` wrapper object): `title` **required**; optional
`excerpt`, `content`, `published` (bool), `published_at`, `handle`, `author`,
`seo_title`, `seo_description`, `seo_keywords` (array), `blog_ids` (array),
`image` (`{src,width,height}`). Never fabricate SEO fields or an image URL —
include only what the user gave.

## Blogs

```bash
shop blogs list    --params '{"page_size":10}'
shop blogs get     --params '{"id":"<id>"}'
shop blogs count
shop blogs create  --params '{"blog":{"title":"…"}}'    # NOTE: --params, see below
shop blogs update  --params '{"id":"<id>"}' --data '{"blog":{…}}'
shop blogs delete  --params '{"id":"<id>"}'             # --dry-run first
```

### `blogs create` quirk — no `--data` flag

The registry maps the blog object as a parameter, so the leaf **rejects `--data`**
(`unknown flag: --data`). Pass the blog object through `--params`:

```bash
shop blogs create --params '{"blog":{"title":"News"}}'
```

Only this leaf; `blogs update` takes `--data` normally.

### `blogs update` field casing

The schema lists the update body's title field as `Title` (capital T) alongside
lowercase `handle` / `seo_title` / `seo_description` / `seo_keywords` — confirm with
`shoplazza schema shop.blogs.update` before writing the body rather than assuming
lowercase.

## Boundaries

- 买家评价 / product reviews → `products comments` (shoplazza-products), not articles.
- 自定义页面 (About Us, policies) → `shop pages`, not a blog article.

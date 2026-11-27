# shop pages & redirects — custom pages, URL redirect rules

## Pages (custom storefront pages: About Us, policies, brand story)

Content pages, **not** theme/template editing.

```bash
shop pages list         --page-size 10                  # page_size optional (default 10, ≥1)
shop pages get          --page-id <id>
shop pages count        --title …                     # optional title filter
shop pages search       --url /pages/about-us         # url REQUIRED
shop pages batch-get    --data '{"ids":[1,2,3]}'
shop pages create       --data '{"page":{"title":"关于我们","content":"…"}}'
shop pages update       --page-id <id> --data '{"page":{"title":"…"}}'
shop pages delete       --page-id <id>                     # --dry-run first
shop pages batch-delete --data '{"ids":[1,2,3]}'                     # ALWAYS --dry-run + restate count
```

Body (`page` wrapper object): `title` **required** — the only required field; optional
`content` (HTML), `url` (handle), `meta_title`, `meta_keywords` (array),
`meta_description`, `independent_seo` (bool). On create, send title + whatever content
the user gave — **do not fabricate SEO fields or a URL handle**. `pages update` requires
`title` in the body too (same schema).

## Redirects (301-style URL rules)

```bash
shop redirects list
shop redirects get    --redirect-id <id>
shop redirects search --data '{"from_url":"/old-sale"}'              # from_url REQUIRED, POST
shop redirects create --data '{"redirect":{"from_url":"/old-sale","redirect_url":"/new-sale","status":"open"}}'
shop redirects update --redirect-id <id> --data '{"redirect":{"from_url":"…","redirect_url":"…","status":"open"}}'
shop redirects delete --redirect-id <id>                         # --dry-run first
```

- Body (`redirect` wrapper object): `from_url`, `redirect_url`, `status` — **all three
  required**, on update as well.
- Direction: `from_url` = the OLD path being redirected, `redirect_url` = the NEW
  destination. Don't swap them.

### `status` — required; `"open"` = active (verified live)

`status` is a required string. The schema doesn't enumerate its values, but a **live redirect
was verified to use `status: "open"`** for an **active** rule — so for a normal active redirect,
send `"open"`. Do NOT invent alternatives like `"301"` / `"permanent"`. The disabled/off value
isn't confirmed; if you need to toggle a rule off, look it up first:

```bash
shop redirects list --jq '.data.redirects[].status'        # values live rules actually use
shoplazza schema shop.redirects.create                     # if the enum gets documented upstream
```

If you need a non-`open` value and can't confirm it, ask the user rather than guessing.

## Boundaries

- 页面装修 / 模板 / theme sections → `themes` module, not `shop pages`.
- Blog posts → [blogs-articles.md](blogs-articles.md).

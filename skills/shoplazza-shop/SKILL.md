---
name: shoplazza-shop
description: >-
  Store-level configuration and content for a shoplazza store — use when the user works on:
  shop info / settings (店铺信息 / 店铺设置 / 主域名 / 币种 / 客服邮箱 / customer email);
  sales & traffic analytics (销量 / 销售额 / 流量 / 报表 / 数据分析 / analytics / sales
  report / PV UV / conversion / SKU 排行 / best-selling SKU); the media library & file
  uploads (素材库 / 媒体库 / 上传文件 / files / upload file — store assets, NOT product
  images); metafields / 元字段 / 自定义字段 — ALL metafields live HERE, including
  "给商品加字段" metafields ON products / orders / customers (definitions + resource +
  shop metafields); blogs & articles (博客 / 文章 / 发文章 / blog post); custom storefront
  pages (自定义页面 / 关于我们 / About Us / policy page — content, not theme editing);
  URL redirects (重定向 / 301 / URL 跳转); markets (市场 / 多市场 / 站点 / 发布到日本市场);
  store languages (语言 / 多语言 / 翻译 / 开通日语); real-time shipping-rate carrier
  services (实时运费报价 / 承运商 / carrier service — rate quoting at checkout). NOT the
  product catalog / product images / 商品图片 (→ shoplazza-products); NOT discount codes /
  优惠码 (→ shoplazza-discounts); NOT order search / order counts / 订单统计 / fulfillment
  tracking carriers / 物流查询 (→ orders domain); NOT theme / template editing (→ themes).
---

# shoplazza CLI — shop module

**CRITICAL — before anything else, use Read on [`../shoplazza-common/SKILL.md`](../shoplazza-common/SKILL.md).**
It owns every cross-cutting mechanic: the three access tiers, the output envelope (`.data`),
`--dry-run`, `--jq` (incl. "don't pass `-r`"), `--fields`, `schema`, `api rest`, auth /
profiles, and the safety protocol. This file covers only the shop domain and never repeats them.

## Overview

The `shop` module is the store-level **config & content** domain: shop info/settings,
sales & traffic analytics, the media library (files), metafields (all three families),
blogs & articles, custom pages, URL redirects, markets, store languages, and real-time
shipping-rate carrier services. Broad and shallow — this file is the router; per-sub-area
depth lives in [`references/`](#references).

The **only shortcut is `+upload-file`** — everything else is a spec leaf
(`--params` / `--data`). All three access tiers apply (tier rule in shoplazza-common).

## Command map

Intent → command, highest-fit tier first. Authoritative flags/params live in
`shoplazza shop <cmd> --help` and `shoplazza schema shop.<cmd>`, not this table.

| User intent | Command |
|-------------|---------|
| **Store info** — 查店铺信息 / domain / currency / plan | `shop info get` (optional `--params '{"fields":["id","domain"]}'`) |
| 改客服邮箱 / change customer-service email (the ONLY editable field) | `shop info get --jq '.data.id'` → `shop info update --params '{"shop_id":"<id>"}' --data '{"shop":{"customer_email":"…"}}'` |
| **Analytics** — 销售额/订单数报表 for a date range | `shop analytics overview --data '{"begin_time":"<unix-str>","end_time":"<unix-str>","indicator":["sales","orders"]}'` → [references/analytics.md](references/analytics.md) |
| SKU 销量排行 / best-selling SKU | `shop analytics by-sku --data '{…,"sort_by":"sales_total","sort_direction":"desc"}'` |
| SPU / landing-page / UTM breakdown | `shop analytics by-spu` (extra required `type`) / `by-land-page` (required `dimensions`) / `by-utm` |
| **Media library** — 上传文件到素材库 (public URL only) | `shop +upload-file --source-url <url> [--source-url <url> …] [--folder <name>]` |
| Check an upload task / 上传进度 | `shop files task --params '{"task_id":"<id>"}'` (the shortcut already fetches it once) |
| Browse the media library / 看素材库 | `shop files list` (folder defaults to `all_upload`; `page_size` ≤ 300) |
| File detail / delete a file | `shop files {get,delete} --params '{"file_uri":"<uri>"}'` |
| **Metafields** — 店铺级 metafield | `shop metafields-shop create --data '{"namespace":"…","key":"…","type":"string","value":"…"}'` → [references/metafields.md](references/metafields.md) |
| Metafield ON a product/order/customer… (给商品加字段) | `shop metafields-resource create --params '{"owner_resource":"product","owner_id":"<id>"}' --data '{"definition_id":"…","namespace":"…","key":"…","type":"…","value":…}'` |
| Define a metafield schema (元字段定义) | `shop metafields-definition create --params '{"owner_resource":"…"}' --data '{"name":"…","namespace":"…","key":"…","type":"…"}'` |
| **Blog content** — 发博客文章 / new blog post | `shop articles create --data '{"article":{"title":"…","content":"…","blog_ids":["…"]}}'` → [references/blogs-articles.md](references/blogs-articles.md) |
| Manage blogs (containers of articles) | `shop blogs list` / `shop blogs create --data '{"blog":{"Title":"…"}}'` (title key is capital **`Title`**) |
| **Custom pages** — 自定义页面 / 关于我们 / About Us | `shop pages create --data '{"page":{"title":"…","content":"…"}}'` → [references/pages-redirects.md](references/pages-redirects.md) |
| **URL redirect** — 301 / 跳转 | `shop redirects create --data '{"redirect":{"from_url":"…","redirect_url":"…","status":"…"}}'` (`status` required — see Gotchas) |
| **Markets** — 建市场 / 发布到某国 | `shop markets create --data '{"name":"…","countries":["JP"]}'` → [references/markets.md](references/markets.md) |
| **Languages** — 开通语言 / 多语言 | `shop languages supported` → `add` → `enable` → `publish` → [references/languages.md](references/languages.md) |
| **Carrier services** — 注册实时运费报价承运商 | `shop carrier-services create --data '{"carrier_service":{"name":"…","callback_url":"…","carrier_code":"…"}}'` → [references/carrier-services.md](references/carrier-services.md) |

## Write discipline

Config domain — no full ask-matrix, but these writes have **required no-default fields**.
Missing or ambiguous → ask with `AskUserQuestion` (one batch), never fabricate. Never ask
about defaulted/optional fields.

| Operation | Must ASK if missing | Default silently / never ask |
|---|---|---|
| `+upload-file` | a **public** URL (local file → degrade: explain the API only fetches public URLs, ask for a link or a host) | `--folder` (default `all_upload`) |
| `pages create` | `title` | `url`, `meta_*`, `independent_seo` — omit; never fabricate SEO fields |
| `redirects create` | `from_url`, `redirect_url`; `status` value if the wording doesn't settle it (enum not in schema — see Gotchas) | — |
| `metafields-shop create` | `namespace`, `key`, `type`, `value` | `description`, `definition_id` (optional here) |
| `metafields-resource create` | `owner_resource`+`owner_id`, `definition_id`, `namespace`, `key`, `value` | `description`; `type` — resolve from the definition (`metafields-definition get`) before asking |
| `markets create` | `name`, `countries` | `confirm` — **never silently pass `true`** (it deletes occupied markets) |
| `carrier-services create` | `name`, `callback_url` (the user's own endpoint — never invent), `carrier_code` | `active`, `logo`, `short_desc` |
| `languages add` | the language(s); resolve IETF codes via `languages supported` | — |
| `info update` | the new `customer_email` | `shop_id` — resolve it yourself via `info get`, don't ask |

Destructive leaves (`pages batch-delete`, `markets delete` — irreversible, `files delete`,
`redirects delete`, `metafields-* delete`) → `--dry-run` first + restate, per
shoplazza-common → Safety protocol.

## Boundaries

Reads like another domain (or another domain reads like shop):

| Sounds like | Actually belongs to | Command |
|---|---|---|
| 给商品/订单加元字段 / metafield on a product | **HERE** — all metafields live in shop, even product-owned ones | `shop metafields-resource create --params '{"owner_resource":"product",…}'` |
| 商品图片 / 主图 / image attached to a product | `shoplazza-products` | `products images create` |
| 上传文件 / 素材库 / media library asset | **HERE** | `shop +upload-file` / `shop files …` |
| 销量/流量/转化 报表 / sales & traffic analytics | **HERE** | `shop analytics …` |
| 订单量统计 / 搜订单 / order count & search | orders domain | `orders +count` / `orders +search` |
| 实时运费报价承运商 / rate-quoting carrier at checkout | **HERE** | `shop carrier-services …` |
| 物流商查询 / 运单追踪 / fulfillment tracking carriers, shipping zones | orders domain | `orders tracking-carriers` / `orders shipping-schemas` |
| 自定义页面 / 关于我们 / brand-story page | **HERE** (content, not theming) | `shop pages create` |
| 改模板 / 装修 / theme & template editing | `themes` module (no skill yet) | `themes …` |
| 博客/文章 blog & articles | **HERE** (not product reviews) | `shop blogs` / `shop articles` |
| 买家评价 / product reviews | `shoplazza-products` | `products comments …` |
| 优惠码 / 折扣 / discount codes | `shoplazza-discounts` | `discounts +*` |

## Permissions · Scope

Authorization is by **domain** (the unit of `--domain`), not per command. Authorization flow
is in shoplazza-common → Authentication.

| Operation | Needs | Grant |
|---|---|---|
| read (`info get`, `analytics *`, `files list/get/task`, `* list/get/count`) | `shop` read scope | `auth login --domain shop` |
| write (`+upload-file`, `* create/update/delete`, `languages enable/publish`, …) | `shop` write scope | `auth login --domain shop` |

Look up exact scope literals with `shoplazza auth scopes`; `--domain shop` expands into them.

## Gotchas

Domain-specific pitfalls only (generic ones — `.data` prefix, `--jq` without `-r` — are in
shoplazza-common).

| Symptom | Cause | Fix |
|---------|-------|-----|
| Analytics call rejected / empty | All 5 `analytics` endpoints are **POST**: filters go in `--data`, not `--params`; `begin_time`/`end_time` are **unix-second timestamps passed as STRINGS** (`"1782864000"`) | Compute the timestamps, quote them as strings, send via `--data` |
| Tried to update shop name/address | `info update` exposes **one** editable field: `shop.customer_email` | Anything else is not updatable via this endpoint; say so |
| `info update` needs a `shop_id` the user can't know | `shop_id` is a required path param | Resolve first: `shop info get --jq '.data.id'` |
| Asked to upload a **local** file | `+upload-file` takes **public URLs, NOT local files** (verbatim in its help); the API fetches the URL | Degrade: no write; ask for a public link or suggest hosting it first |
| Upload "done" but file missing | Upload is an **async task** — POST returns a `task_id`; the shortcut auto-fetches the task once | Re-check with `shop files task --params '{"task_id":"…"}'` until `status`/`success_list` settle |
| `blogs create` title ignored / rejected | Blog title key is capital **`Title`** (required, 1–100), not lowercase `title` | `shop blogs create --data '{"blog":{"Title":"…"}}'` — confirm fields via `schema shop.blogs.create` |
| What values can `redirects` `status` take? | `status` is **required** but the schema documents no enum — **do not invent one** | Check `schema shop.redirects.create`, inspect existing rules (`shop redirects list --jq '.data.redirects[].status'`) or the OpenAPI docs; ask the user if still ambiguous |
| `metafields-resource create` rejected, `metafields-shop create` fine | Asymmetry: resource-level **requires `definition_id`**; shop-level doesn't (optional). Type enums differ too (shop: `JSON`/`string`; resource: `single_line_text_field`, `integer`, …) | See [references/metafields.md](references/metafields.md) |
| Language published but market relations vanished | `languages publish` is a **full replacement** — `market_ids: []` removes ALL relations | Always send the complete target list |
| Added a language, storefront unchanged | Newly added languages are **disabled by default**; unsupported codes are **silently dropped** | `languages enable` after `add`; verify codes via `languages supported` |
| `markets create/update` with `confirm: true` deleted other markets | A country may belong to only ONE market; `confirm: true` auto-deletes the occupied markets | Preview with `markets preview`, `--dry-run`, restate what gets deleted, get explicit consent |
| `metafields-definition get` 404s with id in path | Its `id` is a **query** param, not a path segment | `shop metafields-definition get --params '{"id":123456}'` |

## Recipes

```bash
# 1. Store's primary domain and currency
shop info get --jq '{domain: .data.domain, currency: .data.currency}'

# 2. Change the customer-service email (only editable field; two steps)
SHOP_ID=$(shop info get --jq '.data.id')
shop info update --params "{\"shop_id\":\"$SHOP_ID\"}" --data '{"shop":{"customer_email":"support@brand.com"}}'

# 3. Sales + order count, 2026-07-01 .. 2026-07-15 (unix seconds as STRINGS)
shop analytics overview --data '{"begin_time":"1782864000","end_time":"1784159999","indicator":["sales","orders"]}'

# 4. Best-selling SKU over a window (sort desc by sales metric)
shop analytics by-sku --data '{"begin_time":"1782864000","end_time":"1784159999","sort_by":"sales_total","sort_direction":"desc","page_size":10}'

# 5. Upload a public image to the media library (async task; folder defaults to all_upload)
shop +upload-file --source-url https://example.com/banner.png

# 6. Create an "About Us" custom page
shop pages create --data '{"page":{"title":"关于我们","content":"我们是一家来自杭州的 DTC 品牌"}}'

# 7. Permanent URL redirect (status REQUIRED — pin the value via schema/existing rules first)
shop redirects list --jq '.data.redirects[].status'   # see what values live rules use
shop redirects create --data '{"redirect":{"from_url":"/old-sale","redirect_url":"/new-sale","status":"<verified-value>"}}'

# 8. Shop-level metafield (no definition_id needed at shop level)
shop metafields-shop create --data '{"namespace":"global","key":"support_phone","type":"string","value":"400-800-1234"}'

# 9. Product-level metafield (owner in --params, definition_id REQUIRED in --data)
shop metafields-resource create --params '{"owner_resource":"product","owner_id":"66880"}' \
  --data '{"definition_id":"D8899","namespace":"specs","key":"material","type":"single_line_text_field","value":"cotton"}'

# 10. Launch Japanese and publish it to the Japan market
shop languages supported --jq '.data.languages'   # find the exact IETF code
shop languages add --data '{"codes":["ja-JP"]}'
shop languages enable --params '{"language_code":"ja-JP"}'
MARKET_ID=$(shop markets list --jq '.data.markets[] | select(.name=="Japan") | .id')
shop languages publish --params '{"language_code":"ja-JP"}' --data "{\"market_ids\":[\"$MARKET_ID\"]}"
```

## References

- [references/analytics.md](references/analytics.md) — 5 POST endpoints, indicators/dimensions, sort enums, timestamp rules
- [references/metafields.md](references/metafields.md) — definitions vs resource vs shop metafields, owner_resource enum, type enums
- [references/blogs-articles.md](references/blogs-articles.md) — blogs (containers) vs articles (posts); blog title key is capital `Title`
- [references/pages-redirects.md](references/pages-redirects.md) — custom pages CRUD/batch, redirects & the undocumented status enum
- [references/markets.md](references/markets.md) — market lifecycle, country occupation/confirm, price/tax/domain/language
- [references/languages.md](references/languages.md) — add → enable → publish flow, full-replacement semantics
- [references/carrier-services.md](references/carrier-services.md) — rate-quoting carrier registration
- Per-command flags: `shoplazza shop <cmd> --help`
- Spec-leaf params / body / response: `shoplazza schema shop.<cmd>` (nested: `schema shop.metafields-resource.create`)
- Cross-cutting mechanics (auth / tiers / envelope / `--dry-run` / `--jq` / safety): `../shoplazza-common/SKILL.md`

# shop analytics — sales & traffic reports

Eight spec leaves: seven **POST** reports — every filter/metric goes in `--data` (never
`--params`) — plus one GET lookup with no body (`traffic-channel-options`).
Full field list: `shoplazza schema shop.analytics.<cmd>`.

| Command | Endpoint | Dimension |
|---|---|---|
| `analytics overview` | POST `/data-analysis` | store totals; optional custom/UTM dimensions |
| `analytics by-sku` | POST `/data-analysis/sku` | per-SKU product performance |
| `analytics by-spu` | POST `/data-analysis/spu` | per-SPU (extra **required** `type` field) |
| `analytics by-land-page` | POST `/data-analysis/land-page` | landing pages (**required** `dimensions`) |
| `analytics by-utm` | POST `/data-analysis/utm` | UTM campaign traffic |
| `analytics search-keyword` | POST `/data-analysis/search-keyword` | on-site search keywords (搜索词) |
| `analytics traffic-channel` | POST `/data-analysis/traffic-channel` | traffic type / channel (**required** `dimensions` + `metrics`) |
| `analytics traffic-channel-options` | GET `/data-analysis/traffic-channel/options` | the valid traffic type → channel names (no body) |

## Common body fields (all POST reports)

- `begin_time` / `end_time` — **required**; unix timestamps in **seconds**, passed as
  **strings** (e.g. `"1782864000"`). `end_time` must be greater than `begin_time`.
  Compute them from the user's dates; do not pass ISO dates or integers.
- `time_zone` — optional integer offset in hours, recommended −12..14 (align with the
  store's timezone from `shop info get --jq '.data.timezone'` when precision matters).
- `cursor` / `page_size` — cursor pagination; responses carry `.data.cursor` + `.data.has_more`.
- `sort_by` / `sort_direction` (`asc` | `desc`) — per-endpoint sort fields below.
- `filter_crawler_type` — `no_filter_crawler` (default) | `official_crawler`.

## overview

- `indicator` — **required** array; request exactly what the user asked, no padding.
  Custom indicators: `pv`, `uv`, `add_cart_uv`, `add_cart_qty`, `add_payment_info_uv`,
  `begin_checkout_pv`, `begin_checkout_uv`, `orders`, `sales`, `conversion_rate`,
  `impression`. With UTM dimensions only the UTM-valid subset applies (see schema).
- `dimension` — optional array. Custom: `country_code`. UTM: `utm_source`, `utm_medium`,
  `utm_term`, `utm_campaign`, `utm_content`. **Never mix custom and UTM dimensions** —
  validation fails.
- `dt_by` — time granularity: `dt_by_hour` | `dt_by_day`.
- `filters` — exact-match object keyed by dimension name, AND-combined,
  e.g. `{"country_code":"US"}`.

```bash
# 销售额 + 订单数, by day, US only
shop analytics overview --data '{"begin_time":"1782864000","end_time":"1784159999","indicator":["sales","orders"],"dt_by":"dt_by_day","dimension":["country_code"],"filters":{"country_code":"US"}}'
```

## by-sku

- `sort_by` values: `variant_op_updated_at`, `views_count`, `add_to_cart_count`,
  `order_count`, `sales_total`, `net_sales_total`, `add_to_cart_rate`,
  `add_to_cart_conversion_rate`, `sales_count`, `views_rate`.
  "卖得最好" / best-selling → **default to `sales_total` desc (revenue); do NOT stall on
  units-vs-revenue** — pick revenue, state the assumption, and offer `sales_count` (units)
  if the user meant quantity. Always `sort_direction: desc`.
- Filters: `collection_id`, `keyword` (fuzzy: title/ID/SKU/SPU/tags/note),
  `sales_platform` (`shopping_action` | `shoplazza` | `mocart`), `search_model`
  (`base` default | `advanced` — unlocks `collection_name`, `title`, `tag_list`,
  `category`, `sub_category`, `cost_price_min/max`, `product_note`, `vendor`,
  `created_at_min_at`/`created_at_max_at`).

## by-spu

Same shape as by-sku plus a **required `type`** field ("Type of resource to analyze" —
its values are not enumerated in the schema output; check `schema shop.analytics.by-spu`
and don't guess). `sort_by` here uses product-lifecycle fields (`created_at`,
`first_published_at`, `published_at`, `product_op_updated_at`, … — see schema).

## by-land-page

- `dimensions` — **required** array: `land_url_path` (landing URL path),
  `last_template_name` (page type), `last_referrer_show` (last interactive source).
- `sort_by`: `pv`, `uv`, `add_cart_uv`, `begin_checkout_uv`, `add_payment_info_uv`,
  `orders`, `sales`, … (see schema).
- `filters` — nested filter tree (arrays = OR, objects = AND, leaves =
  `{"operator":…,"value":…}`) — see the schema description before building one.

## by-utm

- Only `begin_time`/`end_time` required.
- `sort_by`: `view_client_count`, `uv_rate`, `product_views_count`,
  `add_to_cart_count`, `begin_checkout_count`, … (see schema).
- `date_by` — `""` for totals, `"day"` for a daily breakdown.
- `filters` — AND-combined list like
  `[{"title":"utm_medium","prerequisite":"includes","values":["cpc"]}]`.

## search-keyword

- `sort_by`: `count` (searches), `percent` (share of all searches), `uv` (searching
  users), `has_result`, `click_rate`. `sort_direction` defaults to `desc`.
- Filters: `keyword` (fuzzy), `country_code` (e.g. `"US"`), `has_result` — a **string**,
  `"1"` = returned results, `"0"` = no results (搜了没结果的词); a number is rejected.
- Rows: `.data.data[]` → `{item, count, percent, uv, has_result, click_rate}` (`item` is the
  keyword). A window with no searches returns `.data` as `{}` — report "no search data",
  not an error.

```bash
# 没搜到结果的热门搜索词 Top 10
shop analytics search-keyword --data '{"begin_time":"1782864000","end_time":"1784159999","has_result":"0","sort_by":"count","sort_direction":"desc","page_size":10}'
```

## traffic-channel

- `dimensions` — **required** array: `last_referrer_first_show` (traffic type, e.g.
  `Direct`, `Social`) and/or `last_referrer_show` (channel, e.g. `Facebook`).
- `metrics` — **required** array: `uv`, `sales`, `orders`, and their shares of the grand
  total `uv_rate`, `sales_rate`, `orders_rate`. `sort_by` must be one of the requested metrics.
- `has_summary: true` adds the grand-total row at `.data.summary`.
- `filters` — a JSON object **encoded as a string** (an object is rejected with 400), keyed
  by dimension, entries AND-combined. `in` / `not in` take an array, `like` / `not like` a
  string, `=` `>` `>=` `<` `<=` a scalar. Unknown keys or operators are silently ignored —
  a filter "not taking effect" is usually a typo.
- Exact type/channel names: `analytics traffic-channel-options` →
  `.data.traffic_channels[]` `{last_referrer_first_show, last_referrer_shows[]}`. Look them
  up before filtering; don't guess the spelling.
- Rows: `.data.data[]`. Metric values are display strings (`"32.00"`, `"33.33%"`); do math
  and comparisons on the `*_original` numbers (`sales_original`, `uv_rate_original`, …).

```bash
# Social 流量下各渠道的访客 / 订单 / 销售额, 带合计行
shop analytics traffic-channel --data '{"begin_time":"1782864000","end_time":"1784159999","dimensions":["last_referrer_show"],"metrics":["uv","orders","sales"],"sort_by":"uv","sort_direction":"desc","has_summary":true,"filters":"{\"last_referrer_first_show\":{\"operator\":\"in\",\"value\":[\"Social\"]}}"}'
```

## Boundary

Order-level questions ("how many orders", "find order X") are the orders domain
(`orders +count` / `orders +search`); `shop analytics` is for aggregated sales/traffic
reporting.

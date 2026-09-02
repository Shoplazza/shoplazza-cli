# shop analytics — sales & traffic reports

Five spec leaves, **all POST** — every filter/metric goes in `--data` (never `--params`).
Full field list: `shoplazza schema shop.analytics.<cmd>`.

| Command | Endpoint | Dimension |
|---|---|---|
| `analytics overview` | POST `/data-analysis` | store totals; optional custom/UTM dimensions |
| `analytics by-sku` | POST `/data-analysis/sku` | per-SKU product performance |
| `analytics by-spu` | POST `/data-analysis/spu` | per-SPU (extra **required** `type` field) |
| `analytics by-land-page` | POST `/data-analysis/land-page` | landing pages (**required** `dimensions`) |
| `analytics by-utm` | POST `/data-analysis/utm` | UTM campaign traffic |

## Common body fields (all five)

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

## Boundary

Order-level questions ("how many orders", "find order X") are the orders domain
(`orders +count` / `orders +search`); `shop analytics` is for aggregated sales/traffic
reporting.

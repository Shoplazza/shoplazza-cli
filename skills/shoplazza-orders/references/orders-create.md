# orders create — manual / draft order creation (代客下单)

Create an order on a customer's behalf ("手工建单 / 代客下单 / manual order"). This is a
**spec leaf** — there is **no `+create` shortcut** for orders. The whole request is one heavy
JSON body passed via `--data`; everything nests under a single required `order` wrapper. Always
verify the live shape with `schema orders.create --view request` before building — this file is
the reconciled summary, not a substitute for it.

## Command

```bash
orders create --data '{"order":{…}}'      # POST /openapi/2026-01/orders — body only, no path/query params
```

Money and a real person's shipping details are involved: **restate the order (buyer, items,
prices, shipping, total) and `--dry-run` first**, then run for real. Never fabricate a buyer
identity, an address, a price, or a variant id (see the never-fabricate list below).

## Body shape (verified against `schema orders.create --view request`)

Top-level: a single required `order` object. Its fields:

| `order.<field>` | Req | Type | Notes |
|---|---|---|---|
| `line_items` | **yes** | array | ≥1 item; shapes below |
| `shipping_address` | **yes** | object | required sub-fields below |
| `shipping_line` | **yes** | object | required sub-fields below |
| `tax_total` | **yes** | string | order tax as a decimal string; `"0.00"` when none |
| `currency_code` | **yes** | string | ISO 4217; must match the store/market currency (`shop info get --jq '.data.currency'`) |
| `tags` | no | string[] | free labels |
| `note` | no | string | internal order note |
| `discount_application` | no | object | display-only: `{"discount_code","title"}` — records a code, does not price the cart |
| `order_confirm_notify` | no | integer | `1` = email the buyer an order confirmation |
| `fulfillment_notify` / `partial_fulfillment_notify` / `order_delivered_notify` | no | integer | `1` = send that buyer notification |
| `discount` / `payment_line` / `config` | no | string | typed **string** in this schema (registry flattening) — leave out of a plain draft order (see Gotchas) |

### `line_items[]`

| Field | Req | Type | Notes |
|---|---|---|---|
| `quantity` | **yes** | integer | the ONLY schema-required line field |
| `variant_id` | no | string | reference a real variant; if `price` is omitted the platform prices it from that variant |
| `product_id` | no | string | optional companion to `variant_id` |
| `price` | no | string | unit price (money — from the user or resolved from the variant; never invent) |
| `product_title` | no | string | label for a **custom** line that has no variant |
| `total_price` | no | string | line total for a custom line |
| `use_discount` | no | boolean | |

Two usable line shapes:
- **Catalog line** — `variant_id` + `quantity` (+ optional `price`). Prefer this; get the
  `variant_id` from `products +search` / `products get`.
- **Custom line** — `product_title` + `price` + `total_price` + `quantity` (no catalog variant).

### `shipping_address` (required object)

- **Required**: `last_name`, `email`, `country_code`, `province`, `province_code`, `city`,
  `address`, `zip`.
- Optional: `first_name`, `country` (display name), `phone`, `phone_area_code`, `area`,
  `address1`, `company`, `cpf`.
- `country_code` / `province_code` are **ISO codes** (`US`, `CA`), NOT the display names that go
  in `country` / `province`. `first_name` is **not** required — only `last_name` is.

### `shipping_line` (required object)

- **Required**: `shipping_name` (display label, e.g. "Standard Shipping"), `shipping_price`
  (money, decimal string; `"0.00"` for free).
- Optional: `shipping_desc`, `delivery_method` (integer), `business_info.time_remark`.

## Required-vs-ask (what to ASK, infer, or default)

| Field | Handling |
|---|---|
| line item `variant_id` (what they're buying) | **ASK / look up** — resolve via `products +search`; never invent a variant id |
| line item `price`, `total_price` | **money — never fabricate**; take the user's number, or omit `price` on a catalog line to let the variant price it |
| `quantity` | ASK only if not stated; do not assume >1 |
| `shipping_address.*` (buyer identity + address) | **never fabricate** a name, email, or address — ASK for any missing required sub-field |
| `shipping_price`, `tax_total` | **money** — use the stated amounts; default `"0.00"` only when the user implies no shipping/no tax (a manual order often zeroes them) and restate that assumption |
| `shipping_name` | a display **label**, not money/identity — default a neutral `"Standard Shipping"` (or the carrier/method the user named); do NOT block the order asking for it |
| `currency_code` | **infer** from the store (`shop info get --jq '.data.currency'`); confirm if the buyer is clearly in another currency zone |
| notify flags, `tags`, `note`, `discount_application` | omit unless the user asked for them |
| `payment_line` / `discount` / `config` | omit — record payment after creation with `orders pay` |

## Minimal valid body (custom line, required fields only)

```bash
orders create --data '{"order":{"line_items":[{"product_title":"Custom item","quantity":1,"price":"29.90","total_price":"29.90"}],"shipping_address":{"last_name":"Doe","email":"john@example.com","country_code":"US","province":"California","province_code":"CA","city":"Los Angeles","address":"123 Main St","zip":"90012"},"shipping_line":{"shipping_name":"Standard Shipping","shipping_price":"0.00"},"tax_total":"0.00","currency_code":"GBP"}}' --dry-run
```

## Fuller body (catalog + custom lines, optionals, notify)

```bash
orders create --data '{"order":{"line_items":[{"variant_id":"dd305ee4-fb07-4467-b846-27abd038f6ea","quantity":2,"price":"19.99"},{"product_title":"Gift wrap","quantity":1,"price":"3.00","total_price":"3.00"}],"shipping_address":{"first_name":"John","last_name":"Doe","email":"john@example.com","phone_area_code":"1","phone":"3105550100","country":"United States","country_code":"US","province":"California","province_code":"CA","city":"Los Angeles","address":"123 Main St","zip":"90012"},"shipping_line":{"shipping_name":"Standard Shipping","shipping_price":"5.00","business_info":{"time_remark":"3-5 days"}},"tax_total":"0.00","currency_code":"GBP","tags":["manual","vip"],"note":"phoned in","discount_application":{"discount_code":"WELCOME","title":"Welcome"},"order_confirm_notify":1}}' --dry-run
```

The `variant_id` above is illustrative — get a real one from `products +search`.

## Gotchas

| Symptom | Cause | Fix |
|---|---|---|
| Body rejected for missing fields | The `order` wrapper or a required sub-field is absent | Everything nests under `order`; the required set is `line_items[].quantity`, `shipping_address` (last_name, email, country_code, province, province_code, city, address, zip), `shipping_line` (shipping_name, shipping_price), `tax_total`, `currency_code` |
| Sent `country`/`province` names but the order still fails | `country_code` / `province_code` (ISO) are the required keys, not the display names | Send `"country_code":"US"`, `"province_code":"CA"`; `country`/`province` display names are optional |
| Copied the `orders pay` `payment_line` OBJECT into `create` | In `orders create` the schema types `payment_line` as a **string** (unlike `orders pay`, where it is an object) | Omit `payment_line` on create; record payment afterward with `orders pay` (see SKILL.md) |
| Invented a `variant_id` or a unit `price` | Both are catalog/money facts | Resolve the `variant_id` via `products +search`; omit `price` on a catalog line to let the variant price it, or use the user's stated price |
| Currency mismatch rejected | `currency_code` must match the store/market | Check `shop info get --jq '.data.currency'` before sending another |

## References

- Request shape (authority): `shoplazza schema orders.create --view request`
- After creating: mark paid → SKILL.md `orders pay`; ship → `orders +ship`; cancel/delete →
  `orders cancel` / `orders delete` (both `--dry-run` first)
- Cross-cutting mechanics (`--dry-run`, `--data`, `.data` envelope, safety): `../shoplazza-common/SKILL.md`

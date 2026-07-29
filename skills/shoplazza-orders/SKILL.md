---
name: shoplazza-orders
description: >-
  Use when the user wants to manage a shoplazza store's orders through the CLI — searching /
  counting / viewing orders (查订单 / 找订单 / 订单列表 / 订单详情 / 新订单 / order search),
  shipping and fulfillment (发货 / 标记发货 / 物流 / 运单号 / 改运单号 / ship an order / update
  tracking), refunding a buyer (退款 / 给顾客退钱 / 退钱给买家 / refund an order — buyer refunds
  live HERE, not in discounts and not in billing), cancelling / creating / updating / marking
  orders paid (取消订单 / 代客下单 / 标记支付), an order's payment transactions (订单支付流水 /
  支付交易记录 / buyer payment transactions — app-charge transactions → shoplazza-billing),
  fraud risk records (欺诈 / 风控 / 高风险订单 / order risk), after-sales records (售后 /
  post-sales), shipping zones and rates (运费区域 / 邮费 / 运费方案 / shipping zones — real-time
  rate carriers → shoplazza-shop carrier-services), and tracking-carrier lookup (快递承运商 /
  运单号识别承运商 / tracking carrier detect). A customer's orders by email (按邮箱查某客户的订单)
  are searched HERE; the customer record itself → shoplazza-customers. NOT product stock /
  库存 / 补货 (→ shoplazza-products).
---

# shoplazza CLI — orders module

**CRITICAL — before anything else, use Read on [`../shoplazza-common/SKILL.md`](../shoplazza-common/SKILL.md).**
It owns every cross-cutting mechanic: the three access tiers, the output envelope (`.data`),
`--dry-run`, `--jq` (incl. "don't pass `-r`"), `--fields`, `schema`, `api rest`, auth / profiles,
and the safety protocol. This file covers only the orders domain and never repeats them.

## Overview

The `orders` module manages the order lifecycle (search, count, get, create, update, cancel,
pay, delete) and its sub-resources: fulfillments (shipping records), refunds, fraud risks,
payment transactions, after-sales (post-sales) records, shipping zones/rates, and tracking
carriers. All three access tiers apply (tier-selection rule is in shoplazza-common — prefer
`+shortcut`).

Shortcuts exist for **`+search` / `+count` / `+ship` / `+refund` / `+update-tracking` only** —
every other operation (get, cancel, create, pay, and all sub-resources) is a spec leaf.
Orders is a large domain: this file is the router plus the high-frequency operations;
per-sub-resource depth lives in [`references/`](#references).

## Command map

Intent → command, highest-fit tier first. The authoritative flags/params live in
`shoplazza orders <cmd> --help` and `shoplazza schema orders.<cmd>`, not this table.

| User intent | Command |
|-------------|---------|
| Search / filter orders | `orders +search [--keyword <no/name/email>] [--status …] [--financial-status …] [--fulfillment-status …] [--customer-id <id>] [--since <t>] [--until <t>]` |
| Count orders | `orders +count` — same filters as `+search` except `--keyword` / `--customer-id` |
| Ship an order (发货) | `orders +ship --order-id <id> --tracking <no> [--company <name>] [--company-code <code>] [--line-items id:qty,…] [--notify]` |
| Refund a buyer (退款; money → `--dry-run` first) | `orders +refund --order-id <id> --amount <n> [--note <txt>] [--return-items] [--payment-line-id <id>]` |
| Fix / update tracking info | `orders +update-tracking --order-id <id> --fulfillment-id <id> --tracking <no> [--company <name>] [--tracking-url <url>] [--notify]` |
| Get one order by internal ID | `orders get --params '{"order_id":"<id>"}'` |
| Get order by order NUMBER (#…) | `orders get-by-number --params '{"number":"<number>"}'` |
| List with filters `+search` lacks (multi-status arrays, ids, skus, tags, fuzzy search…) | `orders list --params '{…}'` (see `schema orders.list`) |
| Create an order on a customer's behalf (手工建单 / 代客下单) | `orders create --data '{"order":{…}}'` — heavy nested body → [references/orders-create.md](references/orders-create.md) |
| Update order note / tags / shipping address | `orders update --params '{"order_id":"<id>"}' --data '{"order":{…}}'` |
| Cancel an order (destructive → `--dry-run` first) | `orders cancel --params '{"order_id":"<id>"}' [--data '{"reason":"…"}']` |
| Mark an order paid | `orders pay --params '{"order_id":"<id>"}' [--data '{…}']` (no body = bogus test payment) |
| Delete an order (destructive → `--dry-run` first) | `orders delete --params '{"order_id":"<id>"}'` |
| Fulfillment records CRUD / cancel / complete | `orders fulfillments …` → [references/fulfillments.md](references/fulfillments.md) |
| Refund records (one order / store-wide) | `orders refunds …` → [references/refunds.md](references/refunds.md) |
| Fraud risk records (标记高风险) | `orders risks …` → [references/risks.md](references/risks.md) |
| Payment transactions on an order (支付流水) | `orders transactions list --params '{"order_id":"<id>"}'` → [references/transactions.md](references/transactions.md) |
| After-sales records (售后) | `orders post-sales …` → [references/post-sales.md](references/post-sales.md) |
| Shipping zones / rates / available lines (运费区域) | `orders shipping-schemas …` → [references/shipping-schemas.md](references/shipping-schemas.md) |
| Which carrier is this tracking number? (承运商识别) | `orders tracking-carriers detect --params '{"tracking_number":"<no>"}'` → [references/tracking-carriers.md](references/tracking-carriers.md) |

## Acting on a request

"发货 / 退款 / 取消订单 / ship / refund / cancel" is an **action**, not a question:

1. **Match intent to a shortcut** via the trigger table below (sub-resource leaves are in
   `references/`).
2. **Check required fields** (each shortcut's *no-default* flags). Missing or ambiguous →
   **ask with `AskUserQuestion`, never fabricate** — especially `--amount` (money) and
   `--tracking` (a real-world tracking number you cannot invent). Bundle missing fields into
   ONE question batch.
3. **Never ask about a flag that has a CLI default or is omit-to-disable.** Let the default win.
4. **All required present** → execute, with safety per shoplazza-common:
   - **`+refund` moves money — ALWAYS `--dry-run` first**, restate in plain language (which
     order, how much, note, inventory returned or not), wait for the user's go-ahead, then
     run for real.
   - **Destructive leaves** (`orders cancel` / `orders delete` / `fulfillments cancel` /
     `risks delete` / `post-sales delete` / `shipping-schemas delete-zone`) — `--dry-run`
     first + restate, wait for the user's go-ahead, then run.
   - `+ship` / `+update-tracking` / `+search` / `+count` are safe to run directly once
     required values are confirmed.

### Trigger phrase → shortcut

| User says | Shortcut | How to extract values |
|---|---|---|
| 查订单 / 找订单 / 今天有没有新订单 / search or list orders | `+search` | order no / customer name / email → `--keyword`; "今天/最近 N 天" → `--since` only (leave `--until` open); status words → the enum table below |
| 还没发货的订单 / unshipped orders | `+search --fulfillment-status waiting` | 未发货 = `waiting` — there is **no** `unfulfilled` value |
| 某客户 / 某邮箱的订单 / orders from customer X | `+search` | customer id → `--customer-id`; an email → `--keyword <email>` (keyword matches email) — never stuff an id into `--keyword` |
| 有多少订单 / 订单总数 / how many orders | `+count` | "总共" = no filters, do not ask for a time range; count from `.data.count` |
| 发货 / 标记发货 / ship order X | `+ship` | tracking no → `--tracking` verbatim; carrier name → `--company`; "通知买家" → `--notify`; whole order = omit `--line-items` |
| 部分发货 / ship only some items | `+ship --line-items id:qty,…` | per-line quantities as `line-id:qty` pairs |
| 退款 / 给买家退 X 元 / refund the buyer | `+refund` | amount → `--amount` verbatim; stated reason → `--note`; "库存退回去/restock" → `--return-items` |
| 改运单号 / 运单号填错了 / fix the tracking number | `+update-tracking` | needs all three: `--order-id`, `--fulfillment-id`, `--tracking` |
| 订单号 #… 的详情 / look up order by number | `get-by-number` (leaf) | strip the leading `#` → `--params '{"number":"…"}'`; internal IDs go to `get` instead |
| 手工建单 / 代客下单 / create an order for a customer | `create` (leaf) | heavy nested body under `order` → [references/orders-create.md](references/orders-create.md); resolve `variant_id` via `products +search`; never fabricate buyer identity, address, or price |
| 取消订单 / cancel order X | `cancel` (leaf) | id → `--params`; a stated reason → `--data '{"reason":"…"}'` (optional); **not** `fulfillments cancel` |
| 取消发货记录 / cancel a fulfillment | `fulfillments cancel` (leaf) | both `order_id` and `fulfillment_id` in `--params`; **not** `orders cancel` |
| 标记高风险 / 像欺诈单 / flag as fraud | `risks create` (leaf) | 高/中/低 → `level` `high`/`medium`/`low`; stated reason → `details` array → [references/risks.md](references/risks.md) |
| 订单支付流水 / payment transactions | `transactions list` (leaf) | order id → `--params`; this is buyer payments, not billing |
| 运单号 X 是哪家承运商 / which carrier | `tracking-carriers detect` (leaf) | `--params '{"tracking_number":"…"}'` — don't `list` + match by hand |
| 设置运费区域 / 邮费 / shipping zones & rates | `shipping-schemas` leaves | → [references/shipping-schemas.md](references/shipping-schemas.md) |

If multiple rows match, **ask which one**. Don't guess.

### Required-vs-ask matrix

Only **no-default** flags are askable.

| Shortcut | Must ASK if user did not specify | Infer if possible | Default silently |
|---|---|---|---|
| `+refund` | `--amount` (**money — never fabricate**; you may offer to look up the order total via `orders get` for a full refund, but still confirm the number); `--order-id` if absent | `--note` from the stated reason; `--return-items` from 库存退回/restock wording | `--payment-line-id` (only needed when the order has multiple payment lines — the CLI errors with candidates; pick or ask then) |
| `+ship` | `--tracking` (**never invent a tracking number**); `--order-id` if absent | `--company` from a named carrier (keep the user's naming); `--notify` from 通知买家 wording | `--line-items` (omit = all fulfillable items), `--company-code` |
| `+update-tracking` | whichever of `--order-id` / `--fulfillment-id` / `--tracking` is missing | `--company`, `--tracking-url` only if the user gave them | `--notify` (set only on explicit re-notify wording) |
| `+search` / `+count` | *(nothing — all filters optional; never ask)* | filters from wording via the enum table | `--page-limit`; time bounds only from the user's own time words |
| `cancel` (leaf) | order id if absent | `reason` body only if the user stated one | `reason` (optional — omit if not given) |
| `create` (leaf) | any missing required buyer/address sub-field (`last_name`, `email`, `country_code`, `province_code`, `city`, `address`, `zip`) and the line item(s) — **never fabricate identity, address, or price**; ask or look up `variant_id` via `products +search` | `currency_code` from the store (`shop info get --jq '.data.currency'`); a neutral `shipping_name` label ("Standard Shipping"); an omitted catalog `price` (the variant prices it) | notify flags, `tags`, `note`, `discount_application`, `payment_line` (record payment via `orders pay`) |

### Never-ask list

Flags with a CLI default or omit-to-disable — they never appear in a question:
`--notify` (omit unless the user asks to notify) · `--line-items` (default: all fulfillable) ·
`--company` / `--company-code` (optional carrier info) · `--payment-line-id` (only after a
multi-payment-line error) · `--note` (only if a reason was given) · `--return-items` (explicit
wording only) · `--tracking-url` · `--page-limit` · `--since` / `--until` (only from the
user's own time words — never invent a time range).

### Decision examples

| User says | Verdict |
|---|---|
| "订单 111222 退 15.50 给买家，备注 wrong size，库存退回" | REFUND — `+refund --order-id 111222 --amount 15.50 --note "wrong size" --return-items`; `--dry-run` first, restate, wait for go-ahead, then run |
| "给订单 333444 退个款" | ASK — the amount (money; never invent). Do NOT ask which order — it's in the utterance. May offer a full-refund lookup, still confirm |
| "订单 555 可以发货了，运单号 YT123，中通，通知买家" | SHIP — `+ship --order-id 555 --tracking YT123 --company 中通 --notify`; safe to run directly |
| "把订单 666 标记发货" | ASK — the tracking number (required, never fabricate); carrier is optional, offer alongside but don't block on it |
| "找最近 7 天还没发货的订单" | SEARCH — `+search --fulfillment-status waiting --since <today-7d>`; no `--until` |
| "取消订单 777，原因是买家申请" | CANCEL — `orders cancel --params '{"order_id":"777"}' --data '{"reason":"买家申请"}'`; `--dry-run` first + restate |
| "订单 888 像欺诈单，标成高风险" | RISK — `orders risks create --params '{"order_id":"888"}' --data '{"risk":{"level":"high","details":["…"]}}'`; restate before writing |
| "手工帮客户下个单：1 件 variant V123，单价 29.9，寄给 John Doe (john@x.com)，美国加州洛杉矶 123 Main St 90012" | CREATE — build the nested `order` body (line_items `variant_id`+`quantity`+`price`, full `shipping_address` incl. `province_code`, `shipping_line`, `tax_total`, `currency_code`), restate + `--dry-run` first → [references/orders-create.md](references/orders-create.md) |
| "帮客户下个单" (no items / buyer given) | ASK — which variant(s) & quantity, and the buyer's shipping details (name, email, address); **never fabricate** them. `shipping_price`/`tax_total` are money — use stated amounts, or `"0.00"` only when the user implies no charge and restate that |

## Status enums (memorize these three)

From `schema orders.list` — used by `+search` / `+count` / `list`:

| Field | Values |
|---|---|
| `--status` (order) | `opened` (pending payment) · `placed` (in progress) · `finished` · `cancelled` |
| `--financial-status` | `waiting` · `paying` · `authorized` · `partially_paid` · `paid` · `cancelled` · `failed` · `refunding` · `refund_failed` · `refunded` · `partially_refunded` |
| `--fulfillment-status` | `initialled` · `waiting` (**未发货 / waiting to ship**) · `partially_shipped` · `shipped` · `partially_finished` · `finished` · `cancelled` · `returning` · `partially_returned` · `returned` |

**`unfulfilled` / `fulfilled` are NOT valid values** — "还没发货" maps to `waiting`.
Each shortcut flag takes ONE value; to filter on several at once use the `list` leaf, whose
`status` / `financial_status` / `fulfillment_status` params are JSON arrays.
`+search --since/--until` bound the **placed** time (`placed_at_min`/`placed_at_max`),
ISO date or unix ts.

## Boundaries

Reads like orders, actually belongs elsewhere (and lookalikes this domain owns):

| Sounds like | Actually belongs to | Command |
|---|---|---|
| 退款 / 给顾客退钱 / refund a buyer | **HERE** (not discounts — 折扣/优惠码 price the cart; not billing — app charges) | `orders +refund` |
| 订单支付流水 / buyer payment transactions | **HERE** | `orders transactions list` |
| 应用收费流水 / app-charge transactions billed to the merchant | `shoplazza-billing` | `billing transactions` |
| 快递承运商查询 / 运单号识别 / tracking carrier lookup | **HERE** | `orders tracking-carriers {detect,list}` |
| 注册实时运费报价承运商 / real-time rate quote carriers | `shoplazza-shop` | `shop carrier-services` |
| 运费区域 / 邮费方案 / shipping zones & rates | **HERE** | `orders shipping-schemas …` |
| 按邮箱查某客户的订单 / a customer's orders | **HERE** (`+search --keyword <email>` or `--customer-id`) | resolving the id via `customers +search` first is fine — the final query is an orders query |
| 客户资料本身 / the customer record | `shoplazza-customers` | `customers +search` / `customers get` |
| 库存 / 补货 / product stock | `shoplazza-products` | `products +stock` / `products inventory` |
| 退款时库存退回 / restock on refund | **HERE** | `orders +refund --return-items` |

## Permissions · Scope

Authorization is by **domain** (the unit of `--domain`), not per command. Authorization flow is
in shoplazza-common → Authentication.

| Operation | Needs | Grant |
|---|---|---|
| read (`get` / `get-by-number` / `list` / `+search` / `+count` / sub-resource reads) | `orders` read scope | `auth login --domain orders` |
| write (`+ship` / `+refund` / `+update-tracking` / `create` / `update` / `cancel` / `pay` / `delete` / sub-resource writes) | `orders` write scope | `auth login --domain orders` |

Look up exact scope literals with `shoplazza auth scopes`; `--domain orders` expands into them.

## Gotchas

Domain-specific pitfalls only (generic ones — `.data` prefix, `--jq` without `-r` — are in
shoplazza-common).

| Symptom | Cause | Fix |
|---------|-------|-----|
| Filtered on `unfulfilled` / `fulfilled` | Not in the enum | 未发货 = `waiting`; see the enum table above |
| `orders get` finds nothing for a `#…` number | `get` takes the internal `order_id`; order numbers are a different key | `get-by-number --params '{"number":"…"}'` (strip the `#`) |
| `+refund` errors "order has N payment_lines" | Multi-payment-line order needs an explicit choice | Re-run with `--payment-line-id` from the listed candidates |
| `+refund` / `+ship` dry-run prints TWO requests | These shortcuts GET the order first (resolve payment lines / fulfillable items), then POST | Expected. The dry-run POST body omits looked-up data (e.g. `refund_line_items`, empty `payment_line_id`) — the live run fills them in |
| Cancelled the order when only a shipment should die (or vice versa) | 取消订单 ≠ 取消发货记录 | Order → `orders cancel`; fulfillment → `orders fulfillments cancel` (both path ids) |
| Hand-built `refunds create` body for a simple refund | The leaf needs the full body incl. `refund_payments` | Prefer `+refund`; leaf only for split/itemized refunds → [references/refunds.md](references/refunds.md) |
| Store-wide `refunds list` for one order's records | Two different endpoints | One order → `refunds list-by-order --params '{"order_id":"…"}'` |
| `--status` with a comma list on `+search` | Shortcut flags are single-valued | Multi-status → `orders list --params '{"status":["opened","placed"]}'` |
| `unknown flag: --fields` | No orders command has `--fields` | Project with `--jq` |
| `orders create` rejected for missing fields | Whole body nests under an `order` wrapper; `line_items[].quantity`, `shipping_address` (last_name, email, country_code, province, province_code, city, address, zip), `shipping_line` (shipping_name, shipping_price), `tax_total`, `currency_code` are ALL required | Build the full nested body → [references/orders-create.md](references/orders-create.md) (verified against `schema orders.create --view request`) |
| `orders create` order fails despite `country`/`province` set | `country_code` / `province_code` (ISO: `US`, `CA`) are the required keys — the display-name `country` / `province` are optional | Send both codes; see [references/orders-create.md](references/orders-create.md) |
| Copied the `orders pay` `payment_line` object into `orders create` | In `orders create` the schema types `payment_line` as a **string** (registry flattening), unlike `orders pay` where it is an object | Omit `payment_line` on create; record payment afterward with `orders pay` |
| `orders pay` with no body "succeeded" unexpectedly | Empty body = bogus **test** payment | Pass `payment_line` (payment_channel, payment_method, transaction_no) for a real channel, or `gateway` for a custom method |
| Schema summaries say "shopping zone" | Registry typo — they mean *shipping* zone | Ignore; commands are `shipping-schemas {create,update,delete}-zone` |

## Recipes

```bash
# 1. Orders placed in the last 7 days that are not yet shipped
orders +search --fulfillment-status waiting --since <today-7d, e.g. 2026-07-16>

# 2. Paid orders for one customer
orders +search --customer-id 100200 --financial-status paid

# 3. All orders from a buyer email
orders +search --keyword alice@example.com

# 4. Total order count, no filters
orders +count --jq '.data.count'

# 5. Order detail by order number
orders get-by-number --params '{"number":"2026071500123"}' --jq '.data.order'

# 6. Ship the whole order and notify the buyer
orders +ship --order-id 4300123 --tracking SF0011223344 --company "顺丰" --notify

# 7. Fix a mistyped tracking number (no re-notify)
orders +update-tracking --order-id 991100 --fulfillment-id 77001 --tracking YT5544332211

# 8. Partial refund + return inventory — dry-run, restate, wait for go-ahead, then run
orders +refund --order-id 887799 --amount 19.90 --note "wrong size" --return-items --dry-run
orders +refund --order-id 887799 --amount 19.90 --note "wrong size" --return-items

# 9. Cancel an order with a reason — dry-run first
orders cancel --params '{"order_id":"660011"}' --data '{"reason":"买家重复下单"}' --dry-run
orders cancel --params '{"order_id":"660011"}' --data '{"reason":"买家重复下单"}'

# 10. Flag an order as high fraud risk with a reason
orders risks create --params '{"order_id":"445500"}' \
  --data '{"risk":{"level":"high","details":["收货地址与 IP 所在国家不一致"]}}'

# 11. Which carrier owns this tracking number?
orders tracking-carriers detect --params '{"tracking_number":"9400111899223344556677"}'

# 12. Payment transactions on an order
orders transactions list --params '{"order_id":"778899"}'

# 13. One order's refund records
orders refunds list-by-order --params '{"order_id":"220033"}' --jq '.data.records'
```

## References

- [references/orders-create.md](references/orders-create.md) — manual/draft order creation: full nested body (required vs optional), required-vs-ask, minimal + fuller examples
- [references/fulfillments.md](references/fulfillments.md) — fulfillment records: leaf CRUD, cancel/complete, body shapes
- [references/refunds.md](references/refunds.md) — refund record body (split/itemized refunds), store-wide vs per-order lists
- [references/risks.md](references/risks.md) — fraud risk records: level enum, details, CRUD
- [references/transactions.md](references/transactions.md) — buyer payment transactions, status enum
- [references/post-sales.md](references/post-sales.md) — after-sales records: list filters, delete
- [references/shipping-schemas.md](references/shipping-schemas.md) — general schema, zones, available shipping lines (GB special case)
- [references/tracking-carriers.md](references/tracking-carriers.md) — carrier detect / list
- Per-command flags: `shoplazza orders <cmd> --help`
- Spec-leaf params / body / response: `shoplazza schema orders.<cmd>` (nested: `schema orders.fulfillments.create`)
- Cross-cutting mechanics (auth / tiers / output envelope / `--dry-run` / `--jq` / safety): `../shoplazza-common/SKILL.md`
- Shortcut source of truth: `shortcuts/orders/*.go`

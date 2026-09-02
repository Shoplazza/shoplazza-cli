# orders refunds — refund records

Money returned to a buyer for one order — full or partial, covering products, shipping, tips,
and fees, across one or more payment lines. **For a simple "refund X to the buyer" use
`orders +refund`** (SKILL.md; `--dry-run` first — it moves money). Use the `refunds create`
leaf only for split / itemized refunds the shortcut can't express.

## Commands

| Intent | Command |
|---|---|
| One order's refund records | `orders refunds list-by-order --params '{"order_id":"<id>"}'` → `.data.records[]` + `.data.order_financial_status` + `.data.order_status` |
| Store-wide refund records | `orders refunds list --params '{…}'` → `.data.records[]` (cursor; `page_size` 1-100, default 10) |
| Count store-wide records | `orders refunds count --params '{…}'` → `.data.count` |
| Create a refund record (money → `--dry-run` first) | `orders refunds create --params '{"order_id":"<id>"}' --data '{"refund":{…}}'` → `.data.refund_record_id` + `.data.post_sale_id` |
| Finish an in-progress refund (write → `--dry-run` first) | `orders refunds finish --params '{"order_id":"<id>"}' --data '{"post_sale_id":"<id>","refund_record_id":"<id>"}'` → `.data.refund_record` |

Store-wide `list` / `count` filters: `order_ids` (≤10) · `refund_order_ids` (≤20) ·
`refund_statuses` (enum `pending` in progress / `finished` / `failed`) · `sort_by`
(`created_at`/`updated_at`) · `sort_direction` (`asc`/`desc`) · `created_at_start/end` ·
`updated_at_start/end`.

**Per-order vs store-wide:** "订单 X 的退款记录" → `list-by-order` (path param). The
store-wide `list` is for cross-order dashboards.

## `refunds create` body (`schema orders.refunds.create`)

```json
{"refund": {
  "refund_total": "29.99",
  "refund_shipping_total": "0", "refund_tip": "0",
  "refund_additional_total": "0", "refund_product_total": "29.99",
  "refund_line_items": [
    {"line_item_id": "<id>", "refund_item_type": "auto", "quantity": 1, "return_inventory": true}
  ],
  "refund_payments": [
    {"payment_line_id": "<id>", "refund_price": "29.99"}
  ],
  "refund_additional_prices": [{"name": "…", "price": "…"}],
  "note": "…"
}}
```

- Required: `refund_total`; per line item `line_item_id`; per payment `payment_line_id` +
  `refund_price`. Everything else optional.
- `refund_item_type` enum: `auto` · `shipped` · `waiting_ship`.
- Payment line ids come from `orders get` → `.data.order.payment_lines[].id`.
- Creating a refund also creates an after-sales record (`post_sale_id` in the response) —
  see [post-sales.md](post-sales.md).
- Refund amounts are money: never fabricate; confirm with the user, `--dry-run` first.

## `refunds finish` — mark an in-progress refund as refunded

Marks the order's in-progress refund records as refunded successfully; the order then becomes
partially or fully refunded. **Only for orders paid through a custom payment channel or the
test channel (`bogus`)** — platform-settled channels finish on their own.

- Required: path `order_id`; body `post_sale_id` + `refund_record_id` (both returned by
  `refunds create` / `+refund`, or from `refunds list-by-order` records).
- Optional body: `refund_time` (RFC3339 third-party completion time), `transaction_number`,
  `payment_channel`, `extra_info` (`{"pos":"…"}`).
- Finalizes a refund's status — restate and `--dry-run` first like other order writes.

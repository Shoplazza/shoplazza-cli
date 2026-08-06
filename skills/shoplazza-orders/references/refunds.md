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

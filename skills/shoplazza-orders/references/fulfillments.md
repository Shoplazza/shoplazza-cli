# orders fulfillments — shipment records

A fulfillment is one shipment of items on an order (tracking number + carrier). For the two
high-frequency flows use the shortcuts in SKILL.md: **create → `orders +ship`**, **edit
tracking → `orders +update-tracking`**. Reach for these leaves for everything else.

## Commands

| Intent | Command |
|---|---|
| List an order's fulfillments | `orders fulfillments list --params '{"order_id":"<id>"}'` → `.data.fulfillments[]` (cursor pagination; `page_size` 1-250, default 10; `created_at_min/max`, `updated_at_min/max`) |
| Count them | `orders fulfillments count --params '{"order_id":"<id>"}'` → `.data.count` (same time filters) |
| Get one | `orders fulfillments get --params '{"order_id":"<id>","fulfillment_id":"<fid>"}'` |
| Create (prefer `+ship`) | `orders fulfillments create --params '{"order_id":"<id>"}' --data '{"fulfillment":{…}}'` |
| Update (prefer `+update-tracking`) | `orders fulfillments update --params '{"order_id":"<id>","fulfillment_id":"<fid>"}' --data '{"fulfillment":{…}}'` |
| Complete (mark delivered) | `orders fulfillments complete --params '{"order_id":"<id>","fulfillment_id":"<fid>"}'` (no body; already-finished returns the details) |
| Cancel a fulfillment (destructive → `--dry-run` first) | `orders fulfillments cancel --params '{"order_id":"<id>","fulfillment_id":"<fid>"}'` (no body; NOT `orders cancel`) |

## Body shapes (`schema orders.fulfillments.<cmd>`)

`create` — `fulfillment` object, **`line_items` required** (unlike `+ship`, which auto-fills
all fulfillable items from the order):

```json
{"fulfillment": {
  "line_items": [{"id": "<line-item-id>", "ship_quantity": 1}],
  "tracking_number": "…", "tracking_company": "…",
  "tracking_company_code": "…", "tracking_url": "…", "phone_number": "…"
}}
```

`id` is the only required line-item field; get line-item ids from `orders get`
(`.data.order.line_items[].id`). `+ship` also validates qty ≤ `fulfillable_quantity` — the
leaf does not, so check yourself.

`update` — all fields optional; send only what changes:
`tracking_number` · `tracking_company` · `tracking_company_code` · `tracking_url` ·
`send_email` (bool — re-notify the customer) · `phone_number`.
`--notify` on the shortcuts maps to `send_email: true`.

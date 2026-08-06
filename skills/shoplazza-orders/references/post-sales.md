# orders post-sales — after-sales records

After-sales (售后) records associated with orders — returns, exchanges, refund aftermath.
Creating a refund (`+refund` / `refunds create`) automatically creates one; its id comes back
as `post_sale_id`.

## Commands

| Intent | Command |
|---|---|
| List after-sales records | `orders post-sales list --params '{…}'` → `.data.orders[]` (cursor; `page_size` default 20) |
| Delete one (destructive → `--dry-run` first) | `orders post-sales delete --params '{"post_sale_id":"<id>"}'` |

`list` filters: `created_at_min` / `created_at_max` (ISO time) · `status` — enum `pending` ·
`processing` · `finished`.

Note: the list is store-wide (no `order_id` param). To trace one order's after-sales, start
from the `post_sale_id` returned by its refund creation, or from
`refunds list-by-order` records.

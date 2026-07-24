# products suppliers · procurement — reference

Supply-side operations: supplier profiles and procurement (purchase) orders submitted to
them. Receiving a procurement is one of the legitimate ways stock increases — remember the
platform-wide rule: **inventory only goes up** (see
[inventory-locations.md](inventory-locations.md)).

## Suppliers

Subcommands: `create` · `get` · `list` · `update` (no delete).
Verify shapes with `shoplazza schema products.suppliers.<cmd>`.

| Command | HTTP | Params / body |
|---|---|---|
| `suppliers create` | `POST /suppliers` | `--data '{"supplier":{"title":"…"}}'` — `title` is the only required field (`url` optional) |
| `suppliers get` | `GET /suppliers/{id}` | `--params '{"id":"<id>"}'` |
| `suppliers list` | `GET /suppliers` | |
| `suppliers update` | `PUT /suppliers/{id}` | `--params` + `--data '{"supplier":{…}}'` |

## Procurement orders

Lifecycle: **create → add items → receive (or cancel)**.
Verify shapes with `shoplazza schema products.procurement.<cmd>`.

| Command | HTTP | Params / body |
|---|---|---|
| `procurement create` | `POST /procurements` | `--data '{"procurement":{"supplier_id":"…"}}'` — `supplier_id` required, `note` optional |
| `procurement get` / `list` | `GET /procurements[/{procurement_id}]` | |
| `procurement update` | `PUT /procurements/{procurement_id}` | |
| `procurement cancel` | `PATCH /procurements/{procurement_id}/cancel` | destructive → `--dry-run` first |
| `procurement list-items` | `GET /procurements/{procurement_id}/items` | |
| `procurement batch-create-items` | `POST /procurements/{procurement_id}/items` | `items[]`: `product_id` + `variant_id` + `transfer_quantity` — all three required per item |
| `procurement batch-update-items` | `PUT /procurements/{procurement_id}/items` | |
| `procurement batch-delete-items` | `DELETE /procurements/{procurement_id}/items` | destructive → `--dry-run` first |
| `procurement receive` | `PATCH /procurements/{procurement_id}/receive` | `items[]`: `procurement_item_id` required; `received_quantity` / `rejected_quantity` / `rejected_reason` optional |

```bash
# 1. Open a procurement against a supplier
products procurement create --data '{"procurement":{"supplier_id":"SUP1","note":"Q3 restock"}}'

# 2. Add line items (quantities from the user — never invented)
products procurement batch-create-items --params '{"procurement_id":"PROC1"}' \
  --data '{"items":[{"product_id":"P1","variant_id":"V1","transfer_quantity":100}]}'

# 3. Receive it — this is what actually books the stock in
products procurement receive --params '{"procurement_id":"PROC1"}' \
  --data '{"items":[{"procurement_item_id":"ITEM1","received_quantity":100}]}'
```

`transfer_quantity` and `received_quantity` are ordered/received goods counts — treat them
like money fields: take them from the user's own words or documents, never default them.

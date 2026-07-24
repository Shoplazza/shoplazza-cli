# products inventory · locations — reference

Inventory tracks stock per **inventory item** per **location**. For the everyday case
("give variant X more stock") use the `products +stock` shortcut (SKILL.md) — it resolves the
item and location for you. Come here for multi-location work or item-level settings.

## ⚠️ The whole surface is ADD-ONLY

There is **no way to decrease stock** through the CLI or the underlying API:

- `+stock --adjust` must be > 0 (the API rejects 0 and negative values).
- `+stock --set` sets an absolute target but is **increase-only** — a target below the
  current level cannot be applied.
- The `inventory set-stock` (POST `/inventory_levels/set`) and `inventory update-level`
  (PUT `/inventory_levels`) leaves **also only add** — despite names like "Set inventory
  quantity", both increase the level.

A "reduce stock by N" request must **degrade**: emit no write, explain the limitation, and
offer alternatives (`products +unpublish`, inventory policy `deny` when stock hits 0).
Never fabricate a negative `--adjust` or an under-current `--set`.

## Concepts

| Term | Keyed by | Meaning |
|---|---|---|
| Inventory **item** | `inventory_item_id` | The stockable thing behind a variant (tracking on/off, policy) |
| Inventory **level** | `inventory_item_id` + `location_id` | The quantity of one item at one location |
| Location | `location_id` | Warehouse / retail site |

`inventory_item_id` ≠ `variant_id`. Map variant → item first:

```bash
products inventory list-by-variant --params '{"variant_ids":["12345"]}'
```

## Inventory commands

Verify shapes with `shoplazza schema products.inventory.<cmd>`.

| Command | HTTP | Notes |
|---|---|---|
| `inventory list-by-variant` | `GET /inventory_items/variant` | `variant_ids` (query array, required) — the variant→item bridge |
| `inventory list-items` | `GET /inventory_items` | `inventory_item_ids` required |
| `inventory get-item` | `GET /inventory_items/{inventory_item_id}` | |
| `inventory update-item` | `PUT /inventory_items/{inventory_item_id}` | body `inventory_item.tracking` (bool), `.tracking_policy` |
| `inventory list-levels` | `GET /inventory_levels` | filter by `inventory_item_ids` / `location_ids` |
| `inventory set-stock` | `POST /inventory_levels/set` | body `inventory_item_id`, `location_id`, `stock` — **adds** |
| `inventory update-level` | `PUT /inventory_levels` | body `inventory_item_id`, `location_id`, `stock_adjustment` — **adds** |
| `inventory create-level` | `POST /inventory_levels` | connect an item to a location (body: both ids) |
| `inventory delete-level` | `DELETE /inventory_levels` | disconnect an item from a location (body: both ids) |

`delete-level` removes the item–location *connection*, it is not a stock decrement.

## Locations commands

| Command | HTTP | Notes |
|---|---|---|
| `locations list` | `GET /locations` | `status`, `sort_by`, cursor paging |
| `locations count` | `GET /locations/count` | |
| `locations get` | `GET /locations/{location_id}` | |
| `locations get-default` | `GET /locations/default` | the location `+stock` uses when `--location-id` is omitted |
| `locations set-default` | `POST /locations/default` | `location_id` as **query param** via `--params` |
| `locations deactivate` | `POST /locations/deactivate` | query params `location_id`, `target_location_id` (where remaining inventory goes) |
| `locations edit-priority` | `POST /locations/priority` | see `schema products.locations.edit-priority` |
| `locations inventory-levels` | `GET /locations/{location_id}/inventory_levels` | all levels at one location |

## Recipes

```bash
# Stock of variant 12345 across all locations
ITEM=$(products inventory list-by-variant --params '{"variant_ids":["12345"]}' \
  --jq '.data.variant_inventory_items[0].inventory_item_id')
products inventory list-levels --params "{\"inventory_item_ids\":[\"$ITEM\"]}"

# Add 50 units at a specific location (shortcut — preferred)
products +stock --variant-id 12345 --adjust 50 --location-id 777

# Turn off inventory tracking for an item
products inventory update-item --params "{\"inventory_item_id\":\"$ITEM\"}" \
  --data '{"inventory_item":{"tracking":false}}'
```

(Response field paths verified against `schema … --view response`; re-check before scripting
against a different CLI version.)

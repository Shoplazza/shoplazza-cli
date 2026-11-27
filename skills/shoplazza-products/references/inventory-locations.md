# products inventory · locations — reference

Inventory tracks stock per **inventory item** per **location**. For the everyday case
("give variant X more/less stock") use the `products +stock` shortcut (SKILL.md) — it resolves
the item and location for you. Come here for multi-location work or item-level settings.

## ⚠️ Both level writes are deltas — know the routes

Whatever their names say, both `inventory_levels` stock writes add to the level (verified
live): `inventory update-level` (PUT `/inventory_levels`) adds `stock_adjustment`, which must
be > 0; `inventory set-stock` (POST `/inventory_levels/set`) adds its signed `stock`, so a
negative value subtracts and 0 changes nothing. Neither sets an absolute value. Where the
item is not stocked yet, `set-stock` creates the level and `update-level` returns 404
`inventory level not found`. `products +stock` wraps both:

- `+stock --adjust N`: positive adds via `update-level`, negative subtracts via `set-stock`.
- `+stock --set N`: reads the current level and writes the difference the same way.
- Both work at any location where the variant is stocked (`--location-id`, else the default
  location); anywhere else `+stock` refuses and prints the `inventory create-level` command
  that connects it. Stock may go below 0: the platform stores negative stock.
- Any change is **refused** before writing when the product does not track inventory
  (`inventory_item.tracking` off): both level writes would return 400, and `variants update`
  silently ignores `inventory_quantity`.

**Zero reads back as absent**: `+stock --set 0` sets a tracked variant to 0 and tracking stays
on (verified live), but the API omits zero values, so `stock` / `inventory_quantity` come back
missing (`null`). That is 0, not untracked: read tracking from `inventory_item.tracking` /
`product.inventory_tracking` (also omitted when false).

`variants update` with `inventory_quantity` is a third route: it sets the **default location's**
level to that value. The variant's `inventory_quantity` reads back as the **total** across
locations, so on a multi-location item it shows the value plus the other locations' stock
(verified live). Prefer `+stock`.

## Concepts

| Term | Keyed by | Meaning |
|---|---|---|
| Inventory **item** | `inventory_item_id` | The stockable thing behind a variant (tracking on/off, policy) |
| Inventory **level** | `inventory_item_id` + `location_id` | The quantity of one item at one location |
| Location | `location_id` | Warehouse / retail site |

`inventory_item_id` ≠ `variant_id`. Map variant → item first:

```bash
products inventory list-by-variant --variant-ids 12345
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
| `inventory set-stock` | `POST /inventory_levels/set` | body `inventory_item_id`, `location_id`, `stock` — **adds** the signed value (negative subtracts) |
| `inventory update-level` | `PUT /inventory_levels` | body `inventory_item_id`, `location_id`, `stock_adjustment` — **adds**, > 0 only |
| `inventory create-level` | `POST /inventory_levels` | connect an item to a location (body: both ids); the new level starts at 0 |
| `inventory delete-level` | `DELETE /inventory_levels` | disconnect an item from a location (body: both ids) |

`delete-level` removes the item–location connection and discards the stock held there;
reconnecting starts from 0. An item must stay stocked at one location at least.

## Locations commands

| Command | HTTP | Notes |
|---|---|---|
| `locations list` | `GET /locations` | `status` as a number (0 all, 1 active, 2 inactive), `sort_by`, cursor paging |
| `locations count` | `GET /locations/count` | |
| `locations get` | `GET /locations/{location_id}` | |
| `locations get-default` | `GET /locations/default` | the location `+stock` uses when `--location-id` is omitted |
| `locations set-default` | `POST /locations/default` | `--location-id` (a **query param**) |
| `locations deactivate` | `POST /locations/deactivate` | query params `location_id`, `target_location_id` (where remaining inventory goes) |
| `locations edit-priority` | `POST /locations/priority` | see `schema products.locations.edit-priority` |
| `locations inventory-levels` | `GET /locations/{location_id}/inventory_levels` | all levels at one location |

## Recipes

```bash
# Stock of variant 12345 across all locations
ITEM=$(products inventory list-by-variant --variant-ids 12345 \
  --jq '.data.variant_inventory_items[0].inventory_item_id')
products inventory list-levels --inventory-item-ids "$ITEM"

# Add 50 units at a specific location (shortcut — preferred)
products +stock --variant-id 12345 --adjust 50 --location-id 777

# Same, when all you have is the product id (single-variant product; a
# multi-variant one is refused with its candidates listed)
products +stock --product-id 7ab09fcd-2ee7-4118-8c4d-e44a10d51caf --adjust 50

# Decrease by 30 at the default location
products +stock --variant-id 12345 --adjust -30

# Turn off inventory tracking for an item
products inventory update-item --inventory-item-id "$ITEM" \
  --data '{"inventory_item":{"tracking":false}}'
```

(Response field paths verified against `schema … --view response`; re-check before scripting
against a different CLI version.)

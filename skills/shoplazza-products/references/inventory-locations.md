# products inventory · locations — reference

Inventory tracks stock per **inventory item** per **location**. For the everyday case
("give variant X more/less stock") use the `products +stock` shortcut (SKILL.md) — it resolves
the item and location for you. Come here for multi-location work or item-level settings.

## ⚠️ Decreases are gated — know the two write routes

The `inventory_levels` write leaves **only add**: `inventory set-stock` (POST
`/inventory_levels/set`) and `inventory update-level` (PUT `/inventory_levels`) both
increase the level despite names like "Set inventory quantity", and both reject
`stock_adjustment ≤ 0`.

Decreases exist, but ride a different primitive: `PUT /variants/{variant_id}` with
`variant.inventory_quantity` — an **absolute set** (not a delta) that lands on the
**default location**. `products +stock` wraps both routes and gates the decrement:

- `+stock --adjust N`: positive adds at any location; negative decreases.
- `+stock --set N`: absolute target (≥ 0), either direction.
- A decrease is **refused** by the CLI when: the result would go below 0 (the API would
  happily store a negative — never let it), the effective `--location-id` is not the
  default location, or the item holds levels at **multiple locations**
  (`inventory_quantity` semantics are only verified for single-location items).

**Tracked zero is unreachable** (all three routes verified live): `variant.inventory_quantity: 0`
**clears tracking** (reads back `null`, like an untracked variant) instead of setting zero;
`set-stock` with `stock: 0` returns `ok:true` but is **silently ignored**; `update-level`
rejects `stock_adjustment ≤ 0`. To "sell out" a variant, decrease to 1 and take the last
unit from another variant, or ask whether untracked (`null`) is acceptable — never claim 0 was set.

When the gate fires, degrade honestly: relay the CLI's reason, offer alternatives
(`products +unpublish`, inventory policy `deny` when stock hits 0). Never bypass the gate
by calling `variants update` with `inventory_quantity` directly on a multi-location item.

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

# Same, when all you have is the product id (single-variant product; a
# multi-variant one is refused with its candidates listed)
products +stock --product-id 7ab09fcd-2ee7-4118-8c4d-e44a10d51caf --adjust 50

# Decrease by 30 (default location, single-location item)
products +stock --variant-id 12345 --adjust -30

# Turn off inventory tracking for an item
products inventory update-item --params "{\"inventory_item_id\":\"$ITEM\"}" \
  --data '{"inventory_item":{"tracking":false}}'
```

(Response field paths verified against `schema … --view response`; re-check before scripting
against a different CLI version.)

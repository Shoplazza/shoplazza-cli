# products collections · collects · categories — reference

Three different things:

| Object | What it is | Commands |
|---|---|---|
| **collection** | The curated collection itself: title, image, SEO, sort order, smart rules | `products collections …` |
| **collect** | One product↔collection membership record | `products collects …` |
| **category** | Platform category tree for storefront navigation (read-only here) | `products categories list` |

"把商品加进合集 / add product to collection" = **`collects create`** — never `collections update`.

## Collections

Verify shapes with `shoplazza schema products.collections.<cmd>`.

| Command | HTTP | Notes |
|---|---|---|
| `collections create` | `POST /collections` | body `collection.title` is the **only required field** — do not fabricate description/SEO/image |
| `collections get` / `list` / `count` | `GET /collections[/{id}|/count]` | |
| `collections update` | `PUT /collections/{id}` | metadata of the collection, not its membership |
| `collections delete` | `DELETE /collections/{id}` | destructive → `--dry-run` first |
| `collections update-smart-rule` | `PATCH /collections/{id}/smart-rule` | smart collections only |
| `collections async-create` | `POST /collections/async` | large/smart collection creation as an async task |
| `collections async-update-smart-rule` | `PATCH /collections/{id}/smart-rule/async` | |
| `collections get-async-task` | `GET /collections/async-task/{id}` | poll an async task's status |

```bash
# Minimal curated collection
products collections create --data '{"collection":{"title":"Summer Sale"}}'
```

**Smart collections**: set `collection.smart: true` with `collection.match_rules`
(`disjunctive` + `rule_modules`) in the create body — products then join by rule instead of
by collect records. Membership of a smart collection is rule-managed; don't hand-edit it
with collects. Rule shapes: `schema products.collections.async-create --view request`.

## Collects (membership records)

| Command | HTTP | Notes |
|---|---|---|
| `collects create` | `POST /collects` | body `collect.collection_id` + `collect.product_id` — **both required, don't swap them** |
| `collects batch-create` | `POST /collects/batch` | body is flat: `collection_id` + `product_ids` (array) — one collection, many products |
| `collects list` / `get` / `count` | `GET /collects[/{id}|/count]` | response list nests at `.data.collects[]` |
| `collects delete` | `DELETE /collects/{id}` | removes the product from the collection (the collect record's own id, not a product id) |

```bash
# One product into one collection
products collects create --data '{"collect":{"collection_id":"999000","product_id":"555777"}}'

# Many products into one collection — note the FLAT body (no "collect" wrapper)
products collects batch-create --data '{"collection_id":"999000","product_ids":["1","2","3"]}'
```

Use `create` for a single association; reach for `batch-create` only with multiple products.

## Categories

```bash
products categories list --params '{"pid":"<parent-id>"}'   # children of a node; omit for roots
```

Query params: `pid` (parent id), `ids`. Categories are the platform taxonomy used by
`product.category_id` — they are not editable from this module.

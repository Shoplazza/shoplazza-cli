# products variants — reference

Variants are the sellable units (SKUs) of a product: price, options (size/color), stock,
weight, barcode. All commands here are spec leaves — for price changes prefer the
`products +set-price` shortcut, for stock the `products +stock` shortcut, and for anything
touching the option matrix (adding/removing spec values or dimensions, multi-spec creation)
the `products +set-variants` shortcut (all in SKILL.md). Never hand-write a `variants` array
in a `products update` body: it is a full replace — unlisted variants are deleted, and listed
variants without their `id` are recreated with sku/stock reset.
Each takes `--variant-id`, `--sku`, or `--product-id`, so you rarely need to resolve by hand.

## Command surface

Verify shapes with `shoplazza schema products.variants.<cmd>` before calling.

| Command | HTTP | Params / body |
|---|---|---|
| `variants list` | `GET /products/{product_id}/variants` | `--params '{"product_id":"<id>"}'` |
| `variants count` | `GET /products/{product_id}/variants/count` | `--params '{"product_id":"<id>"}'` |
| `variants get` | `GET /variants/{variant_id}` | `--params '{"variant_id":"<id>"}'` — **no product_id** |
| `variants create` | `POST /products/{product_id}/variants` | `--params '{"product_id":"<id>"}' --data '{"variant":{…}}'` |
| `variants update` | `PUT /variants/{variant_id}` | `--params '{"variant_id":"<id>"}' --data '{"variant":{…}}'` — **no product_id** |
| `variants delete` | `DELETE /products/{product_id}/variants/{variant_id}` | `--params '{"product_id":"<pid>","variant_id":"<vid>"}'` — needs **both** ids |
| `variants list-by-sku` | `GET /products/sku/{sku}/variants` | `--params '{"sku":"<sku>"}'` |
| `variants update-by-sku` | `PUT /variants/sku/{sku}` | `--params '{"sku":"<sku>"}' --data '{"refuse_multi_result":…,"variant":{…}}'` |

## Path scoping — the main footgun

`get` and `update` address a variant globally (`/variants/{variant_id}`); `list`, `create`,
and `delete` are product-scoped. Passing `product_id` to `update`, or omitting it on
`delete`, fails.

## Updating a variant

Send **only the fields to change**, nested under the required `variant` object. The schema
marks some fields (e.g. `price`) required on the body, but the CLI does not pre-validate
body fields and the endpoint accepts partial updates — do not pad the body with fields the
user never mentioned.

```bash
# Change option1 to XL — nothing else touched
products variants update --params '{"variant_id":"24680"}' \
  --data '{"variant":{"option1":"XL"}}'
```

Notable body fields (see `schema products.variants.update --view request` for all):
`option1/2/3`, `price`, `compare_at_price`, `sku`, `barcode`, `weight`, `weight_unit`,
`cost_price`, `note`, `image_id` / `image.src`, `inventory_quantity`.

`inventory_quantity` in an update body is an **absolute set** and CAN decrease stock — but
the write lands on the **default location**, and its semantics on multi-location items are
unverified (see [inventory-locations.md](inventory-locations.md)). Prefer
`products +stock --adjust -N` / `--set N`, which applies the safety gates for you; never
call this field directly on a multi-location item.

## Resolving a product id to a variant id

A product id is what a merchant has; a variant id is what the write endpoints need.
`products +set-price --product-id` and `products +stock --product-id` make that hop for you and
**refuse a multi-variant product** rather than guessing. Do it by hand only when the product has
several variants and you know which one the user meant:

```bash
# All variants of a product
products variants list --params '{"product_id":"9036f033-c605-4b6f-8a08-1b7ae090791c"}'

# The variant the user named ("the XL one") — select by option, never by position
products variants list --params '{"product_id":"9036f033-c605-4b6f-8a08-1b7ae090791c"}' \
  --jq '.data.variants[] | select(.option2=="XL") | .id'
```

`.data.variants[0].id` is safe **only** once you have confirmed the product has exactly one
variant. Otherwise it silently writes to the wrong sellable unit.

## SKU-based access

- `list-by-sku` returns every variant carrying the SKU — SKUs are **not unique** across
  products.
- `update-by-sku` requires `refuse_multi_result` in the body: set it `true` so a multi-match
  fails instead of updating an unintended variant (this is the same protection
  `+set-price --sku` gives you for prices).

```bash
# Which variants carry this SKU?
products variants list-by-sku --params '{"sku":"TSHIRT-RED-M"}'
```

## Creating a variant

`variants create` body requires `variant.price` and `variant.position` (see
`schema products.variants.create --view request`). For a brand-new single-variant product,
use `products +create` instead — it builds the variant for you.

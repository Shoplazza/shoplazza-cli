# shop metafields — definitions, resource metafields, shop metafields

All metafields live in the **shop** module — even a metafield attached to a product or an
order. Three families:

| Family | What it is | Key identity |
|---|---|---|
| `metafields-definition` | the schema/type of a custom field (define once) | `owner_resource` (+ definition `id`) |
| `metafields-resource` | a value attached to ONE resource instance (product 66880, order …) | `owner_resource` + `owner_id` path params |
| `metafields-shop` | a value attached to the shop itself | plain `id` |

`owner_resource` enum (definition + resource families): `shop`, `product`, `product_image`,
`product_variant`, `order`, `page`, `customer`, `collection`, `blog`, `article`, `app`.

## The create asymmetry (most common trap)

| | `metafields-resource create` | `metafields-shop create` |
|---|---|---|
| Owner | `--owner-resource product --owner-id <id>` | implicit (current shop) |
| `definition_id` | **REQUIRED** in `--data` | optional |
| `type` enum | `date`, `date_time`, `weight`, `volume`, `dimension`, `integer`, `number_decimal`, `file_reference`, `single_line_text_field`, `multi_line_text_field`, `json`, `color`, `rating`, `url`, `boolean` | `JSON`, `string` only |
| Body nesting | fields at top level (no wrapper object) | fields at top level (no wrapper object) |

Both require `namespace`, `key`, `type`, `value`. `value` accepts any JSON value (string /
number / bool / object / list) — a plain `"value":"cotton"` is fine; its interpretation is
driven by `type`. If the user gave a `definition_id` but no type, resolve it instead of
asking: `shop metafields-definition get --metafield-definition-id <id> --jq '.data.metafield_definition.type'`.

```bash
# Shop-level (no definition needed)
shop metafields-shop create --data '{"namespace":"global","key":"support_phone","type":"string","value":"400-800-1234"}'

# Product-level (definition REQUIRED; owner as flags)
shop metafields-resource create --owner-resource product --owner-id 66880 \
  --data '{"definition_id":"D8899","namespace":"specs","key":"material","type":"single_line_text_field","value":"cotton"}'
```

## Definitions

```bash
# Define a schema first if none exists (name/namespace/key/type all required)
shop metafields-definition create --owner-resource product \
  --data '{"name":"Material","namespace":"specs","key":"material","type":"single_line_text_field"}'
shop metafields-definition list --owner-resource product
shop metafields-definition get  --metafield-definition-id 123456     # id is a QUERY param, not path
shop metafields-definition update --metafield-definition-id 123456 --data '{"name":"…","description":"…"}'
shop metafields-definition count --owner-resource product --definition-ids '1001,1002'  # definition_ids required
shop metafields-definition count-by-group
shop metafields-definition delete --metafield-definition-id 123456   # --dry-run first
```

Definition `type` supports the resource enum **plus `string`**.

## Reads / updates / deletes

```bash
# Resource metafields (owner path params on every call)
shop metafields-resource list   --owner-resource product --owner-id 66880   # filters: namespace,key,type,definition_ids,create/update_at_min/max,cursor,page_size
shop metafields-resource get    --owner-resource product --owner-id 66880 --metafield-id <mid>
shop metafields-resource update --owner-resource product --owner-id 66880 --metafield-id <mid> --data '{"type":"single_line_text_field","value":"linen"}'   # value+type REQUIRED
shop metafields-resource count  --owner-resource product --owner-id 66880
shop metafields-resource delete --owner-resource product --owner-id 66880 --metafield-id <mid>

# Shop metafields
shop metafields-shop list    # filters: namespace,key,type,limit(default 50),cursor,fields
shop metafields-shop get    --metafield-id 123456
shop metafields-shop update --metafield-id 123456 --data '{"type":"string","value":"new"}'   # type+value REQUIRED
shop metafields-shop delete --metafield-id 123456
```

Updates are PATCH but **`type` + `value` are still required** in both families — send them
even when only changing the value.

## Boundaries

- "给商品加图片" is NOT a metafield — `products images create` (shoplazza-products).
- Product attributes like price/tags/variants are the products domain; metafields are for
  *custom* data hung off a resource.

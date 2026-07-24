# orders shipping-schemas — shipping zones, rates, available lines

How the store calculates and presents delivery options: the general shipping schema, its
shipping zones (countries/provinces + rate plans), and the shipping lines available for a
given order + address. This is the home of "运费区域 / 邮费 / shipping zones & rates".

**Boundary:** registering an external carrier that quotes *real-time* rates is
`shop carrier-services` (`shoplazza-shop`), not here.

Registry typo warning: schema summaries say "shopping zone" — they mean shipping zone.

## Commands

| Intent | Command |
|---|---|
| Read the general schema | `orders shipping-schemas get-general` → `.data.shipping_schemas` |
| Create/modify the general schema | `orders shipping-schemas save-general --data '{"shipping_schemas":{…}}'` (`name` required; include `id` to modify) |
| Create a zone | `orders shipping-schemas create-zone --data '{"shipping":{…}}'` |
| Update a zone | `orders shipping-schemas update-zone --params '{"id":"<zone-id>"}' --data '{"shipping":{…}}'` |
| Delete a zone (destructive → `--dry-run` first) | `orders shipping-schemas delete-zone --params '{"id":"<zone-id>"}'` |
| Which shipping options fit an order + address? | `orders shipping-schemas get-available-lines --data '{…}'` → `.data.shipping_lines[]` (+ `invalid_products`, `state`, `message`) |

## `create-zone` body (`schema orders.shipping-schemas.create-zone`)

```json
{"shipping": {
  "schema_id": "<general-schema-id>",
  "name": "EU zone",
  "support_cod": 0,
  "plans": [{"name": "Standard", "rule_type": "…", "rate_type": "…", "rate_amount": "…"}],
  "areas": [{"country_name": "Germany", "country_code": "DE", "province_codes": []}]
}}
```

Required: `schema_id`, `name`; per plan `name`; per area `country_name` + `country_code`.
Rate/rule fields are numerous — run `schema orders.shipping-schemas.create-zone --view request`
and mirror an existing zone from `get-general` rather than guessing.

## `get-available-lines` body

Required: `order_id`, `country_code`. Optional: `province_code`, `zip`.
**GB special case:** shipping differs across GB-ENG / GB-SCT / GB-WLS / GB-NIR, so an empty
`province_code` matches nothing — pass a specific code, `ALL`, or a `zip` (the province is
derived from the postal-code prefix). Other countries default to `ALL`.

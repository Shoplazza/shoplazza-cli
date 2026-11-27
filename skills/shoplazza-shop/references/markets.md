# shop markets — multi-market selling (市场 / 站点)

A market groups countries with their own domain mode, pricing, tax, languages, and
product assignment. **A country may belong to only ONE market** — that constraint drives
the create/update `confirm` semantics below.

## Lifecycle

```bash
shop markets list    --only-active true --with-countries-detail true   # → .data.markets[]
shop markets get     --market-id <id>
shop markets create  --data '{"name":"Japan","countries":["JP"]}'
shop markets update  --market-id <id> --data '{"name":"…","countries":["JP","KR"]}'  # countries = FULL replacement
shop markets delete  --market-id <id>        # IRREVERSIBLE; primary market cannot be deleted; --dry-run first
shop markets preview --data '{"countries":["JP"],"except_market_id":"<id>"}'  # which markets WOULD be deleted
```

- `create`: `name` (≤50 chars, unique) + `countries` (ISO 3166-1 alpha-2, UPPER case)
  required.
- **`confirm` is destructive**: with `confirm:false` (default), claiming an occupied
  country is rejected and the response lists the markets that would be deleted; resubmit
  with `confirm:true` and those markets are **auto-deleted**. Never set `confirm:true`
  silently — run `markets preview` / the rejected first call, restate what gets deleted,
  and get explicit consent.
- `update` `countries` is a full replacement, not incremental.

## Status / domain / pricing / tax / language

```bash
shop markets update-status --market-id <id> --data '{"status":"active"}'   # active | paused
shop markets update-domain --market-id <id> --data '{"domain_type":"sub_path","domain_value":"jp"}'
shop markets update-price  --market-id <id> --data '{"currency":"JPY","price_adjust":10,"local_currency_enabled":true}'
shop markets update-tax    --market-id <id> --data '{"product_tax_included":true}'
shop markets update-default-language --market-id <id> --data '{"language_code":"ja-JP"}'
```

- `update-status`: `paused` blocks checkout; the **primary market cannot be paused**.
- `update-domain`: `domain_type` = `primary` (independent domain) | `sub_path` (suffix
  under the primary domain; `domain_value` required, letters/digits/underscore, must not
  clash with a language-code abbreviation). **Switching domain_type on a non-primary
  market deletes its language configurations** and re-copies them from the primary market.
- `update-price`: `currency` (ISO 4217, upper case) required; `price_adjust` is a
  percentage in −99..999 (positive raises, negative lowers).
- `update-default-language`: the language must **already be published to this market**
  (see [languages.md](languages.md)).

## Product assignment

```bash
shop markets list-product   --market-id <id> --keyword … --is-excluded false --page-size 10
shop markets add-product    --market-id <id> --data '{"product_ids":["<pid>",…]}'
shop markets delete-product --market-id <id> --data '{"product_ids":["<pid>",…]}'
shop markets update-product-price --market-id <id> --data '{"products":[{"id":"<pid>","operation":"…","fixed_price":"19.99"}]}'
shop markets list-language  --market-id <id>
```

`update-product-price` items require `id` + `operation` (its enum is not documented in
the schema output — check `schema shop.markets.update-product-price` / docs, don't guess);
`fixed_price` is a string.

## Boundaries

- 语言开通/发布 → `shop languages` ([languages.md](languages.md)); markets only hold the
  relations.
- Shipping rates per region → `orders shipping-schemas` (orders domain), not markets.

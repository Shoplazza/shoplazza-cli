# shop markets — multi-market selling (市场 / 站点)

A market groups countries with their own domain mode, pricing, tax, languages, and
product assignment. **A country may belong to only ONE market** — that constraint drives
the create/update/primary-country `confirm` semantics below.

## Lifecycle

```bash
shop markets list    --params '{"only_active":true,"with_countries_detail":true}'   # → .data.markets[]
shop markets get     --params '{"id":"<id>"}'
shop markets create  --data '{"name":"Japan","countries":["JP"]}'
shop markets update  --params '{"id":"<id>"}' --data '{"name":"…","countries":["JP","KR"]}'  # countries = FULL replacement
shop markets delete  --params '{"id":"<id>"}'        # IRREVERSIBLE; primary market cannot be deleted; --dry-run first
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

## Primary market country (主市场国家)

```bash
shop markets list-primary-country                                    # → .data.count, .data.countries[] (current primary country first)
shop markets update-primary-country --data '{"country_code":"FR"}'   # FR must be in that list
```

- `markets update` can't edit the primary market — switching its country is this pair.
- Candidates are the countries that use the **primary market's currency**; each item carries
  the market it belongs to now (`market_id` / `market_name` / `market_status`, all omitted
  when it's in no market) and `is_primary`. Only the country changes, never the currency.
  Match markets by `market_id`: the primary market's `market_name` here can differ from the
  name `markets list` shows.
- `country_code`: ISO 3166-1 alpha-2, UPPER case. Not in the list (another currency or an
  unknown code) → 400 `Invalid parameter`. Moving to a country with another currency
  means changing the shop currency — a separate, bigger change; never do it implicitly.
- Side effects — `--dry-run` and restate before writing:
  - the new country is moved out of its current market; if that market had **only** this
    country it is **deleted**, so the first call returns 406 naming it — resend with
    `"confirm":true` only after explicit consent (same rule as create/update);
  - if that market is left with one country, its local-currency display is turned off;
  - the **old primary country ends up in no market** — add it to a market afterwards if the
    user still sells there.
- Sending the current primary country is a no-op success.

## Status / domain / pricing / tax / language

```bash
shop markets update-status --params '{"id":"<id>"}' --data '{"status":"active"}'   # active | paused
shop markets update-domain --params '{"id":"<id>"}' --data '{"domain_type":"sub_path","domain_value":"jp"}'
shop markets update-price  --params '{"id":"<id>"}' --data '{"currency":"JPY","price_adjust":10,"local_currency_enabled":true}'
shop markets update-tax    --params '{"id":"<id>"}' --data '{"product_tax_included":true}'
shop markets update-default-language --params '{"id":"<id>"}' --data '{"language_code":"ja-JP"}'
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
shop markets list-product   --params '{"id":"<id>","keyword":"…","is_excluded":false,"page_size":10}'
shop markets add-product    --params '{"id":"<id>"}' --data '{"product_ids":["<pid>",…]}'
shop markets delete-product --params '{"id":"<id>"}' --data '{"product_ids":["<pid>",…]}'
shop markets update-product-price --params '{"id":"<id>"}' --data '{"products":[{"id":"<pid>","operation":"…","fixed_price":"19.99"}]}'
shop markets list-language  --params '{"id":"<id>"}'
```

`update-product-price` items require `id` + `operation` (its enum is not documented in
the schema output — check `schema shop.markets.update-product-price` / docs, don't guess);
`fixed_price` is a string.

## Boundaries

- 语言开通/发布 → `shop languages` ([languages.md](languages.md)); markets only hold the
  relations.
- Shipping rates per region → `orders shipping-schemas` (orders domain), not markets.

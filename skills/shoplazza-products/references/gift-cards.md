# products gift-cards — reference

Gift cards are **stored value redeemable as store credit at checkout**. They live in the
products domain — not in discounts (优惠码/折扣码 → `shoplazza-discounts`) and not in billing.
Treat creation like a money operation: **never fabricate `initial_value` or `code`**, and
`--dry-run` before the real write.

## Command surface — note what's missing

Subcommands: `batch-create` · `create` · `disable` · `get` · `list` · `update`.
**There is no `delete`.** "把礼品卡删掉/作废" degrades to `disable` — it permanently stops
redemption; say explicitly that you are disabling, not deleting.

Verify shapes with `shoplazza schema products.gift-cards.<cmd>`.

| Command | HTTP | Params / body |
|---|---|---|
| `gift-cards create` | `POST /gift_cards` | `--data '{"gift_card":{…}}'` — `code` and `initial_value` **required** |
| `gift-cards batch-create` | `POST /gift_cards/batch` | `--data '{"gift_cards":[{…},…]}'` |
| `gift-cards get` | `GET /gift_cards/{id}` | `--params '{"id":"<id>"}'` |
| `gift-cards list` | `GET /gift_cards` | filters: `keyword`, `status`, `balance_status`, `initial_value_min/max`, date ranges, cursor paging |
| `gift-cards update` | `PUT /gift_cards/{id}` | body accepts **only** `expires_on`, `note`, `template_suffix` |
| `gift-cards disable` | `POST /gift_cards/{id}/disable` | `--params '{"id":"<id>"}'`, no body |

## Creating

Required body fields (both under the `gift_card` object, both **askable — never invented**):

- `code` — the card code the customer will redeem.
- `initial_value` — the stored value, **a string** in the schema (e.g. `"100"`).

Optional: `expires_on` (date), `note`, `customer_id` (bind to a customer), `send_email`
(bool), `currency`, `template_suffix`. Clarifying which customer to bind is fine; inventing
a customer id is not.

```bash
# Fixed-value card, expiry from the user's own words — dry-run first (money)
products gift-cards create \
  --data '{"gift_card":{"code":"GIFT2026","initial_value":"100","expires_on":"2026-12-31"}}' \
  --dry-run
products gift-cards create \
  --data '{"gift_card":{"code":"GIFT2026","initial_value":"100","expires_on":"2026-12-31"}}'
```

## Balance and lookup

The card object (under `.data.gift_card`) carries `balance`, `initial_value`, `enabled`,
`status`, `expires_on`, `disabled_at`, `customer_id`, `last_characters`, …

```bash
# Remaining balance
products gift-cards get --params '{"id":"<id>"}' --jq '.data.gift_card.balance'

# Find a card when you only have (part of) the code
products gift-cards list --params '{"keyword":"GC9988"}'
```

`get` needs the card **id**; when the user gives a code, resolve it via `list --params
'{"keyword":…}'` first.

## Updating vs disabling

- Value/balance is **not editable** — `update` only touches `expires_on` / `note` /
  `template_suffix`. "改面值" cannot be done; disable and issue a new card instead.
- `disable` is the endpoint of record for stopping a card. Destructive intent → restate
  (which card, and that it cannot be re-enabled via this CLI surface) or `--dry-run` first.

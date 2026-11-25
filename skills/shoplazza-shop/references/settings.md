# shop settings — password protection, anti-spider, wholesale, order number, notifications (店铺设置 / 商家通知)

The four store settings are nested objects on the shop itself: `shop info get` returns them,
`shop info update` writes them inside the `shop` wrapper. Merchant notification switches are a
separate pair of commands under `shop notifications`.

## Commands

```bash
shop info get                                              # every field incl. the 4 groups
shop info get --params '{"fields":["wholesale","order_number"]}'   # group names work in fields
shop info update --params '{"shop_id":"<id>"}' --data '{"shop":{"anti_spider":{"enabled":true}}}'
shop notifications get
shop notifications update --data '{"notifications":{"order_enabled":true}}'
```

`shop_id` comes from `shop info get --jq '.data.id'`. Every write is **partial**: send only
what changes; omitted fields (and omitted groups) keep their current value.

## Fields

| Group (`shop.<group>`) | Field | Notes |
|---|---|---|
| `password_protection` | `enabled` | Locks the storefront behind a password — restate + `--dry-run` before turning it on |
| | `password` | ≤ 100 chars; must not be empty while protection is on. Ask the user — never invent one |
| | `access_tips` | Text on the password page, ≤ 500 chars |
| `anti_spider` | `enabled` | Anti-crawler protection |
| `wholesale` | `enabled` | Tiered wholesale; the plan must support it and no conflicting feature may be in use |
| | `mixed_enabled` | Mixed wholesale; always `false` while `enabled` is `false` |
| | `mixed_min_quantity` | Minimum quantity for mixed wholesale; kept (not cleared) when mixed is turned off |
| `order_number` | `prefix` | ≤ 20 chars; format must fit `order_id_type`; the plan must support custom prefixes |
| | `suffix` | Not allowed when `order_id_type` is `sequential` |
| | `number_type` | `sequential` (8-digit increasing) / `shop_id_random` (shop ID + random digits) |
| | `order_id_type` | `sequential` (8-digit increasing) / `alphanumeric_random` (3 letters + 5 digits) |

`prefix` + `suffix` together ≤ 20 chars. Send `""` to clear a prefix or suffix — omitting it
keeps the current value.

| Notifications (`notifications.<field>`) | Meaning |
|---|---|
| `order_enabled` | New-order alert to the merchant (新订单提醒) |
| `delivered_enabled` | Order-delivered alert (商家收货通知) |
| `review_enabled` | New-review alert (评论通知) |

## Errors from `info update`

Basic fields and each group are written **independently and concurrently, with no
rollback**. When anything fails, the message lists every failure in field order, then the
parts that were saved:

```text
wholesale: <reason>; order_number: Invalid suffix (updated: name, anti_spider)
```

- Basic-field errors carry no group prefix; `(updated: …)` lists saved basic fields by name
  and saved groups by group name.
- Status: one failure (or several with the same status) keeps that status; mixed statuses →
  422; any 5xx → 500.
- Report what was saved and what failed; resend only the failed part. Do not resend the whole
  body blindly — the saved parts are already written.

| Error | Cause | Fix |
|---|---|---|
| 422 `Please bind a payment card before changing this setting` | Password protection `enabled` / `access_tips` need a bound payment card | The merchant binds a card in the admin; nothing to do from the CLI |
| 422 on `wholesale` | The plan doesn't support wholesale, or a conflicting feature is in use (the message names it) | Relay the message; no CLI workaround |
| 406 on `order_number` (e.g. `Invalid suffix`, `Invalid prefix`) | Suffix with `order_id_type: sequential`, a prefix that doesn't fit `order_id_type`, or a plan without custom prefixes | Adjust per the Fields table, or relay the message |
| 422 on `email` / `customer_email` / `financial_email` | Invalid email format | Ask for a valid address |
| 400 `… value length must be at most …` | Over a max length above | Shorten the value |

`currency` cannot change once the store has orders.

## Errors from `info get`

With no `fields`, `info get` reads the basic info and all four groups; any failing part
fails the call, with the same `group: reason` message. Narrow `fields` to what the task
needs. Unknown names in `fields` are rejected with 400.

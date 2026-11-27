# shop settings — password protection, anti-spider, wholesale, order number, customer privacy, timezone, notifications (店铺设置 / 商家通知)

The five store settings are nested objects on the shop itself: `shop info get` returns them,
`shop info update` writes them inside the `shop` wrapper. `timezone` is a plain shop field, not
a group. Merchant notification switches are a separate pair of commands under
`shop notifications`.

## Commands

```bash
shop info get                                              # every field incl. the 4 groups
shop info get --params '{"fields":["wholesale","order_number"]}'   # group names work in fields
shop info update --params '{"shop_id":"<id>"}' --data '{"shop":{"anti_spider":{"enabled":true}}}'
shop info update --params '{"shop_id":"<id>"}' --data '{"shop":{"customer_privacy":{"enabled":true}}}'
shop info update --params '{"shop_id":"<id>"}' --data '{"shop":{"timezone":"Asia/Tokyo"}}'
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
| `customer_privacy` | `enabled` | Customer privacy switch — one switch for both GDPR and CCPA, as in the admin. Writing it sets both; reads `true` when either is on |
| | `privacy_app_status` | Read-only: `not_install` / `installed` / `uninstalled` / `created`; `not_install` when the shop has no privacy app |
| | `app_gdpr_enabled` / `app_ccpa_enabled` | Read-only: the privacy app's own switches; `false` without the app. Not writable here |

`prefix` + `suffix` together ≤ 20 chars. Send `""` to clear a prefix or suffix — omitting it
keeps the current value.

`timezone` (a plain field on `shop`) takes either a supported IANA name (`Asia/Tokyo`,
case-sensitive) or the internal code `info get` returns in `.data.timezone` (`+0800`, `+0900`,
`-050010` — cities sharing an offset get a numeric suffix). Either way it is stored, and read
back, as the internal code. Three IANA names are shared by several cities and map to a default:
`Etc/GMT+8` → `-0800`, `America/Halifax` → `-04008`, `Etc/GMT-2` → `+02003`; send the internal
code to pick another city in those groups (e.g. `-04009`, which has no IANA name of its own).
Offsets like `+08:00`, display names and padded values return 422; `""` leaves it unchanged.

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
| 422 `Invalid parameter timezone…` | `timezone` is neither a supported IANA name nor an internal code; for an IANA name the shop doesn't support, the message lists same-offset codes | Send a supported IANA name or one of the listed codes |
| 400 `… value length must be at most …` | Over a max length above | Shorten the value |

`currency` cannot change once the store has orders.

## Errors from `info get`

With no `fields`, `info get` reads the basic info and all five groups; any failing part
fails the call, with the same `group: reason` message. Narrow `fields` to what the task
needs. Unknown names in `fields` are rejected with 400.

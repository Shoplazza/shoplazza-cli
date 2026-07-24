---
name: shoplazza-customers
description: >-
  Use when the user wants to manage customers (buyers) on a shoplazza store through the CLI —
  create a customer profile, find a customer by email or phone, get customer details or the
  total customer count, update names / tags / email or SMS marketing subscription, or manage a
  customer's shipping addresses including the default checkout address. Triggers include
  客户 / 买家 / 顾客 / 客户资料 / 新建客户 / 按邮箱查客户 / 按手机号查客户 / 给客户打标签 /
  退订营销 / 收货地址 / 默认地址 / customer profile / new customer / find customer by email
  or phone / customer count / tag a customer / unsubscribe marketing / shipping address /
  default address. Finding the customer RECORD by email (按邮箱找客户资料) belongs HERE
  (customers +search); that customer's ORDER HISTORY (该客户下过的订单 / orders placed by
  this email) belongs to shoplazza-orders.
---

# shoplazza CLI — customers module

**CRITICAL — before anything else, use Read on [`../shoplazza-common/SKILL.md`](../shoplazza-common/SKILL.md).**
It owns every cross-cutting mechanic: the three access tiers, the output envelope (`.data`),
`--dry-run`, `--jq` (incl. "don't pass `-r`"), `--fields`, `schema`, `api rest`, auth / profiles,
and the safety protocol. This file covers only the customers domain and never repeats them.

## Overview

The `customers` module manages customer (buyer) profiles and their shipping addresses. All
three access tiers apply (tier-selection rule is in shoplazza-common — prefer `+shortcut`).

Shortcuts are **`+create` and `+search` only**. There is **no `+count`** — counting is the
`count` spec leaf — and no `+update` / `+delete`; updates are the `update` leaf, and this
module has **no customer-delete command at all** (only addresses have `delete`).

## Command map

Intent → command, highest-fit tier first. The authoritative flags/params live in
`shoplazza customers <cmd> --help` and `shoplazza schema customers.<cmd>`, not this table.

| User intent | Command |
|-------------|---------|
| Create a customer | `customers +create (--email <e> \| --phone <p>) [--first-name --last-name --tags --no-marketing]` |
| Find customer by email / phone | `customers +search --email <e>` / `customers +search --phone <p>` |
| Total customer count | `customers count` (leaf — there is NO `+count`) |
| Get one customer by ID | `customers get --params '{"customer_id":"<id>"}'` |
| List / filter customers (cursor paging, ids, contact) | `customers list --params '{…}'` |
| Update name / tags / marketing subscription | `customers update --params '{"customer_id":"<id>"}' --data '{"customer":{…}}'` |
| List a customer's addresses | `customers addresses list --params '{"customer_id":"<id>"}'` |
| Add an address | `customers addresses create --params '{"customer_id":"<id>"}' --data '{"address":{…}}'` |
| Get / update / delete one address | `customers addresses {get,update,delete} --params '{"customer_id":"<cid>","address_id":"<aid>"}'` |
| Set the default shipping address | `customers addresses set-default --params '{"customer_id":"<cid>","address_id":"<aid>"}'` (no `--data`) |

## Acting on a request

"新建客户" / "add a new customer" / "给客户打标签" / "unsubscribe them" is an **action**, not a
question:

1. Match intent to a command via the trigger table.
2. Check the **required, no-default values** against the user's own words. Missing or ambiguous
   → **ask with `AskUserQuestion`, never fabricate** — above all the contact identifier:
   **never invent an email address or phone number.**
3. **Never ask about anything with a CLI default** (marketing defaults to subscribe; names and
   tags are optional). Let the default win.
4. All required present → execute. Writes here are non-destructive single-record updates;
   `--dry-run` first only if the user asks (see shoplazza-common → Safety protocol).

### Trigger phrase → command

| User says | Command | How to extract values |
|---|---|---|
| 新建客户 / 建个客户档案 / add a new customer | `+create` | exactly ONE of `--email` / `--phone`; a full name like "Jane Doe" splits into `--first-name Jane --last-name Doe` |
| 不要订阅营销 / 不订阅邮件 / don't subscribe to marketing | `+create … --no-marketing` | default is subscribe — the flag MUST be set on explicit opt-out wording |
| 按邮箱查客户 / find the customer by email | `+search --email <e>` | the ask is the customer record, not their orders |
| 按手机号查客户 / look up by phone | `+search --phone <p>` | do not invent `--since`/`--until` time bounds |
| 一共多少客户 / 注册客户数 / how many customers | `count` | leaf, no flags; extract with `--jq '.data.count'` |
| 给客户 X 打标签 / tag customer X | `update` | tags is a WHOLE-ARRAY replace — read-merge-write (see Gotchas) |
| 退订邮件营销 / unsubscribe email marketing | `update` with `{"customer":{"accepts_marketing":false}}` | email only — do NOT touch `accepts_sms_marketing` |
| 退订短信营销 / unsubscribe SMS marketing | `update` with `{"customer":{"accepts_sms_marketing":false}}` | SMS only — do NOT touch `accepts_marketing` |
| 加一个收货地址 / add a shipping address | `addresses create` | body requires `country` + `country_code` |
| 把地址设为默认 / set the default address | `addresses set-default` | both IDs are path params via `--params`; no body |

### Required-vs-ask matrix

Only **no-default** values are askable.

| Command | Must ASK if unspecified | Infer if possible | Default silently |
|---|---|---|---|
| `+create` | the contact identifier — **exactly one** of `--email` / `--phone` (if you ask, offer both and explain only ONE is needed) | `--first-name`/`--last-name` from a stated name; `--no-marketing` from opt-out wording; `--tags` if the user names tags | marketing (default subscribe), `--tags` (omit), names (optional — never ask) |
| `+search` | which customer — an email or phone to search for, when the user wants a specific person but gave neither | filter flag from what they gave (email vs phone) | `--page-limit`, `--fields`, `--since`, `--until` (all omit) |
| `update` | `customer_id`; the new value if wording gave none | which field from wording (标签→`tags`, 邮件营销→`accepts_marketing`, 短信→`accepts_sms_marketing`) | every field the user did not mention — never send it |
| `addresses create` | `customer_id`; `country` + `country_code` (required body fields) | other address fields from wording | `default` and all optional fields (omit) |
| `addresses set-default` | `customer_id` + `address_id` | — | — (no body at all) |

### Never-ask list

Optional or defaulted — never a question: `--first-name` / `--last-name` / `--tags` (`+create`,
optional) · marketing subscription (`+create` defaults to subscribe; set `--no-marketing` only
on explicit opt-out wording, never ask) · `--page-limit` / `--fields` / `--since` / `--until`
(`+search`; omit — never invent time bounds) · optional address fields incl. `default`.

## Boundaries

Reads like customers, actually belongs elsewhere (and vice versa):

| Sounds like customers | Actually belongs to | Command |
|---|---|---|
| 该客户下过的订单 / order history for jane@example.com | `shoplazza-orders` | an orders search filtered by that customer |
| 按邮箱找到这个客户 / customer profile for an email | **this module** (not orders) | `customers +search --email <e>` |

Direction decides: **资料 (profile/record) → customers; 订单 (orders placed) → orders.**
Resolving a customer ID here as an intermediate step for an orders query is fine — the final
answer must come from the owning domain.

## Permissions · Scope

Authorization is by **domain** (the unit of `--domain`), not per command. Authorization flow is
in shoplazza-common → Authentication.

| Operation | Needs | Grant |
|---|---|---|
| read (`get` / `list` / `count` / `+search` / `addresses get\|list`) | `customers` read scope | `auth login --domain customers` |
| write (`+create` / `create` / `update` / `addresses create\|update\|delete\|set-default`) | `customers` write scope | `auth login --domain customers` |

Look up exact scope literals with `shoplazza auth scopes`; `--domain customers` expands into them.

## Gotchas

Domain-specific pitfalls only (generic ones — `.data` prefix, `--jq` without `-r` — are in
shoplazza-common).

| Symptom | Cause | Fix |
|---------|-------|-----|
| `customers +count` → unknown command | Shortcuts are `+create`/`+search` only (unlike products/orders, customers has no `+count`) | `customers count` (leaf), extract with `--jq '.data.count'` — never list-and-count manually |
| `+create` rejected: "exactly one of --email or --phone is required" | Strict XOR — both given, or neither | Pass exactly one; if the user gave neither, ASK — never fabricate contact info |
| Spec-leaf `create` fails without `contact_type` | The leaf body requires `contact_type`; `+create` fills it automatically (`email` / `phone` to match the flag) | Prefer `+create`; on the leaf, set `"contact_type"` to match the identifier you send |
| Customer's old tags vanished after tagging | `update` `tags` REPLACES the whole array | Read-merge-write: `get` current tags, append, then `update` with the merged array |
| Unsubscribed more than the user asked | Email (`accepts_marketing`) and SMS (`accepts_sms_marketing`) are separate booleans | Change only the one named; leave the other out of the body |
| Tried to change email / phone via `update` | The update body only accepts `first_name` / `last_name` / `accepts_marketing` / `accepts_sms_marketing` / `tags` | Not expressible via `update` — check `schema customers.update` |
| Leaf `list` ignores a `phone` param | `list` documents `email` and `contact` query params, not `phone` | Prefer `+search --phone`; on the leaf use `"contact"` (matches email or phone) |
| Wanted to delete a customer | No customer-delete command exists in this module (only `addresses delete`) | Not expressible at tiers 1–2; confirm an endpoint exists before reaching for `api rest` |
| Set default via `addresses update` with `"default":true` | A dedicated endpoint owns this | `addresses set-default --params '{"customer_id":…,"address_id":…}'` — path params only, no `--data` |
| `+search --since/--until` returns "wrong" customers | They map to `created_at_min`/`created_at_max` — bounds on the customer's CREATION time | Only pass them when the user asks for a signup-time window |

## Recipes

```bash
# 1. New customer with email, opted out of marketing emails
customers +create --email jane.doe@example.com --first-name Jane --last-name Doe --no-marketing

# 2. Find a customer by phone (raw record; --fields also works on +search)
customers +search --phone 13800138000 --jq '.data.customers[] | {id, name, email, phone}'

# 3. Total registered customers
customers count --jq '.data.count'

# 4. Tag customer 424242 as VIP without clobbering existing tags (read-merge-write)
customers get --params '{"customer_id":"424242"}' --jq '.data.customer.tags'
# → e.g. ["wholesale"]; append and write the MERGED array back:
customers update --params '{"customer_id":"424242"}' --data '{"customer":{"tags":["wholesale","VIP"]}}'

# 5. Unsubscribe email marketing only (SMS untouched)
customers update --params '{"customer_id":"887766"}' --data '{"customer":{"accepts_marketing":false}}'

# 6. Set the default shipping address
customers addresses set-default --params '{"customer_id":"335577","address_id":"ADDR99"}'

# 7. List a customer's address book
customers addresses list --params '{"customer_id":"335577"}' --jq '.data.addresses[] | {id, name, address1, default}'
```

## References

- Per-command flags: `shoplazza customers <cmd> --help`
- Spec-leaf params / body / response: `shoplazza schema customers.<cmd>` (addresses: `schema customers.addresses.<cmd>`)
- Cross-cutting mechanics (auth / tiers / output envelope / `--dry-run` / `--jq` / safety): `../shoplazza-common/SKILL.md`
- Shortcut source of truth: `shortcuts/customers/*.go`

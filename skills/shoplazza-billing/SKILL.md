---
name: shoplazza-billing
description: >-
  Use when an app developer needs to bill a Shoplazza merchant for app usage through the CLI —
  one-time application charges, recurring (subscription) charges, metered usage charges, and
  app-charge transaction records. Triggers include 应用收费 / 订阅费 / 一次性收费 / 用量费 / 计费 /
  app billing / recurring / one-time / usage charge. NOT buyer order payments or refunds
  (那是 shoplazza-orders 的 transactions / +refund).
---

# shoplazza CLI — billing module

**CRITICAL — before anything else, use Read on [`../shoplazza-common/SKILL.md`](../shoplazza-common/SKILL.md).**
It owns every cross-cutting mechanic: access tiers, the output envelope (`.data`), `--dry-run`,
`--jq`, `schema`, `api rest`, auth / profiles, and the safety protocol. This file covers only
the billing domain and never repeats them.

## Overview

The billing module lets an **app developer** charge a merchant for app usage: **one-time** fees,
**recurring** subscriptions, and metered **usage** charges (plus **transaction** records). It is
**spec-leaf only** — there are no `+shortcuts`. Every charge moves **real money**, so
`--dry-run` first and restate the charge to the user before executing (shoplazza-common → Safety).

Charges are **two-phase**: `create` alone moves no money — the charge sits pending until the
merchant opens the approval URL the response returns (`confirm_url` for one-time,
`confirmation_url` for recurring) and approves it in the Shoplazza admin, after which they land
on your `return_url`. Usage charges skip approval — they bill under an already-approved
recurring charge.

## Command map

Intent → command. Authoritative params/body live in `shoplazza schema billing.<group>.<cmd>`.

| User intent | Command |
|-------------|---------|
| Create a one-time charge (money → `--dry-run` first) | `billing one-time create --data '{"application_charge":{"name":"…","price":"<amt>","return_url":"…"}}' --dry-run` |
| Get / list one-time charges | `billing one-time get --params '{"charge_id":"<id>"}'` · `billing one-time list` |
| One-time charge transactions | `billing one-time transactions --params '{"charge_id":"<id>"}'` |
| Create a recurring (subscription) charge (money → `--dry-run` first) | `billing recurring create --data '{"recurring_application_charge":{"name":"…","price":<amt>,"return_url":"…"}}' --dry-run` (optional `trial_days`, `capped_amount`, `terms`) |
| Get / list recurring charges | `billing recurring get --params '{"charge_id":"<id>"}'` · `billing recurring list` |
| Update a recurring charge | `billing recurring update --params '{"charge_id":"<id>"}' --data '{"recurring_application_charge":{…}}'` |
| Cancel a recurring charge (`--dry-run` first) | `billing recurring cancel --params '{"charge_id":"<id>"}' --dry-run` |
| Recurring charge transactions | `billing recurring transactions --params '{"charge_id":"<id>"}'` |
| Record a usage charge under a recurring charge (money → `--dry-run` first) | `billing usage create --params '{"charge_id":"<id>"}' --data '{"usage_charge":{"description":"…","price":<amt>}}' --dry-run` |
| Get / list usage charges | `billing usage get --params '{…}'` · `billing usage list --params '{"charge_id":"<id>"}'` |
| Get one app-charge transaction | `billing transactions get --params '{"transaction_id":"<id>"}'` |

## Acting on a request

Billing writes move **real money**. Before any create / cancel:

1. Match intent to the leaf (no shortcuts here).
2. Check required fields — **ask with `AskUserQuestion` if missing, never fabricate an amount**:
   - one-time `create`: `name`, `price`, `return_url`
   - recurring `create`: `name`, `price`, `return_url`
   - usage `create`: `charge_id` (the parent recurring charge), `description`, `price`
   - recurring `cancel`: `charge_id`
3. `--dry-run` first, **restate** ("charge merchant $X — one-time / $X per month subscription / $X usage under CHG…, returns to <url>; **pending until the merchant approves via the returned confirm link**"), **stop and wait for the user's go-ahead**, then execute.
   For `cancel`, restate **what** is being cancelled, not a bare id echo: `recurring get` first
   (name / price / status — also catches an already-cancelled charge) in the same reply as the
   `cancel --dry-run` preview.
4. After a one-time / recurring `create` succeeds, **hand the approval URL to the user** —
   `--jq '.data.application_charge | {id, status, confirm_url}'` (one-time) /
   `--jq '.data.recurring_application_charge | {id, status, confirmation_url}'` (recurring) —
   and say plainly: **no money is charged until the merchant opens it and approves**.
5. **If the platform rejects the charge, never retry with a different amount.** A rejected price is
   almost always a listing-configuration problem (see Gotchas), not a value to search for — probing
   amounts risks creating a charge the user never approved. Report the rejection, name the
   partner-backend config it needs, and stop.

## Boundaries

Reads like billing, actually belongs elsewhere:

| Sounds like billing | Actually belongs to | Command |
|---|---|---|
| 交易记录 = 买家给店铺付的钱 / order payment transactions | `shoplazza-orders` | `orders transactions` |
| 订单退款 / refund a buyer | `shoplazza-orders` | `orders +refund` |

**billing transactions = the app's charges to the merchant**; a buyer's payment for an order is
`orders transactions`. (Serves the `app-charge-transactions` / `order-payment-transactions`
routing collisions.)

## Permissions · Scope

**Billing needs no store scope.** `auth login --domain billing` expands to **zero** scopes —
that is correct, not a bug: app-charge endpoints are authorized by the app installation itself,
not by a store OAuth grant. Verified live — on a profile holding neither `read_finance` nor
`write_finance`, `recurring list` returned `ok:true` and `recurring create` reached business
validation (400 / 422), never 401/403.

**So do not run `auth login --domain billing`.** Authorization *replaces* the grant server-side,
so a 0-scope login can revoke the scopes other domains rely on. A 401/403 here means the
**account** login is missing or expired — check `auth status`, don't chase scopes.

## Gotchas

Domain-specific only (generic `.data` / `--dry-run` / `--jq` rules are in shoplazza-common).

| Symptom | Cause | Fix |
|---------|-------|-----|
| Charge rejected on `price` type | **`price` type differs by charge**: one-time = **string** (`"99.99"`), recurring & usage = **number** (`29`, `0.05`) | Match the type — quote it for one-time, bare number for recurring/usage |
| `create` succeeded but the merchant is never billed | Charges are two-phase: `create` returns a **pending** charge; nothing activates until the merchant approves it via the returned approval URL | Hand the URL to the user and say no money moves until the merchant approves — a `create` `ok:true` is NOT "charged" |
| jq for the approval URL returns `null` | The field name **differs by charge**: one-time = `confirm_url`, recurring = `confirmation_url` (usage has neither — no approval step) | Match the field to the charge type |
| `usage create` fails without a charge | `charge_id` (the parent recurring charge) is a **required path param** | `--params '{"charge_id":"<recurring charge id>"}'` |
| Created a real charge without preview | Charges are real money | `--dry-run` first, restate to the user, wait for their go-ahead, then run |
| `recurring create` → 422 `The price configuration in the plan does not match with the listing, please check` | The platform only accepts a price matching a pricing plan configured in the app's **listing** (partner backend); arbitrary amounts are refused. **Not fixable from the CLI** — this module has no listing/plan endpoint (`schema billing` confirms) | Tell the user to add/adjust that plan in the app listing, then re-run the same command unchanged. Do NOT try other prices |
| `recurring create` → bare `400 Bad Request`, no field info, body matches `schema` | Adding `capped_amount` makes the backend return a specific 422 instead (verified live: identical body + `capped_amount` → the listing 422 above). Whether the 400 shares that root cause, or is its own `capped_amount` check, is **unconfirmed** — `schema` marks the field optional | Don't hunt for a malformed field — re-send once with `capped_amount` **to surface the real message**, then act on it. It is a diagnostic, not a fix: never retry with other prices, and don't assume the charge will go through once it's set |

## Recipes

```bash
# 1. One-time $99.99 setup fee (price is a STRING)
billing one-time create --data '{"application_charge":{"name":"Setup Fee","price":"99.99","return_url":"https://app.example.com/billing/done"}}' --dry-run
# real run: add --jq '.data.application_charge | {id, status, confirm_url}' and hand the URL over

# 2. Recurring $29/mo subscription, 7-day free trial (price is a NUMBER)
billing recurring create --data '{"recurring_application_charge":{"name":"Pro Plan","price":29,"return_url":"https://app.example.com/billing/ok","trial_days":7}}' --dry-run
# real run: add --jq '.data.recurring_application_charge | {id, status, confirmation_url}'

# 3. Usage charge $0.05 under recurring charge CHG7788 (price NUMBER)
billing usage create --params '{"charge_id":"CHG7788"}' --data '{"usage_charge":{"description":"API calls overage","price":0.05}}' --dry-run

# 4. Cancel a recurring charge: look up what it is, preview, then STOP for the user's go-ahead
billing recurring get --params '{"charge_id":"CHG5566"}' --jq '.data.recurring_application_charge | {name, price, status}'
billing recurring cancel --params '{"charge_id":"CHG5566"}' --dry-run
```

## References

- Per-command flags: `shoplazza billing <group> <cmd> --help`
- Params / body / response: `shoplazza schema billing.<group>.<cmd>`
- Cross-cutting mechanics (auth / tiers / output envelope / `--dry-run` / safety): `../shoplazza-common/SKILL.md`

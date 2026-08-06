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
3. `--dry-run` first, **restate** ("charge merchant $X — one-time / $X per month subscription / $X usage under CHG…, returns to <url>"), **stop and wait for the user's go-ahead**, then execute.

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

Authorization is by domain. Grant: `auth login --domain billing`. Look up exact scope literals
with `shoplazza auth scopes`.

## Gotchas

Domain-specific only (generic `.data` / `--dry-run` / `--jq` rules are in shoplazza-common).

| Symptom | Cause | Fix |
|---------|-------|-----|
| Charge rejected on `price` type | **`price` type differs by charge**: one-time = **string** (`"99.99"`), recurring & usage = **number** (`29`, `0.05`) | Match the type — quote it for one-time, bare number for recurring/usage |
| `usage create` fails without a charge | `charge_id` (the parent recurring charge) is a **required path param** | `--params '{"charge_id":"<recurring charge id>"}'` |
| Created a real charge without preview | Charges are real money | `--dry-run` first, restate to the user, wait for their go-ahead, then run |

## Recipes

```bash
# 1. One-time $99.99 setup fee (price is a STRING)
billing one-time create --data '{"application_charge":{"name":"Setup Fee","price":"99.99","return_url":"https://app.example.com/billing/done"}}' --dry-run

# 2. Recurring $29/mo subscription, 7-day free trial (price is a NUMBER)
billing recurring create --data '{"recurring_application_charge":{"name":"Pro Plan","price":29,"return_url":"https://app.example.com/billing/ok","trial_days":7}}' --dry-run

# 3. Usage charge $0.05 under recurring charge CHG7788 (price NUMBER)
billing usage create --params '{"charge_id":"CHG7788"}' --data '{"usage_charge":{"description":"API calls overage","price":0.05}}' --dry-run

# 4. Cancel a recurring charge
billing recurring cancel --params '{"charge_id":"CHG5566"}' --dry-run
```

## References

- Per-command flags: `shoplazza billing <group> <cmd> --help`
- Params / body / response: `shoplazza schema billing.<group>.<cmd>`
- Cross-cutting mechanics (auth / tiers / output envelope / `--dry-run` / safety): `../shoplazza-common/SKILL.md`

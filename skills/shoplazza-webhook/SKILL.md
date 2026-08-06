---
name: shoplazza-webhook
description: >-
  Use when an app developer needs to manage webhook subscriptions on a Shoplazza store through
  the CLI — subscribe to store events (order created, product updated, …) so the platform pushes
  notifications to an app's endpoint, and list / get / update / delete / count those
  subscriptions. Triggers include webhook / 事件订阅 / 事件回调 / 事件通知 / 推送通知 / 回调地址 /
  subscribe to an event / event notification. NOT the events themselves in shoplazza-orders /
  shoplazza-products — this domain owns the SUBSCRIPTION.
---

# shoplazza CLI — webhook module

**CRITICAL — before anything else, use Read on [`../shoplazza-common/SKILL.md`](../shoplazza-common/SKILL.md).**
It owns every cross-cutting mechanic: access tiers, the output envelope (`.data`), `--dry-run`,
`--jq`, `schema`, `api rest`, auth / profiles, and the safety protocol. This file covers only the
webhook domain and never repeats them.

## Overview

The webhook module manages **event subscriptions**: a webhook binds a store **event topic** (e.g.
`orders/create`) to an **address** (an HTTPS endpoint the platform calls when the event fires).
It is **spec-leaf only** — no `+shortcuts`. Full CRUD + count.

## Command map

Intent → command. Authoritative params/body live in `shoplazza schema webhook.<cmd>`.

| User intent | Command |
|-------------|---------|
| Subscribe to an event (create) | `webhook create --data '{"webhook":{"topic":"<event>","address":"<https-url>"}}'` |
| List all webhooks | `webhook list` |
| Count webhooks (optionally filtered) | `webhook count --params '{"address":"<url>","topic":"<event>"}'` (params optional) |
| Get one webhook | `webhook get --params '{"id":"<id>"}'` |
| Update a webhook's address / topic | `webhook update --params '{"id":"<id>"}' --data '{"webhook":{"address":"<url>","topic":"<event>"}}'` |
| Delete a webhook | `webhook delete --params '{"id":"<id>"}'` |

**Event topics** look like `orders/create`, `orders/fulfilled`, `products/update`, etc. The full
topic list isn't introspectable from the binary — confirm a topic with `shoplazza schema
webhook.create` / the Open Platform docs; don't invent one.

## Acting on a request

1. Match intent to the leaf (no shortcuts here).
2. `create` needs two required fields — **ask with `AskUserQuestion` if either is missing, never
   fabricate**: the **event topic** and the **address** (HTTPS endpoint). For `update`/`get`/
   `delete`, the webhook **`id`** is required.
3. `create`/`update`/`delete` are safe to run directly once required fields are present; use
   `--dry-run` first only if the user asks (subscriptions don't move money, but `delete` is
   destructive — restate which subscription before deleting).

## Boundaries

Reads like an event/order/product task, actually belongs here:

| Sounds like orders / products | Actually belongs to | Command |
|---|---|---|
| "订单发货后自动通知我的系统" / notify my system when an order ships | **HERE** (webhook) — it's a subscription | `webhook create --data '{"webhook":{"topic":"orders/fulfilled",…}}'` |
| "商品更新时通知我" / get notified when a product is updated | **HERE** (webhook) | `webhook create --data '{"webhook":{"topic":"products/update",…}}'` |

Subscribing to an event is a **webhook**, not an `orders`/`products` command. (Serves the
`event-webhook` routing collision.)

## Permissions · Scope

Authorization is by domain. Grant: `auth login --domain webhook`. Look up exact scope literals
with `shoplazza auth scopes`.

## Gotchas

Domain-specific only (generic rules are in shoplazza-common).

| Symptom | Cause | Fix |
|---------|-------|-----|
| Guessed a `topic` and it was rejected | The topic enum isn't in the binary's help | Confirm via `schema webhook.create` / Open Platform docs; ask the user if unsure — don't invent |
| `create` fails | `topic` and `address` are both required in the `webhook` body | `--data '{"webhook":{"topic":"…","address":"…"}}'` |
| `address` rejected | Endpoints must be reachable HTTPS URLs | Pass a full `https://` URL |

## Recipes

```bash
# 1. Subscribe: push order-created events to an endpoint
webhook create --data '{"webhook":{"topic":"orders/create","address":"https://api.myapp.com/hooks/orders"}}'

# 2. List current subscriptions
webhook list

# 3. Update a webhook's address
webhook update --params '{"id":"246"}' --data '{"webhook":{"address":"https://new.example.com/hook"}}'

# 4. Delete a subscription (restate which one first)
webhook delete --params '{"id":"135"}'

# 5. Count webhooks pointing at an address
webhook count --params '{"address":"https://api.myapp.com/hooks"}'
```

## References

- Per-command flags: `shoplazza webhook <cmd> --help`
- Params / body / response: `shoplazza schema webhook.<cmd>`
- Cross-cutting mechanics (auth / tiers / output envelope / `--dry-run` / safety): `../shoplazza-common/SKILL.md`

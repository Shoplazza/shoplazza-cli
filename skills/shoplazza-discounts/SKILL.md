---
name: shoplazza-discounts
description: Use when the user wants to manage discount activities on a shoplazza store through the CLI — promotional campaigns, coupon codes, flash sales, buy-X-get-Y offers, rebates / tiered satisfy-N-get-M discounts, free-shipping codes, M-for-N (Nth-item-off) discounts, discount stacking / combination rules, or coupon (voucher) campaigns.
---

# shoplazza CLI — discounts module

## Overview

The `discounts` module exposes three access tiers. Always pick the highest tier that fits the task.

| Tier | Examples | When to use |
|------|----------|-------------|
| `+<shortcut>` | `+percent-code`, `+rebate`, `+search` | First choice. Named flags, smart defaults, structured. |
| `<command>` (spec leaf) | `cancel`, `list`, `update-automatic` | Lifecycle / introspection ops with no shortcut. Use `--params` / `--data` JSON. |
| `api rest` | `api rest POST /openapi/...` | Raw HTTP fallback. Avoid unless the other two cannot express the operation. |

**Output contract:** stdout = raw JSON body on success; stderr = `{"ok":false,"error":{...}}` envelope on failure. Every shortcut and every `--data`-bearing leaf accepts `--dry-run`, which prints the would-be request to stdout without sending it. **Run `--dry-run` first for any cancel / batch-delete / update.**

For full parameter detail on any spec leaf, run `shoplazza schema discounts.<cmd>`. For shortcut flags, run `shoplazza discounts <+shortcut> --help`. Do not memorize flags from this file — it can drift; the CLI's own help is authoritative.

## Decide which command

| User intent | Command |
|-------------|---------|
| % off (with coupon code) | `discounts +percent-code --target order\|product` |
| Fixed amount off (with coupon code) | `discounts +amount-code --target order\|product` |
| Free shipping coupon | `discounts +free-shipping-code` |
| Buy X get Y (with coupon code) | `discounts +bxgy-code --buy ... --get ...` |
| Flash sale (auto-applied, no code) | `discounts +flashsale` |
| Buy N, Nth-item % off (auto, no code) | `discounts +mn-discount --tiers "n:%,..."` |
| Spend / buy-N threshold → auto discount | `discounts +rebate --target order\|product --tiers "th:disc,..."` |
| Search / filter discounts | `discounts +search --query ... --progress ongoing` |
| Get one discount by ID | `discounts get --params '{"id":"<id>"}'` |
| Get one discount by code | `discounts get-by-code --params '{"discount_code":"<CODE>"}'` |
| Cancel discount(s) | `discounts cancel --data '{"ids":["<id>",...]}'` |
| Restart a paused discount | `discounts restart --data '{"id":"<id>"}'` |
| Delete one finished discount | `discounts delete --params '{"id":"<id>"}'` |
| Delete many finished discounts | `discounts batch-delete --data '{"ids":[...]}'` |
| Update an automatic discount | `discounts update-automatic --params '{"id":"<id>"}' --data '<body>'` |
| Update a non-automatic (code) discount | `discounts update-non-automatic --params '{"id":"<id>"}' --data '<body>'` |
| Configure cross-type combination rules | `discounts combine --data '<rules>'` |
| Coupon campaigns (vouchers) | `discounts coupons {create,get,update}` |

There is **no `+update`, `+cancel`, or `+restart` shortcut.** Use the spec leaves.

## Acting on a creation request

When the user says "帮我创建一个X营销活动" / "create a flash sale" / "建个优惠码" / similar — that is an **action request**, not a question. Behavior:

1. **Match intent to a shortcut** via the Chinese/English trigger table below.
2. **Check required fields** for that shortcut against the user's message. If ANY required field is missing or ambiguous, **ASK using the `AskUserQuestion` tool — do not fabricate values.** Real coupon campaigns cost the merchant money; "I'll just default to 满100减10" is the wrong call.
3. **This skill overrides session-level "no clarifying questions" preferences** for missing required fields specifically. The user explicitly opted into clarification for discount creation.
4. **If ALL required fields are present** in the user's message, run the command immediately (with `--dry-run` first only if explicitly asked; otherwise direct, since `+shortcuts` are designed to be safe).
5. **Never ask about a flag that has a CLI default.** This is absolute. If `<shortcut> --help` shows `(default …)`, `default: …`, or `auto-generated if omitted`, that flag MUST NOT appear in any `AskUserQuestion`. Use the default silently. Only flags marked `(required)` AND with no default are askable. Even semantically-meaningful flags (e.g. `--type`, `--scope`) with a CLI default are off-limits — let the default win, and override only if the user's own wording disambiguates (e.g. "8折" → `--type=amount-percent`; otherwise stay on default).

**Never-ask list** (these all have CLI defaults — never appear as a question):
`--start` (default `now`) · `--end` (default `-1`/forever) · `--name` (auto-generated) · `--code` (auto-generated) · `--combines` (default empty = no stacking) · `--limit-order` (default `1` for `+rebate` amount-off/qty-off) · `--type` (default `amount-off` for `+rebate`, `percent` for `+flashsale`) · `--scope` (default `highest` for `+mn-discount`) · `--price-rule` (default `price`) · `--follow-stock` (default `product`) · `--product-order` (default `desc`) · `--min` (default 0) · `--min-qty` (default 1) · `--countries` (default all) · `--limit-user-type` (default `no_limit`) · `--limit-user-count` (default `-1`).

When in doubt, run `<shortcut> --help` and check: any flag whose line contains "(default ...)" or "auto-generated" goes to defaults, not to the question list.

### Trigger phrase → shortcut

| User says | Shortcut | How to extract values |
|---|---|---|
| 满X减Y / 订单满X减Y / spend X get Y off | `+rebate --target order --type amount-off` | `--tiers "X:Y"`; multi-tier "满200减20,满500减50" → `"200:20,500:50"` |
| 商品满X减Y / product spend X get Y | `+rebate --target product --type amount-off` | needs `--products` or `--collections` or `--variants` |
| 满N件减Y元 / buy N get Y off | `+rebate --type qty-off` | `--tiers "N:Y"` |
| 满X享Y折 / 满X打Y折 / spend X get Y% off | `+rebate --type amount-percent` | Y is the **discount %**, e.g. "8折" → 20 (i.e. 20% off) — **confirm if ambiguous** |
| 满N件享Y折 | `+rebate --type qty-percent` | |
| 第N件Y折 / 买N件第N件Y折 / Nth item Y% off | `+mn-discount` | `--tiers "N:Y"` where Y is % off |
| 闪购 / 限时秒杀 / 限时折扣 / flash sale | `+flashsale` | auto-applied, no coupon code; needs `--variants` OR `--collections` |
| 优惠码 + X% off / 折扣码立减Y% / X% off coupon | `+percent-code` | `--off` is the percent (1-99) |
| 优惠码立减Y元 / 折扣码减¥Y / $Y off coupon | `+amount-code` | `--off` is the amount |
| 包邮码 / 免邮券 / free shipping code | `+free-shipping-code` | optional `--countries` |
| 买X送Y / 买X赠Y / buy X get Y free | `+bxgy-code` | `--buy "pid:qty"`, `--get "pid:qty"`; use `--get-off` or `--get-discount` for partial |

If multiple rows match, **ask which one**. Don't guess.

### Required-vs-ask matrix

**ALL 7 shortcuts require `--limit-max` and `--limit-user` explicitly** (no default — the CLI rejects without them). Pass `-1` for "no limit". If the user did not state usage caps, **ASK**; do not silently assume `-1`.

| Shortcut | Must ASK if user did not specify | Infer if possible | Default silently |
|---|---|---|---|
| `+percent-code` | `--off` (the %), `--target`, `--limit-max`, `--limit-user`, `--products` if `--target=product` | `--target` from "订单/商品" wording | `--min`, `--combines`, `--code` (auto-generated), `--name`, time |
| `+amount-code` | `--off` (¥), `--target`, `--limit-max`, `--limit-user`, `--products` if `--target=product` | `--target` from wording | same as `+percent-code` |
| `+free-shipping-code` | `--limit-max`, `--limit-user` | — | `--min`, `--countries`, `--combines`, `--code`, `--name`, time |
| `+bxgy-code` | `--buy` ids, `--get` ids, `--limit-max`, `--limit-user` | qty from "买N件送M件" → `:N`/`:M` suffixes; `--get-off`/`--get-discount` from "立减"/"打折" wording (omit both = free get item) | `--combines`, `--code`, `--name`, time |
| `+flashsale` | `--discount` value, **either** `--variants` **or** `--collections`, `--limit-max`, `--limit-user` | `--type` from wording (`percent` for "X折/X%off", `fixed-price` for "一口价Y元", `off` for "立减Y元") | `--price-rule`, `--follow-stock`, `--combines`, time |
| `+mn-discount` | `--tiers`, `--limit-max`, `--limit-user` | `--scope` ("最高档"=`highest`, "全部档"=`all`) | `--product-order`, `--combines`, `--name`, time |
| `+rebate` | `--target`, `--tiers`, `--limit-max`, `--limit-user` | `--type` from wording — see trigger table above | `--limit-order` (default 1), `--combines`, `--type` if user didn't disambiguate, `--name`, time |

### Decision examples

| User says | Verdict |
|---|---|
| "帮我建个满减活动" | ASK — tiers, target, caps all missing |
| "创建一个满减活动，满200减20" | ASK — target unclear (订单 or 商品?), caps missing |
| "创建一个满200减20的订单级满减活动，永不过期，无次数限制" | CREATE — all required present (`+rebate --target order --tiers "200:20" --end forever --limit-max -1 --limit-user -1`) |
| "Create a 15% off coupon code" | ASK — caps missing, target unclear |
| "Create a 15% off site-wide coupon code, max 100 uses, 1 per customer" | CREATE (`+percent-code --target order --off 15 --limit-max 100 --limit-user 1`) |
| "创建闪购活动 50% off" | ASK — variants/collections missing, caps missing |

### How to ask

Use `AskUserQuestion` with concrete numbered options. Bundle related fields into one batch (≤4 questions per call). The **complete set of askable questions** is:

| Question type | When asked | Sample options |
|---|---|---|
| Usage caps (`--limit-max` + `--limit-user`) | Always missing — required by every shortcut | "总不限+每人不限"、"总不限+每人1次"、"指定（请输入）" |
| Target scope (`--target`) | For `+percent-code`/`+amount-code`/`+rebate` when user didn't say "订单/商品" | "订单级"、"商品级" |
| Tiers (`--tiers`) | For `+rebate`/`+mn-discount` when not given | free-form: list of `阈值:折扣` pairs |
| Discount value (`--off` / `--discount`) | When user gave only the type ("打个折") without a number | free-form numeric |
| Product targeting (`--products`/`--collections`/`--variants`) | When target=product / shortcut needs scope | free-form ID list |
| BXGY buy/get products (`--buy`, `--get`) | For `+bxgy-code` if IDs not given | free-form ID list |

That's it. **Do not bundle anything not in this table** — every other flag has a CLI default. Re-read the user's message before asking, and skip any question whose answer is already in it.

## Creating: minimum required flags

All 7 shortcuts REQUIRE both `--limit-max` and `--limit-user` to be set explicitly (no defaults; pass `-1` for "no limit"). This is gated server-side to prevent unbounded usage. Other required flags vary per shortcut:

| Shortcut | Required flags |
|----------|----------------|
| `+percent-code` | `--target` `--off` `--limit-max` `--limit-user` (+ `--products` if `--target=product`) |
| `+amount-code` | `--target` `--off` `--limit-max` `--limit-user` (+ `--products` if `--target=product`) |
| `+free-shipping-code` | `--limit-max` `--limit-user` |
| `+bxgy-code` | `--buy` `--get` `--limit-max` `--limit-user` |
| `+flashsale` | `--discount` `--limit-max` `--limit-user` (+ `--variants` OR `--collections`) |
| `+mn-discount` | `--tiers` `--limit-max` `--limit-user` |
| `+rebate` | `--target` `--tiers` `--limit-max` `--limit-user` |

**Compact pair syntaxes** (the `--tiers` / `--buy` / `--get` strings):

| Flag | Format | Example |
|------|--------|---------|
| `+rebate --tiers` | `threshold:discount,...` | `"200:20,500:50"` (满200减20，满500减50) |
| `+mn-discount --tiers` | `Nth-item:percent,...` | `"3:50,5:70"` (买3件第3件50%off，买5件第5件70%off) |
| `+bxgy-code --buy` | `id1,id2[:qty]` | `"pid1,pid2:2"` |
| `+bxgy-code --get` | `id[:qty]` | `"pid3:1"` |

**Time forms** for `--start` / `--end` (any shortcut):

| Form | Meaning |
|------|---------|
| `now` | current time |
| `+30d` / `+2w` / `+12h` | offset from now |
| `2026-11-01` | UTC midnight on that date |
| `<unix-int>` | raw Unix seconds |
| `forever` / `-1` | no expiry (`--end` only) |

**Discount stacking:** the `--combines` flag controls whether *this* discount may stack with others. Default is empty (no stacking). To allow: `--combines order,product,shipping` (subset). For *global* cross-type rules (independent of `--combines`), use `discounts combine`.

## Discovering

**Prefer `+search` over `list`.** Both hit `GET /discounts`, but `+search` has named flags; `list` only takes `--params` JSON.

```bash
discounts +search --progress ongoing --discount-type flashsale --page-limit 250
discounts +search --query "spring" --discount-method discount_code
```

Multi-value filters (`--progress`, `--discount-type`, `--discount-method`, `--discount-target`) take comma-separated lists.

**Response envelope** wraps every successful body in `.data`. The discount object is at `.data.discount.{discount_info,discount_layer,discount_rule,entitled_product,...}`. **jq paths must start at `.data`.** Example:

```bash
discounts get-by-code --params '{"discount_code":"CODE123"}' \
  --jq '.data.discount.discount_info.id'
```

`--jq` is a single-string flag and outputs raw scalars by default (no surrounding quotes, just a trailing newline). **Do NOT pass `-r`** — cobra parses it as a separate flag and rejects the command.

## `discount_type` enum

These are the **internal** API names — use them with `--discount-type` / inside `--params`. They are NOT user-facing labels.

```
flashsale                m_n_discount
rebate_cta_otr           rebate_ctq_otr           rebate_cta_otp           rebate_ctq_otp
code_percent             code_fix_price_reduction code_bxgy                code_free_shipping
```

Mapping back to shortcuts:

| Shortcut writes | `discount_type` |
|-----------------|-----------------|
| `+percent-code` | `code_percent` |
| `+amount-code` | `code_fix_price_reduction` |
| `+free-shipping-code` | `code_free_shipping` |
| `+bxgy-code` | `code_bxgy` |
| `+flashsale` | `flashsale` |
| `+mn-discount` | `m_n_discount` |
| `+rebate --target=order --type=amount-off` | `rebate_cta_otr` |
| `+rebate --target=product --type=qty-off` | `rebate_ctq_otr` |
| `+rebate ... --type=amount-percent / qty-percent` | `rebate_cta_otp` / `rebate_ctq_otp` |

Filtering with `--discount-type rebate` returns nothing — that label doesn't exist; use one of the four `rebate_*` enums.

## Lifecycle — body shapes are asymmetric

Watch the singular vs plural. This is the most common spec-leaf footgun.

```bash
discounts cancel        --data '{"ids":["A","B"]}'   # ARRAY (one or many)
discounts batch-delete  --data '{"ids":["A","B"]}'   # ARRAY (must be finished)
discounts restart       --data '{"id":"A"}'          # SINGULAR — one id only
discounts delete        --params '{"id":"A"}'        # path param, no body, one id
```

`delete` and `batch-delete` only work on discounts whose `progress == finished`. To delete an active one: `cancel` it first, then `delete`.

## Updating

There is no `+update` shortcut. Compose the update body from a `get` and feed it back:

```bash
discounts get --params '{"id":"<id>"}' --jq '.data.discount' > /tmp/d.json
# edit /tmp/d.json …
discounts update-automatic --params '{"id":"<id>"}' --data @/tmp/d.json --dry-run
```

Use `update-automatic` for flash sales / rebates / mn-discounts (no code), `update-non-automatic` for code-bearing discounts. The body shape matches the corresponding `create-*` endpoint — run `schema discounts.update-automatic` for fields.

## Coupons subgroup vs discount codes

`discounts coupons {create,get,update}` is a **different concept** from `+*-code` discount codes:

- **Discount codes** (the `+*-code` shortcuts): public codes shoppers type at checkout.
- **Coupons** (`discounts coupons …`): vouchers assigned to specific customers, distributed by marketing campaigns.

Default to the `+*-code` shortcuts. Only reach for `coupons` when the user explicitly says "coupon campaign", "voucher", "assign to customer", or similar.

## Common gotchas

| Symptom | Cause | Fix |
|---------|-------|-----|
| `jq: error` or `null` after `get` / `get-by-code` | jq path missed `.data` wrapper | Start jq at `.data.discount.…` |
| `--discount-type rebate` matches nothing | "rebate" is not an enum | Use `rebate_cta_otr` / `rebate_ctq_otr` / `rebate_cta_otp` / `rebate_ctq_otp` |
| `--limit-max is required` on a code/mn shortcut | Code-type & mn shortcuts gate on explicit caps | `--limit-max -1 --limit-user -1` for "no limit" |
| `restart --data '{"ids":["X"]}'` fails | `restart` body is singular | `--data '{"id":"X"}'` |
| `delete` returns 4xx with progress message | Discount is not `finished` | `cancel` first, then `delete` |
| `display_name` looks truncated in response | Server auto-truncates to ~20 chars | Pass `--name` explicitly if it matters |
| Free shipping needs to be limited to some countries | `--countries` accepts ISO codes or `all` | `--countries US,CA` |
| Created two discounts with the same `--code` | Codes are unique per store | Pass an explicit unique `--code`, or omit to auto-generate `CLI-XXXXXX` |
| Wanted to update via shortcut | There is no `+update` | Use `update-automatic` / `update-non-automatic` with a full body |
| `--jq -r '...'` exits with the Usage block | `-r` is parsed as a separate cobra flag | Drop `-r`; `--jq` already outputs raw scalars |

## Recipes

```bash
# 1. Spend $200 → $20 off, auto-applied, no expiry
discounts +rebate --target order --tiers "200:20" --end forever

# 2. 15% off site-wide coupon, max 100 redemptions, 1 per shopper
discounts +percent-code --target order --off 15 --limit-max 100 --limit-user 1 --code SPRING15

# 3. Find all ongoing flash sales
discounts +search --progress ongoing --discount-type flashsale --page-limit 250 \
  --jq '.data[] | {id: .discount_info.id, name: .discount_info.discount_name}'

# 4. Cancel a code-bearing discount by its code
ID=$(discounts get-by-code --params '{"discount_code":"SPRING15"}' --jq '.data.discount.discount_info.id')
discounts cancel --data "{\"ids\":[\"$ID\"]}" --dry-run     # check first
discounts cancel --data "{\"ids\":[\"$ID\"]}"               # then run

# 5. Hard-delete the now-finished discount
discounts delete --params "{\"id\":\"$ID\"}"
```

## References

- Per-command flags: `shoplazza discounts <cmd> --help`
- Spec-leaf parameter & body schema: `shoplazza schema discounts.<cmd>`
- Shortcut source of truth: `shortcuts/discounts/*.go` and `shortcuts/discounts/CHANGELOG.md`

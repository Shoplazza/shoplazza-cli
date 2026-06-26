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
| % off (with coupon code) | `discounts +percent-code --target order\|product --percent <1-99>` |
| Fixed amount off (with coupon code) | `discounts +amount-code --target order\|product --off <amt>` |
| Free shipping coupon | `discounts +free-shipping-code` |
| Buy X get Y (with coupon code) | `discounts +bxgy-code` (buy-side scope + get-side scope) |
| Flash sale (auto-applied, no code) | `discounts +flashsale --value ...` |
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
2. **Check required fields** (the *no-default* flags listed per shortcut below) against the user's message. If ANY required field is missing or ambiguous, **ASK using the `AskUserQuestion` tool — do not fabricate values.** Real discounts cost the merchant money; "I'll just default to 满100减10" is the wrong call.
3. **For these required, no-default fields, this skill overrides session-level "no clarifying questions" preferences.** Usage caps are NOT in this set anymore (see below).
4. **If ALL required fields are present** in the user's message, run the command immediately (with `--dry-run` first only if explicitly asked; otherwise direct, since `+shortcuts` are designed to be safe).
5. **Never ask about a flag that has a CLI default.** This is absolute. If `<shortcut> --help` shows `(default …)`, `default: …`, or `auto-generated if omitted`, that flag MUST NOT appear in any `AskUserQuestion`. Use the default silently. Even semantically-meaningful flags (e.g. `--type`, `--scope`) with a CLI default are off-limits — let the default win, and override only if the user's own wording disambiguates (e.g. "8折" → `--type=amount-percent`; otherwise stay on default).

**Usage caps are now optional (omit = no limit).** `--limit-max` / `--limit-user` (and flashsale's `--limit-user-variant/-product/-all`) have **no default to set** — omit them and the discount is unlimited. **Do NOT ask about caps** and do NOT pass `-1` (the CLI rejects `-1`: "must be > 0"). One soft exception: for an **open-ended, money-costing code/coupon campaign** (no end date + no caps), it is fine to *mention* in your reply that redemptions will be unlimited — but mention, do not block, and do not turn it into an `AskUserQuestion`.

**Never-ask list** (all have CLI defaults or omit-to-disable — never appear as a question):
`--start` (default `now`) · `--end` (default `-1`/forever) · `--name` (auto-generated) · `--code` (auto-generated) · `--combines` (default empty = no stacking) · `--limit-max` / `--limit-user` (omit = no limit) · `--limit-user-variant` / `--limit-user-product` / `--limit-user-all` (flashsale; omit = no limit) · `--limit-order` (`+bxgy-code`, default `1`) · `--limit-order-once` (`+amount-code` / `+rebate`, default `true`) · `--type` (default `amount-off` for `+rebate`, `percent` for `+flashsale`) · `--scope` (default `highest` for `+mn-discount`) · `--price-rule` (default `price` for `+flashsale`) · `--price-sort` (default `desc` for `+mn-discount`) · `--min-amount` (default 0) · `--min-quantity` (default 1) · `--countries` (default all) · `--stock` (omit = follow product stock) · `--exclude` (default off) · `--customer-segments` (omit = all customers eligible).

When in doubt, run `<shortcut> --help` and check: any flag whose line contains "(default ...)", "auto-generated", or "omit …" goes to defaults, not to the question list.

### Trigger phrase → shortcut

| User says | Shortcut | How to extract values |
|---|---|---|
| 满X减Y / 订单满X减Y / spend X get Y off | `+rebate --target order --type amount-off` | `--tiers "X:Y"`; multi-tier "满200减20,满500减50" → `"200:20,500:50"` |
| 商品满X减Y / product spend X get Y | `+rebate --target product --type amount-off` | needs `--products` or `--collections` or `--variants` |
| 满N件减Y元 / buy N get Y off | `+rebate --type qty-off` | `--tiers "N:Y"` |
| 满X享Y折 / 满X打Y折 / spend X get Y% off | `+rebate --type amount-percent` | Y is the **discount %**, e.g. "8折" → 20 (i.e. 20% off) — **confirm if ambiguous** |
| 满N件享Y折 | `+rebate --type qty-percent` | |
| 第N件Y折 / 买N件第N件Y折 / Nth item Y% off | `+mn-discount` | `--tiers "N:Y"` where Y is % off |
| 闪购 / 限时秒杀 / 限时折扣 / flash sale | `+flashsale` | auto-applied, no coupon code; `--value` + `--type`; scope via `--variants`/`--collections` (omit = all products) |
| 优惠码 + X% off / 折扣码立减Y% / X% off coupon | `+percent-code` | `--percent` is the percent (1-99) |
| 优惠码立减Y元 / 折扣码减¥Y / $Y off coupon | `+amount-code` | `--off` is the amount |
| 包邮码 / 免邮券 / free shipping code | `+free-shipping-code` | optional `--off` (partial), `--countries` |
| 买X送Y / 买X赠Y / buy X get Y free | `+bxgy-code` | buy-side scope + `--buy-quantity`/`--buy-amount`; get-side scope + `--get-quantity`; `--get-free` / `--get-percent` / `--get-off` |

If multiple rows match, **ask which one**. Don't guess.

### Required-vs-ask matrix

Only the **no-default** flags are askable. Caps are NOT here — they default to no-limit.

| Shortcut | Must ASK if user did not specify | Infer if possible | Default silently |
|---|---|---|---|
| `+percent-code` | `--percent` (the %), `--target`, `--products`/`--variants`/`--collections` if `--target=product` | `--target` from "订单/商品" wording | `--min-amount`, `--min-quantity`, caps, `--combines`, `--code`, `--name`, time, `--customer-segments`, `--exclude` |
| `+amount-code` | `--off` (¥), `--target`, scope if `--target=product` | `--target` from wording | same as `+percent-code`, plus `--limit-order-once` |
| `+free-shipping-code` | — (nothing strictly required) | `--off` only if user said "立减Y元运费" | `--off`, `--min-amount`, `--min-quantity`, `--countries`, caps, `--combines`, `--code`, `--name`, time |
| `+bxgy-code` | buy-side scope (`--products`/`--variants`/`--collections`), `--buy-quantity` **or** `--buy-amount`, get-side scope (`--get-products`/`--get-variants`/`--get-collections`), `--get-quantity`, one of `--get-free`/`--get-percent`/`--get-off` | buy/get qty from "买N件送M件"; `--get-percent`/`--get-off` from "打折"/"立减" wording (neither = `--get-free`) | `--limit-order` (default 1), caps, `--combines`, `--code`, `--name`, time, `--exclude` |
| `+flashsale` | `--value` | `--type` from wording (`percent` for "X折/X%off", `fixed-price` for "一口价Y元", `off` for "立减Y元"); scope from named products | scope (omit = all products), `--price-rule`, `--stock`, per-user caps, `--combines`, `--name`, time |
| `+mn-discount` | `--tiers` | `--scope` ("最高档"=`highest`, "全部档"=`all`) | `--scope`, `--price-sort`, scope flags, caps, `--combines`, `--name`, time, `--exclude` |
| `+rebate` | `--target`, `--tiers` | `--type` from wording — see trigger table above | scope if `--target=order`, `--limit-order-once`, caps, `--combines`, `--type` if user didn't disambiguate, `--name`, time |

### Decision examples

| User says | Verdict |
|---|---|
| "帮我建个满减活动" | ASK — tiers + target missing |
| "创建一个满减活动，满200减20" | ASK — target unclear (订单 or 商品?) |
| "创建一个满200减20的订单级满减活动，永不过期" | CREATE — all required present (`+rebate --target order --tiers "200:20" --end forever`); caps omitted = unlimited |
| "Create a 15% off coupon code" | ASK — target unclear (order or product?) |
| "Create a 15% off site-wide coupon code, max 100 uses, 1 per customer" | CREATE (`+percent-code --target order --percent 15 --limit-max 100 --limit-user 1`) |
| "创建闪购活动 50% off" | CREATE — `--value 50` is enough (`+flashsale --value 50`); scope optional. Optionally mention it will apply store-wide unless products are specified. |

### How to ask

Use `AskUserQuestion` with concrete numbered options. Bundle related fields into one batch (≤4 questions per call). The **complete set of askable questions** is:

| Question type | When asked | Sample options |
|---|---|---|
| Target scope (`--target`) | For `+percent-code`/`+amount-code`/`+rebate` when user didn't say "订单/商品" | "订单级"、"商品级" |
| Tiers (`--tiers`) | For `+rebate`/`+mn-discount` when not given | free-form: list of `阈值:折扣` pairs |
| Discount value (`--percent` / `--off` / `--value`) | When user gave only the type ("打个折") without a number | free-form numeric |
| Product targeting (`--products`/`--collections`/`--variants`) | When target=product / a shortcut needs scope | free-form ID list |
| BXGY buy/get sides | For `+bxgy-code` if scope or trigger qty not given | free-form ID list + quantities |

That's it. **Do not bundle anything not in this table** — every other flag (including usage caps) has a CLI default or omit-to-disable. Re-read the user's message before asking, and skip any question whose answer is already in it.

## Creating: minimum required flags

Required = flags the CLI rejects the command without (no default). Usage caps are **not** required — omit for no limit.

| Shortcut | Required flags |
|----------|----------------|
| `+percent-code` | `--target` `--percent` (+ a scope flag if `--target=product`) |
| `+amount-code` | `--target` `--off` (+ a scope flag if `--target=product`) |
| `+free-shipping-code` | *(none)* |
| `+bxgy-code` | buy scope · (`--buy-quantity` \| `--buy-amount`) · get scope · `--get-quantity` · (`--get-free` \| `--get-percent` \| `--get-off`) |
| `+flashsale` | `--value` |
| `+mn-discount` | `--tiers` |
| `+rebate` | `--target` `--tiers` (+ a scope flag if `--target=product`) |

"Scope flag" = one of `--products` / `--variants` / `--collections` (mutually exclusive).

**Compact pair / list syntaxes:**

| Flag | Format | Example |
|------|--------|---------|
| `+rebate --tiers` | `threshold:discount,...` | `"200:20,500:50"` (满200减20，满500减50) |
| `+mn-discount --tiers` | `Nth-item:percent,...` | `"3:50,5:70"` (买3件第3件50%off，买5件第5件70%off) |
| `+bxgy-code` buy side | `--products P1,P2 --buy-quantity 2` (or `--buy-amount 100`) | qty/amount across the buy-side items that triggers the offer |
| `+bxgy-code` get side | `--get-products P3 --get-quantity 1 --get-free` | how many get-side items are discounted, and how |

**Time forms** for `--start` / `--end` (any shortcut):

| Form | Meaning |
|------|---------|
| `now` | current time |
| `+30d` / `+2w` / `+12h` | offset from now |
| `2026-11-01` | UTC midnight on that date |
| `2026-11-01T08:00:00` | explicit UTC datetime |
| `<unix-int>` | raw Unix seconds |
| `forever` / `-1` | no expiry (`--end` only) |

**Targeting / eligibility helpers** (all optional, on most shortcuts):
- `--exclude` — turns the scope IDs into a **blocklist** (apply to all products EXCEPT those listed). Needs a scope flag to have effect.
- `--customer-segments <ids>` — restrict the discount to customers in the given segment IDs (omit = all customers eligible).

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
| `--limit-max must be > 0 (got -1)` | Caps are positive-only now; `-1` is rejected | **Omit** the cap entirely for "no limit" — don't pass `-1` |
| `unknown flag: --off` on `+percent-code` | percent-code uses `--percent`, not `--off` | `--percent 15` (1-99) |
| `unknown flag: --discount` on `+flashsale` | flashsale uses `--value` | `--value 50` (+ `--type`) |
| `+bxgy-code --buy/--get` rejected | Those flags don't exist | Buy side: scope flag + `--buy-quantity`/`--buy-amount`; get side: `--get-*` scope + `--get-quantity` + `--get-free`/`--get-percent`/`--get-off` |
| `restart --data '{"ids":["X"]}'` fails | `restart` body is singular | `--data '{"id":"X"}'` |
| `delete` returns 4xx with progress message | Discount is not `finished` | `cancel` first, then `delete` |
| `--target=product` rejected with "requires one of --products/--collections/--variants" | Product scope is mandatory when target=product | Pass a scope flag |
| `display_name` looks truncated in response | Server auto-truncates to ~20 chars | Pass `--name` explicitly if it matters |
| Created two discounts with the same `--code` | Codes are unique per store | Pass an explicit unique `--code`, or omit to auto-generate `CLI-XXXXXX` |
| Wanted to update via shortcut | There is no `+update` | Use `update-automatic` / `update-non-automatic` with a full body |
| `--jq -r '...'` exits with the Usage block | `-r` is parsed as a separate cobra flag | Drop `-r`; `--jq` already outputs raw scalars |

## Recipes

```bash
# 1. Spend $200 → $20 off, auto-applied, no expiry, unlimited
discounts +rebate --target order --tiers "200:20" --end forever

# 2. 15% off site-wide coupon, max 100 redemptions, 1 per shopper
discounts +percent-code --target order --percent 15 --limit-max 100 --limit-user 1 --code SPRING15

# 3. Buy 2 of P1, get 1 of P2 free, single set per order
discounts +bxgy-code --products P1 --buy-quantity 2 --get-products P2 --get-quantity 1 --get-free

# 4. Find all ongoing flash sales
discounts +search --progress ongoing --discount-type flashsale --page-limit 250 \
  --jq '.data[] | {id: .discount_info.id, name: .discount_info.discount_name}'

# 5. Cancel a code-bearing discount by its code
ID=$(discounts get-by-code --params '{"discount_code":"SPRING15"}' --jq '.data.discount.discount_info.id')
discounts cancel --data "{\"ids\":[\"$ID\"]}" --dry-run     # check first
discounts cancel --data "{\"ids\":[\"$ID\"]}"               # then run

# 6. Hard-delete the now-finished discount
discounts delete --params "{\"id\":\"$ID\"}"
```

## References

- Per-command flags: `shoplazza discounts <cmd> --help`
- Spec-leaf parameter & body schema: `shoplazza schema discounts.<cmd>`
- Shortcut source of truth: `shortcuts/discounts/*.go`

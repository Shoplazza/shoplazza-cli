---
name: shoplazza-products
description: >-
  Use when the user wants to manage a shoplazza store's product catalog through the CLI —
  creating / publishing / unpublishing products (建品 / 新建商品 / 上新 / 上架 / 下架),
  searching or counting the catalog (搜商品 / 找商品 / 商品数量 / how many products I sell),
  changing prices (改价 / 调价 / set price), adjusting stock (库存 / 补货 / 加库存 / 减库存 /
  inventory), product tags (标签), variants / SKUs (变体 / 规格), collections
  and collects (商品合集 / 商品分组), product images (商品图片 / 主图), gift cards (礼品卡 /
  储值卡 — gift cards live HERE, not in discounts), customer product reviews (买家评价 / 评论
  / product reviews), warehouse locations (仓库 / 库位), suppliers and procurement (供应商 /
  采购单 / 进货). NOT discount codes / coupons / 优惠码 / 折扣码 / 满减 (→
  shoplazza-discounts); NOT the store media library / file uploads / 媒体库文件 and NOT
  metafields / 元字段 (→ shoplazza-shop); NOT order fulfillment / 发货 (→ orders domain).
---

# shoplazza CLI — products module

**CRITICAL — before anything else, use Read on [`../shoplazza-common/SKILL.md`](../shoplazza-common/SKILL.md).**
It owns every cross-cutting mechanic: the three access tiers, the output envelope (`.data`),
`--dry-run`, `--jq` (incl. "don't pass `-r`"), `--fields`, `schema`, `api rest`, auth / profiles,
and the safety protocol. This file covers only the products domain and never repeats them.

## Overview

The `products` module manages the product catalog: products and their variants, prices,
inventory, tags, images, collections, gift cards, customer reviews, warehouse locations,
suppliers, and procurement. All three access tiers apply (tier-selection rule is in
shoplazza-common — prefer `+shortcut`).

There is **no `+update` or `+delete` shortcut** — updates and deletes are spec leaves.
Products is a large domain: this file is the router plus the high-frequency operations;
per-sub-resource depth lives in [`references/`](#references) (variants, inventory/locations,
collections/collects, images, gift cards, comments, procurement/suppliers).

## Command map

Intent → command, highest-fit tier first. The authoritative flags/params live in
`shoplazza products <cmd> --help` and `shoplazza schema products.<cmd>`, not this table.

| User intent | Command |
|-------------|---------|
| Search / filter products | `products +search [--keyword <title-word>] [--vendor <exact>] [--published published\|unpublished\|any] [--collection-id <id>] [--fields id,title]` |
| Count products | `products +count [--published …]` |
| Create a single-variant product | `products +create --title <name> --price <n> --image <url> [--published]` |
| Publish a product | `products +publish --id <product-id>` |
| Unpublish (下架) a product | `products +unpublish --id <product-id>` |
| Change a variant's price | `products +set-price (--variant-id <id> \| --sku <sku> [--all]) --price <n>` |
| Adjust stock (add or decrease) | `products +stock --variant-id <id> (--adjust <±n> \| --set <n>) [--location-id <id>]` |
| Add / remove / replace tags | `products +tag --id <product-id> (--add a,b \| --remove c,d \| --set x,y)` |
| Get one product | `products get --params '{"product_id":"<id>"}'` |
| List (filters `+search` lacks: ids, handles, date ranges, spus…) | `products list --params '{…}'` |
| Multi-variant / full-control create | `products create --data '{"product":{…}}'` (see `schema products.create`) |
| Update product fields | `products update --params '{"product_id":"<id>"}' --data '{"product":{…}}'` |
| Delete ONE product | `products delete --params '{"product_id":"<id>"}'` |
| Delete MANY products | `products batch-delete --params '{"product_ids":["<id>",…]}'` |
| List a product's variants | `products variants list --params '{"product_id":"<id>"}'` → [references/variants.md](references/variants.md) |
| Update a variant (options, sku, weight…) | `products variants update --params '{"variant_id":"<id>"}' --data '{"variant":{…}}'` |
| Look up variants by SKU | `products variants list-by-sku --params '{"sku":"<sku>"}'` |
| Create a curated collection | `products collections create --data '{"collection":{"title":"…"}}'` → [references/collections-collects.md](references/collections-collects.md) |
| Add a product to a collection | `products collects create --data '{"collect":{"collection_id":"<cid>","product_id":"<pid>"}}'` |
| Attach an image to a product | `products images create --params '{"product_id":"<id>"}' --data '{"image":{"src":"<url>"}}'` → [references/images.md](references/images.md) |
| Create a gift card (stored value = money → `--dry-run` first) | `products gift-cards create --data '{"gift_card":{"code":"…","initial_value":"…","expires_on":"YYYY-MM-DD"}}' --dry-run` → [references/gift-cards.md](references/gift-cards.md) |
| Check a gift card's balance | `products gift-cards get --params '{"id":"<id>"}' --jq '.data.gift_card.balance'` |
| Stop / "delete" a gift card (irreversible → `--dry-run` first + go-ahead) | `products gift-cards disable --params '{"id":"<id>"}' --dry-run` (**no delete endpoint exists**; disabling permanently stops redemption) |
| Inventory levels / items, locations | `products inventory …` / `products locations …` → [references/inventory-locations.md](references/inventory-locations.md) |
| Customer reviews (评价) | `products comments {list,create,batch-create}` → [references/comments.md](references/comments.md) |
| Suppliers / procurement orders | `products suppliers …` / `products procurement …` → [references/procurement-suppliers.md](references/procurement-suppliers.md) |
| Storefront categories | `products categories list` |

## Acting on a request

"新建商品 / 上架 / 调价 / 补货 / create a product / restock" is an **action**, not a question:

1. **Match intent to a shortcut** via the trigger table below.
2. **Check required fields** (each shortcut's *no-default* flags). Missing or ambiguous →
   **ask with `AskUserQuestion`, never fabricate** — especially `--price` (money) and
   `--image` (an asset URL you cannot invent). Bundle the missing fields into ONE question
   batch, not a series.
3. **Never ask about a flag that has a CLI default or is omit-to-disable.** Let the default win.
4. **All required present** → run it. `+shortcuts` are safe to run directly; use `--dry-run`
   first for deletes, batch ops, and gift-card creation (stored value = money), per
   shoplazza-common → Safety protocol.
5. **Degrade honestly**: if the platform cannot do it (e.g. decreasing stock at a
   **non-default location** or on a **multi-location item** — see
   [references/inventory-locations.md](references/inventory-locations.md)), emit **no write
   command**; explain the limitation and offer a legitimate alternative.

### Trigger phrase → shortcut

| User says | Shortcut | How to extract values |
|---|---|---|
| 搜商品 / 找一下商品 / find / list products | `+search` | title word → `--keyword` (matches title only); 供应商/vendor → `--vendor` (exact match, verbatim); 未上架 → `--published unpublished`; 已上架 → `--published published` |
| 有多少商品 / 商品总数 / how many products do I sell | `+count` | 已上架 → `--published published`; count comes from `.data.count` — never count by hand |
| 新建商品 / 建品 / 上新 / create a new product | `+create` | product name → `--title` verbatim; 售价 → `--price`; 主图/图片 URL → `--image`; "上架" wording → add `--published`; "存草稿/draft" → **omit** `--published` |
| 上架商品 X / publish product X | `+publish` | id → `--id`; only a name? resolve via `+search --keyword` first |
| 下架 / 隐藏商品 / unpublish | `+unpublish` | id → `--id` |
| 改价 / 调价 / change the price to X | `+set-price` | variant id → `--variant-id`; SKU → `--sku`; new price → `--price` verbatim; **`--price 0` CLEARS the price** (does not display ¥0) — surface that difference and get the user's confirmation before passing 0 |
| 划线价 / compare-at price | `+set-price --compare-price` | only when the user names it |
| 库存加 N / 补货 N 件 / add N stock | `+stock --adjust N` | 加/增加 N = positive delta → `--adjust N` |
| 库存设为 N / set stock to N | `+stock --set N` | absolute target (≥ 0), either direction; decreases are gated to the **default location** of a **single-location item** |
| 减库存 / 库存扣掉 N / reduce stock | `+stock --adjust -N` | 减/扣掉 N = negative delta → `--adjust -N`; the CLI refuses if the result would go below 0, if `--location-id` isn't the default location, or if the item is stocked at multiple locations — in those cases explain and offer `+unpublish` / stock-policy `deny` instead |
| 加标签（保留原有）/ add tags | `+tag --add` | tags comma-separated; existing tags kept |
| 去掉某标签 / remove a tag | `+tag --remove` | missing tags are ignored |
| 标签整个换成… / replace all tags | `+tag --set` | replaces the full list — only on explicit "replace/换成/只保留" wording |

If multiple rows match, **ask which one**. Don't guess.

### Required-vs-ask matrix

Only **no-default** flags are askable.

| Shortcut | Must ASK if user did not specify | Infer if possible | Default silently |
|---|---|---|---|
| `+create` | `--price`, `--image` (one batch; **never fabricate a price or an image URL**) | `--title` from the product name in the utterance; `--published` from 上架/publish wording | `--published` (omit = draft), `--sku`, `--stock`, `--stock-policy` (default `deny`), `--compare-price`, `--tags`, `--collection-ids` |
| `+set-price` | `--price` (never invent a number) | target flag from what the user gave: an id → `--variant-id`, a SKU → `--sku` | `--compare-price`; `--all` (only when the user explicitly wants every variant matching the SKU) |
| `+stock` | the amount; `--variant-id` if no variant is identified | `--adjust` vs `--set` from wording (加 N = `--adjust N`; 减/扣 N = `--adjust -N`; 设为 N = `--set N`) | `--location-id` (defaults to the default location) |
| `+publish` / `+unpublish` | `--id` (or resolve it via `+search` by name) | — | — (takes only `--id`) |
| `+tag` | `--id`, the tag values | add/remove/set mode from wording | — |
| `+search` / `+count` | *(nothing — all filters optional)* | filters from wording | `--page-limit`, `--fields` |

**Sub-resource asks** (leaf-tier, same discipline):
- `gift-cards create` — must ask `code` and `initial_value` if missing (**stored value = money;
  never fabricate**). `customer_id` / `send_email` are optional: clarifying the customer is
  fine, inventing one is not.
- `variants update` — send **only** the fields the user asked to change; never pad the body.

### Never-ask list

Flags with a CLI default or omit-to-disable — they never appear in a question:
`--location-id` (default location) · `--stock-policy` (default `deny`) · `--published`
(omit = draft; set only on explicit publish wording) · `--compare-price` · `--sku` /
`--stock` / `--tags` / `--collection-ids` on `+create` (optional) · `--page-limit` ·
`--fields` (use only when the user asks for specific fields) · `--all` (explicit bulk
override only).

### Decision examples

| User says | Verdict |
|---|---|
| "新建一个商品：Lycra 瑜伽裤，售价 39.99，主图用 https://…/yoga.jpg，先存草稿" | CREATE — `+create --title "Lycra 瑜伽裤" --price 39.99 --image https://…/yoga.jpg`; 存草稿 = **omit** `--published` |
| "帮我上架一个新的 T恤 商品" | ASK — price + image missing (one batch); title `T恤` is usable; plan `--published` (user said 上架) |
| "把 SKU TSHIRT-RED-M 的价格改成 24.99" | CREATE — `+set-price --sku TSHIRT-RED-M --price 24.99`; no `--all` for a single-variant intent |
| "给变体 998877 调一下价格" | ASK — the new price; target `--variant-id 998877` is already known |
| "给变体 12345 的库存加 50 件" | CREATE — `+stock --variant-id 12345 --adjust 50` |
| "把变体 88664 的库存减少 30 件" | CREATE — `+stock --variant-id 88664 --adjust -30`; if the CLI refuses (below 0 / non-default location / multi-location item), relay the reason and offer alternatives |
| "给老客户发一张礼品卡" | ASK — `initial_value` + `code` (money; never fabricate) |

## Boundaries

Reads like products, actually belongs elsewhere (and lookalikes this domain owns):

| Sounds like | Actually belongs to | Command |
|---|---|---|
| 优惠码 / 折扣码 / 满减 / flash sale / discount & coupon codes | `shoplazza-discounts` | `discounts +*` |
| 礼品卡 / 储值卡 / gift card (redeemable store credit) | **HERE** (not discounts, not billing) | `products gift-cards …` |
| 上传文件到媒体库 / store media library asset | `shoplazza-shop` | `shop files` / `shop +upload-file` |
| 给商品挂图 / image attached to a product | **HERE** | `products images create` |
| 商品元字段 / metafields on products or shop | `shoplazza-shop` | `shop metafields-resource` / `metafields-definition` |
| 库存同步 / 补货 / inventory & warehouse stock | **HERE** (not orders/fulfillment) | `products +stock` / `products inventory` |
| 买家评价 / 评论 / product reviews | **HERE** (not shop blogs/pages content) | `products comments …` |
| 博客 / 自定义页面 / storefront content pages | `shoplazza-shop` | `shop blogs` / `shop pages` |

**Collections vs collects — intra-domain trap:** `collections` are the curated collection
objects themselves (title, image, SEO, smart rules); `collects` are the *association records*
linking a product to a collection. "把商品加进合集" = `collects create`, never
`collections update`. See [references/collections-collects.md](references/collections-collects.md).

## Permissions · Scope

Authorization is by **domain** (the unit of `--domain`), not per command. Authorization flow is
in shoplazza-common → Authentication.

| Operation | Needs | Grant |
|---|---|---|
| read (`get` / `list` / `+search` / `+count` / sub-resource reads) | `products` read scope | `auth login --domain products` |
| write (`+create` / `+publish` / `+set-price` / `+stock` / `+tag` / create / update / delete / sub-resource writes) | `products` write scope | `auth login --domain products` |

Look up exact scope literals with `shoplazza auth scopes`; `--domain products` expands into them.

## Gotchas

Domain-specific pitfalls only (generic ones — `.data` prefix, `--jq` without `-r` — are in
shoplazza-common).

| Symptom | Cause | Fix |
|---------|-------|-----|
| Stock decrease rejected | The `inventory_levels` write leaves (`set-stock` / `update-level`) only add; decreases ride `variant.inventory_quantity`, which is default-location-only and unverified on multi-location items — `+stock` gates accordingly | Use `+stock --adjust -N` / `--set N`; when the gate fires, explain and offer alternatives (unpublish, stock-policy) |
| Product published when the user wanted a draft | `--published` on `+create` is a bare boolean — passing it publishes | Omit `--published` entirely for a draft (draft is the default) |
| `unknown flag: --fields` on `+count` / other shortcuts | In this module `--fields` exists on `+search` **only** | Project with `--jq` elsewhere |
| Tags wiped out after an update | `products update` (leaf) replaces the whole tag list; so does `+tag --set` | Use `+tag --add` / `--remove` — existing tags kept |
| `+set-price --sku` refused with a candidate list | The SKU matched several variants — the CLI refuses a multi-match | Pick the right `--variant-id` from the listed candidates, or pass `--all` only for an explicit "all of them" |
| Price vanished after `+set-price --price 0` | `'0'` / `'0.00'` **clears** the price rather than setting zero | Only pass 0 when the user wants the price cleared |
| Wanted `gift-cards delete` | No delete endpoint exists (subcommands: batch-create / create / disable / get / list / update) | `gift-cards disable` stops redemption; tell the user it disables, not deletes |
| `variants update` 404 with product_id in path | `variants get` / `update` are variant-scoped: `/variants/{variant_id}` — no product_id | Only `variants list` / `create` / `delete` are product-scoped (`delete` needs BOTH ids) |
| `inventory set-stock` rejects a variant id | Inventory levels key on `inventory_item_id`, which ≠ variant id | Map first: `inventory list-by-variant --params '{"variant_ids":["<id>"]}'` |
| `comments create` rejected for missing fields | `star`, `like`, `created_at`, `user_name`, `content`, `product_id` are ALL required | See [references/comments.md](references/comments.md); use `like: 0` if unknown |
| "加进合集" attempted via `collections update` | Product↔collection membership is a `collect` record | `collects create` (one) / `collects batch-create` (many) |
| Invented `+delete` / `+update` shortcut | They don't exist | `delete` / `batch-delete` / `update` are spec leaves (see Command map) |

## Recipes

```bash
# 1. All unpublished products from vendor Nike
products +search --vendor Nike --published unpublished

# 2. Ids + titles only, of products with "hoodie" in the title
products +search --keyword hoodie --fields id,title

# 3. How many published products?
products +count --published published --jq '.data.count'

# 4. Create a draft product (draft = no --published)
products +create --title "Lycra 瑜伽裤" --price 39.99 --image https://cdn.example.com/yoga.jpg

# 5. Publish a product you only know by name
ID=$(products +search --keyword "瑜伽裤" --jq '.data.products[0].id')
products +publish --id "$ID"

# 6. Append tags without clobbering the existing ones
products +tag --id 445566 --add summer,sale

# 7. Delete one product — dry-run, restate, wait for the user's go-ahead, then run
products delete --params '{"product_id":"778899"}' --dry-run
products delete --params '{"product_id":"778899"}'          # only after the user agrees (next turn)

# 8. Batch delete — always dry-run first and restate the count
products batch-delete --params '{"product_ids":["111222","333444","555666"]}' --dry-run
products batch-delete --params '{"product_ids":["111222","333444","555666"]}'

# 9. Change one variant's size option (partial body — only the field to change)
products variants update --params '{"variant_id":"24680"}' --data '{"variant":{"option1":"XL"}}'

# 10. Put a product into a collection
products collects create --data '{"collect":{"collection_id":"999000","product_id":"555777"}}'

# 11. "Delete" a gift card you only know by code — resolve the id, preview the disable
#     (irreversible) in the SAME reply, then wait for the user's go-ahead
ID=$(products gift-cards list --params '{"keyword":"GC9988"}' --jq '.data.gift_cards[0].id')
products gift-cards disable --params '{"id":"'$ID'"}' --dry-run
```

## References

- [references/variants.md](references/variants.md) — variants CRUD, SKU lookup/update, option handling
- [references/inventory-locations.md](references/inventory-locations.md) — inventory items vs levels, decrement gates, locations
- [references/collections-collects.md](references/collections-collects.md) — curated & smart collections, collects, categories
- [references/images.md](references/images.md) — product image CRUD
- [references/gift-cards.md](references/gift-cards.md) — gift card lifecycle (no delete; disable)
- [references/comments.md](references/comments.md) — customer reviews (import, list)
- [references/procurement-suppliers.md](references/procurement-suppliers.md) — suppliers, procurement orders
- Per-command flags: `shoplazza products <cmd> --help`
- Spec-leaf params / body / response: `shoplazza schema products.<cmd>` (nested: `schema products.variants.update`)
- Cross-cutting mechanics (auth / tiers / output envelope / `--dry-run` / `--jq` / safety): `../shoplazza-common/SKILL.md`
- Shortcut source of truth: `shortcuts/products/*.go`

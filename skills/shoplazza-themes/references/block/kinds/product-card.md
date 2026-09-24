# themes product-card — tiers, wiring and setting-id contract for product-list cards

What a product-list card looks like, which components it uses, and which settings it declares.
Three axes: list form (L tier) × in-card elements (P tier) × follow-up extension points (X).
Read in this order: §1 Decide → the §2 / §3 tiers you hit → §4 Wiring → §5 Settings and
extensions → §6 Pitfalls. Skip every tier you did not hit.

Not covered here:
- How to read products and collections, `product` fields, price rules, `isMock` →
  [objects.md](../objects.md)
- One card showing one product (hero product, single-product poster, buy on the card) →
  [product-single.md](product-single.md)
- Setting types and their fields, presets, setting ownership → [schema-rules.md](../schema-rules.md)
- Liquid syntax, filter limits, scoped classes → [liquid-rules.md](../liquid-rules.md)
- Component attributes and skeletons → the `../ljs/ljs-*.md` files you have read

## 1. Decide

### Form first, then elements

A product-list card = one list form (L tier, pick exactly one) + a set of in-card elements
(P tiers, cumulative, switched on bottom-up). Neither decides the other: a carousel list can hold
the plainest card, and a fixed-N list can hold the richest one. Pick the L tier first (it decides
whether the card markup lives in Liquid or in a client-side template), then the P tiers (they
decide what is inside each card). Never the other way round.

### Request words → tier

L tiers (mutually exclusive)

| Request mentions | Form |
|---|---|
| Default; a row / featured slot / best sellers / picks / the first few (放一排 / 推荐位 / 热卖 / 精选 / 前几个) | L1 fixed N |
| Load more / more appear on scroll / too many for one screen / no page jumps (加载更多 / 往下滚再出 / 一次看不完 / 不想跳页) | L2 async load-more |
| Swipe sideways / carousel / N per screen / autoplay / left-right arrows (横着滑 / 轮播 / 一屏露几个 / 自动播 / 左右翻) | L3 carousel |

Ambiguity:
- Both "swipe" and "load more" → L3 (a carousel is already unbounded).
- "Pagination" / "page numbers" (分页 / 页码) → L2, and say in your reply that page numbers are
  not built (the setting contract in §5 has no page-number setting).
- "Pick products myself" / "add products one by one" / "in my order" (自己挑几个 / 一个个添加商品 /
  按我排的顺序) → the data source is still a collection. Tell the merchant in your reply: which
  products appear, and in what order, is maintained in the collection; the card does not hold
  individual products.

P tiers (cumulative)

| Tier | Elements | Trigger (no trigger → don't build it) |
|---|---|---|
| P0 | Image / title / price (incl. compare-at) / add to cart (incl. in-card panel) | Always |
| P1-a | Discount badge / sold-out badge | discount, % off, save X, sold out (折扣 / 打几折 / 省多少 / 卖光 / 售罄) |
| P1-b | Image ratio / fill mode | uneven heights, uniform ratio, don't crop (高低不齐 / 统一比例 / 别裁掉) |
| P1-c | Second image on hover | swap image on hover, second image (鼠标划上换图 / 第二张图) |
| P1-d | Fixed line under the title / custom badge | add a small line of text, add a "new" badge (加一行小字 / 加个新品标) |
| P1-e | Title lines / text alignment / corner radius | title too long, centered, rounded corners (标题太长 / 居中 / 圆角) |
| P2-a | Image carousel inside the card | images can flip, more than one image, image carousel (图能翻 / 不止一张图 / 图片轮播) |
| P2-b | Style (image option) swatches + main-image sync | color thumbnails, clickable styles, click a color to change the image (颜色小图 / 款式能点 / 点颜色换图) |

Fixed defaults for vague requests (do not improvise): a request that only says "make a product
list card / featured slot / best-seller slot" → **L1 + P0 + P1-a**. Add to cart follows
"Add-to-cart area" in §4 per product; nothing else is switched on, and not one extra setting is
declared. Build less and wait for the follow-up request (that is what the §5 extension points
are for).

Dependencies and exclusions

| Constraint | Meaning |
|---|---|
| P2-a ⊗ P1-c | Two image-swap mechanisms on one media area fight each other. If both trigger, take P2-a and say so in your reply |
| P2-b → P0 media area | Swatch sync changes the main image, so the media area must exist |
| L2 ⊗ P2, and no in-card panel in L2 | L2 card markup lives in a client-side template. Rebuilding slideshow + variants + sync there means rewriting the theme's full product-card template inside one block — beyond a single card. Not built by default. Add to cart in L2 is always the options link from the L2 row of the quick-add table in §4. If the request wants both "load more" and "style switching", pick L1 and explain the trade-off in your reply |

### Tier → what to write, which docs to read

Each tier's settings are the rows with the same tier name in §5 "Setting tiers and id contract".
An empty "Also read" cell means nothing extra.

| Tier | Write | Component docs | Also read |
|---|---|---|---|
| L1 | `<div class="…__grid">` + `{% for p in col.products limit: show_count %}` | [ljs-img](../ljs/ljs-img.md) | — |
| L2 | `ljs-list` (`manual`, first page in Liquid) + inline `<template>` for later pages | [ljs-img](../ljs/ljs-img.md) [ljs-list](../ljs/ljs-list.md) [ljs-currency](../ljs/ljs-currency.md) | [product-card-async.md](product-card-async.md) |
| L3 | `ljs-carousel visible-count`, one card per slide | [ljs-img](../ljs/ljs-img.md) [ljs-carousel](../ljs/ljs-carousel.md) (+ [ljs-slide-indicator](../ljs/ljs-slide-indicator.md)) | — |
| P0 | `ljs-img` / `<a>` / `ljs-currency` / `ljs-product-form`; for multi-option products add a panel div with `.none` + `ljs-variants auto-add-to-cart`. Build the full add to cart even when the request only names image / title / price; skip it only when the request explicitly says "display only, not for sale" | + [ljs-currency](../ljs/ljs-currency.md) [ljs-product-form](../ljs/ljs-product-form.md) [ljs-variants](../ljs/ljs-variants.md) (panel; required for L1 / L3) | — |
| P1-a | Static `<span class="…__badge">` | — | — |
| P1-b | `aspect-ratio` on the media box | — | — |
| P1-c | Two stacked `ljs-img` + CSS | — | — |
| P1-d | `<p>` after the title / `<span>` in the media area | — | — |
| P1-e | Title class / container class | — | — |
| P2-a | `ljs-carousel` | + [ljs-carousel](../ljs/ljs-carousel.md) | [product-card-interaction.md](product-card-interaction.md) |
| P2-b | `fieldset` + `ljs-selector` (`@select` → `goToSlide(path=)`) + hidden radio per item | + [ljs-selector](../ljs/ljs-selector.md) | [product-card-interaction.md](product-card-interaction.md) |

## 2. List form

### L1 fixed N (default)

Plain Liquid + CSS grid, no async component: the data is already in `collection.products`, so
there is no reason to send another request.

```liquid
{% assign col = collections[block.settings.collection.id] %}
{% unless col.products.size > 0 %}
  {% assign col = default_collection %}
{% endunless %}
{% assign show_count = block.settings.product_limit %}
{% if col.products.size < show_count %}
  {% assign show_count = col.products.size %}
{% endif %}

<div class="{{ root_cls }}__grid" {{ block.shoplaza_attributes }}>
  {% for p in col.products limit: show_count %}
    {% comment %} one card = P0 skeleton + the P1/P2 tiers you hit {% endcomment %}
  {% endfor %}
</div>
```

- Fall back on `products.size`, not `col.id` (the fixed fallback-chain pattern in
  [objects.md](../objects.md)).
- Cap with `limit:`; don't loop `{% for i in (1..n) %}` and index into the array.
- Mobile-first grid: `grid-template-columns: repeat({{ mobile_per_row }}, minmax(0,1fr))`, switch to
  `pc_per_row` under `@media (min-width: 960px)`.
- Use `minmax(0, 1fr)` and give the card root `min-width: 0`, or long titles break the grid.

The L2 skeleton, rules and client-template limits are in
[product-card-async.md](product-card-async.md) — read it when you pick L2.

### L3 carousel list

`ljs-carousel`, one product card per slide. "N per screen" comes from `visible-count`, not from
packing several cards into one slide.

```liquid
<ljs-carousel
  id="pc-{{ block_id }}"
  layout="container"
  visible-count="(min-width:960px) {{ block.settings.carousel_visible_pc }}, {{ block.settings.carousel_visible_mobile }}"
  advance-count="1"
  initial-slide="0"
  {{ block.shoplaza_attributes }}
  {% if block.settings.show_arrows %}controls{% endif %}
  {% if block.settings.enable_loop %}loop{% endif %}
  {% if block.settings.enable_autoplay %}autoplay delay="{{ block.settings.autoplay_delay | times: 1000 }}"{% endif %}
>
  {% for p in col.products limit: show_count %}
    <div class="{{ root_cls }}__slide">…one card…</div>
  {% endfor %}
</ljs-carousel>
```

- Multi-column sliding needs both `visible-count` (component paging) and a matching per-slide
  CSS width (actual column width); follow [ljs-carousel.md](../ljs/ljs-carousel.md).
- Each direct child = one slide. No `<style>` and no HTML comments inside a slide (they render
  as blank slides).
- `delay` is seconds in the setting → milliseconds with `| times: 1000`.
- Dots use `ljs-slide-indicator`; its `carousel-id` must match and `size` is required.
- On mobile, "a bit more than one" = a fractional `visible-count` (e.g. `1.2`) so shoppers see there
  is more.
- The list carousel and the in-card image carousel both use `ljs-carousel` (the in-card one is in
  [product-card-interaction.md](product-card-interaction.md)); ids keep them apart. Never use
  `ljs-slideshow`.

## 3. In-card elements

### P0 skeleton: image → title → price → buy

The order is fixed; missing any one leaves a broken card. The media area is the card's only
positioning context — badges, the add-to-cart icon and the in-card panel all hang off it.

```liquid
{% assign price_now = p.price | default: p.price_min %}
{% assign show_compare = false %}
{% if price_now < p.compare_at_price %}
  {% assign show_compare = true %}
{% endif %}

<div class="{{ root_cls }}__card">
  {% comment %} 1 media: only the image is wrapped in <a>; no href in placeholder state {% endcomment %}
  <div class="{{ root_cls }}__media">
    <a class="{{ root_cls }}__media-link"{% unless p.isMock %} href="{{ p.url }}"{% endunless %}>
      {% if p.isMock %}
        {% capture ph_cls %}{{ root_cls }}__img{% endcapture %}
        {{ 'product-1' | placeholder_img_tag: ph_cls }}
      {% else %}
        <ljs-img class="{{ root_cls }}__img" layout="fill" object-fit="cover"
          src="{{ p.image.src | img_url: '540x' }}"
          alt="{{ p.image.alt | default: p.title | escape }}"></ljs-img>
      {% endif %}
    </a>
    {% comment %} badges (P1-a), the panel and the icon-style add-to-cart go here, siblings of the link {% endcomment %}
  </div>

  {% comment %} 2 title; the fixed line under it (P1-d) follows directly {% endcomment %}
  <a class="{{ root_cls }}__title"{% unless p.isMock %} href="{{ p.url }}"{% endunless %}>{{ p.title }}</a>

  {% comment %} 3 price row; swatches (P2-b) follow directly {% endcomment %}
  <div class="{{ root_cls }}__price">
    <ljs-currency class="{{ root_cls }}__price-now" layout="container" value="{{ price_now }}"></ljs-currency>
    {% if show_compare %}
      <s class="{{ root_cls }}__price-was"><ljs-currency layout="container" value="{{ p.compare_at_price }}"></ljs-currency></s>
    {% endif %}
  </div>

  {% comment %} 4 add to cart per §4; the button-style element goes here, the icon style is already in the media area {% endcomment %}
</div>
```

- `{{ block.shoplaza_attributes }}` goes once on the block root (the grid or carousel above), not
  on every card in the loop ([liquid-rules.md](../liquid-rules.md)).
- The media area needs `position: relative` and a fixed shape (`aspect-ratio`); otherwise badges
  and the add-to-cart icon have nothing to anchor to, and the layout jumps before the image loads.
- Placeholder and real image share the `__img` class; style it
  `position:absolute; inset:0; width:100%; height:100%; object-fit:cover`. The `<img>` from
  `placeholder_img_tag` carries only the class you pass — the card has none of the theme's
  utility classes such as `w-full`.
- The link to the product page wraps only the image. Add-to-cart buttons and the panel's radios and
  labels are interactive; inside an `<a>` the browser splits and reorders the tags, and clicking
  the panel navigates away.
- Money output follows the price section of [objects.md](../objects.md): `ljs-currency` brings its
  own `.money` container, don't wrap it again. Give `ljs-currency` `display: inline-block` in the
  price-row CSS so both amounts sit on one line.
- Show the compare-at price by comparing (`price < compare_at_price`), not by checking it is
  non-empty: `compare_at_price` often equals `price`, and a non-empty check strikes through every
  card.
- Placeholder state: no links, no add-to-cart section at all, image via `placeholder_img_tag` —
  one per product in the list, and an inline SVG would multiply by that count (the rest per the
  `isMock` placeholder section of [objects.md](../objects.md)).

### P1-a discount badge and sold-out badge

- Sold out is `p.available == false`; there is no `sold_out` field (§6 "Using `sold_out` for sold
  out").
- When to show a discount, and how to compute the rate or amount: the price section of
  [objects.md](../objects.md).
- If both apply, sold out wins and the discount badge is not shown.
- `discount_style` has three values: `label` (over the image) / `text` (after the price row) /
  `hidden`.
- Position via `badge_position`, four corners (`top_left` / `top_right` / `bottom_left` /
  `bottom_right`), default `top_left`.
- Badges are absolutely positioned and out of the document flow; otherwise turning a switch off
  changes the media-area height.
- When a custom badge (P1-d) sits in the same corner as the discount badge, the discount badge moves
  to the opposite corner.

### P1-b image ratio and fill mode

- `image_ratio` values and how the `original` value works: the merchant-configurable ratio
  section of [ljs-img.md](../ljs/ljs-img.md). A product card's media area carries badges and the
  add-to-cart icon, so always use the ratio-box pattern at the end of that section.
- `image_fill_mode` has two values: `cover` (crop to fill, default) / `contain` (show whole image).
- Don't split into separate PC / mobile ratios; merchants don't need it.

### P1-c second image on hover

Two stacked `ljs-img`, pure CSS swaps `opacity`. Get the second image in one step with
`secondary_image`, which already skips video items — `p.images` mixes in videos, and taking
`images[1]` directly can return a video cover:

```liquid
{% assign image2 = p.images | secondary_image %}
```

- When `image2` is nil, don't output the second image node at all (no empty `src`).
- Wrap the hover effect in `@media (hover: hover)`; touch screens have no hover, and an unwrapped
  rule makes images flicker on some Android devices.
- Never put key information only in the hover state.

### P1-d fixed line under the title / custom badge

Both are merchant-level shared content, not per product: per-product text would have to be set for
every product, and an AI card has no data surface for that.

| Item | Where | When empty |
|---|---|---|
| Line under the title | After `.__title`, before `.__price` | Output no node at all; never an empty `<p>` (it adds a blank line) |
| Custom badge | Inside the media area, after the product link, absolutely positioned | Output no node at all |

Each has its own switch (`show_subtitle` / `show_badge`); turning it off must not change the
layout size.

### P1-e title lines / text alignment / corner radius

| Setting | Values | Implementation |
|---|---|---|
| `title_style` | `full` / `one_line` / `two_line` (default) / `hidden` | CSS `-webkit-line-clamp` with a `min-height` so cards in a row match height; don't cut with a fixed `height` |
| `text_alignment` | `left` (default) / `center` | `text-align` on the card's text area |
| `corner_radius` | range 0–40, step 2, px | `border-radius` on the media area and card root |

The two P2 tiers (in-card image carousel / swatches with main-image sync) have their skeletons and
rules in [product-card-interaction.md](product-card-interaction.md); read it when you hit either.

## 4. Wiring and naming

### Add-to-cart area

Two terms, kept apart:
- **Quick-add button** — completes the add on the card: products with no variants or a single
  variant go straight into the cart; multi-option products open the in-card panel, the shopper
  picks, and the item goes into the cart.
- **Options link** — a link to the product page; the shopper adds to cart there.

On L1 / L3 list cards, every add-to-cart element (icon or full-width) is a quick-add button. The
options link appears only in L2 client-template cards (no panel is possible there).

Three steps in a fixed order: wrap the shell, decide button behavior, write the panel. Add to cart
is a P0 element: build it even if the request only names image / title / price; drop the whole
add-to-cart area only when the request explicitly says "display only, not for sale". The panel is
part of quick add: multi-option products always get it; there is no switch to fall back to the
product page. What the add-to-cart element looks like and where it sits: "Add-to-cart element
style" below.

#### Card root shell and the two flags

The whole card is wrapped in `ljs-data-source` (`source-type="product"`) → `ljs-product-form > form`;
media, title, price, swatches and add to cart are all inside the form. If only the button is
wrapped, the swatch radios and the panel's `ljs-variants` cannot find their host form.

```liquid
{% capture pid %}{{ block_id }}-{{ p.id }}{% endcapture %}
{% capture panel_id %}ppanel-{{ pid }}{% endcapture %}
{% capture pvar_id %}ppvar-{{ pid }}{% endcapture %}

{% assign direct_atc = false %}
{% if p.options == blank or p.options.size == 0 or p.has_only_default_variant %}{% assign direct_atc = true %}{% endif %}
{% assign swatch_atc = false %}
{% comment %} with P2-b on: true when p.options.size == 1 and all three swatch render gates pass {% endcomment %}
{% assign show_panel = false %}
{% if p.available and direct_atc == false and swatch_atc == false %}{% assign show_panel = true %}{% endif %}

<div class="{{ root_cls }}__card">
  {% unless p.isMock %}
    <ljs-data-source id="pds-{{ pid }}" layout="container" source-type="product" source-id="{{ p.id }}">
    <ljs-product-form id="patc-{{ pid }}" layout="container" product-id="{{ p.id }}" show-toast disable-init-toast
      {% if direct_atc %}variant-id="{{ p.variants[0].id }}"{% endif %}
      {% if show_panel %}@atcSuccess="{{ panel_id }}.toggleClass(class='none', force=true);{{ pvar_id }}.clear"{% endif %}>
      <form>
  {% endunless %}

      …media (incl. panel; icon-style add-to-cart here) / title / price / swatches / button-style add-to-cart here…

  {% unless p.isMock %}
      </form>
    </ljs-product-form>
    </ljs-data-source>
  {% endunless %}
</div>
```

- Placeholder cards get no shell and no add to cart — that is what the two `{% unless p.isMock %}`
  blocks do.
- `disable-init-toast` is required: sold-out products still get the shell, and without it the
  component shows a "sold out" toast on init — N sold-out products on a page, N toasts.
- Write `variant-id` only when `direct_atc` is true. Passing `variants[0].id` for a multi-option
  product picks "red, size S" on the shopper's behalf — neither merchant nor shopper chose it.
- Keep all three parts of the no-variant / single-variant condition (§6 "`options.size == 0` as
  the no-variant test"), and don't use `variants.size`: a placeholder product always has 6
  variants (the `isMock` placeholder section of [objects.md](../objects.md)).

#### Quick-add button

One button; take the first row that matches from the top. Sold out is not a branch, it is an
attribute. Text comes from `atc_text` / `sold_out_text`; the L2 options link uses
`select_variant_text`.

| Product | Button |
|---|---|
| `p.available == false` | Render the button with `disabled` and `sold_out_text`; no `role`, no `@tap`, no panel |
| List form is L2 | Options link `<a href="${data.url}">` with `select_variant_text`: the item template cannot tell no-variant / single-variant products apart, so it always links to the product page. No `role="addToCart"`, no `variant-id` |
| `direct_atc` or `swatch_atc` | `<button type="button" role="addToCart">`; the variant comes from the card-root `variant-id` (first case) or the swatch area's hidden radio (second case); no `ljs-variants` in either |
| Everything else (multi-option) | `<button type="button" @tap="{{ panel_id }}.toggleClass(class='none', force=false)">` opens the panel; the shopper picks, then it adds |

L1 / L3 cards never show an options link: multi-option products use the panel, with no
`<a href="{{ p.url }}">` fallback — linking to the product page is the image's and title's job.
Don't write `role="quick-view"` either (§6 "Using a theme's private building blocks").

Declare the three add-to-cart settings together and read all three in the body: `atc_text`,
`sold_out_text` (copy) and `add_cart_style` (style). Declared-but-unread or read-but-undeclared
fails [self-check.md](../self-check.md).

#### Panel

Output only when `show_panel` is true: list form is L1 / L3, the product is available, and it
can't be added directly (`direct_atc` and `swatch_atc` both false).

The panel is pre-rendered inside the media area with `.none`; the button's `@tap` opens it and the
card root's `@atcSuccess` closes it. `panel_id` / `pvar_id` / `pid` are the three values captured in
the shell. The DOM contract, radio prefix, open/close and build timing, and CSS notes are all in
[ljs-variants.md](../ljs/ljs-variants.md) (its skeleton and rules) — not repeated here.

### Add-to-cart element style

"Add-to-cart area" decides what the quick-add button does; the style decides what it looks like.
One setting, `add_cart_style`, same value on PC and mobile, default `icon`.

| Style | Position | Markup |
|---|---|---|
| `icon` | Bottom-right of the media area | Round icon button with an inline plus SVG (not a setting), `aria-label` from `atc_text`; absolutely positioned in the media area, out of flow |
| `button` | After the price row, at the card bottom | Full-width button, text from `atc_text` |
| `hidden` | No add-to-cart element | The rest of the card is unchanged; the card still links to the product page |

Write the add-to-cart element once: one class stem `{{ root_cls }}__atc` plus a style modifier
`--icon` / `--button`; position and look come only from the modifier's CSS, and `hidden` outputs
nothing. Don't write separate icon and button markup with separate class stems picked by
`{% if %}` — the behavior branches get duplicated and drift apart.

```liquid
{% assign atc_style = block.settings.add_cart_style %}
{% unless atc_style == 'hidden' %}
  {% capture atc_cls %}{{ root_cls }}__atc {{ root_cls }}__atc--{{ atc_style }}{% endcapture %}
  {% if p.available == false %}
    <button type="button" class="{{ atc_cls }}" disabled>{{ block.settings.sold_out_text }}</button>
  {% elsif direct_atc or swatch_atc %}
    <button type="button" role="addToCart" class="{{ atc_cls }}" aria-label="{{ block.settings.atc_text | escape }}">…</button>
  {% else %}
    <button type="button" class="{{ atc_cls }}" @tap="{{ panel_id }}.toggleClass(class='none', force=false)" aria-label="{{ block.settings.atc_text | escape }}">…</button>
  {% endif %}
{% endunless %}
```

- Button content by style: `--icon` holds the inline plus SVG, `--button` holds the
  `atc_text` / `sold_out_text` text.
- Place this block in exactly one spot, one of two: inside the media area, with `--button` moved to
  the card bottom by `position:absolute; bottom` (the media area is the positioning context); or
  after the price row, with `--icon` pulled up to the media area's bottom-right by a negative
  `margin`.
- Style never changes behavior: direct add (`role="addToCart"`), open panel (`@tap`) and sold out
  (`disabled`) are branches inside the same markup.
- The icon button shares the media area with badges: when a discount or custom badge is set to
  `bottom_right`, the badge moves to the opposite corner; the icon stays bottom-right.

### Id naming table and the five interactions

Ids a card captures (all built from `{{ block_id }}`, plus `p.id` inside the loop). Inside a loop,
build ids only with `capture`; `assign` + `append` yields an empty string (liquid-rules.md: build
strings with `capture`).

| Purpose | Suggested id |
|---|---|
| Card-scoped class prefix | `{{ root_cls }}` |
| List carousel (L3) | `pc-{{ block_id }}` |
| Async list (L2) | `pl-{{ block_id }}` |
| In-card image carousel (P2-a, `ljs-carousel`) | `pimg-{{ block_id }}-{{ p.id }}` |
| Async list data source (L2) | `pds-{{ block_id }}` |
| Panel container | `ppanel-{{ block_id }}-{{ p.id }}` |
| `ljs-variants` in the panel | `ppvar-{{ block_id }}-{{ p.id }}` |
| Add-to-cart form | `patc-{{ block_id }}-{{ p.id }}` |

The five interactions (closed list — nothing outside these five)

| # | Interaction | How | When |
|---|---|---|---|
| 1 | Second image on hover | Pure CSS `opacity` | P1-c |
| 2 | Add-to-cart success feedback | `show-toast` built into the component; extra actions via `@atcSuccess` | P0 |
| 3 | Sold-out greyed out | `disabled` on the button, decided at Liquid render time, not by changing the DOM at runtime | P0 |
| 4 | Swatch ↔ main image | `@select="<image carousel id>.goToSlide(path=event.targetOption)"` on `ljs-selector`; each item's `option` value is the image src ([product-card-interaction.md](product-card-interaction.md)) | P2-b only |
| 5 | Open/close panel + close on add success | `<id>.toggleClass(class='none', force=true/false)` + `@atcSuccess` ("Add-to-cart area") | P0, cards that render a panel |

Not built: quantity steppers in the card, opening a cart drawer after add, calling a theme's
quick-view popup (§6 "Using a theme's private building blocks"), cross-card interactions (clicking
one card affects another).

## 5. Settings and extensions

### Setting tiers and id contract

The same meaning always uses the same id / type / default / enum values. Tier column: **A**
required (missing one leaves a broken card), **B** common (optional, fits naturally); the rest are
declared for the L / P tier you hit. L1 has no form-specific settings.

| Tier | Meaning | id | type | default | Notes |
|---|---|---|---|---|---|
| A | Source collection | `collection` | collection | — | |
| A | Total shown | `product_limit` | range 2–24 / 1 | 8 | |
| A | Per row, PC | `pc_per_row` | range 2–6 / 1 | 4 | |
| A | Per row, mobile | `mobile_per_row` | select 1 / 2 / 3 | `2` | PC and mobile must be two separate settings; with only one, the merchant can't change mobile density |
| B | Section heading | `heading` | text | 热卖推荐 / Featured | |
| B | Subheading | `subheading` | text / textarea | empty | `text` for one line, `textarea` for several |
| B | Card background / text color | `bg_color` / `text_color` | color | `#FFFFFF` / `#111111` | One `color` per configurable part, background and text in pairs |
| B | View all | `show_view_all` / `view_all_text` | checkbox / text | false / 查看全部 / View all | The link points straight at `col.url`; no extra url setting |
| P1-b | Image ratio | `image_ratio` | select original, 1/1, 4/3, 3/4, 2/3, 16/9 | `1/1` | |
| P1-b | Image fill | `image_fill_mode` | select cover, contain | `cover` | |
| P1-e | Title lines | `title_style` | select full, one_line, two_line, hidden | `two_line` | |
| P1-e | Text alignment | `text_alignment` | select left, center | `left` | |
| P1-e | Corner radius | `corner_radius` | range 0–40 / 2 | 12 | |
| P1-a | Discount style | `discount_style` | select label, text, hidden | `label` | |
| P1-a | Badge position | `badge_position` | select top_left, top_right, bottom_left, bottom_right | `top_left` | |
| P1-a | Sold-out badge | `show_sold_out_badge` | checkbox | true | |
| P1-c | Second image on hover | `show_hover_image` | checkbox | false | |
| P1-d | Line under title | `show_subtitle` / `subtitle_text` | checkbox / text | false / empty | |
| P1-d | Custom badge | `show_badge` / `badge_text` | checkbox / text | false / empty | |
| P2-a | In-card image carousel | `image_carousel` / `image_carousel_style` | checkbox / select bar, arrow, dot | false / `dot` | |
| P2-b | Style swatches | `show_variant_swatch` / `swatch_option_names` / `swatch_per_row` | checkbox / text / range 3–8 / 1 | false / `color,colour,style` / 5 | |
| P0 | Add-to-cart style | `add_cart_style` | select icon, button, hidden | `icon` | Button colors are extra paired `color` settings |
| P0 | Add-to-cart text | `atc_text` | text | 加入购物车 / Add to cart | |
| P0 | Options-link text | `select_variant_text` | text | 选择规格 / Select options | Declared only in L2, replacing `atc_text` / `add_cart_style`; renders the options link |
| P0 | Sold-out text | `sold_out_text` | text | 售罄 / Sold out | |
| L2 | Batch size | `page_size` | range 2–12 / 1 | 6 | |
| L2 | Load mode | `load_mode` | select scroll, click | `scroll` | |
| L2 | Load button text | `load_more_text` | text | 加载更多 / Load more | |
| L3 | Per screen, PC | `carousel_visible_pc` | range 2–6 / 1 | 4 | |
| L3 | Per screen, mobile | `carousel_visible_mobile` | select 1, 1.2, 2, 2.2 | `1.2` | |
| L3 | Autoplay | `enable_autoplay` / `autoplay_delay` | checkbox / range 2–10 / 1 | false / 6 | |
| L3 | Loop | `enable_loop` | checkbox | true | |
| L3 | Arrows / dots | `show_arrows` / `show_dots` | checkbox / checkbox | true / true | |


- Themes usually keep a product card's display switches in global theme settings. A card never
  reads `settings.*` (liquid-rules.md: never read theme global settings); declare each switch
  you need in the card's own schema per the table above.
- Settings with two defaults are merchant-facing copy; use the one that matches the card copy
  language (the storefront's primary-market language; see [schema-rules.md](../schema-rules.md)).

### Product fields get generic placeholders only

Product title, price, compare-at price, main image and product link are not in the contract above:
their values come from the product the merchant picks, not from copy the merchant types. When a
request names specific products, it names which products this card should show — not data to copy
into the card.

Whether these fields are read from the `product` / `collection` object or made into inline child
items the merchant fills in, `default` and `presets` get only generic placeholders — the same
values as the always-available `default_product` — so the preview looks the same whether the
merchant has not picked a product or has not changed a default:

| Field | Placeholder |
|---|---|
| Product title | 商品标题 / Product Title |
| Price | `88.88` |
| Compare-at price | `99.99` — must be higher than the price, or neither the strike-through nor the discount badge shows in the preview |
| Main image, product link | empty |

- Give prices as bare numbers without a currency symbol: a string in a `range` preset is a type
  mismatch, and `ljs-currency` adds the store currency symbol.
- Never write a real product's name, price or image into the schema. `presets` values are what
  every store gets when it installs this card; one store's products hard-coded there show up as a
  stranger's product on every other store. A product name also follows the store's products, not
  the card copy language (checked in [self-check.md](../self-check.md)).
- Fields read dynamically from objects need no invented sample data: when the merchant has not
  picked anything, `default_product` / `default_collection` fill in. Field-by-field fallback values
  and which elements to drop in placeholder state: the placeholder-object and `isMock` sections of
  [objects.md](../objects.md).

### Extension points and id freeze

When a follow-up request loses values the merchant already configured, the cause is always a
changed id / type / enum, never the insertion position. Three rules:

1. Append only: new settings go at the end of `settings`. When changing an existing AI card, add
   to `settings` and `presets[0]` together (the editor's initial values come from presets); see
   [edit-discipline.md](../edit-discipline.md).
2. Never: rename an existing id, change its type, change a `select` option's `value` (`label` may
   change), delete a setting, narrow a `range` `min` / `max` (old values fall out of range — only
   widen), or turn a `checkbox` into a `select`.
3. Meaning really changed → new id and keep the old one:
   `{{ block.settings.image_ratio_v2 | default: block.settings.image_ratio }}`. Keep the old id in
   the schema with an `info` marking it deprecated (the `default:` chain already satisfies "every
   setting is read").

Extension points

| # | Follow-up request | Insert at | New settings | Must not touch | Accept when |
|---|---|---|---|---|---|
| X1 | Add a fixed line under the title | After `.__title`, before `.__price` | `show_subtitle` + `subtitle_text` | Don't change `title_style`; don't make it a child block (merchant-level shared text) | No blank line when off; same text on every card |
| X2 | Add a badge on the image | Inside the media area, after the product link | `show_badge` + `badge_text` (reuse `badge_position`) | Don't change the image ratio; keep the badge out of flow; move the discount badge to the opposite corner when they share one | Badge overlays without squeezing the image; media height unchanged when off |
| X3 | Configurable image ratio | Media-box CSS | `image_ratio` + `image_fill_mode` | Don't split PC / mobile ratios; don't change the `ljs-img` `layout` | Equal heights per row; no distortion; space reserved before load |
| X4 | One per row on mobile | Only the grid's mobile branch | Append `1` to `mobile_per_row` options | Don't change `pc_per_row` | Single column at 360px with no overflow |
| X5 | Truncate long titles | Title class | `title_style` | No fixed `height` | Two-line ellipsis; equal heights per row |
| X6 | Show the amount saved | End of the price row | Append `amount` to `discount_style` | Don't change the output filters for current / compare-at price | Nothing rendered without a discount |
| X7 | Add "view all" | After the list container | `show_view_all` + `view_all_text` | Link only to `col.url` | Not rendered in placeholder state |
| X8 | Images can flip (upgrade to P2-a) | Replace the product link in the media area with the carousel | `image_carousel` + `image_carousel_style` | Off must fall back to a static single image; don't delete the hover-image logic (make them mutually exclusive) | No controls on single-image products; off returns the DOM to the static image |
| X9 | Clickable color swatches (upgrade to P2-b) | After the price row, before add to cart | `show_variant_swatch` + `swatch_option_names` + `swatch_per_row` | Render nothing when `option_thumbnails` returns empty | Clicking a swatch changes the main image; selected state visible |

## 6. Pitfalls

### Using a theme's private building blocks

A theme's own icon and product snippets, its global config data source, its popup ids (quick
shop, cart drawer), `role="quick-view"` — all belong to one specific theme, with names and
implementations that differ per theme. A card that uses them silently breaks when the theme
changes. Icons are inline SVG, data comes from `ljs-data-source`, add to cart goes through
`ljs-product-form`, and multi-option products use the in-card panel.

### Using `sold_out` for sold out

The product object has no `sold_out` field; it is always falsy, so sold-out products still show add
to cart. Sold out is always `p.available == false` (field list in the product and collection
fields section of [objects.md](../objects.md)).

### Writing your own add-to-cart fetch

A hand-written `fetch('/api/cart', …)` bypasses the component's error handling, localized toasts
and analytics. With `ljs-product-form` available, never hand-write it.

### `options.size == 0` as the no-variant test

When `options` is nil, `options.size` is also nil, and `nil == 0` is false: writing only
`options.size == 0` treats no-variant products as multi-option, so clicking opens a panel instead
of adding to cart. Always write `p.options == blank or p.options.size == 0 or p.has_only_default_variant`.

Component attributes come only from the `../ljs/ljs-*.md` files you have read; attribute tables
seen anywhere else don't count.

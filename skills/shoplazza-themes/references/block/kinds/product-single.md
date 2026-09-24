# themes product-single — forms, wiring and setting-id contract for single-product cards

Read this when one card shows one product (the merchant picks one product in the editor). Two axes:
form (display or purchase, pick one) × elements (cumulative, only with a trigger).

Not covered here:
- How to read the product, `product` fields, price rules, `isMock` → [objects.md](../objects.md)
- One card showing several products (collection / featured slot / list) →
  [product-card.md](product-card.md)
- Setting types and their fields → [schema-rules.md](../schema-rules.md)
- Component attributes and skeletons → the `../ljs/ljs-*.md` files you have read

## Reading order

Pick the form → read that form's skeleton → add elements by trigger → for the purchase form also
"Purchase tiers" → "Wiring and ids" → "Setting-id contract" → "Pitfalls".

## Decide

### Request words → form

| Request mentions | Form |
|---|---|
| Default; hero product / feature one product / one product on the home page / buy directly / add to cart / choose options / buy now (主推单品 / 精选一个商品 / 首页放一个商品 / 直接买 / 加购 / 选规格 / 立即购买) | **Purchase**: gallery + title + price + option picker + add to cart; the shopper orders on the card |
| Go to the product page / learn more / single-product poster / image and text for one product / display only / drive traffic to the product page (跳详情 / 了解更多 / 单品海报 / 图文配一个商品 / 只展示不卖 / 引流到详情页) | **Display**: large image + title + price + copy + a button to the product page; no purchase components |

Ambiguity: both "poster" and "add to cart" → purchase. "Poster" is only a layout; the purchase form
can carry a large image and copy too.

### Elements and triggers

| Element | Trigger (no trigger → don't build it) | Form |
|---|---|---|
| Main image / title / price (incl. compare-at and price range) | Always | Both |
| Gallery (multi-image carousel + thumbnail strip) | Purchase: on by default. Display: only for "several images" / "images can flip" (多张图 / 图能翻) | Both |
| Merchant copy | add an intro / selling points / a one-line pitch (加一段介绍 / 卖点 / 一句话文案) | Both |
| Discount / sold-out / custom badge | Same triggers as P1-a / P1-d in [product-card.md](product-card.md) (§1) | Both |
| Button to the product page | Always in display | Display |
| Option picker | Shown whenever the product has several options, no trigger needed | Purchase |
| Style swatches | color thumbnails / color chips / styles with images (颜色小图 / 色块 / 款式带图) | Purchase |
| Quantity box | choose quantity / how many / plus-minus (选数量 / 买几件 / 加减号) | Purchase |
| Buy now | buy now / straight to checkout / one-click order (立即购买 / 直接结算 / 一键下单) | Purchase |

Fixed default for vague requests (do not improvise):

> A request that only says "put one hero product / feature one product" → purchase form + main
> image (carousel with thumbnail strip when there are several images) + title + price + option
> picker + add to cart. Copy, badges, quantity box and buy now stay off, and not one extra setting
> is declared; wait for the follow-up request.

Boundaries (both forms)

- The main image doesn't follow the selected variant, and the price isn't recalculated per variant:
  the price area is static Liquid output, showing a range when variants differ in price. Tell the
  merchant both points in your reply.
- No automatic best-seller pick: that needs a request plus client-side template rendering — beyond a
  single card. To change the product, the merchant re-picks it in the editor.
- No stock progress bar and no sales count: stock and sales are not among the usable fields in the
  product and collection fields section of [objects.md](../objects.md).
- A countdown is not part of this card type; when the request names one, stack it at the top of the
  info column per [ljs-countdown.md](../ljs/ljs-countdown.md).

### Form / element → what to write, which docs, which settings

An empty "Also read" cell means nothing extra.

| Form / element | Write | Component docs | Also read | Settings (see "Setting-id contract") |
|---|---|---|---|---|
| Display | Two columns: media `<a>` + info column; the button is an `<a href>` | [ljs-img](../ljs/ljs-img.md) [ljs-currency](../ljs/ljs-currency.md) | — | `product` `layout` `button_text` `sold_out_text` |
| Purchase | Two columns; the whole card inside `ljs-data-source → ljs-product-form > form` | [ljs-img](../ljs/ljs-img.md) [ljs-currency](../ljs/ljs-currency.md) [ljs-product-form](../ljs/ljs-product-form.md) | — | `product` `layout` `atc_text` `sold_out_text` |
| Gallery | `ljs-carousel` + thumbnail strip | + [ljs-carousel](../ljs/ljs-carousel.md) | [product-card-interaction.md](product-card-interaction.md) (only the P2-a rule table) | + `show_thumbnails` |
| Merchant copy | One `<div>` in the info column | — | — | + `text` |
| Discount / sold-out / custom badge | Absolutely positioned `<span>` in the media area | — | [product-card.md](product-card.md) (only "P1-a" and "P1-d") | + `discount_style` `badge_position` `show_sold_out_badge` / `show_badge` `badge_text` |
| Option picker | `ljs-variants` + one `fieldset` per option | + [ljs-variants](../ljs/ljs-variants.md) | — | — |
| Style swatches | `ljs-img` inside the option `label` | — | [product-card-interaction.md](product-card-interaction.md) (only the P2-b image lookup) | + `show_variant_swatch` `swatch_option_names` |
| Quantity box | `ljs-quantity` | + [ljs-quantity](../ljs/ljs-quantity.md) | — | + `show_quantity` `qty_label` |
| Buy now | `<button type="button" role="buyNow">` | — | — | + `show_buy_now` `buy_now_text` |

## Data and shared variables

Both forms start the same way: get the product, fall back to the placeholder, compute prices. Name
the variable `p`, as in the component examples.

```liquid
{% assign p = all_products[block.settings.product.id] %}
{% unless p.id %}
  {% assign p = default_product %}
{% endunless %}
{% assign price_now = p.price | default: p.price_min %}
{% assign show_compare = false %}
{% if price_now < p.compare_at_price %}
  {% assign show_compare = true %}
{% endif %}
{% assign can_buy = false %}
{% unless p.isMock %}
  {% if p.available %}
    {% assign can_buy = true %}
  {% endif %}
{% endunless %}
{% capture ph_cls %}{{ root_cls }}__img{% endcapture %}
```

- Fall back on `p.id`, not `p` (the fixed fallback-chain pattern in [objects.md](../objects.md)).
- Build `can_buy` as `unless isMock` around `if available`; don't write `p.isMock == false`: on a
  real product `isMock` is empty, and empty is not equal to `false`.
- `ph_cls` is the class passed to `placeholder_svg_tag`: the SVG carries only that class, so it must
  be the card's own class — never theme utility classes such as `w-full h-full`
  ([liquid-rules.md](../liquid-rules.md), placeholder images).

### Price area

```liquid
<div class="{{ root_cls }}__price">
  {% if p.price_min != p.price_max %}
    <ljs-currency layout="container" value="{{ p.price_min }}"></ljs-currency>
    <span>–</span>
    <ljs-currency layout="container" value="{{ p.price_max }}"></ljs-currency>
  {% else %}
    <ljs-currency class="{{ root_cls }}__price-now" layout="container" value="{{ price_now }}"></ljs-currency>
    {% if show_compare %}
      <s class="{{ root_cls }}__price-was"><ljs-currency layout="container" value="{{ p.compare_at_price }}"></ljs-currency></s>
    {% endif %}
  {% endif %}
</div>
```

A price range shows both amounts with a short dash between them — no "from" (起) word, which would
need another copy setting that follows the card copy language. Money output follows the price
section of [objects.md](../objects.md); give `ljs-currency` `display: inline-block` in the price-row
CSS.

### Two-column layout

Mobile stacks vertically, image on top; from 960px two equal columns, and `layout` decides whether
the image is left or right. The grid sits on `__body`, the direct parent of the media area and the
info column — in the purchase form those two are wrapped in three purchase-shell layers, so a grid on
the card root would see only one child.

```css
.{{ root_cls }}__body { display: grid; grid-template-columns: minmax(0, 1fr); gap: 16px; }
.{{ root_cls }}__media { position: relative; aspect-ratio: 1 / 1; overflow: hidden; }
.{{ root_cls }}__img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
@media (min-width: 960px) {
  .{{ root_cls }}__body { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 40px; align-items: center; }
  .{{ root_cls }}--image_right .{{ root_cls }}__info { order: -1; }
}
```

For image-right, put `order` on the info column; the media-area rule keeps only positioning and size.

## Display skeleton

```liquid
<div class="{{ root_cls }} {{ root_cls }}--{{ block.settings.layout }}" {{ block.shoplaza_attributes }}>
  <div class="{{ root_cls }}__body">
    <a class="{{ root_cls }}__media"{% unless p.isMock %} href="{{ p.url }}"{% endunless %}>
      {% if p.isMock %}
        {{ 'product-1' | placeholder_svg_tag: ph_cls }}
      {% else %}
        <ljs-img class="{{ root_cls }}__img" layout="fill" object-fit="cover"
          src="{{ p.image.src | img_url: '1080x' }}"
          alt="{{ p.image.alt | default: p.title | escape }}"></ljs-img>
      {% endif %}
      {% comment %} discount / sold-out / custom badges go here {% endcomment %}
    </a>

    <div class="{{ root_cls }}__info">
      <a class="{{ root_cls }}__title"{% unless p.isMock %} href="{{ p.url }}"{% endunless %}>{{ p.title }}</a>
      {% comment %} price area (see "Price area") {% endcomment %}
      {% if block.settings.text != blank %}
        <div class="{{ root_cls }}__text">{{ block.settings.text }}</div>
      {% endif %}
      {% if p.available %}
        <a class="{{ root_cls }}__btn"{% unless p.isMock %} href="{{ p.url }}"{% endunless %}>{{ block.settings.button_text }}</a>
      {% else %}
        <button type="button" class="{{ root_cls }}__btn" disabled>{{ block.settings.sold_out_text }}</button>
      {% endif %}
    </div>
  </div>
</div>
```

- The display form never contains `ljs-product-form` / `ljs-variants` / `ljs-quantity`: the button
  only goes to the product page.
- Placeholder state per the `isMock` placeholder section of [objects.md](../objects.md): links
  removed, image via `placeholder_svg_tag` (the media area appears only once), title and price
  rendered as usual.
- For several images, swap the media area for the "Gallery" block; nothing else changes.

## Purchase skeleton

The three purchase-shell layers are output only when `can_buy`; in placeholder and whole-product
sold-out states the card holds no purchase component at all.

```liquid
{% capture pid %}{{ block_id }}-{{ p.id }}{% endcapture %}
{% capture form_id %}patc-{{ pid }}{% endcapture %}
{% capture ds_id %}pds-{{ pid }}{% endcapture %}
{% capture pvar_id %}pvar-{{ pid }}{% endcapture %}
{% capture img_id %}pimg-{{ pid }}{% endcapture %}
{% capture qty_id %}qty-{{ pid }}{% endcapture %}
{% assign no_variant = false %}
{% if p.options == blank or p.options.size == 0 or p.has_only_default_variant %}
  {% assign no_variant = true %}
{% endif %}

<div class="{{ root_cls }} {{ root_cls }}--{{ block.settings.layout }}" {{ block.shoplaza_attributes }}>
  {% if can_buy %}
    <ljs-data-source id="{{ ds_id }}" layout="container" source-type="product" source-id="{{ p.id }}">
    <ljs-product-form id="{{ form_id }}" layout="container" product-id="{{ p.id }}" show-toast
      {% if no_variant %}variant-id="{{ p.variants[0].id }}"{% endif %}
      {% if block.settings.show_quantity %}@productChange="{{ qty_id }}.update(value=event.quantity,max=event.max)"{% endif %}>
      <form>
  {% endif %}

    <div class="{{ root_cls }}__body">
      <div class="{{ root_cls }}__media">
        {% comment %} single image or "Gallery"; discount / sold-out / custom badges go here {% endcomment %}
      </div>

      <div class="{{ root_cls }}__info">
        <a class="{{ root_cls }}__title"{% unless p.isMock %} href="{{ p.url }}"{% endunless %}>{{ p.title }}</a>
        {% comment %} price area (see "Price area") {% endcomment %}
        {% if block.settings.text != blank %}
          <div class="{{ root_cls }}__text">{{ block.settings.text }}</div>
        {% endif %}

        {% if can_buy and no_variant == false %}
          {% comment %} option picker (see "Option picker") {% endcomment %}
        {% endif %}
        {% if can_buy and block.settings.show_quantity %}
          {% comment %} quantity box: skeleton per ljs-quantity.md, inside the form, before the buttons {% endcomment %}
        {% endif %}

        {% if p.isMock %}
        {% elsif p.available == false %}
          <button type="button" class="{{ root_cls }}__btn" disabled>{{ block.settings.sold_out_text }}</button>
        {% else %}
          <button type="button" role="addToCart" class="{{ root_cls }}__btn">{{ block.settings.atc_text }}</button>
          {% if block.settings.show_buy_now %}
            <button type="button" role="buyNow" class="{{ root_cls }}__btn {{ root_cls }}__btn--buynow">{{ block.settings.buy_now_text }}</button>
          {% endif %}
        {% endif %}
      </div>
    </div>

  {% if can_buy %}
      </form>
    </ljs-product-form>
    </ljs-data-source>
  {% endif %}
</div>
```

- The three shell layers wrap the same way with or without variants: `ljs-variants` reads its data
  through the card-root `ljs-data-source`, and the extra layer doesn't affect add to cart for
  no-variant products. One skeleton has fewer failure points than two branches.
- Media, title and price are all inside the `form` — not just the buttons
  ([ljs-product-form.md](../ljs/ljs-product-form.md): the component is the card root, not a button
  wrapper).
- Write the add-to-cart text directly inside `<button>`; don't wrap it in `<span role="content">` —
  the storefront then shows the theme's translated default text instead of the merchant's
  `atc_text`.
- Sold-out and non-existent combinations are handled by the components themselves (the variant
  picker marks those options `soldout` / `no_exits`); don't compute availability in the card.

- When the shopper clicks add to cart or buy now before choosing every option, the component shows
  the prompt; the card writes no validation.

### Gallery

When `p.images.size > 1`, the media area holds a carousel; otherwise a single `ljs-img` (as in the
display media area). A placeholder product has `images.size` 5, so the `isMock` branch comes first.

```liquid
{% if p.isMock %}
  {{ 'product-1' | placeholder_svg_tag: ph_cls }}
{% elsif p.images.size > 1 %}
  {% assign w0 = p.images[0].width | default: 800 %}
  {% assign h0 = p.images[0].height | default: 800 %}
  <ljs-carousel id="{{ img_id }}" layout="responsive" width="{{ w0 }}" height="{{ h0 }}" controls loop>
    {% for im in p.images limit: 8 %}
      <div class="{{ root_cls }}__frame">
        <ljs-img layout="fill" object-fit="cover"
          src="{{ im.src | img_url: '1080x' }}" alt="{{ im.alt | default: p.title | escape }}"></ljs-img>
      </div>
    {% endfor %}
  </ljs-carousel>
  {% if block.settings.show_thumbnails %}
    <div class="{{ root_cls }}__thumbs">
      {% for im in p.images limit: 8 %}
        <button type="button" class="{{ root_cls }}__thumb"
          @tap="{{ img_id }}.goToSlide(index={{ forloop.index0 }})" aria-label="{{ forloop.index }}">
          <ljs-img layout="fill" object-fit="cover"
            src="{{ im.src | img_url: '160x' }}" alt="{{ im.alt | default: p.title | escape }}"></ljs-img>
        </button>
      {% endfor %}
    </div>
  {% endif %}
{% else %}
  <ljs-img class="{{ root_cls }}__img" layout="fill" object-fit="cover"
    src="{{ p.image.src | img_url: '1080x' }}" alt="{{ p.image.alt | default: p.title | escape }}"></ljs-img>
{% endif %}
```

- `layout="responsive"` + the first image's real size, `controls` for the built-in arrows, and the
  single-image fallback — the same three rules as the P2-a table in
  [product-card-interaction.md](product-card-interaction.md).
- When the carousel renders, don't force the square `aspect-ratio` / `overflow: hidden` of
  `__media` on it: the carousel takes its ratio from `width` / `height`, and the thumbnail strip
  sits below it. Keep the square box for the single-image and placeholder branches.
- Slide `src` uses `img_url: '1080x'`: this card has no src-matching sync, so raw URLs aren't
  needed.
- The thumbnail strip sits below the carousel; each thumbnail is a `<button type="button">` whose
  `@tap` calls `goToSlide(index=)`. No current-slide highlight.
- `.__thumb` needs `position: relative` + `aspect-ratio: 1 / 1` so the `layout="fill"` image has a
  box to fill.

### Option picker

One `fieldset` per option; each value is a `div` with `option` wrapping radio + label. It sits inside
the `form`, before the quantity box and buttons.

```liquid
<ljs-variants id="{{ pvar_id }}" layout="container" product-id="{{ p.id }}"
  src="custom:{{ ds_id }}.getData" disabled-default-value interference>
  {% for opt in p.options %}
    {% capture opt_idx %}{{ forloop.index0 }}{% endcapture %}
    <fieldset class="{{ root_cls }}__optgroup" name="{{ opt.name | escape }}">
      <legend class="{{ root_cls }}__optname">{{ opt.name }}</legend>
      <div class="{{ root_cls }}__optvalues">
        {% for v in opt.values %}
          {% capture voi %}{{ pvar_id }}-{{ opt_idx }}-{{ forloop.index0 }}{% endcapture %}
          <div option="{{ v | escape }}" class="{{ root_cls }}__optbtn">
            <input type="radio" option="{{ v | escape }}" id="{{ voi }}"
              name="single-{{ pvar_id }}-{{ opt.name | escape }}" value="{{ v | escape }}">
            <label for="{{ voi }}">{{ v }}</label>
          </div>
        {% endfor %}
      </div>
    </fieldset>
  {% endfor %}
</ljs-variants>
```

- `disabled-default-value` is required: nothing is preselected, the shopper picks before adding.
  `interference` is required: unavailable combinations are greyed out.
- No `auto-add-to-cart`: the shopper clicks add to cart or buy now after choosing, rather than
  adding as soon as every option is set.
- No `slide` / `switch-slide`: the main image doesn't follow the variant (see "Boundaries").
- The radio `name` has the literal `single-` prefix followed by `pvar_id`, so it never collides with
  other radios in the card.
- The radio id combines the outer captured `opt_idx` with the inner index, so two options each
  counting from 0 don't collide.
- Option button style: the radio covers the label with `position: absolute; inset: 0; opacity: 0`;
  the selected state is `input:checked + label`. The `soldout` / `no_exits` attributes the component
  adds land on `.__optbtn`; grey out with attribute selectors.

Style swatches (when "color thumbnails" is hit):

- Look up images per the image lookup in "Swatches and main-image sync (P2-b)" of
  [product-card-interaction.md](product-card-interaction.md), which yields `thumbs` and `opt_name`.
- Add images only to the option where `opt.name | downcase == opt_name`: inside its `label`, before
  the text, put `ljs-img layout="fixed" width="32" height="40"`.
- `src` is `t.image.src | img_url: '80x'` from the `thumbs` item where `t.type == v`; values with no
  matching image show text only.

## Purchase tiers

Decide with `options` / `has_only_default_variant` / `available` / `isMock`, never `variants.size`
(a placeholder product always has 6 variants).

| Tier | Condition | What to do |
|---|---|---|
| Placeholder | `p.isMock` | No shell, option picker, quantity box or buttons; main image, title, price as usual |
| Whole product sold out | `p.available == false` | No shell, option picker or quantity box; the button slot shows a `disabled` button with `sold_out_text` |
| No variant / single variant | `p.options == blank` or `p.options.size == 0` or `p.has_only_default_variant` | `ljs-product-form` gets `variant-id="{{ p.variants[0].id }}"`; no `ljs-variants` |
| Multi-option | Everything else | No `variant-id`; the full "Option picker"; the component writes the selected variant into the form |

The `p.options == blank` part can't be dropped; see "`options.size == 0` as the no-variant test" in
[product-card.md](product-card.md) (§6).

## Wiring and ids

Ids a card captures (all built from `{{ block_id }}-{{ p.id }}`, see the top of "Purchase skeleton"):

| Purpose | id |
|---|---|
| Data source | `pds-{{ pid }}` |
| Add-to-cart form | `patc-{{ pid }}` |
| Option picker | `pvar-{{ pid }}` |
| Image carousel | `pimg-{{ pid }}` |
| Quantity box | `qty-{{ pid }}` |

The three interactions (closed list — nothing outside these three)

| # | Interaction | How | When |
|---|---|---|---|
| 1 | Thumbnail → main image | Thumbnail `@tap="{{ img_id }}.goToSlide(index=N)"` | Gallery with `show_thumbnails` |
| 2 | Variant change → quantity limit | `@productChange="{{ qty_id }}.update(value=event.quantity,max=event.max)"` on `ljs-product-form` | `show_quantity` |
| 3 | Add to cart / buy now feedback | `show-toast`, built into the component | Purchase form |

Interaction 2's `value` must be `event.quantity`: changing the quantity also fires `productChange`,
and a hard-coded `value` of `1` resets the count the shopper just set. With no variant selected,
`event.max` is empty and the component keeps the previous limit.

Not built: variant-driven image switching, per-variant price recalculation, opening a cart drawer
after add, calling a theme's popups ("Using a theme's private building blocks" in
[product-card.md](product-card.md), §6).

## Setting-id contract

### The two required settings (missing either leaves a broken card)

| Setting | id | type | default |
|---|---|---|---|
| Product | `product` | `product` | — |
| Image / text position | `layout` | `select` `image_left` / `image_right` | `image_left` |

### Id contract (the same meaning always uses the same id / type / default / enum values)

Ids added by this card type:

| Meaning | id | type | default |
|---|---|---|---|
| Product | `product` | product | — |
| Image / text position | `layout` | select image_left, image_right | `image_left` |
| Merchant copy | `text` | textarea | empty |
| Product-page button text | `button_text` | text | 查看详情 / View details |
| Buy now | `show_buy_now` / `buy_now_text` | checkbox / text | false / 立即购买 / Buy now |
| Quantity box | `show_quantity` / `qty_label` | checkbox / text | false / 数量 / Quantity |
| Thumbnail strip | `show_thumbnails` | checkbox | true |

Ids shared with the list card are copied from the id contract table in
[product-card.md](product-card.md) (§5 "Setting tiers and id contract"); don't invent new names:

- Add-to-cart and sold-out copy: `atc_text` `sold_out_text`
- Badges: `discount_style` `badge_position` `show_sold_out_badge` `show_badge` `badge_text`
- Look: `image_ratio` `image_fill_mode` `corner_radius` `text_alignment` `bg_color` `text_color`
- Style swatches: `show_variant_swatch` `swatch_option_names`

Settings with two defaults are merchant-facing copy; use the one that matches the card copy
language (the storefront's primary-market language; see [schema-rules.md](../schema-rules.md)).

Product title, price, images and link come from the product the merchant picks and stay out of the
schema; `presets` never carry a real product's data either — see "Product fields get generic
placeholders only" in [product-card.md](product-card.md).

### Extension points and id freeze

The three freeze rules are the same as "Extension points and id freeze" in
[product-card.md](product-card.md): append only, never change an existing id / type / enum, and when
the meaning changes add a new id and keep the old one.

| Follow-up request | Insert at | New settings | Must not touch | Accept when |
|---|---|---|---|---|
| Add an intro | After the price area, before the option picker | `text` | Don't change title and price output | No blank line when empty |
| Badge on the image | Last child of the media area | `show_badge` + `badge_text` (reuse `badge_position`) | Keep the badge out of flow | Media height unchanged when off |
| Choose quantity | After the option picker, before the buttons | `show_quantity` + `qty_label` | Don't change the buttons | Interaction 2 on `ljs-product-form`; the count doesn't jump back after a change |
| Add buy now | After the add-to-cart button | `show_buy_now` + `buy_now_text` | Don't change `atc_text` | Clicking before all options are chosen shows a prompt and doesn't go to checkout |
| Colors with thumbnails | Inside the matching option's `label` in the option picker | `show_variant_swatch` + `swatch_option_names` | Values without an image show text only | Thumbnails match option values one to one |
| Images can flip (display → gallery) | Media area | `show_thumbnails` | The carousel stays when the thumbnail strip is off | No carousel on single-image products |
| Display only, not for sale (purchase → display) | Button slot | `button_text` | Keep existing settings such as `atc_text`; don't delete them | No purchase component left in the card |

## Pitfalls

### Button text wrapped in `role="content"`

Inside `ljs-product-form`, a `[role="content"]` node shows the theme's translated text rather than
the merchant's `atc_text`, and follows the theme language. Write the text directly inside `<button>`.

### Hard-coded `value` in interaction 2

`@productChange` also fires when the quantity changes; `update(value=1, …)` resets the shopper's
count to 1. Write `value=event.quantity`.

### `variant-id` on a multi-option product

It picks the first variant for the shopper, so nothing chosen in the option picker counts.
Multi-option products never get `variant-id`; `ljs-variants` writes the selected variant into the
form.

### Option picker built as a panel

The add-to-cart icon, the `.none` panel and `auto-add-to-cart` are the list card's pattern ("Add-to-cart
area" in [product-card.md](product-card.md)). A single-product card's option picker is always
visible, and the shopper clicks a button after choosing.

### Purchase components in placeholder state

`default_product` has no real product id: `ljs-data-source` gets no data and `ljs-product-form`
fails to add. When `can_buy` is false, the three shell layers are not output at all — not
`disabled`.

### Purchase components in the display form

A link-out card has no option picker or add-to-cart flow for `ljs-product-form` / `ljs-variants` /
`ljs-quantity` to serve; they only add markup and data requests, and make the card read as a
purchase card. The display button is only `<a href="{{ p.url }}">`.

Component attributes come only from the `../ljs/ljs-*.md` files you have read; attribute tables
seen anywhere else don't count.

# ljs-product-form — add to cart / buy now host

Hosts variant add-to-cart and checkout. Used on product-detail style cards and quick-add panels.

## Rules

1. It is the **card root**, not a button wrapper: the whole product card sits inside
   `ljs-product-form > form`. Wrap only the button and the card's option radios and add-to-cart
   panel can't find their host form.
2. The list-card shell, button behaviour, and panel rules are in the add-to-cart section of
   [kinds/product-card.md](../kinds/product-card.md); single-product cards have four purchase
   tiers, see [kinds/product-single.md](../kinds/product-single.md). The skeleton below is the
   list-card root form.
3. `role="addToCart"` is the component's hook — never rename it. Write the button as
   `<button type="button">`.
4. For a product with no variants or a single variant, set the variant with the `variant-id`
   attribute on the root. Don't add a hidden `variant_id` input to the `<form>` (the root is
   already the form; it would duplicate).
5. Success/failure feedback uses `show-toast` (boolean: present when on, omitted when off). Never
   fetch or toast yourself.
6. Build `id` as `{{ block_id }}-{{ product.id }}` so several shells and cards on one page don't
   collide.
7. Use only the attributes in the table below. Don't write `buy-now-url` or `manual-create-order`,
   and don't call the theme's own quick-view popup: those depend on one theme's global UI and fail
   silently on another theme.
8. The component sends the cart request. Don't `fetch('/api/cart')` yourself — a hand-written
   request bypasses the component's error handling, translations, and analytics. Don't add a
   multi-variant product directly either (the buyer would get the default variant); follow the
   add-to-cart rules of the matching kind file.
9. Buy now is `<button type="button" role="buyNow">`; the component sends the buyer to checkout
   with the selected variant and quantity. It appears only on a single-product purchase card
   ([kinds/product-single.md](../kinds/product-single.md)). Product-list cards have a single quick
   add button (straight to cart or opens the panel); a button that links to the product page
   appears only in the async load-more list tier (L2,
   [kinds/product-card-async.md](../kinds/product-card-async.md)).
10. Put the button text directly in `<button>`; don't wrap it in `<span role="content">`. The
    component rewrites the text of `[role="content"]` nodes from the theme's language pack, which
    overwrites the merchant's copy.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `product-id` | Product id | yes | `p.id` |
| `show-toast` | Toast for the add-to-cart result | no | boolean; keep it on |
| `variant-id` | Preselected variant | no | required for no-variant / single-variant products: `{{ product.variants[0].id }}`; omit for multi-option products |
| `disable-init-toast` | Suppresses the toast on initialisation | required on product-list cards | sold-out products are still wrapped, and without it each one pops a sold-out toast on load |
| `min-variants-quantity` | Minimum quantity per variant combination | no | must be a valid number; a non-number breaks the component |

## Actions

Call as `<id>.<action>`.

| Action | Effect |
|---|---|
| `setProduct` | Replace the current product data with `data` |
| `addToCart` | Add to cart, same as clicking the `role="addToCart"` button |
| `buyNow` | Buy now with the selected variant |

## Events

Declarative, bind only what you need.

| Event | When | Notes |
|---|---|---|
| `@atcSuccess` | After a successful add | Usually unnecessary (`show-toast` covers feedback); the card's add-to-cart panel closes on it ([kinds/product-card.md](../kinds/product-card.md)) |
| `@atcError` | After a failed add | Same as above |
| `@productChange` | Product data changed (option, variant, or quantity) | Keeps the quantity limit in sync: `<qid>.update(value=event.quantity,max=event.max)`; `event.max` is the selected variant's sellable stock, `event.quantity` the current quantity (see [ljs-quantity.md](ljs-quantity.md)) |
| `@productInvalid` | The selected variant doesn't exist | |
| `@update` | Component data updated | |
| `@orderChange` | Order-related data changed | |
| `@requestStart` / `@requestEnd` | Add / buy request starts / ends | Use for a loading state |
| `@buyNowSuccess` / `@firstBuyNowSuccess` / `@buyNowError` | Buy now succeeded / first success / failed | |
| `@productVariantCombinationChange` | The combination changed in variant-combination mode | |
| `@option{N}Invalid` / `@option{N}Valid` | Option N is unselected / selected; N starts at 1 | Case-sensitive: `Invalid` / `Valid` start with a capital, lowercase won't bind. If it doesn't fire, fall back to `@atcError` |
| `@{optionName}Invalid` / `@{optionName}Valid` | Same, keyed by option name without spaces, e.g. `@ColorInvalid` | Fires together with `@option{N}Invalid`; either form works |

## Skeleton (card root)

How the `direct_atc` / `swatch_atc` booleans are computed is in the add-to-cart section of
[kinds/product-card.md](../kinds/product-card.md). `p` is the product in the list loop.

```liquid
<div class="{{ root_cls }}__card">
  {% unless p.isMock %}
    <ljs-product-form
      id="patc-{{ block_id }}-{{ p.id }}"
      layout="container"
      product-id="{{ p.id }}"
      show-toast
      disable-init-toast
      {% if direct_atc %}variant-id="{{ p.variants[0].id }}"{% endif %}
    >
      <form>
  {% endunless %}

        {% comment %} Media, title, price, and options all sit inside the form {% endcomment %}
        {% comment %} Quick-add button: sold-out disabled / add directly / open panel — which one a product gets is in the add-to-cart button table; only "add directly" is shown here. This slot is the button form; the icon form sits in the media area, one per card per device {% endcomment %}
        <button type="button" role="addToCart" class="{{ root_cls }}__btn">{{ block.settings.atc_text }}</button>

  {% unless p.isMock %}
      </form>
    </ljs-product-form>
  {% endunless %}
</div>
```

List-card shell, button behaviour, and panel: [kinds/product-card.md](../kinds/product-card.md)
(add-to-cart section). Single-product skeleton and tiers:
[kinds/product-single.md](../kinds/product-single.md) (purchase skeleton, purchase tiers).

## Common mistakes

The most common wrong shape wraps only the button in the form; the media area and option radios
stay outside, and the component can't find them:

```liquid
{% comment %} Wrong: card body outside the form {% endcomment %}
<div class="card">
  <div class="media">…</div>
  <ljs-product-form product-id="…"><form><button role="addToCart">Add to cart</button></form></ljs-product-form>
</div>
```

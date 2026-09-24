# ljs-quantity — quantity stepper

The "− 1 +" box next to add-to-cart: the buyer picks how many, and add-to-cart uses that quantity.

Two hosts, both optional elements: products on a list card that can be added from the card (the
"add directly" and "open panel" rows of the add-to-cart section in
[kinds/product-card.md](../kinds/product-card.md)), and the single-product purchase card
([kinds/product-single.md](../kinds/product-single.md)).

> Restricted component: [selection.md](selection.md) bans purchase components as a class; this is
> one of the three exceptions (with [ljs-product-form.md](ljs-product-form.md) and
> [ljs-variants.md](ljs-variants.md)). Both conditions must hold: it sits in the purchase area of a
> product block (a list-card product that can be added from the card, or a single-product purchase
> card), and the matching kind file has been read — [kinds/product-card.md](../kinds/product-card.md)
> for list cards, [kinds/product-single.md](../kinds/product-single.md) for single-product cards.

## Rules

1. If the restriction above isn't met, don't use it. A "pick number of people / servings" counter on
   an image-text card is not this component's job; write a plain input.
2. It needs a host form: the box goes inside the card-root `ljs-product-form > form`. Placed loose on
   the card it's just a counter, and add-to-cart still adds 1.
3. Write `name="quantity"`. The form finds the quantity box without it, so it isn't strictly
   required, but the name keeps the form semantics complete.
4. With `layout="fixed"`, give both `width` and `height` (usually `94×32`); a fixed layout without a
   size collapses. In a flex row you may use `flex-item`, still with width and height.
5. Always give `min` and `max`: `max` is
   `p.selected_or_first_available_variant.available_quantity | default: 9999`, `min` is always `1`.
   The real limit after a variant change is synced by rule 6.
6. Sync the limit when the variant changes: on the card-root `ljs-product-form`, bind
   `@productChange` to `<qid>.update(value=event.quantity,max=event.max)`. `event.max` is the
   selected variant's sellable stock, `event.quantity` the current quantity — changing the quantity
   also fires this event, so a hard-coded `value` would reset what the buyer just typed. With no
   variant selected `event.max` is empty and the box keeps its old limit. Never skip this when the
   card also has `ljs-variants` or an option `ljs-selector`; otherwise a size with 2 left can still
   go to 50.
7. Style only through `icon-class` / `input-class`: `icon-class` lands on the decrease/increase
   buttons, `input-class` on the number input. Don't target the component's internal DOM; borders,
   radius, and font size of the buttons and input all go through these two classes.
8. To change the +/− icons, add child `<svg role="decrease">` / `<svg role="increase">`; the
   component uses them in place of its default icons. Don't draw two extra buttons on top.
9. No hand-written +/− JS (the intent → component table in [selection.md](selection.md): if a
   component covers the interaction, don't hand-write it) — no `onclick`, no `input.value++`.
10. In placeholder state (`p.isMock`) the whole add-to-cart area isn't rendered (add-to-cart section
    of [kinds/product-card.md](../kinds/product-card.md)), and the quantity box goes with it. When
    sold out (`p.available == false`) the button is greyed out and the quantity box is not output
    either.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `layout` | Layout | yes | `fixed` (with `width`/`height`); `flex-item` in a flex row, still with a size |
| `width` / `height` | Size | yes | `94` / `32` |
| `name` | Form field name | recommended | always `quantity` (rule 3) |
| `value` | Initial quantity | yes | always `1` |
| `min` | Lower bound | yes | always `1` |
| `max` | Upper bound | yes | from stock, rule 5 |
| `icon-class` | Class for the +/− buttons | no | one of the two styling hooks |
| `input-class` | Class for the number input | no | same |

Actions: only `update(value=…, max=…, min=…)`. The parameter names are exactly these three; a wrong
name (e.g. `quantity=2`) is silently ignored. A `value` above the current `max` is clamped to `max`,
not rejected.

Events: `quantityChange` / `quantityChangeUnderflow` / `quantityChangeOverflow`. `event.value` is a
number; on overflow it is already the clamped `max` (not the larger number the buyer tried), on
underflow the `min` — to say "at most N", use `event.value` directly. Bind them declaratively
(`@quantityChangeOverflow="…"`).

## Skeleton

```liquid
{% capture pf_id %}pf-{{ block_id }}-{{ p.id }}{% endcapture %}
{% capture qty_id %}qty-{{ block_id }}-{{ p.id }}{% endcapture %}

{% unless p.isMock %}
  {% if p.available %}
    {% assign sv = p.selected_or_first_available_variant %}
    {% assign qty_max = sv.available_quantity | default: 9999 %}

    <ljs-product-form id="{{ pf_id }}" layout="container"
      product-id="{{ p.id }}" variant-id="{{ p.variants[0].id }}">
      <form>
        <div class="{{ root_cls }}__qtyrow">
          <span class="{{ root_cls }}__qtylabel">{{ block.settings.qty_label }}</span>
          <ljs-quantity
            id="{{ qty_id }}"
            name="quantity"
            layout="fixed" width="94" height="32"
            value="1" min="1" max="{{ qty_max }}"
            icon-class="{{ root_cls }}__qtyicon"
            input-class="{{ root_cls }}__qtyinput">
          </ljs-quantity>
        </div>
        <button type="button" role="addToCart" class="{{ root_cls }}__atc">
          {{ block.settings.atc_text }}
        </button>
      </form>
    </ljs-product-form>
  {% endif %}
{% endunless %}
```

When the card also selects variants, put the limit sync on the card-root `ljs-product-form`:

```liquid
<ljs-product-form … @productChange="{{ qty_id }}.update(value=event.quantity,max=event.max)">
```

CSS: the `icon-class` / `input-class` classes go in the block's own styles; control the input width
through `input-class`, not by changing the `display` of the `ljs-quantity` root.

## Common mistakes

Two mistakes the rules alone don't make obvious:

```liquid
{% comment %} Wrong: no host form — the chosen quantity never reaches the cart {% endcomment %}
<div class="card__qty"><ljs-quantity value="1" min="1" max="10"></ljs-quantity></div>

{% comment %} Wrong: card has variants but the limit isn't synced — a size with 2 left still goes to 50 {% endcomment %}
<ljs-variants …></ljs-variants>
<ljs-quantity id="qty-x" max="9999"></ljs-quantity>
```

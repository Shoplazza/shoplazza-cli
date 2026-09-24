# ljs-variants — variant picker (list-card panel and single-product option area)

Two uses:

- On a product-list card, the "tap add-to-cart icon → pick options on the page → straight into the
  cart" panel: add-to-cart section of [kinds/product-card.md](../kinds/product-card.md).
- On a single-product card, an always-visible option area where the buyer picks and then presses a
  button: option-area section of [kinds/product-single.md](../kinds/product-single.md).

> Restricted component: [selection.md](selection.md) bans purchase components as a class; this
> component, [ljs-product-form](ljs-product-form.md), and [ljs-quantity](ljs-quantity.md) are the
> only three exceptions, and all of these must hold: a product block + a list card's add-to-cart
> area or a single-product purchase card + this doc has been read + the matching kind file has been
> read ([kinds/product-card.md](../kinds/product-card.md) for list cards,
> [kinds/product-single.md](../kinds/product-single.md) for single-product cards).

## Rules

1. Two shapes: on a list card, an in-card panel for multi-option products + `auto-add-to-cart`; on
   a single-product card, an always-visible option area without `auto-add-to-cart`, where the buyer
   picks and then presses add-to-cart or buy-now. Swapping the main image from swatches on a list
   card is not this component's job — see the swatch → main-image section of
   [kinds/product-card-interaction.md](../kinds/product-card-interaction.md).
2. It needs the host chain: the whole card sits inside `ljs-data-source`
   (`source-type="product"` + `source-id`) → `ljs-product-form > form`. Once all options are chosen
   the add-to-cart goes through that form.
3. The option DOM is fixed: one `<fieldset name="<option name>">` per option; per value a
   `<div option="<value>">` wrapping `<input type="radio" option="<value>" name="<prefix>-<option name>" value="<value>">`
   + `<label for>`. Both the outer div and the input carry `option`.
4. Prefix the radio `name` (e.g. `quick-add-<component id>-`). Without it the names clash with the
   card's swatch radios and each group clears the other's selection.
5. On a list card the panel starts with the card's own `.none` class (`display: none`); open and
   close it only with `<panelId>.toggleClass(class='none', force=true/false)`. The component isn't
   built while the panel is closed — it builds when it enters the viewport; that is expected, so
   don't switch the open/close to `visibility`.
6. Always fill `product-id`. Without it the component still loads, but variant data fails to match.
7. Use only the attributes in the table.

## Attributes

| Attribute | Purpose | Notes |
|---|---|---|
| `product-id` | Product id | always fill it; without it variant data fails to match |
| `src` | Data entry | required; use the card-root `ljs-data-source` (`source-type="product"`) as `custom:<ds>.getData`. A bare `script:<json id>` also works, but always go through the data source |
| `auto-add-to-cart` | Add to cart automatically once all options are chosen | boolean; the core of the list-card panel — it submits itself, so the panel needs no `role="addToCart"` button. Don't write it in a single-product option area |
| `disabled-default-value` | Don't preselect the first value | boolean; recommended on the list-card panel, required in the single-product option area so the buyer chooses |
| `interference` | Grey out unavailable combinations | boolean; always on |
| `manual` | Don't render automatically after mount | boolean; rendering then waits for `variantsRender` with product data. Omit it for static DOM |
| `select-soldout-suffix` | Suffix for sold-out options | free text, e.g. `"(Sold out)"`. Only changes `<option>` text in a `<select>`; no effect on radios |
| `select-no-exists-suffix` | Suffix for non-existent combinations | free text, e.g. `"(Unavailable)"`. Same: `<option>` only |
| `include-names` | Render only these option names | list of option names: JSON array or comma-separated; others are not rendered |
| `items` | Which field of the data source holds the product | leave unset with a product data source |
| `inherit-url-variant` | Take the initial selection from the URL's variant | boolean |
| `product-swatches` | Option names that get variant thumbnails | same format as `include-names`; only used by client-side `template` rendering, no effect on static DOM |
| `interact` | Hover behaviour | only `hover`: fires `mouseover` / `mouseout` events as the pointer enters/leaves options (for swatch linking) |

Actions: `clear` (clear the selection), `variantsRender` (render from `product` data).

Events: hover events `mouseover` / `mouseout` (plus per-option forms such as
`<optionName>mouseover`). There is no `select` event — quantity limits, button state, and similar
linking go on the host `ljs-product-form`'s `@productChange`, not on this component.


## Skeleton

This is the list-card panel. The single-product option-area skeleton is in the option-area section
of [kinds/product-single.md](../kinds/product-single.md); the option DOM and radio-prefix rules are
the same in both.

```liquid
{% capture panel_id %}ppanel-{{ block_id }}-{{ p.id }}{% endcapture %}
{% capture pvar_id %}ppvar-{{ block_id }}-{{ p.id }}{% endcapture %}

{% comment %}
  Closing the panel is bound on the card-root product-form (not on this component):
  <ljs-product-form … @atcSuccess="{{ panel_id }}.toggleClass(class='none', force=true);{{ pvar_id }}.clear">
  What the opening element looks like and where it sits: kinds/product-card.md, add-to-cart shapes
{% endcomment %}
<button type="button" class="{{ root_cls }}__quickadd"
  @tap="{{ panel_id }}.toggleClass(class='none', force=false)"
  aria-label="{{ block.settings.atc_text | escape }}">+</button>

<div id="{{ panel_id }}" class="{{ root_cls }}__panel none">
  <ljs-variants id="{{ pvar_id }}" layout="container" product-id="{{ p.id }}"
    src="custom:pds-{{ block_id }}-{{ p.id }}.getData"
    auto-add-to-cart disabled-default-value interference>
    {% for opt in p.options %}
      <fieldset class="{{ root_cls }}__optgroup" name="{{ opt.name | escape }}">
        <div class="{{ root_cls }}__optname">
          <span>{{ opt.name }}</span>
          {% if forloop.first %}
            <span class="{{ root_cls }}__panelclose"
              @tap="{{ panel_id }}.toggleClass(class='none', force=true)">&times;</span>
          {% endif %}
        </div>
        <div class="{{ root_cls }}__optvalues">
          {% for v in opt.values %}
            {% capture voi %}{{ pvar_id }}-{{ opt.position }}-{{ forloop.index0 }}{% endcapture %}
            <div option="{{ v | escape }}" class="{{ root_cls }}__optbtn">
              <input type="radio" option="{{ v | escape }}" id="{{ voi }}"
                name="quick-add-{{ pvar_id }}-{{ opt.name | escape }}" value="{{ v | escape }}">
              <label for="{{ voi }}">{{ v }}</label>
            </div>
          {% endfor %}
        </div>
      </fieldset>
    {% endfor %}
  </ljs-variants>
</div>
```

CSS: `.__panel.none { display: none }`; the radio covers its label with
`position: absolute; inset: 0; opacity: 0`, selected state via `input:checked + label`; the panel
is absolutely positioned inside the media area and must not cover the next row of cards.

## Common mistakes

Three mistakes the rules alone don't make obvious:

```liquid
{% comment %} Wrong: no host form around the panel — nowhere to submit {% endcomment %}
<div class="panel none"><ljs-variants auto-add-to-cart>…</ljs-variants></div>

{% comment %} Wrong: radio without a prefix — same name as the swatch radios, they clear each other {% endcomment %}
<input type="radio" name="{{ opt.name }}" value="{{ v }}">

{% comment %} Wrong: using it to swap images by swatch {% endcomment %}
<ljs-variants slide="carousel-id;" switch-slide="Color">…</ljs-variants>
```

Don't write `slide` / `switch-slide` here: on list cards the only image swap is the swatch →
main-image section of [kinds/product-card-interaction.md](../kinds/product-card-interaction.md), and
a single-product card's main image doesn't follow the variant.

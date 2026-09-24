# themes product-card-interaction — in-card image carousel and swatch sync (tier P2)

Read this when the in-card elements reach P2: in-card image carousel (P2-a) and style swatches with
main-image sync (P2-b). The two can stack. How `img_id` is built and the interaction table are in
[product-card.md](product-card.md) (§4 id naming table); the in-card add-to-cart panel is P0 and
lives in [product-card.md](product-card.md) ("Add-to-cart area").

The single-product card ([product-single.md](product-single.md)) borrows only two parts of this
file: the rule table under "In-card image carousel (P2-a)" and the image lookup under "Swatches
and main-image sync (P2-b)".

## In-card image carousel (P2-a)

```liquid
{% assign w0 = p.images[0].width | default: 800 %}
{% assign h0 = p.images[0].height | default: 800 %}
<ljs-carousel id="{{ img_id }}" layout="responsive" width="{{ w0 }}" height="{{ h0 }}" controls loop>
  {% for im in p.images limit: 8 %}
    <a href="{{ p.url }}" draggable="false">
      <ljs-img layout="fill" object-fit="{{ block.settings.image_fill_mode }}"
        src="{{ im.src }}" alt="{{ im.alt | default: p.title | escape }}"></ljs-img>
    </a>
  {% endfor %}
</ljs-carousel>
```

| Item | Requirement |
|---|---|
| `layout` + `width` / `height` | `responsive` + the first image's real width and height. `layout="container"` also renders, but the ratio is unstable |
| Slide `src` | **Use the raw `im.src`; never wrap it in `img_url`** — swatch sync matches slides by src through `path=`, and a resized URL never matches |
| Arrows | `controls` makes the component render its own arrow buttons. Your own children with the `pre` / `next` attribute replace those buttons and are not counted as slides, but you style them yourself |
| Single image | When `p.images.size < 2`, no carousel: fall back to a static single image with no flip controls |
| Block attributes | No `{{ block.shoplaza_attributes }}` here — this carousel repeats per product; the attributes go once on the block root |

Mutually exclusive with "second image on hover"; see the dependency table in
[product-card.md](product-card.md) (§1).

## Swatches and main-image sync (P2-b)

Get the images with the `option_thumbnails` filter:

```liquid
{% assign thumbs = nil %}
{% assign opt_name = '' %}
{% assign names = block.settings.swatch_option_names | split: ',' %}
{% for n in names %}
  {% if thumbs == nil or thumbs.size == 0 %}
    {% assign one = n | strip | downcase | json %}
    {% assign one_arr = '[' | append: one | append: ']' | parse_json %}
    {% assign thumbs = p | option_thumbnails: one_arr %}
    {% if thumbs.size > 0 %}{% assign opt_name = n | strip | downcase %}{% endif %}
  {% endif %}
{% endfor %}
{% assign target_option = nil %}
{% for opt in p.options %}
  {% assign on = opt.name | downcase %}
  {% if on == opt_name %}{% assign target_option = opt %}{% endif %}
{% endfor %}
```

- The second argument must be a non-empty array → wrap one name into a single-element array with
  `json` + `parse_json`. Query one option name at a time; stop at the first non-empty result.
- Preconditions: `p.options` non-empty, `p.variants` non-empty, `p.need_variant_image` true;
  otherwise it returns `[]`.
- Returns `[{type: option value, url: variant url, image: Image object}]`.

**Render the swatch area only when all three gates pass**: `thumbs.size > 0` and `target_option`
is not empty and `thumbs[0].image.src != blank`.

When all three pass and `p.options.size == 1`, set `swatch_atc` from "Add-to-cart area" in
[product-card.md](product-card.md) to `true`: the button is a direct `role="addToCart"`, no panel.
Checking only the first gate produces two broken results: if `target_option` can't be found, the
fieldset gets `name=""` and the selector never receives the option name; if the filter passes
data through without images, a row of empty swatches renders.

```liquid
<fieldset class="{{ root_cls }}__swatches" name="{{ target_option.name | escape }}">
  <ljs-selector layout="container" class="{{ root_cls }}__swatchlist"
    @select="{{ img_id }}.goToSlide(path=event.targetOption)">
    {% for t in thumbs limit: block.settings.swatch_per_row %}
      <div class="{{ root_cls }}__swatch" option="{{ t.image.src }}">
        <input type="radio" name="{{ target_option.name | escape }}"
          value="{{ t.type | escape }}"{% if forloop.first %} checked{% endif %}>
        <ljs-img layout="fixed" width="32" height="40" object-fit="cover" auto-fit
          src="{{ t.image.src | img_url: '80x' }}" alt="{{ t.type | escape }}"></ljs-img>
      </div>
    {% endfor %}
  </ljs-selector>
</fieldset>
```

Four rules; miss any one and nothing syncs:

1. `@select` goes on the `ljs-selector` element itself, not `@tap` on each item — item-level
   `@tap` never fires.
2. Each item's `option` value is the src of that style's image, not the option value —
   `goToSlide(path=event.targetOption)` receives exactly this string and matches slides by src.
3. The `option` src and the carousel slide src must be the same raw URL (`t.image.src` /
   `im.src`). If either side is wrapped in `img_url`, the strings differ: clicking never changes the
   image and nothing reports an error. The thumbnail's own display `ljs-img` may use `img_url`; it
   takes no part in matching.
4. Each item also holds a hidden radio (`name` = option name, `value` = option value) — that path
   feeds add to cart; the `option` path only changes the image. With only `option`, the image
   changes but add to cart doesn't work; with only the radio, add to cart works but the image
   doesn't change.

`ljs-variants` is not used in this tier: image changes come from the selector's `@select`, add to
cart from the radio + the card-root `ljs-product-form`.

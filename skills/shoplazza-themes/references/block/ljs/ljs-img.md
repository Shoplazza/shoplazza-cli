# ljs-img — images

Product and content images; handles the download size for the display size and the empty-image
placeholder. lessjs reference: [spz-img](https://lessjs.shoplazza.com/latest/components/spz-img/)
(write the tag as `ljs-img`).

## Rules

1. `src` should be `{{ xxx | img_url }}`. To control the download size pass a width,
   `img_url: '<width>x'`, chosen from the actual display width (thumbnail `'80x'`, product card
   `'540x'`, large image `'1280x'` and so on) — don't copy one fixed number everywhere.
2. Empty image: don't output `ljs-img`; show a placeholder box with a light background (fixed
   height or `aspect-ratio`). Never an empty `src` — the component treats it as a load failure and
   offers no fallback UI.
3. `layout` is required and takes only `fill` / `responsive` / `fixed` / `fixed-height` /
   `intrinsic` / `flex-item`; without it the component errors and the layout doesn't apply.
4. Fill a container: `layout="fill"` + `object-fit="cover"` (fill is absolutely positioned; the
   parent needs positioning and a size).
5. Fixed ratio: `layout="responsive"` + both `width` and `height` (with either missing it silently
   falls back to intrinsic and the ratio is lost). Both values can be real pixels or the two halves
   of a ratio; for a merchant-configurable ratio see below.
6. Fixed height: `layout="fixed-height"` with only `height`; `width` must be absent or `"auto"` —
   a numeric width makes the component error.
7. `auto-fit`: add as needed (common on product cards). It optimizes the download size for the
   element's rendered width; it works on the CDN URLs `img_url` produces.
8. Images inside initially hidden containers (`display:none` tabs / drawers / popups): `auto-fit`
   can't measure them at mount and defers loading. Don't rely on `auto-fit` there; request an
   explicit size with `img_url: '800x'`.


## Attributes

| Attribute | Purpose | Required | Usage |
|---|---|---|---|
| `src` | Image URL | Yes | Through `\| img_url` |
| `layout` | Layout | Yes | One of the six, see rule 3 |
| `alt` | Alternative text | No | Recommended |
| `width` / `height` | Size | No | Both for `responsive` / `fixed`; only `height` for `fixed-height` |
| `object-fit` | Fit | No | Usually `cover` / `contain` |
| `object-position` | Crop focus | No | With `cover`, e.g. `center top` |
| `auto-fit` | Pick the download size from the container | No | Boolean, no value |

Use only the attributes in the table. Always use this component for images, never a native `<img>`.

The component adds `complete` to the element once the image has loaded — use `ljs-img[complete]`
in CSS for fade-ins or to hide a skeleton.

## Merchant-configurable image ratio

Make the ratio a `select` with id `image_ratio`, and write the values directly as CSS
"width/height" so the template needs no conversion:

| value | Ratio | value | Ratio |
|---|---|---|---|
| `original` | Original | `1/1` | Square |
| `4/3` | Landscape | `3/4` | Portrait |
| `2/3` | Taller | `16/9` | Widescreen |

Usually use `layout="responsive"` and split the ratio into `width` / `height` for the component;
the component builds its own placeholder box, so the wrapper needs no positioning styles:

```liquid
{% assign img = images[item.settings.image] %}
{% assign wh = block.settings.image_ratio | split: '/' %}
{% if item.settings.image != blank %}
  <ljs-img
    src="{{ item.settings.image | img_url: '800x' }}"
    alt="{{ img.alt | escape }}"
    layout="responsive"
    {% if block.settings.image_ratio == 'original' %}
      width="{{ img.width }}"
      height="{{ img.height }}"
    {% else %}
      width="{{ wh[0] }}"
      height="{{ wh[1] }}"
    {% endif %}
    object-fit="cover"
    auto-fit
  ></ljs-img>
{% else %}
  <div class="media-placeholder"></div>
{% endif %}
```

Only the image itself knows the `original` width and height, so take the real pixels from
`images[…]`; the other options split the ratio. Always give both halves — with one missing the
component silently falls back to intrinsic and the ratio is lost.

The empty-image placeholder box copies the configured ratio; `original` has no ratio to follow, so
use a square:

```liquid
<style>
  .{{ root_cls }} .media-placeholder {
    width: 100%;
    background: #f4f4f4;
    {% if block.settings.image_ratio == 'original' %}
      aspect-ratio: 1 / 1;
    {% else %}
      aspect-ratio: {{ block.settings.image_ratio }};
    {% endif %}
  }
</style>
```

To layer things over the image (badges, an add-to-cart icon, a second image on hover), use a ratio
box instead: the wrapper takes the same CSS as the box plus `position: relative`, the image switches
to `layout="fill"`, and the overlays are absolutely positioned inside the box. The image itself is
out of the document flow, so the box's ratio sets the media area's height.

When the crop mode must also be merchant-configurable, add an `image_fill_mode` setting
(`cover` / `contain`) and feed `object-fit` from it; without it hard-code `cover`. An `object-fit`
that reads an undeclared setting id gets an empty value and the attribute stops working (see
[schema-rules.md](../schema-rules.md) → "Every setting needs a reason").

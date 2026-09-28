# ljs-zoom — image zoom

Zooms a product image on click or hover; wraps an `ljs-img`.

## Rules

1. Use `ljs-zoom`. No hand-written zoom or magnifier scripts.
2. The child is an `ljs-img` + `img_url` (fixed width/height recommended), and there must be exactly
   one child: with none the component errors and stops working; with several only the first is
   used.
3. Always set the magnification with `zoom-ratio` (e.g. `"1.8"`). For an external zoom area, set
   `container-id` and add an empty node with that id.
4. `zoom-in` is added by the component while zoomed. Style against it if needed, but never make it a
   schema setting.
5. Give `presets[0]` a default image so the `ljs-img` has a loadable `src` from the start. With no
   image, don't output `ljs-zoom` at all (it would have no child, rule 2); render the placeholder
   box from [ljs-img.md](ljs-img.md) rule 2 in its place.
6. The zoom area needs an explicit size (fixed width/height or `aspect-ratio` + `overflow: hidden` on
   the wrapper or component class); otherwise the click fires but the zoom viewport may collapse or
   overflow.
7. Read `alt` from the image metadata (`images[image].alt`) first; if there is none, read it from a
   setting and put the default copy in `default` / the preset, without `| t`.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `zoom-ratio` | Magnification | yes | e.g. `1.8` |
| `container-id` | Id of an external zoom area | no | needs a matching empty `div` |
| `interact` | Trigger | no | exactly one of `click` / `mouseover` (default) / `touch`; any other value makes the component error |

Events: `zoomIn` / `zoomOut`. No actions.

## Skeleton

```liquid
{% assign image = block.settings.image %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  {% if image != blank %}
    <ljs-zoom
      layout="container"
      zoom-ratio="{{ block.settings.zoom_ratio | default: 1.8 }}"
      {% if block.settings.interact != blank %}interact="{{ block.settings.interact }}"{% endif %}
    >
      <ljs-img
        layout="responsive"
        width="{{ block.settings.image_width | default: 320 }}"
        height="{{ block.settings.image_height | default: 400 }}"
        src="{{ image | img_url }}"
        alt="{{ images[image].alt | escape }}"
        object-fit="cover"
      ></ljs-img>
    </ljs-zoom>
  {% else %}
    <div class="zoom-placeholder" aria-hidden="true"></div>
  {% endif %}
</div>

<style>
  .{{ root_cls }} .zoom-placeholder {
    width: 100%;
    aspect-ratio: {{ block.settings.image_width | default: 320 }} / {{ block.settings.image_height | default: 400 }};
    background: #f4f4f4;
  }
</style>
```

External zoom area (rarely):

```liquid
<ljs-zoom layout="container" zoom-ratio="1.8" container-id="zoom-box-{{ block_id }}">
  ...
</ljs-zoom>
<div id="zoom-box-{{ block_id }}"></div>
```

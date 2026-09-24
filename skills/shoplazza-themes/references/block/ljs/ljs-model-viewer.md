# ljs-model-viewer — 3D model preview

Previews a product or exhibit as a 3D model (`.glb`).

## Rules

1. Use it only when the request explicitly asks for a 3D model preview (see [selection.md](selection.md)).
   The model goes in `src` (a glb URL), the cover in `poster`, the accessible text in `alt`.
2. Wrap it in a relatively positioned container with an explicit height. The viewer fills its
   parent; with no height on the wrapper it collapses and nothing is visible.
3. `user-control` is an inverted switch: without it the zoom/fullscreen controls are on; writing the
   attribute turns them **off**. Write it only when the controls must be hidden.
4. Enter 3D mode with a stable `id` plus `@tap="<id>.enter"` (or `<id>.enter()`). No iframe embeds,
   no hand-written three.js, no hand-written fullscreen or AR scripts.
5. `layout`: take the value from the single-value table in [writing.md](writing.md).
6. Read `alt` from a merchant setting. If there is none, add an `alt` setting and put the default
   copy in its `default` and in the preset.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `src` | glb URL | yes | |
| `poster` | Cover image | yes | usually `\| img_url` |
| `alt` | Accessible description | no | string |
| `id` | Target for `enter` | no | `capture` it from `block_id` |
| `user-control` | Turns off the default zoom/fullscreen controls | no | boolean, no value; inverted — present = off |

Actions: only `enter`. No events. Use only the attributes in the table; names like `autoplay` or
`mp4` are not recognised.

## Skeleton

```liquid
{% capture viewer_id %}model-viewer-{{ block_id }}{% endcapture %}
{% assign poster = block.settings.poster %}

<div
  class="{{ root_cls }}"
  style="--viewer-height: {{ block.settings.viewer_height | default: 500 }}px;"
  {{ block.shoplaza_attributes }}
>
  {% if block.settings.model_src != blank %}
    <div class="model-viewer-wrap">
      <ljs-model-viewer
        id="{{ viewer_id }}"
        layout="container"
        src="{{ block.settings.model_src }}"
        alt="{{ block.settings.alt | escape }}"
        {% if poster != blank %}poster="{{ poster | img_url }}"{% endif %}
      ></ljs-model-viewer>
    </div>
    {% if block.settings.show_enter_btn %}
      <button type="button" @tap="{{ viewer_id }}.enter">{{ block.settings.enter_label }}</button>
    {% endif %}
  {% else %}
    <div class="model-viewer-placeholder" aria-hidden="true"></div>
  {% endif %}
</div>

<style>
  .{{ root_cls }} .model-viewer-wrap {
    position: relative;
    width: 100%;
    height: var(--viewer-height);
  }
</style>
```

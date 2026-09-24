# ljs-scrollbar — custom scrollbar

Tracks the scroll progress of a given container and renders a custom progress bar or step
navigation from a template. `scrollToTarget` scrolls the container so a given child is centred.

## Rules

1. Use it only when the request explicitly asks for a custom scrollbar (see
   [selection.md](selection.md)); don't add it on your own.
2. `container` = the bare `id` (no `#`) of the scrollable container. That node must already be in
   the DOM and its `id` must match exactly. The component wraps no content; it only binds the
   container and renders the control UI.
3. `layout`: take the value from the single-value table in [writing.md](writing.md).
4. The control UI goes in a `<template>` inside the component; progress and button state come from
   the template variables `${progress}` / `${hasPrev}` / `${hasNext}`. Follow
   [template.md](template.md) for template syntax.
5. Steps: the `step` attribute splits the total scroll distance into that many equal steps
   (default `5`). `@tap="<id>.next()"` / `<id>.prev()` must use the component's own `id`.
6. Direction: `scroll-direction` is `vertical` or `horizontal` only, default `vertical`. Unless
   horizontal scrolling is explicitly requested, use vertical.
7. Single root block. If the content needs several items, use at most one level of
   `for item in block.blocks` inside the bound container; don't split the scrollbar into child
   blocks.
8. With `scroll-direction="horizontal"` the container must overflow horizontally (`overflow-x`);
   with `vertical` it needs a fixed height and must overflow vertically (`overflow-y`). If the
   content doesn't overflow, the progress bar never changes.
9. Give the bound container a fixed height: a `viewport_height` setting defaulting to `300`, applied
   with CSS `height` (not just `max-height`). The preset must include enough items to exceed that
   height, so scroll progress is visible in the default state.
10. Default item titles/descriptions go straight into the child `settings[].default` and
    `presets[0].blocks[].settings`, without `| t`; extra empty items render nothing.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `container` | Id of the scroll container | yes | value attribute |
| `step` | Number of equal steps across the total scroll distance | no | number; default `5`; used by `next` / `prev` |
| `scroll-direction` | Scroll direction | no | `vertical` / `horizontal`; default `vertical` |

Template data: `${progress}` (0–100) / `${step}` / `${hasPrev}` / `${hasNext}`.

Actions: `next` / `prev` / `scrollToTarget`. No custom events. `scrollToTarget`'s `src` / `target`
are action parameters (the child's `src` value, or a CSS selector), not component attributes —
don't put them on the tag.


## Skeleton

```liquid
{% capture bar_id %}scrollbar-{{ block_id }}{% endcapture %}
{% capture box_id %}scroll-box-{{ block_id }}{% endcapture %}

<div
  class="{{ root_cls }}"
  {{ block.shoplaza_attributes }}
>
  <div id="{{ box_id }}" class="scroll-box">
    {% for item in block.blocks %}
      <div class="scroll-item" {{ item.shoplaza_attributes }}>
        {% if item.settings.heading != blank %}
          <h3>{{ item.settings.heading }}</h3>
        {% endif %}
        <div>{{ item.settings.body }}</div>
      </div>
    {% endfor %}
  </div>

  <ljs-scrollbar
    id="{{ bar_id }}"
    layout="container"
    container="{{ box_id }}"
    step="{{ block.settings.step | default: 5 }}"
    scroll-direction="{{ block.settings.scroll_direction | default: 'vertical' }}"
  >
    <template>
      <div class="scrollbar-track" style="width: ${progress}%"></div>
    </template>
  </ljs-scrollbar>
</div>

<style>
  .{{ root_cls }} .scroll-box {
    height: {{ block.settings.viewport_height | default: 300 }}px;
    overflow-y: auto;
  }
</style>
```

For prev/next, add buttons inside the template whose `@tap` calls `prev` / `next` on the same `id`.
In the parent schema, `scroll_direction` must default to `vertical` and `step` to `5`.

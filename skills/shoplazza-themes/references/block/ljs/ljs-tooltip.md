# ljs-tooltip — tooltip

A simple text tooltip bubble.

## Rules

1. Use `ljs-tooltip`.
2. `for` must equal the trigger node's `id`. Prefer `content="..."` for the text; for richer content
   put child nodes inside the tooltip and style them with `overlay-class`.
3. `placement` has exactly six values: `top` / `topLeft` / `topRight` / `bottom` / `bottomLeft` /
   `bottomRight`. Anything else is not recognised.
4. No arrow: `no-arrow` (boolean, no value). Trigger mode: `interact` (`hover` by default, or
   `click`).
5. Close with `<id>.close`; listen to `@open` / `@close` if needed.
6. The trigger node and the tooltip both live in the root block; derive the trigger id from
   `block_id` with `capture` to avoid collisions.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `for` | Id of the trigger node | yes | must match the trigger's `id` exactly, or no bubble appears |
| `content` | Plain-text tip | yes, unless the tooltip has child content | simple case |
| `placement` | Position | no | value attribute; default `top` |
| `no-arrow` | Hide the arrow | no | boolean, no value |
| `overlay-class` | Class for the bubble | no | for custom content styling |
| `interact` | Trigger mode | no | `hover` (default) / `click` |
| `target` | CSS selector of an element to move the bubble into | no | instead of the default position |
| `open-delay` | Delay before opening, seconds | no | number |
| `close-delay` | Delay before closing, seconds | no | number |

Actions: `close`.
Events: `open` / `close`.

## Skeleton

```liquid
{% capture trigger_id %}tip-trigger-{{ block_id }}{% endcapture %}
{% capture tooltip_id %}tooltip-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <div id="{{ trigger_id }}" class="tip-trigger">{{ block.settings.trigger_label }}</div>
  <ljs-tooltip
    id="{{ tooltip_id }}"
    layout="logic"
    for="{{ trigger_id }}"
    content="{{ block.settings.tooltip_text | escape }}"
    placement="{{ block.settings.placement | default: 'top' }}"
    {% if block.settings.no_arrow %}no-arrow{% endif %}
  ></ljs-tooltip>
</div>
```

Custom content (no `content`): put the list in child nodes and add `overlay-class`; a close item can
use `@tap="{{ tooltip_id }}.close"`.

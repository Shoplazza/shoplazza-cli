# ljs-selector — option group

Single- or multi-select option group.

## Rules

1. Use `ljs-selector`. Every option node carries `option="..."`; mark the default choice with
   `selected`. Don't toggle a class yourself.
2. `layout`: take the value from the single-value table in [writing.md](writing.md).
3. Allow deselecting by clicking again: `cancelable`. Remember the selection across reloads:
   `record-state` (both boolean, no value). `record-state` matches options by their `name`
   attribute — without a `name` on each option, the selection isn't restored after a reload.
4. `scroll-container` on a child element makes the component scroll the selected option into view
   inside that element.
5. Select programmatically with `<id>.toggle(option='…', value=true)` (`option` = the target
   child's `option` value; `value` = select or deselect); add `isScrollIntoView=true` to scroll it
   into view (needs `scroll-container`). Clear with `clear`.
6. `multiple`: multi-select switch (boolean, no value). Present = several options may be selected;
   absent = single select. Write it only when multi-select is explicitly requested — on a
   single-select group it breaks mutual exclusion.


## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `cancelable` | Click again to deselect | no | boolean, no value |
| `record-state` | Remember the selection | no | boolean, no value; each option needs `name` |
| `multiple` | Multi-select | no | boolean, no value; absent = single select |
| `scroll-container` | Scroll the selected option into view | no | on a **child** element, not the root |
| `option` | Option value | yes | on each **child** option, not the root |
| `selected` | Default selection | no | on a child option |
| `name` | Key for `record-state` | with `record-state` | on each child option |

Actions: `clear` / `toggle`.
Events: `select` / `mouseover` / `mouseout`. The event carries `targetOption` (current option
value), `selectedOptions` (all selected values), and `eventName`.

## Skeleton

```liquid
{% capture selector_id %}selector-{{ block_id }}{% endcapture %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <ljs-selector
    id="{{ selector_id }}"
    layout="container"
    {% if block.settings.cancelable %}cancelable{% endif %}
    {% if block.settings.record_state %}record-state{% endif %}
  >
    {% for item in block.blocks %}
      <div
        option="{{ item.settings.option_value | escape }}"
        name="{{ item.settings.option_value | escape }}"
        {% if item.settings.selected %}selected{% endif %}
        {{ item.shoplaza_attributes }}
      >{{ item.settings.label }}</div>
    {% endfor %}
  </ljs-selector>
</div>
```

Swatch switching linked to the main image: see the swatch → main-image section of
[kinds/product-card-interaction.md](../kinds/product-card-interaction.md).

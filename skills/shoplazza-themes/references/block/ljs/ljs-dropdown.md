# ljs-dropdown — dropdown menus

Menus and quick-link dropdowns. lessjs reference:
[spz-dropdown](https://lessjs.shoplazza.com/latest/components/spz-dropdown/) (write the tag as
`ljs-dropdown`).

## Rules

1. The trigger must look clearly like a button: solid background, rounded corners, centered label
   recommended. Never produce a fake `<select>` look with a dropdown arrow and input border. Open
   with `@tap="<id>.open"`; close with `<id>.close`.
2. Stable `id`; the panel content goes inside the tag as children (e.g. `<ul><li>`).
3. `overlay-style` supports only `top` and `left`, e.g. `top: 8px;`. Never put `width` or other
   unsupported styles there; make the menu as wide as the trigger through CSS on the component, the
   list and its items.
4. Boolean, no value: `auto-orientation`; when off, don't output it.
5. Menu items may use one level of `for item in block.blocks`; never a third level of `blocks`.
6. The root container provides the positioning context (e.g. `position: relative;
   display: inline-block`). When the panel must match the trigger's width, give `width: 100%` to
   `ljs-dropdown`, the menu list and the list items together. `width: 100%` on the options alone is
   not enough — it only fills their own too-narrow parent.
7. Demo menu items close on click with `@tap="<id>.close"`; real links can stay `<a href>`, but
   don't hand-write class toggles.
8. Always bind the `dropdownOpen` / `dropdownClose` events, at least to sync the trigger's
   `aria-expanded`; never write them as `open` / `close`.
9. Default trigger copy and preset option copy go straight into `default` and the preset (in the
   card copy language), not `| t`. Don't change default settings the request spells out; extra empty items
   render nothing.

## Attributes

| Attribute | Purpose | Required | Notes |
|---|---|---|---|
| `id` | For open/close | No | `capture` it from `block_id` |
| `placement` | Where the panel opens | No | Only `bottom` / `bottomLeft` / `bottomRight` / `top` / `topLeft` / `topRight`; default `bottomLeft`. Any other value makes the component error |
| `overlay-style` | Inline offset style string | No | Only `top` / `left`, e.g. `top: 8px;` |
| `auto-orientation` | Flip direction automatically near the viewport edge | No | Boolean, no value |
| `target` | Container the panel is moved into | No | `body` or a CSS selector |

- Actions: `open` / `close`.
- Events: `dropdownOpen` / `dropdownClose`.

One trigger button with one `ljs-dropdown` per card is enough. Don't write an `open` attribute to
start it expanded, and don't lay out a matrix of instances for every placement.

## Skeleton

```liquid
{% capture dropdown_id %}dropdown-{{ block_id }}{% endcapture %}
{% capture trigger_id %}dropdown-trigger-{{ block_id }}{% endcapture %}
{% assign placement = block.settings.placement | default: 'bottomLeft' %}

<div class="{{ root_cls }}" {{ block.shoplaza_attributes }}>
  <button
    id="{{ trigger_id }}"
    class="dropdown-trigger"
    type="button"
    aria-haspopup="menu"
    aria-expanded="false"
    @tap="{{ dropdown_id }}.open"
  >{{ block.settings.trigger_label }}</button>

  <ljs-dropdown
    id="{{ dropdown_id }}"
    class="dropdown-component"
    layout="nodisplay"
    placement="{{ placement }}"
    overlay-style="top: 8px;"
    {% if block.settings.auto_orientation %}auto-orientation{% endif %}
    @dropdownOpen="{{ trigger_id }}.toggleAttribute(key='aria-expanded', force=true, value='true')"
    @dropdownClose="{{ trigger_id }}.toggleAttribute(key='aria-expanded', force=true, value='false')"
  >
    <ul class="dropdown-menu">
      {% for item in block.blocks %}
        <li class="dropdown-item" {{ item.shoplaza_attributes }}>
          {% if item.settings.link.url != blank %}
            <a class="dropdown-option" href="{{ item.settings.link.url }}">{{ item.settings.label }}</a>
          {% else %}
            <button class="dropdown-option" type="button" @tap="{{ dropdown_id }}.close">
              {{ item.settings.label }}
            </button>
          {% endif %}
        </li>
      {% endfor %}
    </ul>
  </ljs-dropdown>
</div>

<style>
  .{{ root_cls }} {
    position: relative;
    display: inline-block;
    width: 100%;
    max-width: 320px;
  }

  .{{ root_cls }} .dropdown-trigger,
  .{{ root_cls }} .dropdown-component,
  .{{ root_cls }} .dropdown-menu,
  .{{ root_cls }} .dropdown-item,
  .{{ root_cls }} .dropdown-option {
    width: 100%;
    box-sizing: border-box;
  }

  .{{ root_cls }} .dropdown-trigger {
    border: 0;
    border-radius: 6px;
    padding: 10px 20px;
    background: #1a1a1a;
    color: #ffffff;
    text-align: center;
    cursor: pointer;
  }

  .{{ root_cls }} .dropdown-component,
  .{{ root_cls }} .dropdown-menu {
    min-width: 100%;
  }
</style>
```
